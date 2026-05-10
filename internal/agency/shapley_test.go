package agency

import (
	"math"
	"testing"

	"github.com/stretchr/testify/require"
)

func TestRoleWeightDirection(t *testing.T) {
	t.Parallel()
	require.Equal(t, 1.0, roleWeight(NodeRoleEvidence))
	require.Equal(t, 0.5, roleWeight(NodeRoleIntervention))
	require.Equal(t, -2.0, roleWeight(NodeRoleConfounder),
		"confounders pull twice as hard as evidence pushes")
	require.Equal(t, 0.5, roleWeight(NodeRoleUnknown))
	require.Equal(t, 0.0, roleWeight(NodeRoleOutcome))
}

func TestAttributeNecessityNilAndEmpty(t *testing.T) {
	t.Parallel()
	require.Nil(t, AttributeNecessity(nil))
	require.Nil(t, AttributeNecessity(&CausalGraph{ProtocolVersion: CausalGraphProtocolVersion}))
}

func TestAttributeNecessityExcludesOutcome(t *testing.T) {
	t.Parallel()
	g := pearlGraphFixture()
	attrs := AttributeNecessity(g)
	require.NotEmpty(t, attrs)
	for _, a := range attrs {
		require.NotEqual(t, NodeRoleOutcome, a.Role,
			"outcome must never appear in the player set")
	}
	// 7-node fixture minus 1 outcome = 6 attributions.
	require.Len(t, attrs, 6)
}

func TestAttributeNecessityRanksByAbsPhi(t *testing.T) {
	t.Parallel()
	attrs := AttributeNecessity(pearlGraphFixture())
	require.NotEmpty(t, attrs)
	for i := 1; i < len(attrs); i++ {
		require.GreaterOrEqual(t, math.Abs(attrs[i-1].Phi), math.Abs(attrs[i].Phi),
			"attributions must be sorted by descending |Phi|")
	}
	for i, a := range attrs {
		require.Equal(t, i+1, a.Rank, "Rank is 1-indexed and matches sort order")
	}
}

func TestAttributeNecessityConfounderHasNegativePhi(t *testing.T) {
	t.Parallel()
	attrs := AttributeNecessity(pearlGraphFixture())
	confounderFound := false
	evidenceFound := false
	for _, a := range attrs {
		if a.Role == NodeRoleConfounder {
			require.Less(t, a.Phi, 0.0,
				"confounder Phi must be negative (it pulls confidence down)")
			confounderFound = true
		}
		if a.Role == NodeRoleEvidence {
			require.Greater(t, a.Phi, 0.0,
				"evidence Phi must be positive")
			evidenceFound = true
		}
	}
	require.True(t, confounderFound)
	require.True(t, evidenceFound)
}

func TestAttributeNecessityExactForSmallGraphs(t *testing.T) {
	t.Parallel()
	attrs := AttributeNecessity(pearlGraphFixture())
	for _, a := range attrs {
		require.False(t, a.Approximate,
			"6-player graph should use exact Shapley, not sampling")
	}
}

func TestAttributeNecessitySamplingIsReproducible(t *testing.T) {
	t.Parallel()
	// Build a graph just over the exact threshold so we exercise the
	// sampling path. 15 evidence nodes is well above exactShapleyMaxPlayers
	// (14) and well under shapleyMaxPlayers (64).
	graph := &CausalGraph{ProtocolVersion: CausalGraphProtocolVersion}
	for i := 0; i < 15; i++ {
		graph.Nodes = append(graph.Nodes, CausalNode{
			ID:      NodeID("n_" + string(rune('a'+i))),
			Role:    NodeRoleEvidence,
			Summary: "evidence " + string(rune('a'+i)),
			Weight:  0.5,
		})
	}
	a := AttributeNecessity(graph)
	b := AttributeNecessity(graph)
	require.Equal(t, len(a), len(b))
	for i := range a {
		require.Equal(t, a[i].NodeID, b[i].NodeID)
		require.True(t, a[i].Approximate)
		require.InDelta(t, a[i].Phi, b[i].Phi, 1e-12,
			"sampled Shapley must be deterministic given same graph")
	}
}

func TestExactShapleySymmetryEqualPhi(t *testing.T) {
	t.Parallel()
	// Two evidence nodes with identical role+weight have identical Phi
	// by Shapley's symmetry axiom.
	graph := &CausalGraph{
		ProtocolVersion: CausalGraphProtocolVersion,
		Nodes: []CausalNode{
			{ID: "e1", Role: NodeRoleEvidence, Weight: 0.7},
			{ID: "e2", Role: NodeRoleEvidence, Weight: 0.7},
			{ID: "c1", Role: NodeRoleConfounder, Weight: 0.3},
		},
	}
	attrs := AttributeNecessity(graph)
	var p1, p2 float64
	for _, a := range attrs {
		if a.NodeID == "e1" {
			p1 = a.Phi
		}
		if a.NodeID == "e2" {
			p2 = a.Phi
		}
	}
	require.InDelta(t, p1, p2, 1e-12, "symmetry: identical players get identical Phi")
}

func TestExactShapleyEfficiencyAxiom(t *testing.T) {
	t.Parallel()
	// Sum of all Phi values must equal v(N) - v({}). This is Shapley's
	// efficiency axiom and pins the implementation against a baseline
	// math identity.
	graph := &CausalGraph{
		ProtocolVersion: CausalGraphProtocolVersion,
		Nodes: []CausalNode{
			{ID: "out", Role: NodeRoleOutcome, Weight: 0.5},
			{ID: "e1", Role: NodeRoleEvidence, Weight: 0.5},
			{ID: "e2", Role: NodeRoleEvidence, Weight: 0.4},
			{ID: "i1", Role: NodeRoleIntervention, Weight: 0.3},
			{ID: "c1", Role: NodeRoleConfounder, Weight: 0.2},
		},
	}
	attrs := AttributeNecessity(graph)
	sumPhi := 0.0
	for _, a := range attrs {
		sumPhi += a.Phi
	}
	// v({}) = sigmoid(0) = 0.5
	// v(N) = sigmoid(1*0.5 + 1*0.4 + 0.5*0.3 + (-2)*0.2)
	//      = sigmoid(0.65)
	expected := sigmoid(0.65) - 0.5
	require.InDelta(t, expected, sumPhi, 1e-9,
		"efficiency: sum of Phi == v(N) - v(empty)")
}

func TestApplyConfounderRiskUpliftPromotesRisk(t *testing.T) {
	t.Parallel()
	v := &GISTVerdict{
		RiskLevel: "low",
		PearlPlan: &PearlPlan{
			Prediction: Prediction{BlockedByConfounder: true},
		},
	}
	applyConfounderRiskUplift(v)
	require.Equal(t, "high", v.RiskLevel)
	require.Contains(t, v.OpenQuestions,
		"Resolve confounders flagged by the Pearl loop before consequential action.")
}

func TestApplyConfounderRiskUpliftIdempotent(t *testing.T) {
	t.Parallel()
	v := &GISTVerdict{
		RiskLevel: "high",
		PearlPlan: &PearlPlan{
			Prediction: Prediction{BlockedByConfounder: true},
		},
	}
	applyConfounderRiskUplift(v)
	applyConfounderRiskUplift(v)
	applyConfounderRiskUplift(v)
	require.Len(t, v.OpenQuestions, 1, "question must not duplicate on repeated calls")
}

func TestApplyConfounderRiskUpliftNoop(t *testing.T) {
	t.Parallel()
	v := &GISTVerdict{RiskLevel: "low"}
	applyConfounderRiskUplift(v) // no PearlPlan
	require.Equal(t, "low", v.RiskLevel)
	require.Empty(t, v.OpenQuestions)

	v2 := &GISTVerdict{
		RiskLevel: "low",
		PearlPlan: &PearlPlan{Prediction: Prediction{BlockedByConfounder: false}},
	}
	applyConfounderRiskUplift(v2)
	require.Equal(t, "low", v2.RiskLevel,
		"no uplift when prediction is not blocked")
	require.Empty(t, v2.OpenQuestions)
}
