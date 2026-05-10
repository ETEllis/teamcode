package agency

import (
	"errors"
	"sort"
)

// DyadProtocolVersion is the wire version for DyadDelta payloads.
// Bump on non-additive changes to the delta shape.
const DyadProtocolVersion = 1

// LabeledVerdict pairs a verdict with a stable identifier so dyad
// deltas can name their base/sibling verdicts. ID is opaque from the
// dyad layer's point of view — callers pick whatever ID scheme makes
// sense for their lattice slot store.
type LabeledVerdict struct {
	ID      string      `json:"id"`
	Verdict GISTVerdict `json:"verdict"`
}

// NodeDiff captures the typed mutation between a base node and its
// sibling. Pointer-typed weight fields distinguish "no change" from
// "changed to zero". Empty role/summary fields mean "unchanged".
type NodeDiff struct {
	NodeID     NodeID   `json:"nodeId"`
	OldRole    NodeRole `json:"oldRole,omitempty"`
	NewRole    NodeRole `json:"newRole,omitempty"`
	OldWeight  *float64 `json:"oldWeight,omitempty"`
	NewWeight  *float64 `json:"newWeight,omitempty"`
	OldSummary string   `json:"oldSummary,omitempty"`
	NewSummary string   `json:"newSummary,omitempty"`
}

// DyadDelta encodes the Hamming-1 difference between a base verdict
// and its sibling. Exactly one of the three mutation forms is set:
//
//	NodeMutation - a node was renamed/re-roled/re-weighted in place
//	NodeAdded    - sibling carries one more node than base
//	NodeRemoved  - sibling carries one fewer node than base
//
// LeafReplaced and LeafIntroduced are the canonical leaf hashes of the
// dropped/gained leaves. They are redundant given the typed mutation,
// but ship them anyway so a verifier can confirm the delta matches the
// Merkle attestations without rehydrating.
type DyadDelta struct {
	ProtocolVersion  int         `json:"protocolVersion"`
	BaseVerdictID    string      `json:"baseVerdictId"`
	SiblingVerdictID string      `json:"siblingVerdictId"`
	LeafReplaced     string      `json:"leafReplaced,omitempty"`
	LeafIntroduced   string      `json:"leafIntroduced,omitempty"`
	NodeMutation     *NodeDiff   `json:"nodeMutation,omitempty"`
	NodeAdded        *CausalNode `json:"nodeAdded,omitempty"`
	NodeRemoved      *NodeID     `json:"nodeRemoved,omitempty"`
}

// DyadCompressionReport is the audit trail for one DyadCompress run.
// SlotsBefore/SlotsAfter let callers verify the storage win.
type DyadCompressionReport struct {
	ProtocolVersion int         `json:"protocolVersion"`
	SlotsBefore     int         `json:"slotsBefore"`
	SlotsAfter      int         `json:"slotsAfter"`
	Deltas          []DyadDelta `json:"deltas"`
	UnpairedIDs     []string    `json:"unpairedIds,omitempty"`
	Notes           []string    `json:"notes,omitempty"`
}

// DyadCompress greedily pairs Hamming-1-related verdicts into base +
// delta records. Returns the compressed (base-only) verdict slice plus
// a report containing the deltas needed to rehydrate the original set.
//
// Pairing is greedy and stable: verdicts are sorted by ID, then for
// each unpaired verdict we scan later IDs for a Hamming-1 partner.
// Each verdict participates in at most one dyad — this is deliberate
// (see PHASE5_DESIGN.md Open Q5: transitive collapse can degrade to
// "everything looks the same").
func DyadCompress(verdicts []LabeledVerdict) ([]LabeledVerdict, DyadCompressionReport) {
	report := DyadCompressionReport{
		ProtocolVersion: DyadProtocolVersion,
		SlotsBefore:     len(verdicts),
	}
	if len(verdicts) < 2 {
		report.SlotsAfter = len(verdicts)
		for _, v := range verdicts {
			report.UnpairedIDs = append(report.UnpairedIDs, v.ID)
		}
		sort.Strings(report.UnpairedIDs)
		return append([]LabeledVerdict(nil), verdicts...), report
	}

	// Sort a working copy by ID for deterministic pairing.
	work := make([]LabeledVerdict, len(verdicts))
	copy(work, verdicts)
	sort.Slice(work, func(i, j int) bool { return work[i].ID < work[j].ID })

	// Precompute attestations once.
	attests := make([]MerkleAttestation, len(work))
	for i, v := range work {
		attests[i] = MerkleAttest(v.Verdict.CausalGraph)
	}

	paired := make([]bool, len(work))
	bases := make([]LabeledVerdict, 0, len(work))
	for i := 0; i < len(work); i++ {
		if paired[i] {
			continue
		}
		// Search for a Hamming-1 partner among later, unpaired verdicts.
		matched := -1
		var bestDelta *DyadDelta
		for j := i + 1; j < len(work); j++ {
			if paired[j] {
				continue
			}
			delta, ok := buildDyadDelta(work[i], attests[i], work[j], attests[j])
			if !ok {
				continue
			}
			matched = j
			bestDelta = delta
			break // greedy first-match
		}
		bases = append(bases, work[i])
		if matched < 0 {
			report.UnpairedIDs = append(report.UnpairedIDs, work[i].ID)
			continue
		}
		paired[i] = true
		paired[matched] = true
		report.Deltas = append(report.Deltas, *bestDelta)
	}
	sort.Strings(report.UnpairedIDs)
	report.SlotsAfter = len(bases)
	return bases, report
}

// buildDyadDelta inspects two verdicts and, if they are exactly
// Hamming-1 related, returns the delta that transforms base into
// sibling. Returns ok=false otherwise.
//
// Three valid Hamming-1 shapes:
//
//  1. Symmetric leaf swap (one leaf differs on each side, same node ID,
//     equal leaf counts) → NodeMutation
//  2. Sibling has one extra leaf (sibling.LeafCount == base.LeafCount + 1) → NodeAdded
//  3. Sibling has one fewer leaf (sibling.LeafCount == base.LeafCount - 1) → NodeRemoved
//
// Anything else (Hamming distance >= 2, or the changed node IDs don't
// align) returns ok=false.
func buildDyadDelta(base LabeledVerdict, baseAtt MerkleAttestation, sibling LabeledVerdict, sibAtt MerkleAttestation) (*DyadDelta, bool) {
	if baseAtt.LeafCount == 0 || sibAtt.LeafCount == 0 {
		return nil, false
	}
	baseSet := baseAtt.LeafSet()
	sibSet := sibAtt.LeafSet()
	if baseSet == nil || sibSet == nil {
		return nil, false
	}
	missingFromSib := []string{}
	extraInSib := []string{}
	for l := range baseSet {
		if _, ok := sibSet[l]; !ok {
			missingFromSib = append(missingFromSib, l)
		}
	}
	for l := range sibSet {
		if _, ok := baseSet[l]; !ok {
			extraInSib = append(extraInSib, l)
		}
	}
	delta := &DyadDelta{
		ProtocolVersion:  DyadProtocolVersion,
		BaseVerdictID:    base.ID,
		SiblingVerdictID: sibling.ID,
	}
	switch {
	case len(missingFromSib) == 1 && len(extraInSib) == 1 && baseAtt.LeafCount == sibAtt.LeafCount:
		// Mutation in place. Resolve which node ID changed.
		baseNode, ok1 := nodeForLeaf(base.Verdict.CausalGraph, missingFromSib[0])
		sibNode, ok2 := nodeForLeaf(sibling.Verdict.CausalGraph, extraInSib[0])
		if !ok1 || !ok2 {
			return nil, false
		}
		// Mutation must be on the *same* node ID — different IDs would mean
		// "added X, removed Y", not a Hamming-1 mutation.
		if baseNode.ID != sibNode.ID {
			return nil, false
		}
		delta.LeafReplaced = missingFromSib[0]
		delta.LeafIntroduced = extraInSib[0]
		delta.NodeMutation = nodeDiff(baseNode, sibNode)
		return delta, true
	case len(missingFromSib) == 0 && len(extraInSib) == 1 && sibAtt.LeafCount == baseAtt.LeafCount+1:
		sibNode, ok := nodeForLeaf(sibling.Verdict.CausalGraph, extraInSib[0])
		if !ok {
			return nil, false
		}
		nodeCopy := sibNode
		delta.LeafIntroduced = extraInSib[0]
		delta.NodeAdded = &nodeCopy
		return delta, true
	case len(missingFromSib) == 1 && len(extraInSib) == 0 && sibAtt.LeafCount == baseAtt.LeafCount-1:
		baseNode, ok := nodeForLeaf(base.Verdict.CausalGraph, missingFromSib[0])
		if !ok {
			return nil, false
		}
		removedID := baseNode.ID
		delta.LeafReplaced = missingFromSib[0]
		delta.NodeRemoved = &removedID
		return delta, true
	}
	return nil, false
}

func nodeForLeaf(graph *CausalGraph, leaf string) (CausalNode, bool) {
	if graph == nil {
		return CausalNode{}, false
	}
	for _, n := range graph.Nodes {
		if canonicalLeafHash(n) == leaf {
			return n, true
		}
	}
	return CausalNode{}, false
}

func nodeDiff(base, sib CausalNode) *NodeDiff {
	diff := &NodeDiff{NodeID: base.ID}
	if base.Role != sib.Role {
		diff.OldRole = base.Role
		diff.NewRole = sib.Role
	}
	if base.Weight != sib.Weight {
		bw, sw := base.Weight, sib.Weight
		diff.OldWeight = &bw
		diff.NewWeight = &sw
	}
	if base.Summary != sib.Summary {
		diff.OldSummary = base.Summary
		diff.NewSummary = sib.Summary
	}
	return diff
}

// DyadHydrate is the inverse of DyadCompress for one (base, delta)
// pair. Returns the rehydrated sibling verdict.
//
// By construction DyadHydrate(base, DyadCompress(base, sibling).delta)
// equals sibling modulo float-rounding at canonical 6-decimal precision.
func DyadHydrate(base LabeledVerdict, delta DyadDelta) (LabeledVerdict, error) {
	if base.Verdict.CausalGraph == nil {
		return LabeledVerdict{}, errors.New("dyad hydrate: base verdict has no causal graph")
	}
	cloned := cloneCausalGraph(base.Verdict.CausalGraph)
	switch {
	case delta.NodeMutation != nil:
		applied := false
		for i := range cloned.Nodes {
			if cloned.Nodes[i].ID != delta.NodeMutation.NodeID {
				continue
			}
			if delta.NodeMutation.NewRole != "" {
				cloned.Nodes[i].Role = delta.NodeMutation.NewRole
			}
			if delta.NodeMutation.NewWeight != nil {
				cloned.Nodes[i].Weight = *delta.NodeMutation.NewWeight
			}
			if delta.NodeMutation.NewSummary != "" {
				cloned.Nodes[i].Summary = delta.NodeMutation.NewSummary
			}
			applied = true
			break
		}
		if !applied {
			return LabeledVerdict{}, errors.New("dyad hydrate: mutation target node not found")
		}
	case delta.NodeAdded != nil:
		cloned.Nodes = append(cloned.Nodes, *delta.NodeAdded)
	case delta.NodeRemoved != nil:
		filtered := make([]CausalNode, 0, len(cloned.Nodes))
		removed := false
		for _, n := range cloned.Nodes {
			if !removed && n.ID == *delta.NodeRemoved {
				removed = true
				continue
			}
			filtered = append(filtered, n)
		}
		if !removed {
			return LabeledVerdict{}, errors.New("dyad hydrate: removal target node not found")
		}
		cloned.Nodes = filtered
	default:
		return LabeledVerdict{}, errors.New("dyad hydrate: delta has no mutation, addition, or removal")
	}
	siblingVerdict := base.Verdict
	siblingVerdict.CausalGraph = cloned
	siblingVerdict.SyncCausalChain()
	return LabeledVerdict{ID: delta.SiblingVerdictID, Verdict: siblingVerdict}, nil
}

func cloneCausalGraph(g *CausalGraph) *CausalGraph {
	if g == nil {
		return nil
	}
	cp := &CausalGraph{ProtocolVersion: g.ProtocolVersion, Nodes: make([]CausalNode, len(g.Nodes))}
	for i, n := range g.Nodes {
		cp.Nodes[i] = cloneCausalNode(n)
	}
	return cp
}

func cloneCausalNode(n CausalNode) CausalNode {
	cn := CausalNode{
		ID:      n.ID,
		Role:    n.Role,
		Summary: n.Summary,
		Weight:  n.Weight,
	}
	if len(n.Parents) > 0 {
		cn.Parents = append([]NodeID(nil), n.Parents...)
	}
	if len(n.AtomRefs) > 0 {
		cn.AtomRefs = append([]string(nil), n.AtomRefs...)
	}
	if len(n.Meta) > 0 {
		cn.Meta = make(map[string]string, len(n.Meta))
		for k, v := range n.Meta {
			cn.Meta[k] = v
		}
	}
	return cn
}
