package agency

import (
	"fmt"
)

// DisputeStatus is the deterministic verdict on a dispute itself,
// produced by Adjudicate(report). It is mechanism-faithful: a dispute
// is upheld iff applying its claim materially shifts the
// counterfactual world (action swap, confidence past threshold, or
// blocking flag flipped), not because of social weight.
type DisputeStatus string

const (
	// DisputeStatusUpheld means the counterfactual swing was large
	// enough to materially change the verdict's behaviour: a
	// recommendation flipped, the BlockedByConfounder flag flipped,
	// or |ΔConfidence| crossed the upholdConfidenceThreshold.
	DisputeStatusUpheld DisputeStatus = "upheld"

	// DisputeStatusNoted means the dispute produced a measurable but
	// sub-threshold swing, OR it was a non-mechanism concern
	// (narrative ground / no-op claim). The dispute stays on record
	// and is surfaced in the inspector, but the verdict is not
	// invalidated.
	DisputeStatusNoted DisputeStatus = "noted"

	// DisputeStatusRejected means the claim transformed the graph
	// but the resulting counterfactual was indistinguishable from
	// the original (no detectable swing in confidence, top-Shapley
	// rank, recommendation, or blocking flag).
	DisputeStatusRejected DisputeStatus = "rejected"
)

// Adjudication is the deterministic ruling on a single dispute. It is
// derived purely from the DisputeReport — no clocks, no randomness,
// no LLM in the loop — so any reviewer (human or agent) can replay
// the same report and arrive at the same status.
type Adjudication struct {
	Status     DisputeStatus `json:"status"`
	Reason     string        `json:"reason,omitempty"`
	SwingScore float64       `json:"swingScore"`
	Mechanical bool          `json:"mechanical"`
}

// Adjudication thresholds. These are the only knobs in the
// adjudication ladder; tightening or loosening them is the only knob
// available to a deployer that wants harsher or kinder review.
//
// upholdConfidenceThreshold is intentionally well above the noise
// floor of the sigmoid value function (≈0.05 for a single low-weight
// node addition), so a dispute has to actually move the needle.
const (
	upholdConfidenceThreshold = 0.15
	notedConfidenceThreshold  = 0.03
)

// Swing-score weights. Composite swing score is:
//
//	|ΔConfidence|
//	+ recommendationFlipBonus     if recommendation flipped
//	+ blockedByConfounderBonus    if blocking flag flipped
//	+ topShapleyChangedBonus      if top-Shapley node changed
//
// Bonus magnitudes are deliberately greater than
// notedConfidenceThreshold so a flip alone always crosses "noted",
// and recommendation flips alone cross "upheld".
const (
	recommendationFlipBonus  = 0.50
	blockedByConfounderBonus = 0.30
	topShapleyChangedBonus   = 0.10
)

// Adjudicate produces a deterministic Adjudication for a single
// DisputeReport. The decision tree is read top-down; first match wins:
//
//  1. Non-mechanism reports (Applied=false, e.g. narrative or
//     malformed claim) → Noted. Mechanical=false.
//  2. Recommendation flipped OR BlockedByConfounder flipped → Upheld.
//  3. |ΔConfidence| ≥ upholdConfidenceThreshold → Upheld.
//  4. |ΔConfidence| ≥ notedConfidenceThreshold OR top-Shapley
//     changed → Noted.
//  5. Otherwise → Rejected.
func Adjudicate(report DisputeReport) Adjudication {
	swing := report.AbsConfidenceSwing()
	if report.RecommendationFlipped {
		swing += recommendationFlipBonus
	}
	if report.BlockedByConfounderFlipped {
		swing += blockedByConfounderBonus
	}
	if report.TopShapleyChanged {
		swing += topShapleyChangedBonus
	}

	// (1) Non-mechanism: narrative ground always lands here, as do
	// claims that referenced missing nodes or empty fields.
	if !report.Applied {
		return Adjudication{
			Status:     DisputeStatusNoted,
			Reason:     "non-mechanism concern (narrative or no-op claim)",
			SwingScore: swing,
			Mechanical: false,
		}
	}

	// (2) Behaviour-flipping swings are always upheld.
	if report.RecommendationFlipped {
		return Adjudication{
			Status: DisputeStatusUpheld,
			Reason: fmt.Sprintf("recommendation flipped: %q -> %q",
				report.OriginalRecommendation, report.CounterfactualRecommendation),
			SwingScore: swing,
			Mechanical: true,
		}
	}
	if report.BlockedByConfounderFlipped {
		return Adjudication{
			Status: DisputeStatusUpheld,
			Reason: fmt.Sprintf("blocked-by-confounder flipped: %v -> %v",
				report.OriginalBlockedByConfounder, report.CounterfactualBlocked),
			SwingScore: swing,
			Mechanical: true,
		}
	}

	// (3) Confidence shift past the upheld threshold.
	if report.AbsConfidenceSwing() >= upholdConfidenceThreshold {
		return Adjudication{
			Status: DisputeStatusUpheld,
			Reason: fmt.Sprintf("confidence swung %+.3f (>= %.2f)",
				report.DeltaConfidence, upholdConfidenceThreshold),
			SwingScore: swing,
			Mechanical: true,
		}
	}

	// (4) Sub-threshold but measurable: noted.
	if report.AbsConfidenceSwing() >= notedConfidenceThreshold {
		return Adjudication{
			Status: DisputeStatusNoted,
			Reason: fmt.Sprintf("confidence swung %+.3f (between %.2f and %.2f)",
				report.DeltaConfidence, notedConfidenceThreshold, upholdConfidenceThreshold),
			SwingScore: swing,
			Mechanical: true,
		}
	}
	if report.TopShapleyChanged {
		return Adjudication{
			Status: DisputeStatusNoted,
			Reason: fmt.Sprintf("top Shapley node changed: %q -> %q",
				report.OriginalTopShapley, report.CounterfactualTopShapley),
			SwingScore: swing,
			Mechanical: true,
		}
	}

	// (5) Default: rejected.
	return Adjudication{
		Status:     DisputeStatusRejected,
		Reason:     "no measurable counterfactual swing",
		SwingScore: swing,
		Mechanical: true,
	}
}

// AdjudicateBatch is a convenience wrapper that adjudicates a slice of
// reports in order. Used by the adversarial reviewer agent (next
// commit) and by any caller that wants per-batch ruling without
// hand-rolling a loop.
func AdjudicateBatch(reports []DisputeReport) []Adjudication {
	out := make([]Adjudication, len(reports))
	for i, r := range reports {
		out[i] = Adjudicate(r)
	}
	return out
}

// DisputeRecord packages a Dispute, its DisputeReport, and the
// resulting Adjudication into a single persistable unit. It is the
// shape carried on the inspector bundle (commit 4) and surfaced to
// the lattice inspector UI.
//
// ProtocolVersion is bumped when the carrier shape changes; the inner
// Report and Adjudication carry their own implicit versions through
// the protocol constants on Dispute and DisputeStatus.
type DisputeRecord struct {
	ProtocolVersion int           `json:"protocolVersion"`
	Report          DisputeReport `json:"report"`
	Adjudication    Adjudication  `json:"adjudication"`
}

// NewDisputeRecord materialises a DisputeRecord from a fully-populated
// DisputeReport (i.e. the output of EvaluateDispute) by adjudicating
// the report and stamping the protocol version.
func NewDisputeRecord(report DisputeReport) DisputeRecord {
	return DisputeRecord{
		ProtocolVersion: DisputeProtocolVersion,
		Report:          report,
		Adjudication:    Adjudicate(report),
	}
}

// IsUpheld returns true iff the record's adjudication is upheld.
// Convenience for filters in the inspector and for action-policy code
// that should refuse to act when any dispute is upheld.
func (r DisputeRecord) IsUpheld() bool {
	return r.Adjudication.Status == DisputeStatusUpheld
}
