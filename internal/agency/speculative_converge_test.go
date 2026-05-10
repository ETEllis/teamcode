package agency

import (
	"testing"
)

func attestFromGraph(agentID string, g *CausalGraph) PeerAttestation {
	return PeerAttestation{AgentID: agentID, Attestation: MerkleAttest(g)}
}

func TestMerkleConverge_EmptyPeers_Divergent(t *testing.T) {
	r := MerkleConverge(nil)
	if r.Status != ConvergenceStatusDivergent {
		t.Errorf("empty peers should be divergent, got %q", r.Status)
	}
	if r.IsGateOpen() {
		t.Errorf("divergent gate must be closed")
	}
	if r.ConsensusRoot != "" {
		t.Errorf("expected empty ConsensusRoot, got %q", r.ConsensusRoot)
	}
}

func TestMerkleConverge_AllAgree_Converged(t *testing.T) {
	g := merkleSampleGraph()
	peers := []PeerAttestation{
		attestFromGraph("a", g),
		attestFromGraph("b", g),
		attestFromGraph("c", g),
	}
	r := MerkleConverge(peers)
	if r.Status != ConvergenceStatusConverged {
		t.Errorf("unanimous cohort should be converged, got %q (quorum=%.2f)", r.Status, r.Quorum)
	}
	if r.Quorum != 1.0 {
		t.Errorf("expected quorum=1.0, got %.3f", r.Quorum)
	}
	if r.ConsensusRoot != peers[0].Attestation.Root {
		t.Errorf("consensus root mismatch")
	}
	if !r.IsGateOpen() {
		t.Errorf("converged gate must be open")
	}
	if len(r.DivergenceLoci) != 0 {
		t.Errorf("unanimous cohort should have no divergence loci, got %d", len(r.DivergenceLoci))
	}
	bucket := r.ConsensusBucketAgents()
	if len(bucket) != 3 {
		t.Errorf("expected all 3 agents in bucket, got %d", len(bucket))
	}
}

func TestMerkleConverge_TwoOfThree_Converged_AtDefaultThreshold(t *testing.T) {
	g1 := merkleSampleGraph()
	g2 := merkleSampleGraph()
	g2.Nodes = append(g2.Nodes, CausalNode{ID: "extra", Role: NodeRoleEvidence, Summary: "extra atom", Weight: 0.3})
	peers := []PeerAttestation{
		attestFromGraph("a", g1),
		attestFromGraph("b", g1),
		attestFromGraph("c", g2),
	}
	r := MerkleConverge(peers)
	if r.Status != ConvergenceStatusConverged {
		t.Errorf("2/3 unanimous should clear default 0.66 threshold; got %q quorum=%.3f", r.Status, r.Quorum)
	}
	if r.ConsensusRoot != peers[0].Attestation.Root {
		t.Errorf("consensus should match the majority graph")
	}
	// c should be the dissenter; we expect a divergence locus pointing at the 'extra' leaf
	// from c's attestation.
	if len(r.DivergenceLoci) == 0 {
		t.Errorf("expected at least one divergence locus from dissenting peer")
	}
	foundC := false
	for _, l := range r.DivergenceLoci {
		for _, agent := range l.Inclusion {
			if agent == "c" {
				foundC = true
			}
		}
	}
	if !foundC {
		t.Errorf("expected dissenter 'c' in some divergence locus inclusion set")
	}
}

func TestMerkleConverge_TwoOfThree_Partial_AtTighterThreshold(t *testing.T) {
	g1 := merkleSampleGraph()
	g2 := merkleSampleGraph()
	g2.Nodes = append(g2.Nodes, CausalNode{ID: "extra", Role: NodeRoleEvidence, Summary: "x", Weight: 0.3})
	peers := []PeerAttestation{
		attestFromGraph("a", g1),
		attestFromGraph("b", g1),
		attestFromGraph("c", g2),
	}
	r := MerkleConvergeWithThreshold(peers, 0.9)
	if r.Status != ConvergenceStatusPartial {
		t.Errorf("2/3 below 0.9 threshold should be partial, got %q", r.Status)
	}
	if !r.IsGateOpen() {
		t.Errorf("partial gate should still be open (with caveats)")
	}
	if r.ConsensusRoot != peers[0].Attestation.Root {
		t.Errorf("partial bucket should still surface the plurality root")
	}
}

func TestMerkleConverge_ThreeWayTie_Divergent(t *testing.T) {
	g1 := merkleSampleGraph()
	g2 := merkleSampleGraph()
	g2.Nodes[0].Weight = 0.5 // change a node so root differs
	g3 := merkleSampleGraph()
	g3.Nodes[1].Role = NodeRoleEvidence // role flip changes root
	peers := []PeerAttestation{
		attestFromGraph("a", g1),
		attestFromGraph("b", g2),
		attestFromGraph("c", g3),
	}
	r := MerkleConverge(peers)
	if r.Status != ConvergenceStatusDivergent {
		t.Errorf("three-way tie should be divergent, got %q", r.Status)
	}
	if r.IsGateOpen() {
		t.Errorf("divergent gate must be closed")
	}
}

func TestMerkleConverge_BucketAgentsSorted(t *testing.T) {
	g := merkleSampleGraph()
	peers := []PeerAttestation{
		attestFromGraph("zulu", g),
		attestFromGraph("alpha", g),
		attestFromGraph("mike", g),
	}
	r := MerkleConverge(peers)
	bucket := r.ConsensusBucketAgents()
	if len(bucket) != 3 {
		t.Fatalf("expected 3 bucket agents, got %d", len(bucket))
	}
	for i := 1; i < len(bucket); i++ {
		if bucket[i-1] > bucket[i] {
			t.Errorf("bucket agents not sorted: %v", bucket)
		}
	}
}

func TestMerkleConverge_Histogram_AllRootsCounted(t *testing.T) {
	g1 := merkleSampleGraph()
	g2 := merkleSampleGraph()
	g2.Nodes[0].Weight = 0.5
	peers := []PeerAttestation{
		attestFromGraph("a", g1),
		attestFromGraph("b", g1),
		attestFromGraph("c", g2),
		attestFromGraph("d", g2),
	}
	r := MerkleConverge(peers)
	if len(r.RootHistogram) != 2 {
		t.Errorf("expected 2 distinct roots in histogram, got %d", len(r.RootHistogram))
	}
	total := 0
	for _, c := range r.RootHistogram {
		total += c
	}
	if total != 4 {
		t.Errorf("histogram sum %d != agent count 4", total)
	}
}

func TestMerkleConverge_BadThreshold_FallsBackToDefault(t *testing.T) {
	peers := []PeerAttestation{attestFromGraph("a", merkleSampleGraph())}
	for _, bad := range []float64{0, -0.5, 1.5, 2.0} {
		r := MerkleConvergeWithThreshold(peers, bad)
		if r.Threshold != DefaultConvergenceThreshold {
			t.Errorf("threshold %v should fall back to default %v, got %v", bad, DefaultConvergenceThreshold, r.Threshold)
		}
	}
}

func TestMerkleConverge_DivergenceSeverity(t *testing.T) {
	g1 := merkleSampleGraph()
	g2 := merkleSampleGraph()
	// remove one node from g2 so it's missing relative to consensus
	g2.Nodes = g2.Nodes[:len(g2.Nodes)-1]
	peers := []PeerAttestation{
		attestFromGraph("a", g1),
		attestFromGraph("b", g1),
		attestFromGraph("c", g2),
	}
	r := MerkleConverge(peers)
	missingFound := false
	for _, l := range r.DivergenceLoci {
		if l.Severity == "missing" {
			missingFound = true
		}
	}
	if !missingFound {
		t.Errorf("expected a 'missing' severity locus when dissenter dropped a node")
	}
}
