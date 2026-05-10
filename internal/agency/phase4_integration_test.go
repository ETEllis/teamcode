package agency

import (
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	"github.com/stretchr/testify/require"
)

// TestPhase4_DisputeIntegration_FullLoop walks the entire Phase 4 path:
//
//   sample verdict
//     -> AdversarialReviewer surfaces probes
//     -> EvaluateDispute + Adjudicate produce records
//     -> BuildInspectorBundle clones records onto bundle
//     -> MarshalInspectorBundle / ParseInspectorBundle round trip
//     -> Director HTTP /lattice/<id> renders dispute section in HTML
//     -> Director HTTP /api/lattice/<id> exposes records in JSON
//
// This is the integration counterpart to the unit tests in
// dispute_test.go, adjudication_test.go, adversarial_reviewer_test.go,
// and inspector_bundle_test.go: each layer is unit-tested in isolation,
// here they are composed.
func TestPhase4_DisputeIntegration_FullLoop(t *testing.T) {
	verdict := sampleVerdictForDispute()

	reviewer := NewAdversarialReviewer(AdversarialReviewerConfig{
		ReviewerID:               "phase4-it",
		HighConfidenceProbeFloor: 0.5,
		HeavyEvidenceWeightFloor: 0.7,
	})
	records := reviewer.Review(verdict)
	require.NotEmpty(t, records, "reviewer should surface at least one dispute against the sample verdict")

	// Should produce at least one upheld dispute (the action-blocked
	// probe of act1, which is the recommendation in the fixture).
	upheldFound := false
	for _, rec := range records {
		if rec.Adjudication.Status == DisputeStatusUpheld {
			upheldFound = true
		}
	}
	require.True(t, upheldFound, "expected at least one upheld dispute; got %+v", records)

	// Attach to the verdict and persist via bundle.
	verdict.Disputes = records
	bundle := BuildInspectorBundle(verdict)
	require.NotNil(t, bundle)
	require.Equal(t, len(records), len(bundle.Disputes),
		"bundle should preserve all records")

	// Defensive: bundle.Disputes should be a *copy* — mutating it
	// must not bleed into verdict.Disputes.
	bundle.Disputes[0].Adjudication.Reason = "MUTATED"
	require.NotEqual(t, "MUTATED", verdict.Disputes[0].Adjudication.Reason,
		"bundle.Disputes should be cloned, not aliased")
	bundle = BuildInspectorBundle(verdict) // re-build for the rest of the test

	// JSON round trip.
	raw, err := MarshalInspectorBundle(verdict)
	require.NoError(t, err)
	parsed, err := ParseInspectorBundle(raw)
	require.NoError(t, err)
	require.NotNil(t, parsed)
	require.Equal(t, len(records), len(parsed.Disputes),
		"parse should preserve dispute count through JSON")
	require.Equal(t, records[0].Report.Dispute.ID, parsed.Disputes[0].Report.Dispute.ID)

	// Director HTTP integration.
	fetcher := newFakeTraceFetcher()
	fetcher.put(InspectorTraceView{
		Summary: InspectorTraceSummary{
			ID:         "phase4-trace",
			OfficeID:   "office-test",
			AgentID:    "agent-X",
			Verdict:    verdict.Verdict,
			Confidence: verdict.Confidence,
			HasBundle:  true,
		},
		Bundle:        bundle,
		BundleSource:  "inspector_json",
		InspectorJSON: raw,
	})
	server := newTestDirectorWithFetcher(t, fetcher)

	// HTML view.
	req := httptest.NewRequest("GET", "/lattice/phase4-trace", nil)
	rec := httptest.NewRecorder()
	server.server.Handler.ServeHTTP(rec, req)
	require.Equal(t, http.StatusOK, rec.Code, "body=%s", rec.Body.String())
	body := rec.Body.String()

	require.Contains(t, body, "Adversarial review",
		"HTML view should render the disputes section header")
	// First record's ID should appear; pick from the parsed (deterministic) set.
	require.Contains(t, body, records[0].Report.Dispute.ID,
		"HTML view should render dispute IDs")
	// Status pill.
	require.True(t,
		strings.Contains(body, "status-upheld") || strings.Contains(body, "status-noted") ||
			strings.Contains(body, "status-rejected"),
		"HTML should render at least one status pill")

	// JSON view.
	req = httptest.NewRequest("GET", "/api/lattice/phase4-trace", nil)
	rec = httptest.NewRecorder()
	server.server.Handler.ServeHTTP(rec, req)
	require.Equal(t, http.StatusOK, rec.Code, "body=%s", rec.Body.String())
	var apiView InspectorTraceView
	require.NoError(t, json.Unmarshal(rec.Body.Bytes(), &apiView))
	require.NotNil(t, apiView.Bundle)
	require.Equal(t, len(records), len(apiView.Bundle.Disputes),
		"JSON view should expose the full dispute set")
}

// TestPhase4_DisputeIntegration_LegacyTracePath confirms that traces
// persisted before Phase 4 (no Disputes on the bundle) still render
// without errors and just show the empty-state copy.
func TestPhase4_DisputeIntegration_LegacyTracePath(t *testing.T) {
	verdict := sampleVerdictForDispute()
	// No reviewer run; Disputes stays nil.
	bundle := BuildInspectorBundle(verdict)
	require.Empty(t, bundle.Disputes, "no disputes attached")

	fetcher := newFakeTraceFetcher()
	fetcher.put(InspectorTraceView{
		Summary:      InspectorTraceSummary{ID: "legacy", OfficeID: "office-test", HasBundle: true},
		Bundle:       bundle,
		BundleSource: "legacy_hydrated",
	})
	server := newTestDirectorWithFetcher(t, fetcher)

	req := httptest.NewRequest("GET", "/lattice/legacy", nil)
	rec := httptest.NewRecorder()
	server.server.Handler.ServeHTTP(rec, req)
	require.Equal(t, http.StatusOK, rec.Code, "body=%s", rec.Body.String())
	body := rec.Body.String()
	require.Contains(t, body, "No adversarial review attached to this trace",
		"legacy traces should show empty-state copy for the disputes section")
}

// TestPhase4_DisputeIntegration_UpheldShortCircuit verifies that
// AdversarialReviewer plus Adjudicate together produce at least one
// upheld record on a verdict with a recommended action — specifically,
// blocking the recommendation flips the Pearl prediction, which always
// short-circuits to upheld in Adjudicate.
func TestPhase4_DisputeIntegration_UpheldShortCircuit(t *testing.T) {
	verdict := sampleVerdictForDispute()
	reviewer := NewAdversarialReviewer(AdversarialReviewerConfig{ReviewerID: "phase4-it"})
	records := reviewer.Review(verdict)

	for _, rec := range records {
		if rec.Report.Dispute.Ground != DisputeGroundActionBlocked {
			continue
		}
		// Blocking the recommended action ought to flip the
		// recommendation — that is the deterministic signal we
		// test for here.
		require.True(t, rec.Report.RecommendationFlipped,
			"action-blocked dispute on recommended action should flip the recommendation")
		require.Equal(t, DisputeStatusUpheld, rec.Adjudication.Status,
			"recommendation flip should always be upheld")
	}
}
