package agency

import (
	"encoding/json"
	"fmt"
	"strings"
)

// GISTInspectorBundleProtocolVersion is bumped when the persisted inspector
// envelope shape changes in a non-additive way. Additive changes (new
// optional fields) do NOT need a bump.
const GISTInspectorBundleProtocolVersion = 1

// GISTInspectorBundle is the persisted projection of a GISTVerdict that the
// lattice inspector renders. It is deliberately a denormalised, self-contained
// JSON envelope so the inspector can render a trace without re-running the
// kernel, and so old traces (missing the bundle) degrade gracefully.
//
// The bundle is written into the agency_gist_traces.inspector_json column at
// trace-store time, alongside the existing legacy blobs (trace_json,
// proof_json, lattice_json). It captures everything Phases 1-2 added on top
// of GISTVerdict that the inspector route needs:
//
//   - CausalGraph (the typed Pearl-shaped DAG, protocol v1)
//   - PearlPlan (abduce / act / predict triple with scores)
//   - Attribution (Shapley necessity ranking)
//   - FlatChain (legacy ordering, retained for tooling that wants a list)
//   - Verdict / RiskLevel / Confidence headline metadata
//   - OpenQuestions / Contradictions counts (for hover summaries)
type GISTInspectorBundle struct {
	ProtocolVersion int      `json:"protocolVersion"`
	Verdict         string   `json:"verdict,omitempty"`
	RiskLevel       string   `json:"riskLevel,omitempty"`
	Confidence      float64  `json:"confidence,omitempty"`
	Degraded        bool     `json:"degraded,omitempty"`
	DegradedReason  string   `json:"degradedReason,omitempty"`
	ExecutionIntent string   `json:"executionIntent,omitempty"`
	OpenQuestions   []string `json:"openQuestions,omitempty"`

	// Number of contradictions reported by the kernel. Stored as a count
	// rather than the full slice so the inspector blob stays compact;
	// the full contradiction objects live in the lattice JSON.
	ContradictionCount int `json:"contradictionCount,omitempty"`
	InterventionCount  int `json:"interventionCount,omitempty"`

	// CausalGraph is the typed Pearl DAG. May be nil for legacy or
	// degraded verdicts; in that case FlatChain is still populated from
	// the legacy []string causal chain.
	CausalGraph *CausalGraph `json:"causalGraph,omitempty"`
	// FlatChain is the deterministic flattening of CausalGraph (or the
	// raw legacy chain when CausalGraph is nil). Inspector renders it as
	// the "Reasoning chain" tab.
	FlatChain []string `json:"flatChain,omitempty"`

	// PearlPlan is the abduction / action / prediction triple from
	// RunPearlLoop. nil if the loop was skipped (degraded verdicts).
	PearlPlan *PearlPlan `json:"pearlPlan,omitempty"`
	// Attribution is the Shapley necessity ranking from
	// AttributeNecessity. Empty if the graph had no players.
	Attribution []NodeAttribution `json:"attribution,omitempty"`

	// Disputes is the adversarial-review record set attached to this
	// verdict. Each entry pairs a Dispute with its DisputeReport
	// (counterfactual swing) and Adjudication (status). Empty for
	// verdicts that have not been routed through the reviewer.
	Disputes []DisputeRecord `json:"disputes,omitempty"`
}

// BuildInspectorBundle builds an inspector bundle from a GISTVerdict.
// The returned bundle is safe to mutate independently of the verdict.
//
// The verdict's CausalChain/CausalGraph relationship is reconciled via
// SyncCausalChain so the bundle's FlatChain agrees with CausalGraph.
func BuildInspectorBundle(verdict GISTVerdict) *GISTInspectorBundle {
	// Defensive copy: clone graph + plan + attribution so the persisted
	// bundle cannot be mutated by later in-memory edits to verdict.
	verdict.SyncCausalChain()

	bundle := &GISTInspectorBundle{
		ProtocolVersion:    GISTInspectorBundleProtocolVersion,
		Verdict:            verdict.Verdict,
		RiskLevel:          verdict.RiskLevel,
		Confidence:         verdict.Confidence,
		Degraded:           verdict.Degraded,
		DegradedReason:     verdict.DegradedReason,
		ExecutionIntent:    verdict.ExecutionIntent,
		OpenQuestions:      append([]string(nil), verdict.OpenQuestions...),
		ContradictionCount: len(verdict.Contradictions),
		InterventionCount:  len(verdict.Interventions),
		FlatChain:          append([]string(nil), verdict.CausalChain...),
	}
	if verdict.CausalGraph != nil {
		bundle.CausalGraph = verdict.CausalGraph.Clone()
	}
	if verdict.PearlPlan != nil {
		clone := *verdict.PearlPlan
		bundle.PearlPlan = &clone
	}
	if len(verdict.Attribution) > 0 {
		bundle.Attribution = append([]NodeAttribution(nil), verdict.Attribution...)
	}
	if len(verdict.Disputes) > 0 {
		bundle.Disputes = append([]DisputeRecord(nil), verdict.Disputes...)
	}
	return bundle
}

// MarshalInspectorBundle serialises a GISTVerdict into the JSON blob
// persisted in agency_gist_traces.inspector_json. A nil/empty verdict
// returns "{}" so the column constraint (NOT NULL) is satisfied.
func MarshalInspectorBundle(verdict GISTVerdict) (string, error) {
	bundle := BuildInspectorBundle(verdict)
	if bundle == nil {
		return "{}", nil
	}
	buf, err := json.Marshal(bundle)
	if err != nil {
		return "{}", fmt.Errorf("marshal inspector bundle: %w", err)
	}
	return string(buf), nil
}

// ParseInspectorBundle parses a persisted inspector_json blob. Empty or
// "{}" blobs return (nil, nil) so callers can distinguish "no bundle
// stored" from a parse error and fall back to legacy reconstruction.
//
// If the blob is non-empty but parses cleanly with protocolVersion == 0,
// it's treated as a legacy/empty record and (nil, nil) is also returned.
func ParseInspectorBundle(raw string) (*GISTInspectorBundle, error) {
	trimmed := strings.TrimSpace(raw)
	if trimmed == "" || trimmed == "{}" || trimmed == "null" {
		return nil, nil
	}
	var bundle GISTInspectorBundle
	if err := json.Unmarshal([]byte(trimmed), &bundle); err != nil {
		return nil, fmt.Errorf("parse inspector bundle: %w", err)
	}
	if bundle.ProtocolVersion == 0 && bundle.CausalGraph == nil &&
		bundle.PearlPlan == nil && len(bundle.Attribution) == 0 &&
		len(bundle.FlatChain) == 0 && len(bundle.Disputes) == 0 {
		return nil, nil
	}
	return &bundle, nil
}

// HydrateInspectorBundleFromLegacy synthesises a best-effort inspector
// bundle for a trace that was persisted before this column existed.
// It uses the legacy trace_json (a GISTTrace) to recover SelectedChain
// and SelectedVerdict; the typed graph is hydrated from the chain via
// HydrateLegacyCausalChain, but PearlPlan and Attribution will be nil
// (they were never computed for that trace).
//
// Returns nil if the legacy trace blob is empty or unparseable; the
// caller should display "no inspector data available" in that case.
func HydrateInspectorBundleFromLegacy(traceJSON string) *GISTInspectorBundle {
	trimmed := strings.TrimSpace(traceJSON)
	if trimmed == "" || trimmed == "{}" {
		return nil
	}
	var trace GISTTrace
	if err := json.Unmarshal([]byte(trimmed), &trace); err != nil {
		return nil
	}
	bundle := &GISTInspectorBundle{
		ProtocolVersion: GISTInspectorBundleProtocolVersion,
		Verdict:         trace.SelectedVerdict,
		FlatChain:       append([]string(nil), trace.SelectedChain...),
	}
	if len(trace.SelectedChain) > 0 {
		bundle.CausalGraph = HydrateLegacyCausalChain(trace.SelectedChain)
	}
	bundle.ContradictionCount = len(trace.ContradictionIDs)
	bundle.InterventionCount = len(trace.InterventionIDs)
	return bundle
}
