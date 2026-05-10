package agency

import (
	"testing"
)

func TestAdjudicate_NarrativeOrNoOp_AlwaysNoted(t *testing.T) {
	report := DisputeReport{
		Dispute: Dispute{Ground: DisputeGroundNarrative, Narrative: "context concern"},
		Applied: false,
	}
	a := Adjudicate(report)
	if a.Status != DisputeStatusNoted {
		t.Errorf("non-applied dispute should be noted, got %q", a.Status)
	}
	if a.Mechanical {
		t.Errorf("non-applied dispute should NOT be flagged mechanical")
	}
}

func TestAdjudicate_RecommendationFlip_Upheld(t *testing.T) {
	report := DisputeReport{
		Dispute:                      Dispute{Ground: DisputeGroundActionBlocked, TargetNode: "act1"},
		Applied:                      true,
		OriginalRecommendation:       "act1",
		CounterfactualRecommendation: "act2",
		RecommendationFlipped:        true,
		DeltaConfidence:              -0.05,
	}
	a := Adjudicate(report)
	if a.Status != DisputeStatusUpheld {
		t.Errorf("recommendation flip must be upheld, got %q (reason=%s)", a.Status, a.Reason)
	}
	if !a.Mechanical {
		t.Errorf("expected mechanical=true")
	}
	if a.SwingScore < recommendationFlipBonus {
		t.Errorf("expected swing >= %.2f from flip bonus, got %.3f", recommendationFlipBonus, a.SwingScore)
	}
}

func TestAdjudicate_BlockedFlip_Upheld(t *testing.T) {
	report := DisputeReport{
		Applied:                     true,
		OriginalBlockedByConfounder: false,
		CounterfactualBlocked:       true,
		BlockedByConfounderFlipped:  true,
		DeltaConfidence:             -0.04,
	}
	a := Adjudicate(report)
	if a.Status != DisputeStatusUpheld {
		t.Errorf("blocked-by-confounder flip must be upheld, got %q", a.Status)
	}
}

func TestAdjudicate_LargeConfidenceSwing_Upheld(t *testing.T) {
	report := DisputeReport{
		Applied:         true,
		DeltaConfidence: -0.20,
	}
	a := Adjudicate(report)
	if a.Status != DisputeStatusUpheld {
		t.Errorf("|Δconfidence|>=%.2f must be upheld, got %q", upholdConfidenceThreshold, a.Status)
	}
}

func TestAdjudicate_SubThresholdSwing_Noted(t *testing.T) {
	report := DisputeReport{
		Applied:         true,
		DeltaConfidence: -0.05,
	}
	a := Adjudicate(report)
	if a.Status != DisputeStatusNoted {
		t.Errorf("sub-threshold swing should be noted, got %q", a.Status)
	}
}

func TestAdjudicate_TopShapleyChange_Noted(t *testing.T) {
	report := DisputeReport{
		Applied:                  true,
		DeltaConfidence:          0.001,
		OriginalTopShapley:       "ev1",
		CounterfactualTopShapley: "ev2",
		TopShapleyChanged:        true,
	}
	a := Adjudicate(report)
	if a.Status != DisputeStatusNoted {
		t.Errorf("top-shapley change without confidence move should be noted, got %q", a.Status)
	}
}

func TestAdjudicate_NoSwing_Rejected(t *testing.T) {
	report := DisputeReport{
		Applied:         true,
		DeltaConfidence: 0.000,
	}
	a := Adjudicate(report)
	if a.Status != DisputeStatusRejected {
		t.Errorf("no-swing dispute should be rejected, got %q", a.Status)
	}
}

func TestAdjudicate_SwingScore_CompositeMonotone(t *testing.T) {
	// A report that flips both the recommendation and the blocking
	// flag should score strictly higher than a report with only one
	// of those signals at the same confidence delta.
	base := DisputeReport{Applied: true, DeltaConfidence: -0.10}
	flipOne := base
	flipOne.RecommendationFlipped = true
	flipBoth := flipOne
	flipBoth.BlockedByConfounderFlipped = true

	a0 := Adjudicate(base).SwingScore
	a1 := Adjudicate(flipOne).SwingScore
	a2 := Adjudicate(flipBoth).SwingScore
	if !(a0 < a1 && a1 < a2) {
		t.Errorf("swing score should be monotone in flag count: %.3f, %.3f, %.3f", a0, a1, a2)
	}
}

func TestAdjudicateBatch_PreservesOrder(t *testing.T) {
	reports := []DisputeReport{
		{Applied: true, DeltaConfidence: -0.20},                                  // upheld
		{Applied: false, Dispute: Dispute{Ground: DisputeGroundNarrative}},      // noted
		{Applied: true, DeltaConfidence: 0.0},                                    // rejected
	}
	out := AdjudicateBatch(reports)
	if len(out) != 3 {
		t.Fatalf("expected 3 adjudications, got %d", len(out))
	}
	want := []DisputeStatus{DisputeStatusUpheld, DisputeStatusNoted, DisputeStatusRejected}
	for i, w := range want {
		if out[i].Status != w {
			t.Errorf("position %d: want %q got %q (%s)", i, w, out[i].Status, out[i].Reason)
		}
	}
}

func TestNewDisputeRecord_StampsProtocolVersion(t *testing.T) {
	report := DisputeReport{Applied: true, DeltaConfidence: -0.30}
	rec := NewDisputeRecord(report)
	if rec.ProtocolVersion != DisputeProtocolVersion {
		t.Errorf("expected protocol %d, got %d", DisputeProtocolVersion, rec.ProtocolVersion)
	}
	if !rec.IsUpheld() {
		t.Errorf("record with -0.30 swing should be upheld; got %q", rec.Adjudication.Status)
	}
}

func TestNewDisputeRecord_RoundTripsReport(t *testing.T) {
	verdict := sampleVerdictForDispute()
	d := Dispute{
		ID:         "rev-001",
		Ground:     DisputeGroundActionBlocked,
		TargetNode: "act1",
	}
	report := EvaluateDispute(verdict, d)
	rec := NewDisputeRecord(report)
	if rec.Report.Dispute.ID != "rev-001" {
		t.Errorf("dispute ID lost in record")
	}
	// Blocking the only intervention should flip the recommendation
	// (which used to point at act1 itself) and uphold the dispute.
	if rec.Adjudication.Status != DisputeStatusUpheld {
		t.Errorf("expected upheld for blocked-recommended-action, got %q (reason=%s)",
			rec.Adjudication.Status, rec.Adjudication.Reason)
	}
}
