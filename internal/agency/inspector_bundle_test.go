package agency

import (
	"encoding/json"
	"strings"
	"testing"

	"github.com/stretchr/testify/require"
)

func sampleVerdictForBundle() GISTVerdict {
	graph := &CausalGraph{
		ProtocolVersion: CausalGraphProtocolVersion,
		Nodes: []CausalNode{
			{ID: "node_outcome", Role: NodeRoleOutcome, Summary: "verdict=approve",
				Parents: []NodeID{"node_evidence", "node_intervention"}},
			{ID: "node_evidence", Role: NodeRoleEvidence, Summary: "user logged in", Weight: 0.9},
			{ID: "node_intervention", Role: NodeRoleIntervention, Summary: "do(publish)", Weight: 0.5},
			{ID: "node_confounder", Role: NodeRoleConfounder, Summary: "stale cache", Weight: 0.3},
		},
	}
	return GISTVerdict{
		Verdict:         "approve",
		Confidence:      0.78,
		RiskLevel:       "low",
		ExecutionIntent: "publish_post",
		OpenQuestions:   []string{"is cache stale?"},
		Contradictions:  make([]GISTContradiction, 2),
		Interventions:   make([]GISTIntervention, 1),
		CausalGraph:     graph,
		PearlPlan: &PearlPlan{
			ProtocolVersion: 1,
			Hypothesis: Hypothesis{
				Evidence:       []NodeID{"node_evidence"},
				Confounders:    []NodeID{"node_confounder"},
				EvidenceWeight: 0.9,
				ConfounderLoad: 0.3,
			},
			Actions: []ActionCandidate{
				{
					NodeID:      "node_intervention",
					Label:       "do(publish)",
					Score:       0.65,
					Risk:        0.05,
					Recommended: true,
				},
			},
			Prediction: Prediction{
				Recommended:         "node_intervention",
				ProjectedConfidence: 0.7,
				Residual:            []string{"is cache stale?"},
			},
		},
		Attribution: []NodeAttribution{
			{NodeID: "node_evidence", Role: NodeRoleEvidence, Phi: 0.42, Rank: 1},
			{NodeID: "node_intervention", Role: NodeRoleIntervention, Phi: 0.18, Rank: 2},
			{NodeID: "node_confounder", Role: NodeRoleConfounder, Phi: -0.31, Rank: 3},
		},
	}
}

func TestBuildInspectorBundleSyncsChainFromGraph(t *testing.T) {
	v := sampleVerdictForBundle()
	v.CausalChain = []string{"stale legacy chain"}
	bundle := BuildInspectorBundle(v)

	require.Equal(t, GISTInspectorBundleProtocolVersion, bundle.ProtocolVersion)
	require.Equal(t, "approve", bundle.Verdict)
	require.NotNil(t, bundle.CausalGraph)
	require.NotEmpty(t, bundle.FlatChain,
		"FlatChain should be derived from the typed graph, not the stale legacy chain")
	for _, line := range bundle.FlatChain {
		require.NotEqual(t, "stale legacy chain", line,
			"sync should overwrite stale legacy chain with graph-derived ordering")
	}
}

func TestBuildInspectorBundleClonesAttribution(t *testing.T) {
	v := sampleVerdictForBundle()
	bundle := BuildInspectorBundle(v)
	require.Len(t, bundle.Attribution, 3)
	bundle.Attribution[0].Phi = -999
	require.NotEqual(t, -999.0, v.Attribution[0].Phi,
		"mutating bundle should not leak back to source verdict")
}

func TestMarshalParseInspectorBundleRoundTrip(t *testing.T) {
	v := sampleVerdictForBundle()
	raw, err := MarshalInspectorBundle(v)
	require.NoError(t, err)
	require.NotEmpty(t, raw)
	require.NotEqual(t, "{}", raw)

	parsed, err := ParseInspectorBundle(raw)
	require.NoError(t, err)
	require.NotNil(t, parsed)
	require.Equal(t, GISTInspectorBundleProtocolVersion, parsed.ProtocolVersion)
	require.Equal(t, "approve", parsed.Verdict)
	require.NotNil(t, parsed.CausalGraph)
	require.Len(t, parsed.CausalGraph.Nodes, 4)
	require.NotNil(t, parsed.PearlPlan)
	require.NotEmpty(t, parsed.PearlPlan.Actions)
	require.Equal(t, "do(publish)", parsed.PearlPlan.Actions[0].Label)
	require.True(t, parsed.PearlPlan.Actions[0].Recommended)
	require.Len(t, parsed.Attribution, 3)
	require.Equal(t, "node_evidence", string(parsed.Attribution[0].NodeID))
}

func TestParseInspectorBundleEmptyShapesReturnNil(t *testing.T) {
	cases := []string{"", "{}", "  ", "\n", "null"}
	for _, in := range cases {
		got, err := ParseInspectorBundle(in)
		require.NoError(t, err, "input=%q", in)
		require.Nil(t, got, "input=%q", in)
	}
}

func TestParseInspectorBundleReturnsErrorOnGarbage(t *testing.T) {
	got, err := ParseInspectorBundle(`{"protocolVersion":`)
	require.Error(t, err)
	require.Nil(t, got)
}

func TestHydrateInspectorBundleFromLegacyRecoversChain(t *testing.T) {
	trace := GISTTrace{
		ID:               "legacy-trace",
		SelectedVerdict:  "approve",
		SelectedChain:    []string{"evidence: user logged in", "intervention: do(publish)"},
		ContradictionIDs: []string{"c1", "c2"},
		InterventionIDs:  []string{"i1"},
	}
	raw, err := json.Marshal(trace)
	require.NoError(t, err)

	bundle := HydrateInspectorBundleFromLegacy(string(raw))
	require.NotNil(t, bundle)
	require.Equal(t, "approve", bundle.Verdict)
	require.NotNil(t, bundle.CausalGraph)
	require.Len(t, bundle.FlatChain, 2)
	require.Equal(t, 2, bundle.ContradictionCount)
	require.Equal(t, 1, bundle.InterventionCount)
}

func TestHydrateInspectorBundleFromLegacyEmptyOrGarbage(t *testing.T) {
	require.Nil(t, HydrateInspectorBundleFromLegacy(""))
	require.Nil(t, HydrateInspectorBundleFromLegacy("{}"))
	require.Nil(t, HydrateInspectorBundleFromLegacy("not json"))
}

func TestMarshalInspectorBundleStableJSONShape(t *testing.T) {
	v := sampleVerdictForBundle()
	raw, err := MarshalInspectorBundle(v)
	require.NoError(t, err)
	// Headline keys we promise to consumers (templates, downstream tools).
	for _, k := range []string{
		`"protocolVersion":1`,
		`"verdict":"approve"`,
		`"causalGraph"`,
		`"pearlPlan"`,
		`"attribution"`,
	} {
		require.True(t, strings.Contains(raw, k),
			"expected key %q in marshaled bundle, got: %s", k, raw)
	}
}

func TestBuildInspectorBundleNilGraphAndPlan(t *testing.T) {
	// Verdict came from the degraded path: only CausalChain populated,
	// no graph, no plan, no attribution. Bundle should still be valid
	// and FlatChain should round-trip.
	v := GISTVerdict{
		Verdict:     "subprocess_unavailable",
		Degraded:    true,
		CausalChain: []string{"subprocess unavailable"},
	}
	bundle := BuildInspectorBundle(v)
	require.NotNil(t, bundle)
	require.True(t, bundle.Degraded)
	require.Equal(t, []string{"subprocess unavailable"}, bundle.FlatChain)
	require.NotNil(t, bundle.CausalGraph,
		"SyncCausalChain should hydrate a graph from the legacy chain so the inspector still has a structured view")
	require.Nil(t, bundle.PearlPlan)
	require.Empty(t, bundle.Attribution)
}
