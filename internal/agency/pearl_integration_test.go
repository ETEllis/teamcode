package agency

import (
	"context"
	"testing"

	"github.com/stretchr/testify/require"
)

// TestGISTCompressEmitsPearlPlanAndAttribution exercises the full Phase 2
// pipeline end-to-end: Python kernel emits a typed CausalGraph; Go runs
// Pearl loop + Shapley; the verdict surfaces both. This is the canonical
// integration test for items #10/#11/#12.
func TestGISTCompressEmitsPearlPlanAndAttribution(t *testing.T) {
	t.Parallel()

	core := NewGISTAgentCore("agent-1", GISTScriptPathForTest(t), DefaultGISTBudget())
	verdict, _, err := core.Compress(context.Background(), replayFixtureAtoms())
	require.NoError(t, err)

	// CausalGraph from the kernel...
	require.NotNil(t, verdict.CausalGraph, "Phase 1 graph must be present")
	require.Equal(t, CausalGraphProtocolVersion, verdict.CausalGraph.ProtocolVersion)
	require.NotEmpty(t, verdict.CausalGraph.Nodes)

	// ...drives Pearl loop computation in Go.
	require.NotNil(t, verdict.PearlPlan, "Pearl loop must run on success path")
	require.Equal(t, PearlLoopProtocolVersion, verdict.PearlPlan.ProtocolVersion)

	// ...and Shapley attribution.
	require.NotEmpty(t, verdict.Attribution, "Attribution must be non-empty when graph is non-empty")
	for i := 1; i < len(verdict.Attribution); i++ {
		require.GreaterOrEqual(t, verdict.Attribution[i-1].Rank, 0)
		require.Equal(t, i+1, verdict.Attribution[i].Rank,
			"attribution Rank field must be 1-indexed and dense")
	}

	// The replay fixture contains a publish/block contradiction, so the
	// kernel should surface at least one confounder node.
	counts := verdict.CausalGraph.CountByRole()
	require.Greater(t, counts[NodeRoleConfounder], 0,
		"replay fixture has positive/negative tension; confounder must appear")
}

// TestGISTCompressConfounderRiskUpliftEndToEnd verifies that when the
// kernel produces a high-confounder-load graph the legacy RiskLevel and
// OpenQuestions surfaces are uplifted, so existing model-router and
// approval consumers see "high" risk without reading PearlPlan.
func TestGISTCompressConfounderRiskUpliftEndToEnd(t *testing.T) {
	t.Parallel()

	core := NewGISTAgentCore("agent-1", GISTScriptPathForTest(t), DefaultGISTBudget())
	// Heavy contradiction: high-weight publish atom against high-weight
	// blocking constraint. This drives the kernel's confounder set up
	// past the Pearl loop's block threshold.
	atoms := []gistAtom{
		{
			ID: "atom_actor", Kind: "actor_identity",
			Content: "role=release id=agent-1", Weight: 1.0,
			Meta: map[string]string{"scale": "agent", "organizationId": "org-1"},
		},
		{
			ID: "atom_publish_a", Kind: "directive",
			Content: "publish and push the release now", Weight: 2.0,
			Meta: map[string]string{"scale": "event", "organizationId": "org-1"},
		},
		{
			ID: "atom_publish_b", Kind: "directive",
			Content: "deploy to production immediately", Weight: 2.0,
			Meta: map[string]string{"scale": "event", "organizationId": "org-1"},
		},
		{
			ID: "atom_block_a", Kind: "constraint",
			Content: "do not publish until tests pass", Weight: 2.0,
			Meta: map[string]string{"scale": "office", "organizationId": "org-1"},
		},
		{
			ID: "atom_block_b", Kind: "constraint",
			Content: "blocked: regression on main", Weight: 2.0,
			Meta: map[string]string{"scale": "office", "organizationId": "org-1"},
		},
	}

	verdict, _, err := core.Compress(context.Background(), atoms)
	require.NoError(t, err)
	require.NotNil(t, verdict.CausalGraph)
	require.NotNil(t, verdict.PearlPlan)

	require.True(t, verdict.PearlPlan.Prediction.BlockedByConfounder,
		"heavy contradiction fixture must push confounder load >= 0.6 (got load=%.3f)",
		verdict.PearlPlan.Hypothesis.ConfounderLoad)
	require.Equal(t, "high", verdict.RiskLevel,
		"confounder block must promote RiskLevel to high")
	require.Contains(t, verdict.OpenQuestions,
		"Resolve confounders flagged by the Pearl loop before consequential action.",
		"confounder block must surface a follow-up question")
}

// TestGISTCompressDegradedPathPreservesLegacyShape pins the Phase 1
// guarantee: when the subprocess is unavailable, the verdict still has
// CausalChain == ["subprocess unavailable"] and no Pearl plan or
// attribution leaks in to break legacy callers.
func TestGISTCompressDegradedPathPreservesLegacyShape(t *testing.T) {
	t.Parallel()

	core := NewGISTAgentCore("agent-1", "/nonexistent/path/missing.py", DefaultGISTBudget())
	verdict, _, err := core.Compress(context.Background(), replayFixtureAtoms())
	require.NoError(t, err)

	require.Equal(t, []string{"subprocess unavailable"}, verdict.CausalChain,
		"degraded path must not be touched by Phase 2")
	require.Nil(t, verdict.CausalGraph,
		"no graph means no Pearl loop input")
	require.Nil(t, verdict.PearlPlan,
		"no Pearl plan on degraded path")
	require.Empty(t, verdict.Attribution,
		"no attribution on degraded path")
}
