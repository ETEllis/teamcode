package agency

import (
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"fmt"
	"sort"
	"strings"
)

// SpeculativeBundleProtocolVersion is the wire-format version of the
// persisted speculative envelope. Bumped on non-additive changes only.
const SpeculativeBundleProtocolVersion = 1

// SpeculativeBundle is the persisted projection of a speculative-tier
// cohort review: peer attestations, optional meta graph, convergence,
// reconciliation, and dyad compression report. It is the geometry
// payload the Lattice Cathedral renders.
//
// The bundle is denormalised on purpose so that the cathedral can be
// rendered from a single SELECT without re-running the kernel and so
// historic rows (missing the bundle) degrade gracefully.
type SpeculativeBundle struct {
	ProtocolVersion int    `json:"protocolVersion"`
	CohortID        string `json:"cohortId,omitempty"`

	// Peers is the cohort under review, with each peer's typed
	// CausalGraph and Merkle attestation embedded so the cathedral
	// can render geometry without follow-up lookups.
	Peers []SpeculativePeer `json:"peers,omitempty"`

	// Meta is the aggregator graph (optional). When present the
	// cathedral renders a meta tile above the cohort floor.
	Meta *SpeculativeMeta `json:"meta,omitempty"`

	// Convergence is the cohort-level Merkle gate report. Required.
	Convergence *ConvergenceReport `json:"convergence,omitempty"`
	// Reconciliation is the meta-vs-peers fidelity report.
	// nil when there is no meta in the cohort.
	Reconciliation *ReconciliationReport `json:"reconciliation,omitempty"`
	// Dyads is the Hamming-1 compression report. Always present;
	// SlotsAfter==SlotsBefore means no pairs were eligible.
	Dyads *DyadCompressionReport `json:"dyads,omitempty"`
}

// SpeculativePeer is one cohort member, ready to be drawn as a tile in
// the cathedral. InCohort marks consensus-bucket membership so the
// renderer can colour-code dissenters separately from the agreeing
// majority. Verdict / Confidence headline data is included so the
// JSON-only API can render without rehydrating the full GIST envelope.
type SpeculativePeer struct {
	AgentID     string            `json:"agentId"`
	Verdict     string            `json:"verdict,omitempty"`
	Confidence  float64           `json:"confidence,omitempty"`
	Graph       *CausalGraph      `json:"graph,omitempty"`
	Attestation MerkleAttestation `json:"attestation"`
	InCohort    bool              `json:"inCohort"`
}

// SpeculativeMeta is the optional aggregator/director projection.
// Reconciliation is computed against this graph; geometry-wise it
// floats above the cohort tiles in the cathedral.
type SpeculativeMeta struct {
	AgentID     string            `json:"agentId"`
	Verdict     string            `json:"verdict,omitempty"`
	Confidence  float64           `json:"confidence,omitempty"`
	Graph       *CausalGraph      `json:"graph,omitempty"`
	Attestation MerkleAttestation `json:"attestation"`
}

// SpeculativeBuildInput is the source data BuildSpeculativeBundle
// consumes. Verdicts is the cohort; Meta is optional. CohortID is a
// caller-supplied label; if empty, BuildSpeculativeBundle derives a
// stable hash of the cohort's Merkle roots so the same cohort always
// produces the same id.
type SpeculativeBuildInput struct {
	CohortID string
	Verdicts []LabeledVerdict
	Meta     *LabeledVerdict
}

// BuildSpeculativeBundle composes a SpeculativeBundle from a cohort of
// LabeledVerdicts. It runs MerkleAttest on every graph, MerkleConverge
// on the cohort, MetaReconcile if a meta is supplied, and DyadCompress
// on the cohort. The returned bundle's Peers slice mirrors the input
// order; consensus-bucket membership is mirrored from the convergence
// report onto each peer's InCohort flag.
//
// Returns nil if the cohort is empty.
func BuildSpeculativeBundle(in SpeculativeBuildInput) *SpeculativeBundle {
	if len(in.Verdicts) == 0 {
		return nil
	}

	// Step 1 — attest every cohort verdict.
	peers := make([]SpeculativePeer, 0, len(in.Verdicts))
	attestations := make([]PeerAttestation, 0, len(in.Verdicts))
	for _, lv := range in.Verdicts {
		att := MerkleAttest(lv.Verdict.CausalGraph)
		peers = append(peers, SpeculativePeer{
			AgentID:     lv.ID,
			Verdict:     lv.Verdict.Verdict,
			Confidence:  lv.Verdict.Confidence,
			Graph:       cloneCausalGraph(lv.Verdict.CausalGraph),
			Attestation: att,
		})
		attestations = append(attestations, PeerAttestation{
			AgentID:     lv.ID,
			Attestation: att,
		})
	}

	// Step 2 — convergence over cohort. Threshold left at default.
	convergence := MerkleConverge(attestations)

	// Step 3 — reconciliation against meta if supplied.
	var reconciliation *ReconciliationReport
	var meta *SpeculativeMeta
	if in.Meta != nil && in.Meta.Verdict.CausalGraph != nil {
		mAtt := MerkleAttest(in.Meta.Verdict.CausalGraph)
		peerGraphs := make([]PeerGraph, 0, len(in.Verdicts))
		for _, lv := range in.Verdicts {
			peerGraphs = append(peerGraphs, PeerGraph{
				AgentID: lv.ID,
				Graph:   lv.Verdict.CausalGraph,
			})
		}
		rep := MetaReconcile(in.Meta.Verdict.CausalGraph, peerGraphs)
		reconciliation = &rep
		meta = &SpeculativeMeta{
			AgentID:     in.Meta.ID,
			Verdict:     in.Meta.Verdict.Verdict,
			Confidence:  in.Meta.Verdict.Confidence,
			Graph:       cloneCausalGraph(in.Meta.Verdict.CausalGraph),
			Attestation: mAtt,
		}
	}

	// Step 4 — dyad compression over the cohort.
	_, dyadReport := DyadCompress(in.Verdicts)

	// Step 5 — mark consensus-bucket members on each peer.
	consensus := map[string]struct{}{}
	for _, id := range convergence.ConsensusBucketAgents() {
		consensus[id] = struct{}{}
	}
	for i := range peers {
		_, ok := consensus[peers[i].AgentID]
		peers[i].InCohort = ok
	}

	cohortID := strings.TrimSpace(in.CohortID)
	if cohortID == "" {
		cohortID = derivedCohortID(attestations)
	}

	return &SpeculativeBundle{
		ProtocolVersion: SpeculativeBundleProtocolVersion,
		CohortID:        cohortID,
		Peers:           peers,
		Meta:            meta,
		Convergence:     &convergence,
		Reconciliation:  reconciliation,
		Dyads:           &dyadReport,
	}
}

// derivedCohortID hashes the cohort's sorted Merkle roots so the same
// cohort always yields the same id. Useful when callers don't supply
// their own slug and we still want a stable cathedral URL.
func derivedCohortID(peers []PeerAttestation) string {
	if len(peers) == 0 {
		return "cohort-empty"
	}
	roots := make([]string, 0, len(peers))
	for _, p := range peers {
		roots = append(roots, p.Attestation.Root)
	}
	sort.Strings(roots)
	h := sha256.New()
	for _, r := range roots {
		_, _ = h.Write([]byte(r))
		_, _ = h.Write([]byte("\x00"))
	}
	return "cohort-" + hex.EncodeToString(h.Sum(nil)[:8])
}

// MarshalSpeculativeBundle serialises a bundle into the JSON blob
// persisted in agency_gist_traces.speculative_json. nil → "{}".
func MarshalSpeculativeBundle(b *SpeculativeBundle) (string, error) {
	if b == nil {
		return "{}", nil
	}
	buf, err := json.Marshal(b)
	if err != nil {
		return "{}", fmt.Errorf("marshal speculative bundle: %w", err)
	}
	return string(buf), nil
}

// ParseSpeculativeBundle parses a persisted speculative_json blob.
// Empty / "{}" / "null" returns (nil, nil) so callers can distinguish
// "no bundle stored" from a parse error.
func ParseSpeculativeBundle(raw string) (*SpeculativeBundle, error) {
	trimmed := strings.TrimSpace(raw)
	if trimmed == "" || trimmed == "{}" || trimmed == "null" {
		return nil, nil
	}
	var bundle SpeculativeBundle
	if err := json.Unmarshal([]byte(trimmed), &bundle); err != nil {
		return nil, fmt.Errorf("parse speculative bundle: %w", err)
	}
	if bundle.ProtocolVersion == 0 && len(bundle.Peers) == 0 &&
		bundle.Meta == nil && bundle.Convergence == nil &&
		bundle.Reconciliation == nil && bundle.Dyads == nil {
		return nil, nil
	}
	return &bundle, nil
}

// CohortAgentIDs returns the AgentIDs of the cohort in input order.
// Used by the cathedral renderer for tile layout.
func (b *SpeculativeBundle) CohortAgentIDs() []string {
	if b == nil {
		return nil
	}
	out := make([]string, 0, len(b.Peers))
	for _, p := range b.Peers {
		out = append(out, p.AgentID)
	}
	return out
}

// HeadlineStatus collapses the bundle's three signals into a single
// human label the inspector can render in a status pill. Order of
// precedence: convergence gate first (the cohort must agree before
// reconciliation matters), then reconciliation, then dyad savings.
func (b *SpeculativeBundle) HeadlineStatus() string {
	if b == nil {
		return "unknown"
	}
	if b.Convergence != nil {
		switch b.Convergence.Status {
		case ConvergenceStatusDivergent:
			return "divergent"
		case ConvergenceStatusPartial:
			return "partial"
		}
	}
	if b.Reconciliation != nil && !b.Reconciliation.IsAcceptable() {
		switch b.Reconciliation.Status {
		case ReconciliationStatusFabricated:
			return "fabricated"
		case ReconciliationStatusDrifted:
			return "drifted"
		}
	}
	return "converged"
}
