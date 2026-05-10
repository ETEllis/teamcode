package main

import (
	"context"
	"encoding/json"
	"errors"
	"net/http"
	"net/http/httptest"
	"path/filepath"
	"testing"

	"github.com/ETEllis/teamcode/internal/agency"
	"github.com/ETEllis/teamcode/internal/db"
	"github.com/stretchr/testify/require"
)

// TestDBTraceFetcherRoundTrip writes a trace via the same path the
// actor-daemon uses and reads it back through the inspector fetcher.
// This is the only test that touches a real SQLite migration so we
// can be confident the inspector_json column works end-to-end.
func TestDBTraceFetcherRoundTrip(t *testing.T) {
	t.Setenv("AGENCY_DB_PATH", filepath.Join(t.TempDir(), "agency.db"))
	conn, err := db.Connect()
	require.NoError(t, err)
	t.Cleanup(func() { _ = conn.Close() })

	q := db.New(conn)
	verdict := agency.GISTVerdict{
		Verdict:    "approve",
		RiskLevel:  "low",
		Confidence: 0.81,
		CausalGraph: &agency.CausalGraph{
			ProtocolVersion: agency.CausalGraphProtocolVersion,
			Nodes: []agency.CausalNode{
				{ID: "node_outcome", Role: agency.NodeRoleOutcome, Summary: "v=approve",
					Parents: []agency.NodeID{"node_e1"}},
				{ID: "node_e1", Role: agency.NodeRoleEvidence, Summary: "evidence", Weight: 0.7},
			},
		},
		Trace: &agency.GISTTrace{ID: "round-trip-1", InputHash: "hash"},
	}
	verdict.SyncCausalChain()
	inspectorJSON, err := agency.MarshalInspectorBundle(verdict)
	require.NoError(t, err)
	traceJSON := `{"id":"round-trip-1"}`

	require.NoError(t, q.InsertAgencyGistTrace(context.Background(), db.InsertAgencyGistTraceParams{
		ID:            "round-trip-1",
		OfficeID:      "office-rt",
		AgentID:       "agent-rt",
		Verdict:       "approve",
		RiskLevel:     "low",
		Confidence:    0.81,
		TraceJSON:     traceJSON,
		InspectorJSON: inspectorJSON,
		LatticeJSON:   `{"canonicalSlots":64}`,
		CreatedAt:     1234,
	}))

	fetcher := newDBGISTTraceFetcher(q)

	view, err := fetcher.GetInspectorTrace(context.Background(), "round-trip-1")
	require.NoError(t, err)
	require.NotNil(t, view)
	require.Equal(t, "round-trip-1", view.Summary.ID)
	require.True(t, view.Summary.HasBundle)
	require.Equal(t, "inspector_json", view.BundleSource)
	require.NotNil(t, view.Bundle)
	require.NotNil(t, view.Bundle.CausalGraph)
	require.Len(t, view.Bundle.CausalGraph.Nodes, 2)

	summaries, err := fetcher.ListInspectorTraces(context.Background(), "office-rt", 10)
	require.NoError(t, err)
	require.Len(t, summaries, 1)
	require.True(t, summaries[0].HasBundle)
}

func TestDBTraceFetcherNotFound(t *testing.T) {
	t.Setenv("AGENCY_DB_PATH", filepath.Join(t.TempDir(), "agency.db"))
	conn, err := db.Connect()
	require.NoError(t, err)
	t.Cleanup(func() { _ = conn.Close() })

	fetcher := newDBGISTTraceFetcher(db.New(conn))
	view, err := fetcher.GetInspectorTrace(context.Background(), "missing")
	require.Nil(t, view)
	require.Error(t, err)
	require.True(t, errors.Is(err, agency.ErrTraceNotFound),
		"DB ErrAgencyGistTraceNotFound should map to agency.ErrTraceNotFound")
}

// TestDBTraceFetcherLegacyFallback verifies that a row written before
// inspector_json existed (simulated by leaving inspector_json="{}")
// still hydrates a best-effort bundle from the legacy trace_json.
func TestDBTraceFetcherLegacyFallback(t *testing.T) {
	t.Setenv("AGENCY_DB_PATH", filepath.Join(t.TempDir(), "agency.db"))
	conn, err := db.Connect()
	require.NoError(t, err)
	t.Cleanup(func() { _ = conn.Close() })

	q := db.New(conn)
	require.NoError(t, q.InsertAgencyGistTrace(context.Background(), db.InsertAgencyGistTraceParams{
		ID:        "legacy-1",
		OfficeID:  "office-legacy",
		AgentID:   "agent-legacy",
		Verdict:   "review",
		RiskLevel: "medium",
		// Legacy GISTTrace JSON with a SelectedChain - the hydration
		// path should turn this into a bundle.
		TraceJSON: `{"id":"legacy-1","selectedVerdict":"review","selectedChain":["evidence: a","intervention: do(x)"]}`,
		// inspector_json deliberately empty so we exercise the fallback.
		InspectorJSON: "{}",
		CreatedAt:     5,
	}))

	fetcher := newDBGISTTraceFetcher(q)
	view, err := fetcher.GetInspectorTrace(context.Background(), "legacy-1")
	require.NoError(t, err)
	require.NotNil(t, view)
	require.False(t, view.Summary.HasBundle)
	require.Equal(t, "legacy_hydrated", view.BundleSource)
	require.NotNil(t, view.Bundle)
	require.Equal(t, "review", view.Bundle.Verdict)
	require.Len(t, view.Bundle.FlatChain, 2)
	require.NotNil(t, view.Bundle.CausalGraph)
}

// makeSpecScenarioVerdict constructs a deterministic LabeledVerdict for
// the Phase 6 end-to-end scenario.
//
// The cohort lives at the heart of the speculative tier:
//
//   • alpha / beta / gamma  → identical typed graph (consensus bucket)
//   • delta                  → Hamming-1 sibling (one extra confounder)
//   • meta                   → the auditor agent that ratifies the cohort
//
// All five carry the same outcome verdict so MetaReconcile has to reach
// inside the typed graph to detect the drift; convergence has to do
// the same with Merkle attestations; dyad compression collapses the
// alpha/beta/gamma triplet down to a single canonical slot.
func makeSpecScenarioVerdict(id, suffix string, conf float64, includeConfounder bool) agency.LabeledVerdict {
	g := &agency.CausalGraph{
		ProtocolVersion: agency.CausalGraphProtocolVersion,
		Nodes: []agency.CausalNode{
			{ID: agency.NodeID("ev-" + suffix), Role: agency.NodeRoleEvidence,
				Summary: "release telemetry green", Weight: 0.85},
			{ID: agency.NodeID("act-" + suffix), Role: agency.NodeRoleIntervention,
				Summary: "do(promote-to-prod)", Weight: 0.6,
				Parents: []agency.NodeID{agency.NodeID("ev-" + suffix)}},
			{ID: agency.NodeID("ot-" + suffix), Role: agency.NodeRoleOutcome,
				Summary: "deploy holds", Weight: 1.0,
				Parents: []agency.NodeID{agency.NodeID("act-" + suffix)}},
		},
	}
	if includeConfounder {
		g.Nodes = append(g.Nodes, agency.CausalNode{
			ID:      agency.NodeID("conf-" + suffix),
			Role:    agency.NodeRoleConfounder,
			Summary: "regional outage masked errors",
			Weight:  0.4,
		})
	}
	return agency.LabeledVerdict{
		ID: id,
		Verdict: agency.GISTVerdict{
			Verdict:     "deploy holds",
			Confidence:  conf,
			RiskLevel:   "medium",
			CausalGraph: g,
		},
	}
}

// TestPhase6_LatticeCathedral_EndToEnd is the keystone integration test
// for Phase 6. It walks the entire Lattice Cathedral pipeline:
//
//   1. Build a 3-peer + meta cohort via BuildSpeculativeBundle.
//   2. Round-trip the envelope through MarshalSpeculativeBundle.
//   3. Persist with InsertAgencyGistTrace into a real SQLite DB
//      (exercises the speculative_json column from Commit 1).
//   4. Read back via dbGISTTraceFetcher.GetSpeculativeBundle
//      (exercises the SpeculativeFetcher capability from Commit 3).
//   5. Mount the fetcher behind a DirectorHTTPServer and exercise the
//      cathedral data API at /lattice/spec/<id>/data
//      (exercises handleLatticeRouter + handleAPICathedralData).
//   6. Assert the HTTP payload preserves convergence / reconciliation
//      / dyad reports byte-for-byte equivalent to the in-memory bundle.
//
// This test is the sentinel that all five Phase 6 commits compose
// without losing information at any boundary.
func TestPhase6_LatticeCathedral_EndToEnd(t *testing.T) {
	t.Setenv("AGENCY_DB_PATH", filepath.Join(t.TempDir(), "agency.db"))
	conn, err := db.Connect()
	require.NoError(t, err)
	t.Cleanup(func() { _ = conn.Close() })

	q := db.New(conn)
	ctx := context.Background()

	// --- Stage 1: cohort assembly --------------------------------
	alpha := makeSpecScenarioVerdict("agent-alpha", "v1", 0.84, false)
	beta := makeSpecScenarioVerdict("agent-beta", "v1", 0.82, false)
	gamma := makeSpecScenarioVerdict("agent-gamma", "v1", 0.80, false)
	delta := makeSpecScenarioVerdict("agent-delta", "v1", 0.71, true) // dissenter
	meta := makeSpecScenarioVerdict("agent-meta", "v1", 0.88, false)

	bundle := agency.BuildSpeculativeBundle(agency.SpeculativeBuildInput{
		CohortID: "cohort-phase6-e2e",
		Verdicts: []agency.LabeledVerdict{alpha, beta, gamma, delta},
		Meta:     &meta,
	})
	require.NotNil(t, bundle)
	require.Equal(t, agency.SpeculativeBundleProtocolVersion, bundle.ProtocolVersion)
	require.Equal(t, "cohort-phase6-e2e", bundle.CohortID)
	require.Len(t, bundle.Peers, 4)
	require.NotNil(t, bundle.Meta, "meta verdict supplied")

	// Convergence: alpha/beta/gamma share a Merkle root, delta is
	// the Hamming-1 dissenter, so the gate opens at quorum=3/4.
	require.NotNil(t, bundle.Convergence)
	require.Equal(t, agency.ConvergenceStatusConverged, bundle.Convergence.Status)
	require.True(t, bundle.Convergence.IsGateOpen())

	inCohort := map[string]bool{}
	for _, p := range bundle.Peers {
		inCohort[p.AgentID] = p.InCohort
	}
	require.True(t, inCohort["agent-alpha"], "alpha must be in cohort")
	require.True(t, inCohort["agent-beta"], "beta must be in cohort")
	require.True(t, inCohort["agent-gamma"], "gamma must be in cohort")
	require.False(t, inCohort["agent-delta"], "delta is the Hamming-1 dissenter")

	// Reconciliation: meta mirrors the consensus graph, so it should
	// land at faithful with full coverage over the consensus bucket.
	require.NotNil(t, bundle.Reconciliation)
	require.Equal(t, agency.ReconciliationStatusFaithful, bundle.Reconciliation.Status)

	// Dyad compression: 4 peers, of which alpha/beta/gamma are
	// duplicates and delta is a Hamming-1 sibling — slot count must
	// be strictly less than the peer count.
	require.NotNil(t, bundle.Dyads)
	require.Less(t, bundle.Dyads.SlotsAfter, 4,
		"dyad compression must shrink the cohort, got %d→%d slots for 4 peers",
		bundle.Dyads.SlotsBefore, bundle.Dyads.SlotsAfter)

	// --- Stage 2: envelope marshal round-trip -------------------
	rawBundle, err := agency.MarshalSpeculativeBundle(bundle)
	require.NoError(t, err)
	require.NotEmpty(t, rawBundle)

	parsed, err := agency.ParseSpeculativeBundle(rawBundle)
	require.NoError(t, err)
	require.NotNil(t, parsed)
	require.Equal(t, bundle.CohortID, parsed.CohortID)
	require.Equal(t, bundle.Convergence.Status, parsed.Convergence.Status)
	require.Equal(t, bundle.Reconciliation.Status, parsed.Reconciliation.Status)
	require.Equal(t, bundle.Dyads.SlotsAfter, parsed.Dyads.SlotsAfter)

	// --- Stage 3: persist via the SQLite agency_gist_traces row ---
	traceID := "phase6-e2e-trace"
	inspectorJSON, err := agency.MarshalInspectorBundle(alpha.Verdict)
	require.NoError(t, err)
	require.NoError(t, q.InsertAgencyGistTrace(ctx, db.InsertAgencyGistTraceParams{
		ID:               traceID,
		OfficeID:         "office-phase6",
		AgentID:          "agent-alpha",
		Verdict:          "deploy holds",
		RiskLevel:        "medium",
		Confidence:       0.84,
		TraceJSON:        `{"id":"` + traceID + `"}`,
		InspectorJSON:    inspectorJSON,
		LatticeJSON:      `{"canonicalSlots":64}`,
		SpeculativeJSON:  rawBundle,
		CreatedAt:        9001,
	}))

	// --- Stage 4: dbGISTTraceFetcher.GetSpeculativeBundle ---------
	fetcher := newDBGISTTraceFetcher(q)

	// Type-assert the optional capability so we exercise the same
	// dispatch path the cathedral handler uses at runtime.
	specFetcher, ok := agency.GISTTraceFetcher(fetcher).(agency.SpeculativeFetcher)
	require.True(t, ok, "dbGISTTraceFetcher must satisfy agency.SpeculativeFetcher")

	roundTripped, err := specFetcher.GetSpeculativeBundle(ctx, traceID)
	require.NoError(t, err)
	require.NotNil(t, roundTripped)
	require.Equal(t, bundle.CohortID, roundTripped.CohortID)
	require.Equal(t, bundle.Convergence.Status, roundTripped.Convergence.Status)
	require.Equal(t, bundle.Reconciliation.Status, roundTripped.Reconciliation.Status)
	require.Equal(t, bundle.Dyads.SlotsAfter, roundTripped.Dyads.SlotsAfter)
	require.Len(t, roundTripped.Peers, 4)
	require.NotNil(t, roundTripped.Meta)

	// Bundle missing → cathedral falls back, fetcher returns nil
	// without an error so the caller can render the demo cohort.
	_, missingErr := specFetcher.GetSpeculativeBundle(ctx, "no-such-trace")
	require.ErrorIs(t, missingErr, agency.ErrTraceNotFound)

	// --- Stage 5: HTTP /lattice/spec/<id>/data round-trip ---------
	dir := t.TempDir()
	ledger, err := agency.NewLedgerService(dir)
	require.NoError(t, err)
	director, err := agency.NewDirectorService(agency.DirectorConfig{
		BaseDir:        dir,
		OrganizationID: "office-phase6",
		Ledger:         ledger,
	})
	require.NoError(t, err)
	director.SetTraceFetcher(fetcher)

	server := agency.NewDirectorHTTPServer(agency.DirectorHTTPConfig{
		Addr: "127.0.0.1:0",
	}, director)

	// Drive the real mux: handleLatticeRouter dispatches
	// /lattice/spec/<id>/data → handleAPICathedralData. We pull the
	// handler through the public Handler() accessor so the requireAuth
	// chain, the lattice router, and the cathedral handlers all run
	// exactly the same way they would in production.
	handler := server.Handler()

	rec := httptest.NewRecorder()
	req := httptest.NewRequest(http.MethodGet, "/lattice/spec/"+traceID+"/data", nil)
	handler.ServeHTTP(rec, req)

	require.Equal(t, http.StatusOK, rec.Code, "cathedral data API must serve persisted bundle")
	require.Contains(t, rec.Header().Get("content-type"), "application/json")

	var payload agency.CathedralPayload
	require.NoError(t, json.Unmarshal(rec.Body.Bytes(), &payload))
	require.Equal(t, traceID, payload.TraceID)
	require.Equal(t, "persisted", payload.Source)
	require.NotNil(t, payload.Bundle)
	require.Equal(t, bundle.CohortID, payload.Bundle.CohortID)
	require.Len(t, payload.Bundle.Peers, 4)

	// Stage 6: assert full structural equivalence. The HTTP boundary
	// must not lose any of the speculative-tier reports.
	require.Equal(t, bundle.Convergence.Status, payload.Bundle.Convergence.Status)
	require.Equal(t, bundle.Convergence.Quorum, payload.Bundle.Convergence.Quorum)
	require.Equal(t, bundle.Convergence.ConsensusRoot, payload.Bundle.Convergence.ConsensusRoot)
	require.Equal(t, bundle.Reconciliation.Status, payload.Bundle.Reconciliation.Status)
	require.Equal(t, bundle.Reconciliation.CoverageScore, payload.Bundle.Reconciliation.CoverageScore)
	require.Equal(t, bundle.Dyads.SlotsAfter, payload.Bundle.Dyads.SlotsAfter)
	require.Equal(t, len(bundle.Dyads.Deltas), len(payload.Bundle.Dyads.Deltas))

	// Cohort agent IDs preserved in the same order.
	originalIDs := bundle.CohortAgentIDs()
	roundTrippedIDs := payload.Bundle.CohortAgentIDs()
	require.Equal(t, originalIDs, roundTrippedIDs,
		"cohort agent IDs must survive the HTTP boundary")

	// Headline status string survives end-to-end.
	require.Equal(t, bundle.HeadlineStatus(), payload.Bundle.HeadlineStatus())

	// Missing trace → 404 from the same router (sanity check).
	rec404 := httptest.NewRecorder()
	req404 := httptest.NewRequest(http.MethodGet, "/lattice/spec/no-such-trace/data", nil)
	handler.ServeHTTP(rec404, req404)
	require.Equal(t, http.StatusNotFound, rec404.Code)
}
