package agency

import (
	"context"
	"encoding/json"
	"os"
	"path/filepath"
	"testing"

	"github.com/stretchr/testify/require"
)

func TestGISTCompressReturnsIdenticalVerdictAndLatticeForSameReplayFixture(t *testing.T) {
	t.Parallel()

	core := NewGISTAgentCore("agent-1", GISTScriptPathForTest(t), DefaultGISTBudget())
	atoms := replayFixtureAtoms()

	verdict1, lattice1, err := core.Compress(context.Background(), atoms)
	require.NoError(t, err)
	verdict2, lattice2, err := core.Compress(context.Background(), atoms)
	require.NoError(t, err)

	require.Equal(t, verdict1, verdict2)
	require.JSONEq(t, lattice1, lattice2)
	require.NotNil(t, verdict1.Trace)
	require.Equal(t, verdict1.Trace.ID, verdict2.Trace.ID)
}

func TestGISTCompressEmitsCanonical64LatticeWithSparseActivation(t *testing.T) {
	t.Parallel()

	core := NewGISTAgentCore("agent-1", GISTScriptPathForTest(t), DefaultGISTBudget())
	verdict, latticeJSON, err := core.Compress(context.Background(), replayFixtureAtoms())
	require.NoError(t, err)
	require.NotNil(t, verdict.Lattice)
	require.Len(t, verdict.Lattice.Slots, 64)
	require.NotEmpty(t, verdict.Lattice.ActiveSlots)
	require.Less(t, len(verdict.Lattice.ActiveSlots), 64)

	var lattice GISTLattice
	require.NoError(t, json.Unmarshal([]byte(latticeJSON), &lattice))
	require.Len(t, lattice.Slots, 64)
	require.Equal(t, verdict.Lattice.ActiveSlots, lattice.ActiveSlots)
	for _, slot := range lattice.Slots {
		if len(slot.AtomRefs) == 0 {
			require.Zero(t, slot.Weight)
		}
	}
}

func TestGISTCompressPreservesContradictionsInsteadOfCollapsingThem(t *testing.T) {
	t.Parallel()

	core := NewGISTAgentCore("agent-1", GISTScriptPathForTest(t), DefaultGISTBudget())
	verdict, _, err := core.Compress(context.Background(), replayFixtureAtoms())
	require.NoError(t, err)

	require.Equal(t, "causal_review_required", verdict.Verdict)
	require.Equal(t, "high", verdict.RiskLevel)
	require.NotEmpty(t, verdict.Contradictions)
	require.Contains(t, verdict.Contradictions[0].Atoms, "atom_publish")
	require.Contains(t, verdict.Contradictions[0].Atoms, "atom_block")
	require.NotNil(t, verdict.Trace)
	require.Contains(t, verdict.Trace.SelectedChain, "atom_publish")
}

func TestGISTCompressEmitsCounterfactualBranchesForCompetingActions(t *testing.T) {
	t.Parallel()

	core := NewGISTAgentCore("agent-1", GISTScriptPathForTest(t), DefaultGISTBudget())
	verdict, _, err := core.Compress(context.Background(), replayFixtureAtoms())
	require.NoError(t, err)

	require.Len(t, verdict.Counterfactuals, 3)
	branches := make([]string, 0, len(verdict.Counterfactuals))
	for _, branch := range verdict.Counterfactuals {
		branches = append(branches, branch.If)
		require.NotEmpty(t, branch.Then)
		require.NotEmpty(t, branch.Risk)
		require.NotEmpty(t, branch.Tests)
	}
	require.Contains(t, branches, "do(publish)")
	require.Contains(t, branches, "do(not_publish)")
	require.Contains(t, branches, "do(request_review)")
}

func TestGISTCompressMarksFallbackAsDegradedWhenSubprocessUnavailable(t *testing.T) {
	t.Parallel()

	core := NewGISTAgentCore("agent-1", filepath.Join(t.TempDir(), "missing.py"), DefaultGISTBudget())
	verdict, latticeJSON, err := core.Compress(context.Background(), replayFixtureAtoms())
	require.NoError(t, err)

	require.Equal(t, "degraded_gist_unavailable", verdict.Verdict)
	require.Equal(t, 0.1, verdict.Confidence)
	require.Equal(t, []string{"subprocess unavailable"}, verdict.CausalChain)
	require.Equal(t, "default_low_confidence", verdict.ExecutionIntent)
	require.True(t, verdict.Degraded)
	require.Equal(t, "unknown", verdict.RiskLevel)
	require.JSONEq(t, "{}", latticeJSON)
}

func TestGISTScriptPathResolvesToRepoScriptsEntryPoint(t *testing.T) {
	t.Setenv("AGENCY_GIST_SCRIPT_PATH", "")
	wd, err := os.Getwd()
	require.NoError(t, err)
	repoRoot := filepath.Dir(filepath.Dir(wd))

	path := GISTScriptPath(t.TempDir())
	require.Equal(t, filepath.Join(repoRoot, "scripts", "gist_subprocess.py"), path)
	require.True(t, NewGISTAgentCore("agent-1", path, DefaultGISTBudget()).subprocessAvailable())
}

func replayFixtureAtoms() []gistAtom {
	return []gistAtom{
		{
			ID:      "atom_actor",
			Kind:    "actor_identity",
			Content: "role=release id=agent-1",
			Weight:  1.0,
			Meta:    map[string]string{"scale": "agent", "organizationId": "org-1"},
		},
		{
			ID:      "atom_publish",
			Kind:    "directive",
			Content: "publish and push the release now",
			Weight:  1.5,
			Meta:    map[string]string{"scale": "event", "organizationId": "org-1"},
		},
		{
			ID:      "atom_block",
			Kind:    "constraint",
			Content: "do not publish until tests pass",
			Weight:  1.2,
			Meta:    map[string]string{"scale": "office", "organizationId": "org-1"},
		},
	}
}

func GISTScriptPathForTest(t *testing.T) string {
	t.Helper()
	wd, err := os.Getwd()
	require.NoError(t, err)
	return filepath.Join(filepath.Dir(filepath.Dir(wd)), "scripts", "gist_subprocess.py")
}
