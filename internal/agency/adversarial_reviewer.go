package agency

import (
	"fmt"
	"sort"
	"strings"
)

// AdversarialReviewer is the Phase 4 reviewer agent. It does not call
// any LLM; instead, it hunts for mechanism-level fault lines in a
// GISTVerdict (negative-Shapley "evidence", lone interventions,
// thin confounder coverage, lopsidedly-heavy evidence) and emits
// plausible Disputes for each. Each dispute is then evaluated by
// EvaluateDispute and adjudicated by Adjudicate, producing a sorted
// slice of DisputeRecords.
//
// The reviewer is deterministic: same verdict in, same record set out
// (modulo the dispute IDs, which are derived from a hash of
// reviewer-id + verdict-id + heuristic-name).
//
// It is intentionally not a daemon — it is composed into the GIST
// verdict pipeline (or surfaced via /api/verdict/review). A daemon
// wrapper is a follow-up if desired.
type AdversarialReviewer struct {
	cfg AdversarialReviewerConfig
}

// AdversarialReviewerConfig tunes reviewer aggression. Defaults are
// returned by DefaultAdversarialReviewerConfig and tuned for "useful
// without spammy."
type AdversarialReviewerConfig struct {
	// ReviewerID stamps every Dispute.ReviewerID this reviewer
	// emits. Defaults to "adversarial-reviewer".
	ReviewerID string

	// MaxDisputes caps the number of records returned (after
	// sorting). Zero means "no cap".
	MaxDisputes int

	// MinAbsSwingForKeep drops reports whose |ΔConfidence| is below
	// the threshold AND that did not flip recommendation /
	// blocking / top-Shapley. 0 keeps everything. Default 0.0
	// (keep everything; let Adjudicate sort upheld vs rejected).
	MinAbsSwingForKeep float64

	// HighConfidenceProbeFloor is the projected-confidence above
	// which we synthesise a "missing-confounder" probe to test
	// robustness. Default 0.7.
	HighConfidenceProbeFloor float64

	// HeavyEvidenceWeightFloor is the per-node weight at which an
	// evidence node becomes a candidate for an evidence-stale
	// probe. Default 0.8.
	HeavyEvidenceWeightFloor float64
}

// DefaultAdversarialReviewerConfig returns the deployment-default
// reviewer config. All fields can be overridden individually.
func DefaultAdversarialReviewerConfig() AdversarialReviewerConfig {
	return AdversarialReviewerConfig{
		ReviewerID:               "adversarial-reviewer",
		MaxDisputes:              8,
		MinAbsSwingForKeep:       0.0,
		HighConfidenceProbeFloor: 0.7,
		HeavyEvidenceWeightFloor: 0.8,
	}
}

// NewAdversarialReviewer builds a reviewer from config. Zero-value
// fields are populated with defaults so callers can override only
// the knobs they care about.
func NewAdversarialReviewer(cfg AdversarialReviewerConfig) *AdversarialReviewer {
	def := DefaultAdversarialReviewerConfig()
	if strings.TrimSpace(cfg.ReviewerID) == "" {
		cfg.ReviewerID = def.ReviewerID
	}
	if cfg.HighConfidenceProbeFloor <= 0 {
		cfg.HighConfidenceProbeFloor = def.HighConfidenceProbeFloor
	}
	if cfg.HeavyEvidenceWeightFloor <= 0 {
		cfg.HeavyEvidenceWeightFloor = def.HeavyEvidenceWeightFloor
	}
	return &AdversarialReviewer{cfg: cfg}
}

// Review hunts plausible disputes against a verdict and returns the
// resulting sorted DisputeRecords. The verdict is treated as
// immutable input.
//
// Heuristics, in order:
//
//  1. mislabeled-role: any evidence node with negative Shapley φ
//     (the supposedly-supportive node is actually pulling confidence
//     down) gets a "is this a confounder?" probe.
//
//  2. action-blocked: every Pearl-recommended action gets a
//     "what if this is infeasible?" probe.
//
//  3. missing-confounder: when projected confidence is high and
//     the existing confounder count is sparse, a synthetic
//     confounder is appended to test robustness.
//
//  4. evidence-stale: heavy evidence nodes (weight ≥ floor) get a
//     "what if this is stale?" probe.
//
// The reviewer never proposes narrative-only disputes; those exist for
// human reviewers.
func (a *AdversarialReviewer) Review(verdict GISTVerdict) []DisputeRecord {
	if a == nil {
		return nil
	}
	verdict.SyncCausalChain()
	if verdict.CausalGraph == nil || len(verdict.CausalGraph.Nodes) == 0 {
		return nil
	}

	disputes := a.proposeMislabelDisputes(verdict)
	disputes = append(disputes, a.proposeActionBlockedDisputes(verdict)...)
	if probe := a.proposeMissingConfounderProbe(verdict); probe != nil {
		disputes = append(disputes, *probe)
	}
	disputes = append(disputes, a.proposeEvidenceStaleDisputes(verdict)...)

	if len(disputes) == 0 {
		return nil
	}

	// Evaluate -> adjudicate -> filter -> sort.
	records := make([]DisputeRecord, 0, len(disputes))
	for _, d := range disputes {
		report := EvaluateDispute(verdict, d)
		if !a.keep(report) {
			continue
		}
		records = append(records, NewDisputeRecord(report))
	}

	sort.SliceStable(records, func(i, j int) bool {
		// Upheld first, then noted, then rejected; within each
		// status, swing-score descending; ID for stability.
		si, sj := statusRank(records[i].Adjudication.Status), statusRank(records[j].Adjudication.Status)
		if si != sj {
			return si < sj
		}
		if records[i].Adjudication.SwingScore != records[j].Adjudication.SwingScore {
			return records[i].Adjudication.SwingScore > records[j].Adjudication.SwingScore
		}
		return records[i].Report.Dispute.ID < records[j].Report.Dispute.ID
	})

	if a.cfg.MaxDisputes > 0 && len(records) > a.cfg.MaxDisputes {
		records = records[:a.cfg.MaxDisputes]
	}
	return records
}

// keep filters out reports that are below the swing floor AND didn't
// flip any flag — those reports are pure noise and would spam the
// inspector.
func (a *AdversarialReviewer) keep(r DisputeReport) bool {
	if a.cfg.MinAbsSwingForKeep <= 0 {
		return true
	}
	if r.RecommendationFlipped || r.BlockedByConfounderFlipped || r.TopShapleyChanged {
		return true
	}
	return r.AbsConfidenceSwing() >= a.cfg.MinAbsSwingForKeep
}

func (a *AdversarialReviewer) proposeMislabelDisputes(verdict GISTVerdict) []Dispute {
	var out []Dispute
	for _, attr := range verdict.Attribution {
		if attr.Role != NodeRoleEvidence {
			continue
		}
		if attr.Phi >= 0 {
			continue
		}
		// Negative φ on an evidence atom: claim it's actually a
		// confounder.
		out = append(out, Dispute{
			ID:         a.disputeID("mislabel", string(attr.NodeID)),
			ReviewerID: a.cfg.ReviewerID,
			Ground:     DisputeGroundMislabeledRole,
			TargetNode: attr.NodeID,
			NewRole:    NodeRoleConfounder,
			Narrative: fmt.Sprintf("evidence node %q has negative Shapley contribution (%.3f) — likely confounder",
				attr.NodeID, attr.Phi),
		})
	}
	return out
}

func (a *AdversarialReviewer) proposeActionBlockedDisputes(verdict GISTVerdict) []Dispute {
	if verdict.PearlPlan == nil {
		return nil
	}
	var out []Dispute
	for _, c := range verdict.PearlPlan.Actions {
		if !c.Recommended {
			continue
		}
		out = append(out, Dispute{
			ID:         a.disputeID("blocked", string(c.NodeID)),
			ReviewerID: a.cfg.ReviewerID,
			Ground:     DisputeGroundActionBlocked,
			TargetNode: c.NodeID,
			Narrative:  fmt.Sprintf("probe: what if recommended action %q is infeasible?", c.NodeID),
		})
	}
	return out
}

func (a *AdversarialReviewer) proposeMissingConfounderProbe(verdict GISTVerdict) *Dispute {
	if verdict.PearlPlan == nil {
		return nil
	}
	if verdict.PearlPlan.Prediction.ProjectedConfidence < a.cfg.HighConfidenceProbeFloor {
		return nil
	}
	if len(verdict.PearlPlan.Hypothesis.Confounders) >= 2 {
		// Already enough confounders on record; reviewer not
		// suspicious.
		return nil
	}
	summary := "unobserved common cause stress test"
	// If the verdict surfaced an open question, use it as the
	// concrete hypothesis for the missing confounder.
	if len(verdict.OpenQuestions) > 0 {
		oq := strings.TrimSpace(verdict.OpenQuestions[0])
		if oq != "" {
			summary = oq
		}
	}
	return &Dispute{
		ID:           a.disputeID("missing-conf", summary),
		ReviewerID:   a.cfg.ReviewerID,
		Ground:       DisputeGroundMissingConfounder,
		AddedSummary: summary,
		AddedWeight:  defaultAddedConfounderWeight,
		Narrative: fmt.Sprintf("high-confidence verdict (%.3f) with sparse confounder coverage — probing robustness",
			verdict.PearlPlan.Prediction.ProjectedConfidence),
	}
}

func (a *AdversarialReviewer) proposeEvidenceStaleDisputes(verdict GISTVerdict) []Dispute {
	if verdict.CausalGraph == nil {
		return nil
	}
	var out []Dispute
	for _, n := range verdict.CausalGraph.Nodes {
		if n.Role != NodeRoleEvidence {
			continue
		}
		if n.Weight < a.cfg.HeavyEvidenceWeightFloor {
			continue
		}
		out = append(out, Dispute{
			ID:          a.disputeID("stale", string(n.ID)),
			ReviewerID:  a.cfg.ReviewerID,
			Ground:      DisputeGroundEvidenceStale,
			TargetNode:  n.ID,
			StaleFactor: defaultStaleFactor,
			Narrative: fmt.Sprintf("heavy evidence node %q (weight %.2f) probed for staleness",
				n.ID, n.Weight),
		})
	}
	return out
}

// disputeID builds a stable ID of the form
// "{reviewerID}:{heuristic}:{slug}". Used so the inspector can
// correlate reviewer probes across re-runs of the same verdict.
func (a *AdversarialReviewer) disputeID(heuristic, key string) string {
	return strings.Join([]string{a.cfg.ReviewerID, heuristic, simpleSlug(key)}, ":")
}

// statusRank assigns a sort key for adjudication statuses. Lower =
// surfaced first.
func statusRank(s DisputeStatus) int {
	switch s {
	case DisputeStatusUpheld:
		return 0
	case DisputeStatusNoted:
		return 1
	case DisputeStatusRejected:
		return 2
	default:
		return 3
	}
}
