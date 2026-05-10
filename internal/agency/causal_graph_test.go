package agency

import (
	"encoding/json"
	"testing"

	"github.com/stretchr/testify/require"
)

func TestCausalGraphFlatChainOrdering(t *testing.T) {
	t.Parallel()
	g := &CausalGraph{
		ProtocolVersion: CausalGraphProtocolVersion,
		Nodes: []CausalNode{
			{ID: "e-low", Role: NodeRoleEvidence, Summary: "low-weight evidence", Weight: 0.2},
			{ID: "c-1", Role: NodeRoleConfounder, Summary: "common cause", Weight: 0.9},
			{ID: "i-do", Role: NodeRoleIntervention, Summary: "do(publish)", Weight: 0.6},
			{ID: "e-hi", Role: NodeRoleEvidence, Summary: "high-weight evidence", Weight: 0.8},
			{ID: "o-1", Role: NodeRoleOutcome, Summary: "verdict=ok", Weight: 0.7},
		},
	}

	chain := g.FlatChain()
	require.Equal(t, []string{
		"outcome: verdict=ok",
		"intervention: do(publish)",
		"evidence: high-weight evidence",
		"evidence: low-weight evidence",
		"confounder: common cause",
	}, chain, "FlatChain must order: outcome, intervention, evidence (desc weight), confounder")
}

func TestCausalGraphFlatChainStableTieBreak(t *testing.T) {
	t.Parallel()
	// Two evidence nodes with identical weights must order by ID for
	// deterministic projection across runs and machines.
	g := &CausalGraph{
		ProtocolVersion: CausalGraphProtocolVersion,
		Nodes: []CausalNode{
			{ID: "z-evidence", Role: NodeRoleEvidence, Summary: "z", Weight: 0.5},
			{ID: "a-evidence", Role: NodeRoleEvidence, Summary: "a", Weight: 0.5},
			{ID: "m-evidence", Role: NodeRoleEvidence, Summary: "m", Weight: 0.5},
		},
	}
	require.Equal(t, []string{
		"evidence: a",
		"evidence: m",
		"evidence: z",
	}, g.FlatChain())
}

func TestHydrateLegacyCausalChainRoundTrip(t *testing.T) {
	t.Parallel()
	chain := []string{
		"action constraint conflict on publish",
		"tests are failing on main",
		"owner approved deploy",
	}
	g := HydrateLegacyCausalChain(chain)
	require.NotNil(t, g)
	require.Equal(t, CausalGraphProtocolVersion, g.ProtocolVersion)
	require.Len(t, g.Nodes, 3)
	for _, n := range g.Nodes {
		require.Equal(t, NodeRoleUnknown, n.Role, "untyped legacy entries become unknown nodes")
	}
	// Round-trip back to the original strings via FlatChain.
	require.Equal(t, chain, g.FlatChain())
}

func TestHydrateLegacyCausalChainParsesRolePrefix(t *testing.T) {
	t.Parallel()
	chain := []string{
		"intervention: do(publish)",
		"evidence: tests passing",
		"confounder: hidden state of CI",
		"outcome: verdict=causal_path_clear",
		"plain text without prefix",
	}
	g := HydrateLegacyCausalChain(chain)
	require.NotNil(t, g)
	require.Len(t, g.Nodes, 5)

	roles := map[string]NodeRole{}
	for _, n := range g.Nodes {
		roles[n.Summary] = n.Role
	}
	require.Equal(t, NodeRoleIntervention, roles["do(publish)"])
	require.Equal(t, NodeRoleEvidence, roles["tests passing"])
	require.Equal(t, NodeRoleConfounder, roles["hidden state of CI"])
	require.Equal(t, NodeRoleOutcome, roles["verdict=causal_path_clear"])
	require.Equal(t, NodeRoleUnknown, roles["plain text without prefix"])
}

func TestHydrateLegacyCausalChainEmpty(t *testing.T) {
	t.Parallel()
	require.Nil(t, HydrateLegacyCausalChain(nil))
	require.Nil(t, HydrateLegacyCausalChain([]string{}))
}

func TestCausalGraphFilterByRole(t *testing.T) {
	t.Parallel()
	g := &CausalGraph{
		ProtocolVersion: CausalGraphProtocolVersion,
		Nodes: []CausalNode{
			{ID: "e1", Role: NodeRoleEvidence, Summary: "e1", Weight: 0.5},
			{ID: "c1", Role: NodeRoleConfounder, Summary: "c1", Weight: 0.5},
			{ID: "i1", Role: NodeRoleIntervention, Summary: "i1", Weight: 0.5},
		},
	}
	filtered := g.FilterByRole(NodeRoleEvidence, NodeRoleIntervention)
	require.NotNil(t, filtered)
	require.Len(t, filtered.Nodes, 2)
	require.Equal(t, NodeRoleEvidence, filtered.Nodes[0].Role)
	require.Equal(t, NodeRoleIntervention, filtered.Nodes[1].Role)

	// Original is untouched.
	require.Len(t, g.Nodes, 3)

	// Empty role set returns nil.
	require.Nil(t, g.FilterByRole())

	// Unmatched role set returns nil.
	require.Nil(t, g.FilterByRole(NodeRoleOutcome))
}

func TestCausalGraphCloneDeepCopy(t *testing.T) {
	t.Parallel()
	g := &CausalGraph{
		ProtocolVersion: CausalGraphProtocolVersion,
		Nodes: []CausalNode{
			{
				ID:       "n1",
				Role:     NodeRoleEvidence,
				Summary:  "n1",
				Parents:  []NodeID{"p1"},
				AtomRefs: []string{"a1", "a2"},
				Meta:     map[string]string{"k": "v"},
				Weight:   0.7,
			},
		},
	}
	dup := g.Clone()
	require.NotNil(t, dup)

	// Mutating the clone must not affect the original.
	dup.Nodes[0].Role = NodeRoleConfounder
	dup.Nodes[0].Parents[0] = "p2"
	dup.Nodes[0].AtomRefs[0] = "x1"
	dup.Nodes[0].Meta["k"] = "v2"

	require.Equal(t, NodeRoleEvidence, g.Nodes[0].Role)
	require.Equal(t, NodeID("p1"), g.Nodes[0].Parents[0])
	require.Equal(t, "a1", g.Nodes[0].AtomRefs[0])
	require.Equal(t, "v", g.Nodes[0].Meta["k"])
}

func TestCausalGraphCountByRole(t *testing.T) {
	t.Parallel()
	g := &CausalGraph{
		Nodes: []CausalNode{
			{ID: "e1", Role: NodeRoleEvidence},
			{ID: "e2", Role: NodeRoleEvidence},
			{ID: "c1", Role: NodeRoleConfounder},
			{ID: "i1", Role: NodeRoleIntervention},
		},
	}
	counts := g.CountByRole()
	require.Equal(t, 2, counts[NodeRoleEvidence])
	require.Equal(t, 1, counts[NodeRoleConfounder])
	require.Equal(t, 1, counts[NodeRoleIntervention])
	require.Equal(t, 0, counts[NodeRoleOutcome])
}

func TestGISTVerdictSyncCausalChainGraphWins(t *testing.T) {
	t.Parallel()
	// Graph is the source of truth: SyncCausalChain overwrites whatever
	// stale string list was sitting in CausalChain.
	v := &GISTVerdict{
		CausalChain: []string{"stale legacy entry"},
		CausalGraph: &CausalGraph{
			ProtocolVersion: CausalGraphProtocolVersion,
			Nodes: []CausalNode{
				{ID: "o", Role: NodeRoleOutcome, Summary: "verdict=ok", Weight: 0.9},
				{ID: "e", Role: NodeRoleEvidence, Summary: "tests pass", Weight: 0.7},
			},
		},
	}
	v.SyncCausalChain()
	require.Equal(t, []string{"outcome: verdict=ok", "evidence: tests pass"}, v.CausalChain)
	require.Len(t, v.CausalGraph.Nodes, 2, "graph is unchanged when it wins")
}

func TestGISTVerdictSyncCausalChainHydratesFromLegacy(t *testing.T) {
	t.Parallel()
	v := &GISTVerdict{
		CausalChain: []string{"observation A", "observation B"},
	}
	v.SyncCausalChain()
	require.NotNil(t, v.CausalGraph)
	require.Equal(t, CausalGraphProtocolVersion, v.CausalGraph.ProtocolVersion)
	require.Len(t, v.CausalGraph.Nodes, 2)
	// Idempotent: a second call must not re-hydrate or duplicate.
	before := v.CausalGraph
	v.SyncCausalChain()
	require.Same(t, before, v.CausalGraph, "second sync is a no-op when graph already exists")
}

func TestGISTVerdictSyncCausalChainNilSafe(t *testing.T) {
	t.Parallel()
	var v *GISTVerdict
	require.NotPanics(t, func() { v.SyncCausalChain() })

	empty := &GISTVerdict{}
	require.NotPanics(t, func() { empty.SyncCausalChain() })
	require.Nil(t, empty.CausalGraph)
	require.Empty(t, empty.CausalChain)
}

func TestCausalGraphJSONShape(t *testing.T) {
	t.Parallel()
	// Confirms wire format matches the Python kernel's emit shape:
	//   { "protocolVersion": 1, "nodes": [ { "id":..., "role":..., ... } ] }
	g := &CausalGraph{
		ProtocolVersion: 1,
		Nodes: []CausalNode{
			{
				ID:       "node_outcome_abc",
				Role:     NodeRoleOutcome,
				Summary:  "verdict=ok",
				Parents:  []NodeID{"node_e1"},
				AtomRefs: []string{},
				Weight:   0.5,
				Meta:     map[string]string{"verdict": "ok"},
			},
		},
	}
	encoded, err := json.Marshal(g)
	require.NoError(t, err)

	var decoded map[string]any
	require.NoError(t, json.Unmarshal(encoded, &decoded))
	require.Equal(t, float64(1), decoded["protocolVersion"])
	nodes := decoded["nodes"].([]any)
	require.Len(t, nodes, 1)
	node := nodes[0].(map[string]any)
	require.Equal(t, "node_outcome_abc", node["id"])
	require.Equal(t, "outcome", node["role"])
	require.Equal(t, "verdict=ok", node["summary"])
}
