package agency

import (
	"crypto/sha256"
	"encoding/hex"
	"sort"
	"strings"
)

// CausalGraphProtocolVersion is the wire version for the typed causal graph
// payload exchanged between the GIST Python subprocess and the Go runtime.
// Bump when CausalGraph or CausalNode shapes change in a non-additive way.
const CausalGraphProtocolVersion = 1

// NodeRole tags the causal function each node plays.
//
// The four canonical roles mirror Pearl's structural causal model vocabulary:
//
//	evidence     - observation that supports or constrains the outcome
//	confounder   - common-cause-style atom that creates spurious dependence
//	intervention - do(x) action node the agent could take
//	outcome      - the verdict / target the chain explains
//
// Unknown is a transitional bucket used when legacy []string causal chains
// are hydrated without explicit role hints. Downstream code should treat
// unknown as "evidence-shaped but not yet classified".
type NodeRole string

const (
	NodeRoleEvidence     NodeRole = "evidence"
	NodeRoleConfounder   NodeRole = "confounder"
	NodeRoleIntervention NodeRole = "intervention"
	NodeRoleOutcome      NodeRole = "outcome"
	NodeRoleUnknown      NodeRole = "unknown"
)

// NodeID is a stable, content-addressable identifier for a CausalNode.
type NodeID string

// CausalNode is a single vertex in the typed causal graph emitted by the
// GIST kernel. It carries enough metadata to drive Pearl-style
// abduction -> action -> prediction loops, role-specific filtering, and
// Shapley/PSE attribution downstream.
type CausalNode struct {
	ID       NodeID            `json:"id"`
	Role     NodeRole          `json:"role"`
	Summary  string            `json:"summary"`
	Parents  []NodeID          `json:"parents,omitempty"`
	AtomRefs []string          `json:"atomRefs,omitempty"`
	Weight   float64           `json:"weight,omitempty"`
	Meta     map[string]string `json:"meta,omitempty"`
}

// CausalGraph is the typed representation of a GIST verdict's reasoning
// chain. It is the long-lived shape; the legacy GISTVerdict.CausalChain
// []string is derived from it via FlatChain().
type CausalGraph struct {
	ProtocolVersion int          `json:"protocolVersion"`
	Nodes           []CausalNode `json:"nodes"`
}

// FlatChain returns a deterministic, human-readable []string view of the
// graph suitable for legacy callers that consumed CausalChain. The ordering
// is:
//
//  1. Outcomes first (an action plan reads top-down to its conclusion).
//  2. Then interventions.
//  3. Then evidence (highest weight first).
//  4. Then confounders.
//  5. Then unknowns (which carry no role prefix).
//
// Within each role bucket nodes are ordered by descending weight, then by
// ID, so the output is stable across runs and across machines.
func (g *CausalGraph) FlatChain() []string {
	if g == nil || len(g.Nodes) == 0 {
		return nil
	}
	roleOrder := map[NodeRole]int{
		NodeRoleOutcome:      0,
		NodeRoleIntervention: 1,
		NodeRoleEvidence:     2,
		NodeRoleConfounder:   3,
		NodeRoleUnknown:      4,
	}
	indexed := make([]CausalNode, len(g.Nodes))
	copy(indexed, g.Nodes)
	sort.SliceStable(indexed, func(i, j int) bool {
		ri, rj := roleOrder[indexed[i].Role], roleOrder[indexed[j].Role]
		if ri != rj {
			return ri < rj
		}
		if indexed[i].Weight != indexed[j].Weight {
			return indexed[i].Weight > indexed[j].Weight
		}
		return indexed[i].ID < indexed[j].ID
	})
	out := make([]string, 0, len(indexed))
	for _, n := range indexed {
		label := strings.TrimSpace(n.Summary)
		if label == "" {
			label = string(n.ID)
		}
		// Unknowns skip the role prefix so legacy chains round-trip
		// losslessly through HydrateLegacyCausalChain -> FlatChain.
		if n.Role != "" && n.Role != NodeRoleUnknown {
			label = string(n.Role) + ": " + label
		}
		out = append(out, label)
	}
	return out
}

// HydrateLegacyCausalChain converts a legacy []string causal chain into a
// minimal CausalGraph so older callers and persisted traces continue to
// flow through Pearl-aware code paths. Each entry becomes a single node
// with a content-hashed ID; weights are assigned in descending rank order
// so FlatChain round-trips back to the original ordering.
//
// Entries with a "role: text" prefix (e.g. "evidence: foo bar") get the
// matching typed role; everything else is parked under NodeRoleUnknown.
func HydrateLegacyCausalChain(chain []string) *CausalGraph {
	if len(chain) == 0 {
		return nil
	}
	graph := &CausalGraph{
		ProtocolVersion: CausalGraphProtocolVersion,
		Nodes:           make([]CausalNode, 0, len(chain)),
	}
	total := float64(len(chain))
	for idx, raw := range chain {
		summary := strings.TrimSpace(raw)
		role := NodeRoleUnknown
		if colon := strings.Index(summary, ":"); colon > 0 {
			candidate := NodeRole(strings.ToLower(strings.TrimSpace(summary[:colon])))
			switch candidate {
			case NodeRoleEvidence, NodeRoleConfounder, NodeRoleIntervention, NodeRoleOutcome:
				role = candidate
				summary = strings.TrimSpace(summary[colon+1:])
			}
		}
		weight := float64(len(chain)-idx) / total
		graph.Nodes = append(graph.Nodes, CausalNode{
			ID:      NodeID(legacyChainNodeID(idx, raw)),
			Role:    role,
			Summary: summary,
			Weight:  weight,
		})
	}
	return graph
}

// FilterByRole returns a copy of the graph keeping only nodes whose Role
// matches one of the supplied roles. Returns nil if the graph is nil or
// no roles match. Useful for Pearl-loop ablation, e.g. drop confounders
// before running a counterfactual prediction.
func (g *CausalGraph) FilterByRole(roles ...NodeRole) *CausalGraph {
	if g == nil {
		return nil
	}
	if len(roles) == 0 {
		return nil
	}
	keep := make(map[NodeRole]struct{}, len(roles))
	for _, r := range roles {
		keep[r] = struct{}{}
	}
	filtered := &CausalGraph{ProtocolVersion: g.ProtocolVersion}
	for _, n := range g.Nodes {
		if _, ok := keep[n.Role]; ok {
			filtered.Nodes = append(filtered.Nodes, n)
		}
	}
	if len(filtered.Nodes) == 0 {
		return nil
	}
	return filtered
}

// Clone returns a deep copy so downstream mutations (filtering, ablation,
// reweighting) never alias shared state across actors.
func (g *CausalGraph) Clone() *CausalGraph {
	if g == nil {
		return nil
	}
	dup := &CausalGraph{
		ProtocolVersion: g.ProtocolVersion,
		Nodes:           make([]CausalNode, len(g.Nodes)),
	}
	for i, n := range g.Nodes {
		nodeCopy := CausalNode{
			ID:      n.ID,
			Role:    n.Role,
			Summary: n.Summary,
			Weight:  n.Weight,
		}
		if len(n.Parents) > 0 {
			nodeCopy.Parents = append([]NodeID(nil), n.Parents...)
		}
		if len(n.AtomRefs) > 0 {
			nodeCopy.AtomRefs = append([]string(nil), n.AtomRefs...)
		}
		if len(n.Meta) > 0 {
			nodeCopy.Meta = make(map[string]string, len(n.Meta))
			for k, v := range n.Meta {
				nodeCopy.Meta[k] = v
			}
		}
		dup.Nodes[i] = nodeCopy
	}
	return dup
}

// CountByRole returns the number of nodes for each known role. Useful for
// quick health checks ("did we get any interventions?") and metrics export.
func (g *CausalGraph) CountByRole() map[NodeRole]int {
	out := map[NodeRole]int{}
	if g == nil {
		return out
	}
	for _, n := range g.Nodes {
		out[n.Role]++
	}
	return out
}

func legacyChainNodeID(idx int, raw string) string {
	h := sha256.New()
	_, _ = h.Write([]byte("legacy-causal-chain"))
	// Encode idx as a fixed-width tag so two adjacent entries with the
	// same content map to distinct IDs.
	_, _ = h.Write([]byte{byte(idx & 0xff), byte((idx >> 8) & 0xff)})
	_, _ = h.Write([]byte(raw))
	return "lcc:" + hex.EncodeToString(h.Sum(nil))[:16]
}
