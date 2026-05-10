package agency

import (
	"testing"
)

func reconcilePeerGraph(extraSummary string) *CausalGraph {
	g := merkleSampleGraph()
	if extraSummary != "" {
		g.Nodes = append(g.Nodes, CausalNode{
			ID: NodeID(extraSummary), Role: NodeRoleEvidence, Summary: extraSummary, Weight: 0.3,
		})
	}
	return g
}

func TestMetaReconcile_NilMeta_Fabricated(t *testing.T) {
	r := MetaReconcile(nil, []PeerGraph{{AgentID: "a", Graph: merkleSampleGraph()}})
	if r.Status != ReconciliationStatusFabricated {
		t.Errorf("nil meta should be fabricated, got %q", r.Status)
	}
	if r.IsAcceptable() {
		t.Errorf("fabricated should not be acceptable")
	}
}

func TestMetaReconcile_NoPeers_Fabricated(t *testing.T) {
	r := MetaReconcile(merkleSampleGraph(), nil)
	if r.Status != ReconciliationStatusFabricated {
		t.Errorf("no peers should be fabricated, got %q", r.Status)
	}
}

func TestMetaReconcile_PerfectMatch_Faithful(t *testing.T) {
	g := merkleSampleGraph()
	peers := []PeerGraph{
		{AgentID: "a", Graph: merkleSampleGraph()},
		{AgentID: "b", Graph: merkleSampleGraph()},
		{AgentID: "c", Graph: merkleSampleGraph()},
	}
	r := MetaReconcile(g, peers)
	if r.Status != ReconciliationStatusFaithful {
		t.Errorf("perfect match should be faithful, got %q (support=%.2f cover=%.2f drift=%.3f)",
			r.Status, r.SupportScore, r.CoverageScore, r.DriftScore)
	}
	if r.SupportScore != 1.0 {
		t.Errorf("perfect support expected 1.0, got %.3f", r.SupportScore)
	}
	if !r.IsAcceptable() {
		t.Errorf("faithful should be acceptable")
	}
}

func TestMetaReconcile_FabricatedNode_FlipsToFabricated(t *testing.T) {
	meta := merkleSampleGraph()
	// Append a node that no peer carries.
	meta.Nodes = append(meta.Nodes, CausalNode{
		ID: "fabricated", Role: NodeRoleEvidence, Summary: "made-up evidence", Weight: 0.9,
	})
	peers := []PeerGraph{
		{AgentID: "a", Graph: merkleSampleGraph()},
		{AgentID: "b", Graph: merkleSampleGraph()},
		{AgentID: "c", Graph: merkleSampleGraph()},
	}
	r := MetaReconcile(meta, peers)
	if r.Status != ReconciliationStatusFabricated {
		t.Errorf("fabricated node should flip status, got %q", r.Status)
	}
	if len(r.UnsupportedIDs) == 0 {
		t.Errorf("expected at least one unsupported ID")
	}
	foundFab := false
	for _, id := range r.UnsupportedIDs {
		if id == "fabricated" {
			foundFab = true
		}
	}
	if !foundFab {
		t.Errorf("expected 'fabricated' in UnsupportedIDs, got %v", r.UnsupportedIDs)
	}
}

func TestMetaReconcile_WeakCoverage_Drifted(t *testing.T) {
	// Meta carries only one of four nodes; peers carry full graph.
	meta := &CausalGraph{Nodes: []CausalNode{
		{ID: "out1", Role: NodeRoleOutcome, Summary: "deploy is healthy", Weight: 1.0},
	}}
	peers := []PeerGraph{
		{AgentID: "a", Graph: merkleSampleGraph()},
		{AgentID: "b", Graph: merkleSampleGraph()},
		{AgentID: "c", Graph: merkleSampleGraph()},
	}
	r := MetaReconcile(meta, peers)
	if r.Status != ReconciliationStatusDrifted {
		t.Errorf("weak coverage should be drifted, got %q (cover=%.2f)", r.Status, r.CoverageScore)
	}
	if r.CoverageScore >= defaultFaithfulCoverage {
		t.Errorf("expected coverage below faithful threshold, got %.3f", r.CoverageScore)
	}
	if !r.IsAcceptable() {
		t.Errorf("drifted should still be acceptable (with caveats)")
	}
	if len(r.UncoveredIDs) == 0 {
		t.Errorf("expected uncovered peer node IDs")
	}
}

func TestMetaReconcile_WeightDrift_Drifted(t *testing.T) {
	meta := merkleSampleGraph()
	for i := range meta.Nodes {
		meta.Nodes[i].Weight = meta.Nodes[i].Weight + 0.20 // every node shifted
	}
	peers := []PeerGraph{
		{AgentID: "a", Graph: merkleSampleGraph()},
		{AgentID: "b", Graph: merkleSampleGraph()},
		{AgentID: "c", Graph: merkleSampleGraph()},
	}
	r := MetaReconcile(meta, peers)
	if r.Status == ReconciliationStatusFaithful {
		t.Errorf("systematic weight drift should not be faithful, got %q drift=%.3f", r.Status, r.DriftScore)
	}
}

func TestMetaReconcile_RelaxedMatch_HitsOnSummary(t *testing.T) {
	// Meta has slight weight differences that fall within tolerance.
	meta := merkleSampleGraph()
	for i := range meta.Nodes {
		meta.Nodes[i].Weight += 0.05 // within 0.25 tolerance
	}
	peers := []PeerGraph{
		{AgentID: "a", Graph: merkleSampleGraph()},
		{AgentID: "b", Graph: merkleSampleGraph()},
		{AgentID: "c", Graph: merkleSampleGraph()},
	}
	r := MetaReconcile(meta, peers)
	if r.SupportScore < 0.99 {
		t.Errorf("slight weight diff within tolerance should still match, got support=%.3f", r.SupportScore)
	}
	// Drift should be small; should still be faithful.
	if r.Status != ReconciliationStatusFaithful {
		t.Errorf("small drift should still be faithful, got %q drift=%.3f", r.Status, r.DriftScore)
	}
}

func TestMetaReconcile_PartialPeerSupport_NotFaithful(t *testing.T) {
	// Half the peers have node 'out1' renamed; meta has 'out1'.
	g1 := merkleSampleGraph()
	g2 := merkleSampleGraph()
	g2.Nodes[3].Summary = "completely different summary that won't match anything"
	g2.Nodes[3].Weight = 0.1
	peers := []PeerGraph{
		{AgentID: "a", Graph: g1},
		{AgentID: "b", Graph: g2},
		{AgentID: "c", Graph: g2},
	}
	r := MetaReconcile(g1, peers)
	// 'out1' on meta only finds support in 1/3 peers (g1), so per-node support 0.33
	// which is below MetaNodeMinSupport floor (0.34).
	if r.Status == ReconciliationStatusFaithful {
		t.Errorf("partial per-node support should not be faithful, got %q", r.Status)
	}
}

func TestMetaReconcile_NodeReportSupportingPeersSorted(t *testing.T) {
	g := merkleSampleGraph()
	peers := []PeerGraph{
		{AgentID: "zulu", Graph: g},
		{AgentID: "alpha", Graph: g},
		{AgentID: "mike", Graph: g},
	}
	r := MetaReconcile(g, peers)
	for _, nr := range r.NodeReports {
		for i := 1; i < len(nr.SupportingPeers); i++ {
			if nr.SupportingPeers[i-1] > nr.SupportingPeers[i] {
				t.Errorf("supporting peers not sorted for node %s: %v", nr.NodeID, nr.SupportingPeers)
			}
		}
	}
}
