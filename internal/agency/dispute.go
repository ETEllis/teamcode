package agency

import (
	"errors"
	"fmt"
	"math"
	"sort"
	"strings"
	"time"
)

// DisputeProtocolVersion is the wire version for persisted DisputeRecord
// payloads. Bump on shape-breaking changes.
const DisputeProtocolVersion = 1

// DisputeGround enumerates the mechanism-level grounds on which an
// adversarial reviewer can dispute a verdict. Each ground (except the
// narrative carve-out) maps to a deterministic transformation of the
// typed CausalGraph; the dispute's evidentiary force is then the
// counterfactual swing between the original verdict and the verdict
// that results from re-running the Pearl loop on the transformed
// graph.
//
// Grounds are intentionally narrow and Pearl-shaped:
//
//   - mislabeled-role     — claim a node's role is wrong (e.g. an
//     "evidence" atom is actually a confounder).
//   - missing-confounder  — claim the graph omits a confounder that,
//     if added, would change action selection.
//   - action-blocked      — claim a recommended intervention is
//     infeasible / disallowed and must be removed from the candidate
//     space.
//   - evidence-stale      — claim an evidence node's weight should be
//     down-rated because it has decayed in relevance.
//   - narrative           — a non-mechanism reviewer concern. No graph
//     mutation. Always routes to DisputeStatusNoted.
type DisputeGround string

const (
	DisputeGroundMislabeledRole    DisputeGround = "mislabeled-role"
	DisputeGroundMissingConfounder DisputeGround = "missing-confounder"
	DisputeGroundActionBlocked     DisputeGround = "action-blocked"
	DisputeGroundEvidenceStale     DisputeGround = "evidence-stale"
	DisputeGroundNarrative         DisputeGround = "narrative"
)

// Dispute is the input lodged by an adversarial reviewer agent. It is a
// claim about a specific GISTVerdict; together with that verdict it can
// be evaluated mechanically by EvaluateDispute.
//
// Field usage by ground:
//
//   - mislabeled-role     — TargetNode + NewRole are required.
//   - missing-confounder  — AddedSummary required; AddedWeight optional
//     (defaults to defaultAddedConfounderWeight).
//   - action-blocked      — TargetNode required; node is removed from
//     the graph so it no longer shows up as an intervention candidate.
//   - evidence-stale      — TargetNode required; StaleFactor is the
//     multiplier applied to the node's weight (defaults to
//     defaultStaleFactor when zero or out of range).
//   - narrative           — Narrative required; graph is unchanged.
//
// CreatedAt is recorded so the persisted DisputeRecord on the inspector
// bundle has a stable timestamp; if zero, EvaluateDispute stamps now().
type Dispute struct {
	ID           string        `json:"id,omitempty"`
	ReviewerID   string        `json:"reviewerId,omitempty"`
	Ground       DisputeGround `json:"ground"`
	TargetNode   NodeID        `json:"targetNode,omitempty"`
	NewRole      NodeRole      `json:"newRole,omitempty"`
	AddedSummary string        `json:"addedSummary,omitempty"`
	AddedWeight  float64       `json:"addedWeight,omitempty"`
	StaleFactor  float64       `json:"staleFactor,omitempty"`
	Narrative    string        `json:"narrative,omitempty"`
	CreatedAt    time.Time     `json:"createdAt,omitempty"`
}

// DisputeReport is the output of EvaluateDispute. It preserves the
// pre-dispute "world" (Original*) and the counterfactual world that
// results from applying the claim and re-running the Pearl loop +
// Shapley attribution. The Delta* fields are convenience computations
// the inspector and Adjudicate consume directly.
//
// Applied=false means the claim was a no-op (typically narrative or
// referenced a node that doesn't exist); Adjudicate routes such
// reports to DisputeStatusNoted rather than rejected, since the
// reviewer's concern is still on record.
type DisputeReport struct {
	Dispute Dispute `json:"dispute"`

	OriginalVerdict             string  `json:"originalVerdict,omitempty"`
	OriginalConfidence          float64 `json:"originalConfidence"`
	OriginalRecommendation      NodeID  `json:"originalRecommendation,omitempty"`
	OriginalTopShapley          NodeID  `json:"originalTopShapley,omitempty"`
	OriginalBlockedByConfounder bool    `json:"originalBlockedByConfounder,omitempty"`

	CounterfactualPlan           *PearlPlan        `json:"counterfactualPlan,omitempty"`
	CounterfactualAttribution    []NodeAttribution `json:"counterfactualAttribution,omitempty"`
	CounterfactualConfidence     float64           `json:"counterfactualConfidence"`
	CounterfactualRecommendation NodeID            `json:"counterfactualRecommendation,omitempty"`
	CounterfactualTopShapley     NodeID            `json:"counterfactualTopShapley,omitempty"`
	CounterfactualBlocked        bool              `json:"counterfactualBlocked,omitempty"`

	DeltaConfidence            float64 `json:"deltaConfidence"`
	RecommendationFlipped      bool    `json:"recommendationFlipped,omitempty"`
	TopShapleyChanged          bool    `json:"topShapleyChanged,omitempty"`
	BlockedByConfounderFlipped bool    `json:"blockedByConfounderFlipped,omitempty"`

	Applied bool     `json:"applied"`
	Notes   []string `json:"notes,omitempty"`
}

// Defaults applied when reviewer leaves a knob at zero. Tuned so that a
// reviewer who only specifies "ground + target" still produces a
// meaningful counterfactual swing.
const (
	defaultAddedConfounderWeight = 0.7
	defaultStaleFactor           = 0.25
)

// ErrDisputeMalformed is returned by EvaluateDispute when a dispute
// references a target node that doesn't exist or is missing a required
// field. Callers can choose to surface the validation error to the
// reviewer or downgrade the report to DisputeStatusNoted via
// Adjudicate.
var ErrDisputeMalformed = errors.New("dispute malformed")

// ApplyDisputeClaim materialises a Dispute against a CausalGraph,
// returning a deep clone with the claim applied (or the original graph
// when the claim is a no-op).
//
// Returned tuple:
//
//   - *CausalGraph — clone with claim applied (never nil for non-nil input)
//   - bool         — whether a non-trivial mutation happened
//   - []string     — diagnostic notes describing what changed (or why the
//     claim was a no-op).
//
// The original graph is never mutated.
func ApplyDisputeClaim(graph *CausalGraph, d Dispute) (*CausalGraph, bool, []string) {
	if graph == nil {
		return nil, false, []string{"no graph to mutate"}
	}
	cf := graph.Clone()
	notes := []string{}
	switch d.Ground {
	case DisputeGroundMislabeledRole:
		if d.TargetNode == "" || d.NewRole == "" {
			return cf, false, []string{"mislabeled-role requires targetNode + newRole"}
		}
		idx := indexOfNode(cf, d.TargetNode)
		if idx < 0 {
			return cf, false, []string{fmt.Sprintf("target node %q not found", d.TargetNode)}
		}
		old := cf.Nodes[idx].Role
		if old == d.NewRole {
			return cf, false, []string{fmt.Sprintf("node %q already has role %q", d.TargetNode, d.NewRole)}
		}
		cf.Nodes[idx].Role = d.NewRole
		notes = append(notes, fmt.Sprintf("node %q role: %q -> %q", d.TargetNode, old, d.NewRole))
		return cf, true, notes
	case DisputeGroundMissingConfounder:
		summary := strings.TrimSpace(d.AddedSummary)
		if summary == "" {
			return cf, false, []string{"missing-confounder requires addedSummary"}
		}
		weight := d.AddedWeight
		if weight <= 0 {
			weight = defaultAddedConfounderWeight
		}
		newID := NodeID("dispute:" + simpleSlug(summary))
		// Avoid accidental collision with an existing node.
		if indexOfNode(cf, newID) >= 0 {
			newID = NodeID(string(newID) + "-cf")
		}
		cf.Nodes = append(cf.Nodes, CausalNode{
			ID:      newID,
			Role:    NodeRoleConfounder,
			Summary: summary,
			Weight:  weight,
			Meta:    map[string]string{"source": "dispute"},
		})
		notes = append(notes, fmt.Sprintf("appended confounder %q (weight %.2f)", newID, weight))
		return cf, true, notes
	case DisputeGroundActionBlocked:
		if d.TargetNode == "" {
			return cf, false, []string{"action-blocked requires targetNode"}
		}
		idx := indexOfNode(cf, d.TargetNode)
		if idx < 0 {
			return cf, false, []string{fmt.Sprintf("target node %q not found", d.TargetNode)}
		}
		// Remove the node so it disappears from the candidate set
		// AND the Shapley player set. Cleanest semantics for "this
		// action is no longer available."
		cf.Nodes = append(cf.Nodes[:idx], cf.Nodes[idx+1:]...)
		notes = append(notes, fmt.Sprintf("removed action %q from graph", d.TargetNode))
		return cf, true, notes
	case DisputeGroundEvidenceStale:
		if d.TargetNode == "" {
			return cf, false, []string{"evidence-stale requires targetNode"}
		}
		idx := indexOfNode(cf, d.TargetNode)
		if idx < 0 {
			return cf, false, []string{fmt.Sprintf("target node %q not found", d.TargetNode)}
		}
		factor := d.StaleFactor
		if factor <= 0 || factor >= 1 {
			factor = defaultStaleFactor
		}
		old := cf.Nodes[idx].Weight
		cf.Nodes[idx].Weight = old * factor
		if cf.Nodes[idx].Meta == nil {
			cf.Nodes[idx].Meta = map[string]string{}
		}
		cf.Nodes[idx].Meta["stale"] = "true"
		notes = append(notes, fmt.Sprintf("node %q weight: %.3f -> %.3f (factor %.2f)",
			d.TargetNode, old, cf.Nodes[idx].Weight, factor))
		return cf, true, notes
	case DisputeGroundNarrative:
		txt := strings.TrimSpace(d.Narrative)
		if txt == "" {
			return cf, false, []string{"narrative requires non-empty text"}
		}
		notes = append(notes, "narrative recorded; graph unchanged")
		return cf, false, notes
	default:
		return cf, false, []string{fmt.Sprintf("unknown dispute ground %q", d.Ground)}
	}
}

// EvaluateDispute is the heart of Phase 4: it takes a verdict and a
// dispute, applies the dispute claim to a clone of the verdict's typed
// causal graph, re-runs the Pearl loop and Shapley attribution on the
// counterfactual graph, and returns a DisputeReport carrying both the
// original and counterfactual states plus the deltas that adjudication
// keys off.
//
// The verdict is treated as immutable input; nothing on the verdict is
// mutated. The dispute's CreatedAt is stamped if zero.
func EvaluateDispute(verdict GISTVerdict, d Dispute) DisputeReport {
	if d.CreatedAt.IsZero() {
		d.CreatedAt = time.Now().UTC()
	}
	report := DisputeReport{Dispute: d}

	// Snapshot original.
	verdict.SyncCausalChain()
	report.OriginalVerdict = verdict.Verdict
	report.OriginalConfidence = verdict.Confidence
	if verdict.PearlPlan != nil {
		report.OriginalRecommendation = verdict.PearlPlan.Prediction.Recommended
		report.OriginalBlockedByConfounder = verdict.PearlPlan.Prediction.BlockedByConfounder
		// Prefer projected confidence when available — that is the
		// number the action policy actually reads.
		if verdict.PearlPlan.Prediction.ProjectedConfidence > 0 {
			report.OriginalConfidence = verdict.PearlPlan.Prediction.ProjectedConfidence
		}
	}
	if len(verdict.Attribution) > 0 {
		report.OriginalTopShapley = verdict.Attribution[0].NodeID
	}

	// Materialise counterfactual.
	cfGraph, applied, notes := ApplyDisputeClaim(verdict.CausalGraph, d)
	report.Applied = applied
	report.Notes = notes

	if cfGraph == nil || len(cfGraph.Nodes) == 0 {
		// Graph was nil or got emptied out. Counterfactual collapses
		// to "no plan / no attribution"; treat as a confidence-zero
		// world.
		report.CounterfactualConfidence = 0
		report.DeltaConfidence = -report.OriginalConfidence
		report.RecommendationFlipped = report.OriginalRecommendation != ""
		report.TopShapleyChanged = report.OriginalTopShapley != ""
		return report
	}

	cfPlan := RunPearlLoop(cfGraph)
	cfAttribution := AttributeNecessity(cfGraph)
	report.CounterfactualPlan = cfPlan
	report.CounterfactualAttribution = cfAttribution
	if cfPlan != nil {
		report.CounterfactualRecommendation = cfPlan.Prediction.Recommended
		report.CounterfactualConfidence = cfPlan.Prediction.ProjectedConfidence
		report.CounterfactualBlocked = cfPlan.Prediction.BlockedByConfounder
	}
	if len(cfAttribution) > 0 {
		report.CounterfactualTopShapley = cfAttribution[0].NodeID
	}

	// Compute swings.
	report.DeltaConfidence = report.CounterfactualConfidence - report.OriginalConfidence
	report.RecommendationFlipped = report.OriginalRecommendation != report.CounterfactualRecommendation
	report.TopShapleyChanged = report.OriginalTopShapley != report.CounterfactualTopShapley
	report.BlockedByConfounderFlipped = report.OriginalBlockedByConfounder != report.CounterfactualBlocked

	return report
}

// AbsConfidenceSwing returns the magnitude of the counterfactual
// confidence shift, clamped to [0,1]. Convenience for adjudication and
// inspector display.
func (r DisputeReport) AbsConfidenceSwing() float64 {
	v := math.Abs(r.DeltaConfidence)
	if v > 1 {
		v = 1
	}
	return v
}

// indexOfNode returns the index of the node with the given ID, or -1.
func indexOfNode(g *CausalGraph, id NodeID) int {
	if g == nil {
		return -1
	}
	for i, n := range g.Nodes {
		if n.ID == id {
			return i
		}
	}
	return -1
}

// simpleSlug returns a lowercased, dash-separated slug suitable for use
// in synthetic node IDs. Collapses runs of non-alphanumeric chars to a
// single dash; trims leading/trailing dashes; truncates to 32 chars.
func simpleSlug(s string) string {
	s = strings.ToLower(strings.TrimSpace(s))
	if s == "" {
		return "anon"
	}
	var b strings.Builder
	dashed := false
	for _, r := range s {
		switch {
		case r >= 'a' && r <= 'z', r >= '0' && r <= '9':
			b.WriteRune(r)
			dashed = false
		default:
			if !dashed && b.Len() > 0 {
				b.WriteRune('-')
				dashed = true
			}
		}
	}
	out := strings.Trim(b.String(), "-")
	if out == "" {
		return "anon"
	}
	if len(out) > 32 {
		out = out[:32]
	}
	return out
}

// SortDisputeReports orders reports by adjudication-relevant signal:
// recommendation flips first, then largest |ΔConfidence|, then top-Shapley
// changes, then ID for stability. Used by the adversarial reviewer to
// surface the strongest disputes first.
func SortDisputeReports(reports []DisputeReport) {
	sort.SliceStable(reports, func(i, j int) bool {
		ri, rj := reports[i], reports[j]
		if ri.RecommendationFlipped != rj.RecommendationFlipped {
			return ri.RecommendationFlipped
		}
		ai, aj := ri.AbsConfidenceSwing(), rj.AbsConfidenceSwing()
		if ai != aj {
			return ai > aj
		}
		if ri.TopShapleyChanged != rj.TopShapleyChanged {
			return ri.TopShapleyChanged
		}
		return ri.Dispute.ID < rj.Dispute.ID
	})
}
