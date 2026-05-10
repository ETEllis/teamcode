package agency

import (
	"testing"
)

// sampleVerdictForDispute fabricates a small but causally-rich verdict
// with two evidence nodes, a confounder, an intervention, and an outcome
// so each DisputeGround has something to bite on.
func sampleVerdictForDispute() GISTVerdict {
	graph := &CausalGraph{
		ProtocolVersion: CausalGraphProtocolVersion,
		Nodes: []CausalNode{
			{ID: "ev1", Role: NodeRoleEvidence, Summary: "log says deploy succeeded", Weight: 1.0},
			{ID: "ev2", Role: NodeRoleEvidence, Summary: "alert resolved within SLO", Weight: 0.8},
			{ID: "conf1", Role: NodeRoleConfounder, Summary: "regional outage masked errors", Weight: 0.4},
			{ID: "act1", Role: NodeRoleIntervention, Summary: "promote to prod", Weight: 0.6},
			{ID: "out1", Role: NodeRoleOutcome, Summary: "deploy is healthy", Weight: 1.0},
		},
	}
	verdict := GISTVerdict{
		Verdict:    "deploy is healthy",
		Confidence: 0.82,
	}
	verdict.CausalGraph = graph
	verdict.PearlPlan = RunPearlLoop(graph)
	verdict.Attribution = AttributeNecessity(graph)
	verdict.SyncCausalChain()
	return verdict
}

func TestEvaluateDispute_MislabeledRole_FlipsConfidenceDownward(t *testing.T) {
	verdict := sampleVerdictForDispute()

	// Reviewer claims ev1 is actually a confounder, not evidence.
	d := Dispute{
		ID:         "rev-001",
		ReviewerID: "adversary",
		Ground:     DisputeGroundMislabeledRole,
		TargetNode: "ev1",
		NewRole:    NodeRoleConfounder,
	}
	report := EvaluateDispute(verdict, d)
	if !report.Applied {
		t.Fatalf("expected applied=true, got %#v notes=%v", report, report.Notes)
	}
	if report.DeltaConfidence >= 0 {
		t.Errorf("expected confidence to drop when evidence is reclassified as confounder, got delta=%.3f",
			report.DeltaConfidence)
	}
	if report.OriginalConfidence == report.CounterfactualConfidence {
		t.Errorf("counterfactual matched original; mutation didn't propagate")
	}
}

func TestEvaluateDispute_MissingConfounder_AddsAndShifts(t *testing.T) {
	verdict := sampleVerdictForDispute()
	d := Dispute{
		ID:           "rev-002",
		Ground:       DisputeGroundMissingConfounder,
		AddedSummary: "Telemetry pipeline was down",
		AddedWeight:  0.9,
	}
	report := EvaluateDispute(verdict, d)
	if !report.Applied {
		t.Fatalf("expected applied=true, got %#v", report)
	}
	if report.DeltaConfidence >= 0 {
		t.Errorf("expected confidence drop when confounder added, got %.3f", report.DeltaConfidence)
	}
	// Counterfactual graph should have one more node.
	if report.CounterfactualPlan == nil {
		t.Fatalf("expected counterfactual plan")
	}
}

func TestEvaluateDispute_ActionBlocked_RemovesFromCandidates(t *testing.T) {
	verdict := sampleVerdictForDispute()
	d := Dispute{
		ID:         "rev-003",
		Ground:     DisputeGroundActionBlocked,
		TargetNode: "act1",
	}
	report := EvaluateDispute(verdict, d)
	if !report.Applied {
		t.Fatalf("expected applied=true, got notes=%v", report.Notes)
	}
	if report.CounterfactualPlan == nil {
		t.Fatalf("expected counterfactual plan")
	}
	for _, c := range report.CounterfactualPlan.Actions {
		if c.NodeID == "act1" {
			t.Errorf("blocked action %q still present in counterfactual candidates", c.NodeID)
		}
	}
	if !report.RecommendationFlipped && report.OriginalRecommendation == "act1" {
		t.Errorf("blocking the recommended action should flip the recommendation; got %q->%q",
			report.OriginalRecommendation, report.CounterfactualRecommendation)
	}
}

func TestEvaluateDispute_EvidenceStale_DegradesWeight(t *testing.T) {
	verdict := sampleVerdictForDispute()
	d := Dispute{
		ID:          "rev-004",
		Ground:      DisputeGroundEvidenceStale,
		TargetNode:  "ev1",
		StaleFactor: 0.1,
	}
	report := EvaluateDispute(verdict, d)
	if !report.Applied {
		t.Fatalf("expected applied=true, got %#v", report)
	}
	if report.DeltaConfidence >= 0 {
		t.Errorf("staling supportive evidence should drop confidence, got %.3f", report.DeltaConfidence)
	}
}

func TestEvaluateDispute_Narrative_NoMutation(t *testing.T) {
	verdict := sampleVerdictForDispute()
	d := Dispute{
		ID:        "rev-005",
		Ground:    DisputeGroundNarrative,
		Narrative: "I dispute the framing of this verdict on procedural grounds.",
	}
	report := EvaluateDispute(verdict, d)
	if report.Applied {
		t.Errorf("narrative ground must not mutate the graph")
	}
	if report.DeltaConfidence != 0 {
		t.Errorf("narrative dispute should produce zero confidence swing, got %.3f",
			report.DeltaConfidence)
	}
}

func TestEvaluateDispute_UnknownTarget_NoOp(t *testing.T) {
	verdict := sampleVerdictForDispute()
	d := Dispute{
		ID:         "rev-006",
		Ground:     DisputeGroundEvidenceStale,
		TargetNode: "does-not-exist",
	}
	report := EvaluateDispute(verdict, d)
	if report.Applied {
		t.Errorf("non-existent target should be a no-op")
	}
	if len(report.Notes) == 0 {
		t.Errorf("expected diagnostic note for missing target")
	}
}

func TestApplyDisputeClaim_LeavesOriginalGraphUnchanged(t *testing.T) {
	verdict := sampleVerdictForDispute()
	originalLen := len(verdict.CausalGraph.Nodes)
	originalEv1Role := verdict.CausalGraph.Nodes[0].Role

	d := Dispute{
		Ground:     DisputeGroundMislabeledRole,
		TargetNode: "ev1",
		NewRole:    NodeRoleConfounder,
	}
	cf, applied, _ := ApplyDisputeClaim(verdict.CausalGraph, d)
	if !applied {
		t.Fatalf("expected applied=true")
	}
	if len(verdict.CausalGraph.Nodes) != originalLen {
		t.Errorf("original graph length mutated: was %d now %d",
			originalLen, len(verdict.CausalGraph.Nodes))
	}
	if verdict.CausalGraph.Nodes[0].Role != originalEv1Role {
		t.Errorf("original node role mutated: was %q now %q",
			originalEv1Role, verdict.CausalGraph.Nodes[0].Role)
	}
	if cf.Nodes[0].Role != NodeRoleConfounder {
		t.Errorf("counterfactual mutation didn't take: %q", cf.Nodes[0].Role)
	}
}

func TestEvaluateDispute_NilGraph(t *testing.T) {
	verdict := GISTVerdict{Verdict: "no chain"}
	d := Dispute{Ground: DisputeGroundMislabeledRole, TargetNode: "x", NewRole: NodeRoleEvidence}
	report := EvaluateDispute(verdict, d)
	if report.Applied {
		t.Errorf("nil-graph dispute should not apply")
	}
}

func TestSortDisputeReports_FlipsFirst(t *testing.T) {
	reports := []DisputeReport{
		{Dispute: Dispute{ID: "a"}, DeltaConfidence: 0.05},
		{Dispute: Dispute{ID: "b"}, DeltaConfidence: -0.30, RecommendationFlipped: true},
		{Dispute: Dispute{ID: "c"}, DeltaConfidence: -0.20},
	}
	SortDisputeReports(reports)
	if reports[0].Dispute.ID != "b" {
		t.Errorf("expected recommendation-flipping report first, got %q", reports[0].Dispute.ID)
	}
	if reports[1].Dispute.ID != "c" {
		t.Errorf("expected larger-swing report second, got %q", reports[1].Dispute.ID)
	}
	if reports[2].Dispute.ID != "a" {
		t.Errorf("expected smallest-swing report last, got %q", reports[2].Dispute.ID)
	}
}

func TestSimpleSlug(t *testing.T) {
	cases := map[string]string{
		"Telemetry pipeline was down": "telemetry-pipeline-was-down",
		"   leading-and-trailing   ":   "leading-and-trailing",
		"$$$ only symbols $$$":         "only-symbols",
		"":                             "anon",
		"!!!!!!!":                      "anon",
		"a-b-c":                        "a-b-c",
	}
	for in, want := range cases {
		if got := simpleSlug(in); got != want {
			t.Errorf("simpleSlug(%q) = %q, want %q", in, got, want)
		}
	}
}
