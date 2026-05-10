package agency

import (
	"math"
	"testing"

	"github.com/stretchr/testify/require"
)

// pearlGraphFixture builds a representative graph: 1 outcome,
// 2 interventions (one risky, one safe), 2 evidence atoms,
// 1 confounder, 1 unknown.
func pearlGraphFixture() *CausalGraph {
	return &CausalGraph{
		ProtocolVersion: CausalGraphProtocolVersion,
		Nodes: []CausalNode{
			{ID: "out", Role: NodeRoleOutcome, Summary: "verdict=ok", Weight: 0.5},
			{ID: "do_publish", Role: NodeRoleIntervention, Summary: "do(publish)", Weight: 0.6},
			{ID: "do_wait", Role: NodeRoleIntervention, Summary: "do(wait)", Weight: 0.3},
			{ID: "ev_tests", Role: NodeRoleEvidence, Summary: "tests pass", Weight: 0.8},
			{ID: "ev_owner", Role: NodeRoleEvidence, Summary: "owner approved", Weight: 0.6},
			{ID: "cf_hidden", Role: NodeRoleConfounder, Summary: "hidden state of CI", Weight: 0.2},
			{ID: "uk_unrelated", Role: NodeRoleUnknown, Summary: "unrelated note", Weight: 0.4},
		},
	}
}

func TestAbduceSplitsRolesAndCountsWeight(t *testing.T) {
	t.Parallel()
	g := pearlGraphFixture()
	h := Abduce(g)
	require.Equal(t, []NodeID{"ev_owner", "ev_tests", "uk_unrelated"}, h.Evidence,
		"unknowns join evidence; ordering is stable by ID")
	require.Equal(t, []NodeID{"cf_hidden"}, h.Confounders)
	// 0.8 + 0.6 + 0.5 * 0.4 = 1.6
	require.InDelta(t, 1.6, h.EvidenceWeight, 1e-9)
	require.InDelta(t, 0.2, h.ConfounderLoad, 1e-9)
}

func TestAbduceNilSafe(t *testing.T) {
	t.Parallel()
	h := Abduce(nil)
	require.Empty(t, h.Evidence)
	require.Empty(t, h.Confounders)
	require.Equal(t, 0.0, h.EvidenceWeight)
	require.Equal(t, 0.0, h.ConfounderLoad)
}

func TestEnumerateActionsRecommendsTopWhenSafe(t *testing.T) {
	t.Parallel()
	g := pearlGraphFixture()
	h := Abduce(g)
	candidates := EnumerateActions(g, h)
	require.Len(t, candidates, 2)
	// Highest-scoring action sorts first.
	require.Equal(t, NodeID("do_publish"), candidates[0].NodeID)
	require.True(t, candidates[0].Recommended, "top action recommended at low confounder load")
	require.False(t, candidates[1].Recommended)
	// Scores live in (0, 1).
	for _, c := range candidates {
		require.Greater(t, c.Score, 0.0)
		require.Less(t, c.Score, 1.0)
	}
}

func TestEnumerateActionsBlockedAtHighConfounderLoad(t *testing.T) {
	t.Parallel()
	g := pearlGraphFixture()
	// Bump confounder load above the block threshold (0.6).
	for i := range g.Nodes {
		if g.Nodes[i].Role == NodeRoleConfounder {
			g.Nodes[i].Weight = 0.9
		}
	}
	h := Abduce(g)
	candidates := EnumerateActions(g, h)
	require.NotEmpty(t, candidates)
	for _, c := range candidates {
		require.False(t, c.Recommended,
			"no action should be recommended when confounder load >= block threshold")
	}
}

func TestPredictBlockedByConfounder(t *testing.T) {
	t.Parallel()
	h := Hypothesis{EvidenceWeight: 1.0, ConfounderLoad: 0.7}
	pred := Predict(nil, h, nil)
	require.True(t, pred.BlockedByConfounder)
	require.NotEmpty(t, pred.Residual)
}

func TestPredictSurfacesUnknownsAsResidual(t *testing.T) {
	t.Parallel()
	g := pearlGraphFixture()
	h := Abduce(g)
	candidates := EnumerateActions(g, h)
	pred := Predict(g, h, candidates)
	require.False(t, pred.BlockedByConfounder)
	require.Equal(t, NodeID("do_publish"), pred.Recommended)
	// Confidence is in (0, 1) and reflects the recommended action's score.
	require.Greater(t, pred.ProjectedConfidence, 0.0)
	require.Less(t, pred.ProjectedConfidence, 1.0)
	// The unknown node is surfaced for follow-up.
	found := false
	for _, r := range pred.Residual {
		if r == "Unclassified node: unrelated note" {
			found = true
			break
		}
	}
	require.True(t, found, "unknowns must show up in residual: %v", pred.Residual)
}

func TestRunPearlLoopNilAndEmpty(t *testing.T) {
	t.Parallel()
	require.Nil(t, RunPearlLoop(nil))
	require.Nil(t, RunPearlLoop(&CausalGraph{ProtocolVersion: CausalGraphProtocolVersion}))
}

func TestRunPearlLoopEndToEnd(t *testing.T) {
	t.Parallel()
	plan := RunPearlLoop(pearlGraphFixture())
	require.NotNil(t, plan)
	require.Equal(t, PearlLoopProtocolVersion, plan.ProtocolVersion)
	require.NotEmpty(t, plan.Hypothesis.Evidence)
	require.NotEmpty(t, plan.Actions)
	require.Equal(t, NodeID("do_publish"), plan.Prediction.Recommended)
}

func TestSigmoidBounds(t *testing.T) {
	t.Parallel()
	require.InDelta(t, 0.5, sigmoid(0), 1e-12)
	require.True(t, sigmoid(10) > 0.999)
	require.True(t, sigmoid(-10) < 0.001)
	require.Equal(t, 1.0, sigmoid(math.Inf(1)))
	require.Equal(t, 0.0, sigmoid(math.Inf(-1)))
}
