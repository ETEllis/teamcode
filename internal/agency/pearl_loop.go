package agency

import (
	"math"
	"sort"
	"strconv"
)

// PearlLoopProtocolVersion is the wire version for PearlPlan payloads
// embedded in a GISTVerdict. Bump on shape-breaking changes.
const PearlLoopProtocolVersion = 1

// Hypothesis ranks the evidence and confounder nodes that make the verdict
// plausible. Evidence supports the verdict; confounders are flagged so the
// agent can decide whether to seek tie-breaking observations before acting.
type Hypothesis struct {
	Evidence       []NodeID `json:"evidence,omitempty"`
	Confounders    []NodeID `json:"confounders,omitempty"`
	EvidenceWeight float64  `json:"evidenceWeight"`
	ConfounderLoad float64  `json:"confounderLoad"`
}

// ActionCandidate scores a single do(x) intervention against the abduced
// hypothesis. Higher Score = better expected outcome under the current
// graph; positive Score means evidence outweighs confounder + risk.
type ActionCandidate struct {
	NodeID      NodeID  `json:"nodeId"`
	Label       string  `json:"label"`
	Score       float64 `json:"score"`
	Risk        float64 `json:"risk"`
	Recommended bool    `json:"recommended"`
}

// Prediction is the projected verdict-drift if Recommended is taken.
// ProjectedConfidence is the expected post-action confidence; Residual lists
// open questions that the action does NOT resolve.
type Prediction struct {
	Recommended         NodeID   `json:"recommended,omitempty"`
	ProjectedConfidence float64  `json:"projectedConfidence"`
	Residual            []string `json:"residual,omitempty"`
	BlockedByConfounder bool     `json:"blockedByConfounder,omitempty"`
}

// PearlPlan packages an abduction -> action -> prediction trace produced by
// RunPearlLoop. It rides on GISTVerdict so downstream actors (model router,
// approval flow, lattice inspector) can read structured causal reasoning
// instead of re-deriving it from the flat causal chain.
type PearlPlan struct {
	ProtocolVersion int               `json:"protocolVersion"`
	Hypothesis      Hypothesis        `json:"hypothesis"`
	Actions         []ActionCandidate `json:"actions,omitempty"`
	Prediction      Prediction        `json:"prediction"`
}

// confounderBlockThreshold is the confounder-load level above which we
// refuse to recommend any action without first resolving the confounders.
// Tuned conservatively: even one weighty confounder should give pause.
const confounderBlockThreshold = 0.6

// interventionRiskFloor is the minimum risk we attribute to any
// intervention so a 0-weight do(x) candidate doesn't get a free pass.
const interventionRiskFloor = 0.05

// Abduce projects a CausalGraph onto its evidence + confounder atoms,
// returning a Hypothesis that ranks them by contribution. The graph is
// not mutated.
func Abduce(graph *CausalGraph) Hypothesis {
	h := Hypothesis{}
	if graph == nil {
		return h
	}
	for _, n := range graph.Nodes {
		switch n.Role {
		case NodeRoleEvidence:
			h.Evidence = append(h.Evidence, n.ID)
			h.EvidenceWeight += n.Weight
		case NodeRoleConfounder:
			h.Confounders = append(h.Confounders, n.ID)
			h.ConfounderLoad += n.Weight
		case NodeRoleUnknown:
			// Unknowns lean supportive but at half weight, since we
			// don't yet know whether they are evidence or confounders.
			h.Evidence = append(h.Evidence, n.ID)
			h.EvidenceWeight += 0.5 * n.Weight
		}
	}
	// Stable ordering for deterministic downstream consumption.
	sort.Slice(h.Evidence, func(i, j int) bool { return h.Evidence[i] < h.Evidence[j] })
	sort.Slice(h.Confounders, func(i, j int) bool { return h.Confounders[i] < h.Confounders[j] })
	return h
}

// EnumerateActions scores every intervention node in the graph against the
// abduced hypothesis. Score is sigmoid(evidence - 2*confounder + 0.5*self),
// which is the same value function used by AttributeNecessity so action
// scores and attribution agree.
func EnumerateActions(graph *CausalGraph, h Hypothesis) []ActionCandidate {
	if graph == nil {
		return nil
	}
	candidates := make([]ActionCandidate, 0)
	for _, n := range graph.Nodes {
		if n.Role != NodeRoleIntervention {
			continue
		}
		risk := interventionRiskFloor + h.ConfounderLoad*0.5
		if metaRisk, ok := n.Meta["risk"]; ok {
			if r := parseFloatMeta(metaRisk); r > risk {
				risk = r
			}
		}
		score := sigmoid(h.EvidenceWeight - 2*h.ConfounderLoad + 0.5*n.Weight - risk)
		candidates = append(candidates, ActionCandidate{
			NodeID: n.ID,
			Label:  labelOrID(n),
			Score:  score,
			Risk:   risk,
		})
	}
	sort.SliceStable(candidates, func(i, j int) bool {
		if candidates[i].Score != candidates[j].Score {
			return candidates[i].Score > candidates[j].Score
		}
		return candidates[i].NodeID < candidates[j].NodeID
	})
	if len(candidates) > 0 && h.ConfounderLoad < confounderBlockThreshold {
		candidates[0].Recommended = true
	}
	return candidates
}

// Predict produces a one-step projection of the verdict given the abduced
// hypothesis and the scored action candidates. If confounder load exceeds
// the block threshold no action is recommended and BlockedByConfounder is
// set so the approval surface can flag the situation.
func Predict(graph *CausalGraph, h Hypothesis, candidates []ActionCandidate) Prediction {
	pred := Prediction{
		ProjectedConfidence: sigmoid(h.EvidenceWeight - 2*h.ConfounderLoad),
	}
	if h.ConfounderLoad >= confounderBlockThreshold {
		pred.BlockedByConfounder = true
		pred.Residual = append(pred.Residual,
			"Resolve confounders before consequential action.")
		return pred
	}
	for _, c := range candidates {
		if c.Recommended {
			pred.Recommended = c.NodeID
			pred.ProjectedConfidence = c.Score
			break
		}
	}
	if graph != nil {
		// Unknowns are a residual reasoning gap — surface them so the
		// agent can either upgrade them to typed roles or seek more
		// evidence.
		for _, n := range graph.Nodes {
			if n.Role == NodeRoleUnknown {
				pred.Residual = append(pred.Residual,
					"Unclassified node: "+labelOrID(n))
			}
		}
	}
	return pred
}

// RunPearlLoop orchestrates abduction -> action enumeration -> prediction
// over a typed CausalGraph and returns a structured PearlPlan. Returns nil
// for a nil or empty graph so callers can lift the result onto a verdict
// without a nil-check tax.
func RunPearlLoop(graph *CausalGraph) *PearlPlan {
	if graph == nil || len(graph.Nodes) == 0 {
		return nil
	}
	h := Abduce(graph)
	actions := EnumerateActions(graph, h)
	pred := Predict(graph, h, actions)
	return &PearlPlan{
		ProtocolVersion: PearlLoopProtocolVersion,
		Hypothesis:      h,
		Actions:         actions,
		Prediction:      pred,
	}
}

// sigmoid maps R -> (0, 1). Used as the canonical squashing function so
// confidence numbers always live in the same range as GISTVerdict.Confidence.
func sigmoid(x float64) float64 {
	if math.IsInf(x, 1) {
		return 1
	}
	if math.IsInf(x, -1) {
		return 0
	}
	return 1.0 / (1.0 + math.Exp(-x))
}

func labelOrID(n CausalNode) string {
	if n.Summary != "" {
		return n.Summary
	}
	return string(n.ID)
}

func parseFloatMeta(s string) float64 {
	// Tolerant: returns 0 on parse failure (which falls back to
	// interventionRiskFloor in EnumerateActions).
	f, err := strconv.ParseFloat(s, 64)
	if err != nil {
		return 0
	}
	return f
}
