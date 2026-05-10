package main

import (
	"context"
	"errors"
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
