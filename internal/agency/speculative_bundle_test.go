package agency

import (
	"encoding/json"
	"testing"

	"github.com/stretchr/testify/require"
)

// fixtureCohortGraph builds a tiny causal graph identifiable by id so
// peers can vary. It mirrors the shape used in
// speculative_integration_test.go but lives here so this test file is
// independent of the integration suite's fixtures.
func fixtureCohortGraph(suffix string) *CausalGraph {
	return &CausalGraph{
		Nodes: []CausalNode{
			{ID: NodeID("ev-" + suffix), Role: NodeRoleEvidence, Summary: "obs", Weight: 0.7},
			{ID: NodeID("ot-" + suffix), Role: NodeRoleOutcome, Summary: "result", Weight: 0.5,
				Parents: []NodeID{NodeID("ev-" + suffix)}},
		},
	}
}

// fixtureCohortVerdict pairs a fixture graph with a labelled GISTVerdict.
func fixtureCohortVerdict(id, suffix string, conf float64) LabeledVerdict {
	return LabeledVerdict{
		ID: id,
		Verdict: GISTVerdict{
			Verdict:     "approved",
			Confidence:  conf,
			CausalGraph: fixtureCohortGraph(suffix),
		},
	}
}

func TestBuildSpeculativeBundle_HappyPath(t *testing.T) {
	a := fixtureCohortVerdict("agent-a", "x", 0.81)
	b := fixtureCohortVerdict("agent-b", "x", 0.79)
	c := fixtureCohortVerdict("agent-c", "x", 0.77)

	bundle := BuildSpeculativeBundle(SpeculativeBuildInput{
		CohortID: "cohort-test",
		Verdicts: []LabeledVerdict{a, b, c},
	})

	require.NotNil(t, bundle)
	require.Equal(t, SpeculativeBundleProtocolVersion, bundle.ProtocolVersion)
	require.Equal(t, "cohort-test", bundle.CohortID)
	require.Len(t, bundle.Peers, 3)
	require.Nil(t, bundle.Meta, "no meta supplied")

	require.NotNil(t, bundle.Convergence)
	require.Equal(t, ConvergenceStatusConverged, bundle.Convergence.Status)

	// All three peers share the same Merkle root → all in cohort.
	for _, p := range bundle.Peers {
		require.True(t, p.InCohort, "peer %s should be in consensus bucket", p.AgentID)
	}

	require.Nil(t, bundle.Reconciliation, "no meta → no reconciliation report")

	require.NotNil(t, bundle.Dyads)
	require.Equal(t, 3, bundle.Dyads.SlotsBefore)
}

func TestBuildSpeculativeBundle_DerivedCohortID(t *testing.T) {
	a := fixtureCohortVerdict("agent-a", "y", 0.5)
	b := fixtureCohortVerdict("agent-b", "y", 0.5)

	bundle := BuildSpeculativeBundle(SpeculativeBuildInput{
		Verdicts: []LabeledVerdict{a, b},
	})
	require.NotNil(t, bundle)
	require.NotEmpty(t, bundle.CohortID)
	require.Contains(t, bundle.CohortID, "cohort-")

	// Same cohort → same derived id (order-independent).
	bundle2 := BuildSpeculativeBundle(SpeculativeBuildInput{
		Verdicts: []LabeledVerdict{b, a},
	})
	require.Equal(t, bundle.CohortID, bundle2.CohortID,
		"derived cohort id must be stable across input ordering")
}

func TestBuildSpeculativeBundle_WithMeta_FaithfulAccepted(t *testing.T) {
	a := fixtureCohortVerdict("agent-a", "z", 0.8)
	b := fixtureCohortVerdict("agent-b", "z", 0.8)

	// Meta uses the same fixture suffix → faithful by construction.
	meta := fixtureCohortVerdict("meta", "z", 0.85)

	bundle := BuildSpeculativeBundle(SpeculativeBuildInput{
		Verdicts: []LabeledVerdict{a, b},
		Meta:     &meta,
	})
	require.NotNil(t, bundle)
	require.NotNil(t, bundle.Meta)
	require.Equal(t, "meta", bundle.Meta.AgentID)
	require.NotNil(t, bundle.Reconciliation)
	require.True(t, bundle.Reconciliation.IsAcceptable(),
		"meta with identical structure should be accepted")
}

func TestBuildSpeculativeBundle_EmptyCohort(t *testing.T) {
	require.Nil(t, BuildSpeculativeBundle(SpeculativeBuildInput{}))
}

func TestSpeculativeBundle_RoundTripMarshalParse(t *testing.T) {
	a := fixtureCohortVerdict("agent-a", "rt", 0.6)
	b := fixtureCohortVerdict("agent-b", "rt", 0.6)
	bundle := BuildSpeculativeBundle(SpeculativeBuildInput{
		CohortID: "cohort-rt",
		Verdicts: []LabeledVerdict{a, b},
	})
	require.NotNil(t, bundle)

	raw, err := MarshalSpeculativeBundle(bundle)
	require.NoError(t, err)
	require.NotEqual(t, "{}", raw)

	parsed, err := ParseSpeculativeBundle(raw)
	require.NoError(t, err)
	require.NotNil(t, parsed)
	require.Equal(t, bundle.CohortID, parsed.CohortID)
	require.Len(t, parsed.Peers, 2)
	require.Equal(t, bundle.Peers[0].Attestation.Root, parsed.Peers[0].Attestation.Root)
}

func TestParseSpeculativeBundle_EmptyAndZeroEnvelope(t *testing.T) {
	for _, raw := range []string{"", "{}", "null", "  "} {
		got, err := ParseSpeculativeBundle(raw)
		require.NoError(t, err, "raw=%q", raw)
		require.Nil(t, got, "raw=%q", raw)
	}

	// A JSON object that parses but carries zero fields → also nil.
	zeroEnv, err := json.Marshal(map[string]any{"unrelated": "field"})
	require.NoError(t, err)
	got, err := ParseSpeculativeBundle(string(zeroEnv))
	require.NoError(t, err)
	require.Nil(t, got, "envelope with no recognised fields → nil")
}

func TestMarshalSpeculativeBundle_NilGivesEmptyObject(t *testing.T) {
	raw, err := MarshalSpeculativeBundle(nil)
	require.NoError(t, err)
	require.Equal(t, "{}", raw)
}

func TestSpeculativeBundle_HeadlineStatus(t *testing.T) {
	// converged + faithful -> "converged"
	good := &SpeculativeBundle{
		Convergence: &ConvergenceReport{Status: ConvergenceStatusConverged},
	}
	require.Equal(t, "converged", good.HeadlineStatus())

	partial := &SpeculativeBundle{
		Convergence: &ConvergenceReport{Status: ConvergenceStatusPartial},
	}
	require.Equal(t, "partial", partial.HeadlineStatus())

	divergent := &SpeculativeBundle{
		Convergence: &ConvergenceReport{Status: ConvergenceStatusDivergent},
	}
	require.Equal(t, "divergent", divergent.HeadlineStatus())

	// converged but reconciliation is fabricated -> "fabricated"
	fab := &SpeculativeBundle{
		Convergence:    &ConvergenceReport{Status: ConvergenceStatusConverged},
		Reconciliation: &ReconciliationReport{Status: ReconciliationStatusFabricated},
	}
	require.Equal(t, "fabricated", fab.HeadlineStatus())

	// nil -> "unknown"
	var nilBundle *SpeculativeBundle
	require.Equal(t, "unknown", nilBundle.HeadlineStatus())
}

func TestSpeculativeBundle_CohortAgentIDs(t *testing.T) {
	bundle := &SpeculativeBundle{Peers: []SpeculativePeer{
		{AgentID: "a"}, {AgentID: "b"}, {AgentID: "c"},
	}}
	require.Equal(t, []string{"a", "b", "c"}, bundle.CohortAgentIDs())

	var nilBundle *SpeculativeBundle
	require.Nil(t, nilBundle.CohortAgentIDs())
}
