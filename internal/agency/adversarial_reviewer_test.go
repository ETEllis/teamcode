package agency

import (
	"strings"
	"testing"
)

func TestReviewer_Defaults_PopulatedFromZero(t *testing.T) {
	r := NewAdversarialReviewer(AdversarialReviewerConfig{})
	if r.cfg.ReviewerID == "" {
		t.Errorf("expected default reviewer ID")
	}
	if r.cfg.HighConfidenceProbeFloor <= 0 {
		t.Errorf("expected non-zero default confidence probe floor")
	}
	if r.cfg.HeavyEvidenceWeightFloor <= 0 {
		t.Errorf("expected non-zero default heavy-evidence floor")
	}
}

func TestReviewer_NilOrEmptyVerdict_ReturnsNothing(t *testing.T) {
	r := NewAdversarialReviewer(AdversarialReviewerConfig{})
	if got := r.Review(GISTVerdict{}); len(got) != 0 {
		t.Errorf("empty verdict should produce no records, got %d", len(got))
	}
}

func TestReviewer_BlockedActionProbe_FlagsRecommendation(t *testing.T) {
	verdict := sampleVerdictForDispute()
	r := NewAdversarialReviewer(AdversarialReviewerConfig{ReviewerID: "rev"})
	records := r.Review(verdict)
	if len(records) == 0 {
		t.Fatalf("expected at least one record")
	}

	foundBlocked := false
	for _, rec := range records {
		if rec.Report.Dispute.Ground != DisputeGroundActionBlocked {
			continue
		}
		foundBlocked = true
		if rec.Report.Dispute.TargetNode != "act1" {
			t.Errorf("expected blocked probe to target act1, got %q", rec.Report.Dispute.TargetNode)
		}
	}
	if !foundBlocked {
		t.Errorf("expected an action-blocked probe; records: %+v", records)
	}
}

func TestReviewer_HeavyEvidenceProbe_GeneratesStaleDispute(t *testing.T) {
	verdict := sampleVerdictForDispute()
	r := NewAdversarialReviewer(AdversarialReviewerConfig{
		ReviewerID:               "rev",
		HighConfidenceProbeFloor: 99, // disable confounder probe
		HeavyEvidenceWeightFloor: 0.7,
	})
	records := r.Review(verdict)
	foundStale := false
	for _, rec := range records {
		if rec.Report.Dispute.Ground == DisputeGroundEvidenceStale {
			foundStale = true
			if rec.Report.Dispute.TargetNode == "" {
				t.Errorf("stale probe must set targetNode")
			}
		}
	}
	if !foundStale {
		t.Errorf("expected at least one evidence-stale probe; records: %+v", records)
	}
}

func TestReviewer_MissingConfounderProbe_OnlyWhenConfidenceHigh(t *testing.T) {
	verdict := sampleVerdictForDispute()

	// Force a high projected confidence so the probe fires.
	verdict.PearlPlan.Prediction.ProjectedConfidence = 0.9

	// Reduce confounders to 1 so the probe is willing to fire.
	verdict.PearlPlan.Hypothesis.Confounders = []NodeID{"conf1"}

	r := NewAdversarialReviewer(AdversarialReviewerConfig{ReviewerID: "rev"})
	records := r.Review(verdict)
	found := false
	for _, rec := range records {
		if rec.Report.Dispute.Ground == DisputeGroundMissingConfounder {
			found = true
		}
	}
	if !found {
		t.Errorf("expected missing-confounder probe under high confidence")
	}
}

func TestReviewer_MissingConfounderProbe_SkippedWhenLowConfidence(t *testing.T) {
	verdict := sampleVerdictForDispute()
	// Force projected confidence below the probe floor.
	if verdict.PearlPlan != nil {
		verdict.PearlPlan.Prediction.ProjectedConfidence = 0.4
	}
	r := NewAdversarialReviewer(AdversarialReviewerConfig{
		ReviewerID:               "rev",
		HighConfidenceProbeFloor: 0.7,
	})
	records := r.Review(verdict)
	for _, rec := range records {
		if rec.Report.Dispute.Ground == DisputeGroundMissingConfounder {
			t.Errorf("missing-confounder probe should not fire below floor; got %#v", rec.Report.Dispute)
		}
	}
}

func TestReviewer_RespectsMaxDisputesCap(t *testing.T) {
	verdict := sampleVerdictForDispute()
	r := NewAdversarialReviewer(AdversarialReviewerConfig{
		ReviewerID:               "rev",
		MaxDisputes:              1,
		HighConfidenceProbeFloor: 0.1, // ensure probe fires
		HeavyEvidenceWeightFloor: 0.1, // ensure stale probes fire
	})
	records := r.Review(verdict)
	if len(records) > 1 {
		t.Errorf("expected MaxDisputes=1 cap, got %d", len(records))
	}
}

func TestReviewer_DeterministicIDs(t *testing.T) {
	verdict := sampleVerdictForDispute()
	r := NewAdversarialReviewer(AdversarialReviewerConfig{ReviewerID: "rev"})
	a := r.Review(verdict)
	b := r.Review(verdict)
	if len(a) != len(b) {
		t.Fatalf("non-deterministic length: %d vs %d", len(a), len(b))
	}
	for i := range a {
		if a[i].Report.Dispute.ID != b[i].Report.Dispute.ID {
			t.Errorf("non-deterministic dispute ID at %d: %q vs %q",
				i, a[i].Report.Dispute.ID, b[i].Report.Dispute.ID)
		}
	}
}

func TestReviewer_SortsUpheldFirst(t *testing.T) {
	verdict := sampleVerdictForDispute()
	r := NewAdversarialReviewer(AdversarialReviewerConfig{
		ReviewerID:               "rev",
		HighConfidenceProbeFloor: 99, // suppress probe so we only have intrinsic disputes
	})
	records := r.Review(verdict)
	if len(records) < 2 {
		t.Skip("need at least two records for ordering check")
	}
	prev := statusRank(records[0].Adjudication.Status)
	for _, rec := range records[1:] {
		curr := statusRank(rec.Adjudication.Status)
		if curr < prev {
			t.Errorf("records out of status order: %s came after %s",
				rec.Adjudication.Status, statusRank(records[0].Adjudication.Status))
		}
		prev = curr
	}
}

func TestReviewer_DisputeIDFormat(t *testing.T) {
	verdict := sampleVerdictForDispute()
	r := NewAdversarialReviewer(AdversarialReviewerConfig{ReviewerID: "alpha"})
	records := r.Review(verdict)
	if len(records) == 0 {
		t.Skip("no records to check")
	}
	for _, rec := range records {
		id := rec.Report.Dispute.ID
		if !strings.HasPrefix(id, "alpha:") {
			t.Errorf("dispute ID %q should start with reviewer ID prefix", id)
		}
		if strings.Count(id, ":") < 2 {
			t.Errorf("dispute ID %q should follow reviewer:heuristic:slug shape", id)
		}
	}
}
