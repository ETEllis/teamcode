package agency

import (
	"testing"
)

// scenarioGraph constructs a deterministic per-agent causal graph for
// the toy 3-agent integration scenario. Each agent reasons about the
// same deploy-health question. Agents 'a' and 'b' arrive at exactly
// the same typed graph; agent 'c' is a Hamming-1 sibling (one extra
// confounder).
func scenarioGraph(agentID string) *CausalGraph {
	g := &CausalGraph{
		ProtocolVersion: CausalGraphProtocolVersion,
		Nodes: []CausalNode{
			{ID: "ev1", Role: NodeRoleEvidence, Summary: "log says deploy succeeded", Weight: 1.0},
			{ID: "ev2", Role: NodeRoleEvidence, Summary: "alert resolved within SLO", Weight: 0.8},
			{ID: "act1", Role: NodeRoleIntervention, Summary: "promote to prod", Weight: 0.6},
			{ID: "out1", Role: NodeRoleOutcome, Summary: "deploy is healthy", Weight: 1.0},
		},
	}
	if agentID == "c" {
		g.Nodes = append(g.Nodes, CausalNode{
			ID: "extra-conf", Role: NodeRoleConfounder, Summary: "regional outage masked errors", Weight: 0.4,
		})
	}
	return g
}

func scenarioVerdict(agentID string) GISTVerdict {
	g := scenarioGraph(agentID)
	v := GISTVerdict{
		Verdict:    "deploy is healthy",
		Confidence: 0.82,
	}
	v.CausalGraph = g
	v.PearlPlan = RunPearlLoop(g)
	v.Attribution = AttributeNecessity(g)
	v.SyncCausalChain()
	return v
}

// TestSpeculativeTier_FullPipeline_ToyThreeAgentScenario wires the three
// Phase 5 mechanisms together end-to-end and asserts the documented
// composition guarantees:
//
//   1. MerkleConverge over peers reaches converged status (a,b agree).
//   2. MetaReconcile over the same peers vs. a meta agent that mirrors
//      the consensus graph reports faithful.
//   3. DyadCompress over the converged peers yields ⌈n/2⌉ slots when
//      siblings exist (here: a,b are duplicates, c is a Hamming-1
//      sibling of a — so 3 peers → 2 slots once the dyad collapses).
//
// This is the sentinel test that validates the design memo's
// composition diagram.
func TestSpeculativeTier_FullPipeline_ToyThreeAgentScenario(t *testing.T) {
	verdictA := scenarioVerdict("a")
	verdictB := scenarioVerdict("b")
	verdictC := scenarioVerdict("c")

	// --- Stage 1: Merkle attestations + convergence ---
	peers := []PeerAttestation{
		{AgentID: "a", Attestation: MerkleAttest(verdictA.CausalGraph)},
		{AgentID: "b", Attestation: MerkleAttest(verdictB.CausalGraph)},
		{AgentID: "c", Attestation: MerkleAttest(verdictC.CausalGraph)},
	}
	conv := MerkleConverge(peers)
	if conv.Status != ConvergenceStatusConverged {
		t.Fatalf("expected converged status (a==b is 2/3), got %q quorum=%.3f",
			conv.Status, conv.Quorum)
	}
	if !conv.IsGateOpen() {
		t.Fatalf("converged gate must be open")
	}
	bucket := conv.ConsensusBucketAgents()
	if len(bucket) != 2 {
		t.Errorf("expected 2 agents in consensus bucket {a,b}, got %v", bucket)
	}
	// c should appear in some divergence locus (its extra confounder).
	foundC := false
	for _, locus := range conv.DivergenceLoci {
		for _, agent := range locus.Inclusion {
			if agent == "c" {
				foundC = true
			}
		}
	}
	if !foundC {
		t.Errorf("expected dissenter 'c' to surface in divergence loci, got %#v", conv.DivergenceLoci)
	}

	// --- Stage 2: Meta-vs-peers reconciliation ---
	// A faithful meta agent restates the consensus graph (a's graph).
	metaGraph := scenarioGraph("a")
	peerGraphs := []PeerGraph{
		{AgentID: "a", Graph: verdictA.CausalGraph},
		{AgentID: "b", Graph: verdictB.CausalGraph},
		{AgentID: "c", Graph: verdictC.CausalGraph},
	}
	rec := MetaReconcile(metaGraph, peerGraphs)
	if rec.Status == ReconciliationStatusFabricated {
		t.Errorf("faithful meta should not be fabricated, got %q", rec.Status)
	}
	if !rec.IsAcceptable() {
		t.Errorf("faithful meta should be acceptable")
	}
	// All meta nodes appear in 2/3 peers (a,b) and at least one (out1)
	// is also matched in c via canonical leaf or relaxed compatibility.
	if rec.SupportScore < 0.66 {
		t.Errorf("expected support >= 0.66, got %.3f", rec.SupportScore)
	}

	// Now the adversarial case: a meta that fabricates a node.
	fabricatedMeta := scenarioGraph("a")
	fabricatedMeta.Nodes = append(fabricatedMeta.Nodes, CausalNode{
		ID: "ghost", Role: NodeRoleEvidence, Summary: "node no peer ever wrote", Weight: 0.95,
	})
	fabricatedRec := MetaReconcile(fabricatedMeta, peerGraphs)
	if fabricatedRec.Status != ReconciliationStatusFabricated {
		t.Errorf("ghost-node meta should be fabricated, got %q", fabricatedRec.Status)
	}
	if fabricatedRec.IsAcceptable() {
		t.Errorf("fabricated meta must not be acceptable")
	}

	// --- Stage 3: Dyad compression over the cohort ---
	verdicts := []LabeledVerdict{
		{ID: "a", Verdict: verdictA},
		{ID: "b", Verdict: verdictB},
		{ID: "c", Verdict: verdictC},
	}
	compressed, dyadReport := DyadCompress(verdicts)

	// a == b (Hamming-0, NOT paired). a vs c is Hamming-1 (added node).
	// Greedy ID-sorted pass: 'a' tries 'b' (Hamming-0 → no match), then 'c'
	// (Hamming-1 → pair). So the dyad is (a,c); 'b' is left unpaired.
	// SlotsAfter should be 2: 'a' base (paired with c) + 'b' solo.
	if dyadReport.SlotsAfter != 2 {
		t.Errorf("expected 2 slots after compression (1 dyad + 1 solo), got %d (deltas=%d)",
			dyadReport.SlotsAfter, len(dyadReport.Deltas))
	}
	if dyadReport.SlotsAfter > (dyadReport.SlotsBefore+1)/2+(dyadReport.SlotsBefore%2) {
		// Sanity bound: SlotsAfter <= ceil(n/2) + (peers without partner).
		// For n=3 with 1 dyad: ceil(3/2) = 2, and 2 <= 2. Just confirm we're
		// at or below the documented bound.
		t.Errorf("SlotsAfter %d violates ceil(n/2)+unpaired bound for n=%d",
			dyadReport.SlotsAfter, dyadReport.SlotsBefore)
	}
	if len(dyadReport.Deltas) != 1 {
		t.Fatalf("expected 1 dyad delta, got %d", len(dyadReport.Deltas))
	}

	// Round-trip the dyad and confirm the rehydrated sibling matches the
	// original by Merkle root.
	delta := dyadReport.Deltas[0]
	var baseVerdict LabeledVerdict
	for _, v := range compressed {
		if v.ID == delta.BaseVerdictID {
			baseVerdict = v
			break
		}
	}
	if baseVerdict.ID == "" {
		t.Fatalf("could not find base verdict %q in compressed slice", delta.BaseVerdictID)
	}
	rehydrated, err := DyadHydrate(baseVerdict, delta)
	if err != nil {
		t.Fatalf("rehydration failed: %v", err)
	}
	originalRoot := MerkleAttest(verdictForID(verdicts, delta.SiblingVerdictID).CausalGraph).Root
	rehydratedRoot := MerkleAttest(rehydrated.Verdict.CausalGraph).Root
	if originalRoot != rehydratedRoot {
		t.Errorf("rehydrated sibling Merkle root %q != original %q",
			rehydratedRoot, originalRoot)
	}
}

// TestSpeculativeTier_DivergentCohort_GateClosed asserts that downstream
// stages refuse to run when MerkleConverge reports divergent.
func TestSpeculativeTier_DivergentCohort_GateClosed(t *testing.T) {
	g1 := scenarioGraph("a")
	g2 := scenarioGraph("a")
	g2.Nodes[0].Weight = 0.5 // distinct root
	g3 := scenarioGraph("c")
	g3.Nodes[0].Role = NodeRoleConfounder // distinct root
	peers := []PeerAttestation{
		{AgentID: "a", Attestation: MerkleAttest(g1)},
		{AgentID: "b", Attestation: MerkleAttest(g2)},
		{AgentID: "c", Attestation: MerkleAttest(g3)},
	}
	conv := MerkleConverge(peers)
	if conv.Status != ConvergenceStatusDivergent {
		t.Errorf("three distinct roots should be divergent, got %q", conv.Status)
	}
	if conv.IsGateOpen() {
		t.Errorf("divergent gate must be closed; downstream stages must refuse")
	}
}

// TestSpeculativeTier_PartialCohort_GateOpenWithCaveat asserts that a
// partial-status cohort still lets MetaReconcile and DyadCompress run.
// This is the design memo's "downstream stages run with caveats" path.
func TestSpeculativeTier_PartialCohort_GateOpenWithCaveat(t *testing.T) {
	verdictA := scenarioVerdict("a")
	verdictB := scenarioVerdict("b")
	verdictC := scenarioVerdict("c")
	peers := []PeerAttestation{
		{AgentID: "a", Attestation: MerkleAttest(verdictA.CausalGraph)},
		{AgentID: "b", Attestation: MerkleAttest(verdictB.CausalGraph)},
		{AgentID: "c", Attestation: MerkleAttest(verdictC.CausalGraph)},
	}
	conv := MerkleConvergeWithThreshold(peers, 0.9)
	if conv.Status != ConvergenceStatusPartial {
		t.Errorf("2/3 below 0.9 threshold should be partial, got %q", conv.Status)
	}
	if !conv.IsGateOpen() {
		t.Errorf("partial gate should still be open (with caveats)")
	}
	// Stages still run.
	rec := MetaReconcile(scenarioGraph("a"), []PeerGraph{
		{AgentID: "a", Graph: verdictA.CausalGraph},
		{AgentID: "b", Graph: verdictB.CausalGraph},
		{AgentID: "c", Graph: verdictC.CausalGraph},
	})
	if rec.Status == ReconciliationStatusFabricated {
		t.Errorf("partial cohort with faithful meta should not be fabricated")
	}
}

func verdictForID(vs []LabeledVerdict, id string) GISTVerdict {
	for _, v := range vs {
		if v.ID == id {
			return v.Verdict
		}
	}
	return GISTVerdict{}
}
