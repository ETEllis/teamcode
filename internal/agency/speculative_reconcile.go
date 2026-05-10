package agency

import (
	"math"
	"sort"
)

// ReconciliationProtocolVersion is the wire version for
// ReconciliationReport. Bump on non-additive shape changes.
const ReconciliationProtocolVersion = 1

// ReconciliationStatus is the three-tier ladder for meta-vs-peers
// faithfulness:
//
//	faithful   - meta summary aligns with peer consensus
//	drifted    - supported but coverage is weak or weights have shifted
//	fabricated - at least one meta node has no peer support; lattice rejects
//
// Lattice consumers MUST refuse to use a fabricated meta verdict and
// fall back to peer aggregation. Drifted is a warning, not a refusal.
type ReconciliationStatus string

const (
	ReconciliationStatusFaithful   ReconciliationStatus = "faithful"
	ReconciliationStatusDrifted    ReconciliationStatus = "drifted"
	ReconciliationStatusFabricated ReconciliationStatus = "fabricated"
)

// Default thresholds for the reconciliation status ladder. These are
// first-pass calibrations — see PHASE5_DESIGN.md Open Q3.
const (
	defaultFaithfulSupport     = 0.66
	defaultFaithfulCoverage    = 0.50
	defaultDriftThreshold      = 0.30
	defaultFabricatedSupport   = 0.20
	defaultMetaNodeMinSupport  = 0.34
	compatibleWeightTolerance  = 0.25
)

// ReconciliationConfig tunes the status thresholds. Zero values mean
// "use the documented default".
type ReconciliationConfig struct {
	FaithfulSupport     float64
	FaithfulCoverage    float64
	DriftThreshold      float64
	FabricatedSupport   float64
	MetaNodeMinSupport  float64
	WeightTolerance     float64
}

// NodeReconciliation is the per-meta-node faithfulness probe. It does
// NOT measure text overlap; it measures *typed structural support*:
// canonical leaf-hash equality first, then a relaxed (role + normalized
// summary + weight-within-tolerance) match for cases where peers
// rounded weights differently or attached extra metadata.
type NodeReconciliation struct {
	NodeID          NodeID   `json:"nodeId"`
	PeerSupport     float64  `json:"peerSupport"`
	RoleAgreement   float64  `json:"roleAgreement"`
	WeightDelta     float64  `json:"weightDelta"`
	SupportingPeers []string `json:"supportingPeers,omitempty"`
}

// ReconciliationReport is the output of MetaReconcile. SupportScore is
// the weighted mean of NodeSupport across meta nodes; CoverageScore is
// the fraction of peer nodes captured by the meta; DriftScore is the
// mean absolute weight delta. Status is derived from these via the
// configured thresholds.
type ReconciliationReport struct {
	ProtocolVersion int                  `json:"protocolVersion"`
	Status          ReconciliationStatus `json:"status"`
	SupportScore    float64              `json:"supportScore"`
	CoverageScore   float64              `json:"coverageScore"`
	DriftScore      float64              `json:"driftScore"`
	NodeReports     []NodeReconciliation `json:"nodeReports"`
	UnsupportedIDs  []NodeID             `json:"unsupportedIDs,omitempty"`
	UncoveredIDs    []NodeID             `json:"uncoveredIDs,omitempty"`
	Notes           []string             `json:"notes,omitempty"`
}

// PeerGraph pairs an agent ID with that peer's typed causal graph.
// Used by MetaReconcile so reports can name the agents that supported
// each meta node.
type PeerGraph struct {
	AgentID string       `json:"agentId"`
	Graph   *CausalGraph `json:"graph"`
}

// MetaReconcile compares a meta-agent's CausalGraph against the peer
// cohort it claims to summarize, producing a faithfulness report.
//
// Composition: this function does NOT itself check Merkle convergence.
// Callers should run MerkleConverge over peers first and refuse to call
// MetaReconcile on a divergent cohort (asking 'did the meta faithfully
// summarize them?' is ill-posed when the cohort has no consensus).
func MetaReconcile(meta *CausalGraph, peers []PeerGraph) ReconciliationReport {
	return MetaReconcileWithConfig(meta, peers, ReconciliationConfig{})
}

// MetaReconcileWithConfig is the configurable form. Zero-valued config
// fields fall back to documented defaults.
func MetaReconcileWithConfig(meta *CausalGraph, peers []PeerGraph, cfg ReconciliationConfig) ReconciliationReport {
	cfg = applyReconciliationDefaults(cfg)
	report := ReconciliationReport{ProtocolVersion: ReconciliationProtocolVersion}

	if meta == nil || len(meta.Nodes) == 0 {
		report.Status = ReconciliationStatusFabricated
		report.Notes = append(report.Notes, "meta graph is empty; nothing to reconcile")
		return report
	}
	validPeers := make([]PeerGraph, 0, len(peers))
	for _, p := range peers {
		if p.Graph != nil && len(p.Graph.Nodes) > 0 {
			validPeers = append(validPeers, p)
		}
	}
	if len(validPeers) == 0 {
		report.Status = ReconciliationStatusFabricated
		report.Notes = append(report.Notes, "no non-empty peers; cannot validate meta")
		return report
	}

	// Per meta-node analysis.
	report.NodeReports = make([]NodeReconciliation, 0, len(meta.Nodes))
	supportSum := 0.0
	driftSum := 0.0
	driftCount := 0
	fabricatedFlag := false
	for _, mNode := range meta.Nodes {
		nr := analyzeMetaNode(mNode, validPeers, cfg)
		report.NodeReports = append(report.NodeReports, nr)
		supportSum += nr.PeerSupport
		if nr.PeerSupport > 0 {
			driftSum += nr.WeightDelta
			driftCount++
		}
		if nr.PeerSupport < cfg.FabricatedSupport {
			report.UnsupportedIDs = append(report.UnsupportedIDs, mNode.ID)
			if nr.RoleAgreement == 0 {
				fabricatedFlag = true
			}
		}
	}
	report.SupportScore = supportSum / float64(len(meta.Nodes))
	if driftCount > 0 {
		report.DriftScore = driftSum / float64(driftCount)
	}

	// Coverage: fraction of distinct peer nodes that map to some meta node.
	report.CoverageScore, report.UncoveredIDs = computeCoverage(meta, validPeers, cfg)

	// Status ladder.
	switch {
	case fabricatedFlag:
		report.Status = ReconciliationStatusFabricated
		report.Notes = append(report.Notes, "at least one meta node has no peer support and no role agreement")
	case report.SupportScore >= cfg.FaithfulSupport &&
		report.CoverageScore >= cfg.FaithfulCoverage &&
		report.DriftScore <= cfg.DriftThreshold &&
		!hasUnsupported(report.NodeReports, cfg.MetaNodeMinSupport):
		report.Status = ReconciliationStatusFaithful
	default:
		report.Status = ReconciliationStatusDrifted
		if report.CoverageScore < cfg.FaithfulCoverage {
			report.Notes = append(report.Notes, "coverage below faithful threshold")
		}
		if report.DriftScore > cfg.DriftThreshold {
			report.Notes = append(report.Notes, "weight drift above threshold")
		}
		if hasUnsupported(report.NodeReports, cfg.MetaNodeMinSupport) {
			report.Notes = append(report.Notes, "one or more meta nodes below per-node support floor")
		}
	}

	return report
}

func applyReconciliationDefaults(cfg ReconciliationConfig) ReconciliationConfig {
	if cfg.FaithfulSupport == 0 {
		cfg.FaithfulSupport = defaultFaithfulSupport
	}
	if cfg.FaithfulCoverage == 0 {
		cfg.FaithfulCoverage = defaultFaithfulCoverage
	}
	if cfg.DriftThreshold == 0 {
		cfg.DriftThreshold = defaultDriftThreshold
	}
	if cfg.FabricatedSupport == 0 {
		cfg.FabricatedSupport = defaultFabricatedSupport
	}
	if cfg.MetaNodeMinSupport == 0 {
		cfg.MetaNodeMinSupport = defaultMetaNodeMinSupport
	}
	if cfg.WeightTolerance == 0 {
		cfg.WeightTolerance = compatibleWeightTolerance
	}
	return cfg
}

// analyzeMetaNode probes one meta node against every peer's graph and
// returns the per-node reconciliation row.
func analyzeMetaNode(mNode CausalNode, peers []PeerGraph, cfg ReconciliationConfig) NodeReconciliation {
	supporting := make([]string, 0, len(peers))
	roleHits := 0
	weightDeltas := []float64{}
	mLeaf := canonicalLeafHash(mNode)
	for _, p := range peers {
		match := findCompatibleNode(p.Graph, mNode, mLeaf, cfg.WeightTolerance)
		if match == nil {
			continue
		}
		supporting = append(supporting, p.AgentID)
		if match.Role == mNode.Role {
			roleHits++
		}
		weightDeltas = append(weightDeltas, math.Abs(match.Weight-mNode.Weight))
	}
	sort.Strings(supporting)
	support := float64(len(supporting)) / float64(len(peers))
	roleAgreement := 0.0
	if len(supporting) > 0 {
		roleAgreement = float64(roleHits) / float64(len(supporting))
	}
	weightDelta := 0.0
	if len(weightDeltas) > 0 {
		s := 0.0
		for _, d := range weightDeltas {
			s += d
		}
		weightDelta = s / float64(len(weightDeltas))
	}
	return NodeReconciliation{
		NodeID:          mNode.ID,
		PeerSupport:     support,
		RoleAgreement:   roleAgreement,
		WeightDelta:     weightDelta,
		SupportingPeers: supporting,
	}
}

// findCompatibleNode walks a peer graph looking for a node that is
// "structurally compatible" with the meta node. First pass: exact
// canonical leaf hash match (mechanism-faithful equivalence). Second
// pass: relaxed match on (role, normalized summary, weight within
// tolerance). The relaxed pass catches the realistic case where peers
// rounded weights differently or wrote summaries with different
// metadata that the meta dropped.
func findCompatibleNode(graph *CausalGraph, m CausalNode, mLeaf string, weightTol float64) *CausalNode {
	if graph == nil {
		return nil
	}
	mSummary := normalizeSummary(m.Summary)
	for i := range graph.Nodes {
		p := &graph.Nodes[i]
		if canonicalLeafHash(*p) == mLeaf {
			return p
		}
	}
	for i := range graph.Nodes {
		p := &graph.Nodes[i]
		if p.Role != m.Role {
			continue
		}
		if normalizeSummary(p.Summary) != mSummary {
			continue
		}
		if math.Abs(p.Weight-m.Weight) > weightTol {
			continue
		}
		return p
	}
	return nil
}

// computeCoverage walks every distinct peer node (deduped by canonical
// leaf hash) and asks whether the meta carries a structurally
// compatible counterpart. Returns the fraction covered and the IDs of
// peer nodes the meta missed.
func computeCoverage(meta *CausalGraph, peers []PeerGraph, cfg ReconciliationConfig) (float64, []NodeID) {
	type peerNodeRef struct {
		id    NodeID
		node  CausalNode
		leaf  string
	}
	seen := map[string]peerNodeRef{}
	for _, p := range peers {
		for _, n := range p.Graph.Nodes {
			leaf := canonicalLeafHash(n)
			if _, ok := seen[leaf]; !ok {
				seen[leaf] = peerNodeRef{id: n.ID, node: n, leaf: leaf}
			}
		}
	}
	if len(seen) == 0 {
		return 0, nil
	}
	covered := 0
	uncovered := make([]NodeID, 0)
	for _, ref := range seen {
		if findCompatibleNode(meta, ref.node, ref.leaf, cfg.WeightTolerance) != nil {
			covered++
		} else {
			uncovered = append(uncovered, ref.id)
		}
	}
	sort.Slice(uncovered, func(i, j int) bool { return uncovered[i] < uncovered[j] })
	return float64(covered) / float64(len(seen)), uncovered
}

func hasUnsupported(reports []NodeReconciliation, floor float64) bool {
	for _, r := range reports {
		if r.PeerSupport < floor {
			return true
		}
	}
	return false
}

// IsAcceptable reports whether the lattice may use this meta verdict.
// Faithful and drifted are acceptable (drifted with caveats); fabricated
// is not.
func (r ReconciliationReport) IsAcceptable() bool {
	return r.Status == ReconciliationStatusFaithful || r.Status == ReconciliationStatusDrifted
}
