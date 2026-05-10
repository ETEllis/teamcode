package db

import (
	"context"
	"errors"
	"path/filepath"
	"testing"

	"github.com/stretchr/testify/require"
)

func TestAgencyGistTracePersistence(t *testing.T) {
	t.Setenv("AGENCY_DB_PATH", filepath.Join(t.TempDir(), "agency.db"))
	conn, err := Connect()
	require.NoError(t, err)
	t.Cleanup(func() { _ = conn.Close() })

	q := New(conn)
	err = q.InsertAgencyGistTrace(context.Background(), InsertAgencyGistTraceParams{
		ID:              "gist-trace-1",
		OfficeID:        "org-1",
		AgentID:         "agent-1",
		Verdict:         "causal_review_required",
		RiskLevel:       "high",
		Confidence:      0.48,
		TraceJSON:       `{"id":"gist-trace-1"}`,
		ProofJSON:       `{"traceId":"gist-trace-1"}`,
		LatticeJSON:     `{"canonicalSlots":64}`,
		InspectorJSON:   `{"protocolVersion":1,"verdict":"causal_review_required"}`,
		InputHash:       "input",
		NextLatticeHash: "next",
		CreatedAt:       123,
	})
	require.NoError(t, err)

	traces, err := q.ListAgencyGistTracesByOffice(context.Background(), "org-1", 10)
	require.NoError(t, err)
	require.Len(t, traces, 1)
	require.Equal(t, "gist-trace-1", traces[0].ID)
	require.Equal(t, "causal_review_required", traces[0].Verdict)
	require.Equal(t, "high", traces[0].RiskLevel)
	require.Equal(t, `{"traceId":"gist-trace-1"}`, traces[0].ProofJSON)
	require.Equal(t,
		`{"protocolVersion":1,"verdict":"causal_review_required"}`,
		traces[0].InspectorJSON)
}

func TestGetAgencyGistTraceByID(t *testing.T) {
	t.Setenv("AGENCY_DB_PATH", filepath.Join(t.TempDir(), "agency.db"))
	conn, err := Connect()
	require.NoError(t, err)
	t.Cleanup(func() { _ = conn.Close() })

	q := New(conn)
	require.NoError(t, q.InsertAgencyGistTrace(context.Background(), InsertAgencyGistTraceParams{
		ID:            "trace-A",
		OfficeID:      "org-A",
		AgentID:       "agent-A",
		Verdict:       "approved",
		RiskLevel:     "low",
		Confidence:    0.91,
		TraceJSON:     `{"id":"trace-A"}`,
		InspectorJSON: `{"protocolVersion":1}`,
		CreatedAt:     1,
	}))

	got, err := q.GetAgencyGistTrace(context.Background(), "trace-A")
	require.NoError(t, err)
	require.Equal(t, "trace-A", got.ID)
	require.Equal(t, "approved", got.Verdict)
	require.Equal(t, `{"protocolVersion":1}`, got.InspectorJSON)

	_, err = q.GetAgencyGistTrace(context.Background(), "missing-trace")
	require.Error(t, err)
	require.True(t, errors.Is(err, ErrAgencyGistTraceNotFound),
		"expected ErrAgencyGistTraceNotFound, got %v", err)
}

func TestAgencyGistTraceUpsertPreservesInspectorJSON(t *testing.T) {
	t.Setenv("AGENCY_DB_PATH", filepath.Join(t.TempDir(), "agency.db"))
	conn, err := Connect()
	require.NoError(t, err)
	t.Cleanup(func() { _ = conn.Close() })

	q := New(conn)
	ctx := context.Background()
	require.NoError(t, q.InsertAgencyGistTrace(ctx, InsertAgencyGistTraceParams{
		ID:            "trace-upsert",
		OfficeID:      "org-X",
		AgentID:       "agent-X",
		Verdict:       "draft",
		TraceJSON:     `{"v":1}`,
		InspectorJSON: `{"protocolVersion":1,"v":1}`,
		CreatedAt:     10,
	}))
	require.NoError(t, q.InsertAgencyGistTrace(ctx, InsertAgencyGistTraceParams{
		ID:            "trace-upsert",
		OfficeID:      "org-X",
		AgentID:       "agent-X",
		Verdict:       "final",
		TraceJSON:     `{"v":2}`,
		InspectorJSON: `{"protocolVersion":1,"v":2}`,
		CreatedAt:     20,
	}))
	got, err := q.GetAgencyGistTrace(ctx, "trace-upsert")
	require.NoError(t, err)
	require.Equal(t, "final", got.Verdict)
	require.Equal(t, `{"protocolVersion":1,"v":2}`, got.InspectorJSON)
	require.Equal(t, int64(20), got.CreatedAt)
}

func TestAgencyGistTracePersistsSpeculativeJSON(t *testing.T) {
	t.Setenv("AGENCY_DB_PATH", filepath.Join(t.TempDir(), "agency.db"))
	conn, err := Connect()
	require.NoError(t, err)
	t.Cleanup(func() { _ = conn.Close() })

	q := New(conn)
	ctx := context.Background()

	// Insert with explicit speculative_json: round-trips on Get + List.
	specJSON := `{"protocolVersion":1,"cohortId":"cohort-1","convergence":{"status":"converged"}}`
	require.NoError(t, q.InsertAgencyGistTrace(ctx, InsertAgencyGistTraceParams{
		ID:              "spec-trace-1",
		OfficeID:        "org-S",
		AgentID:         "meta",
		Verdict:         "review",
		TraceJSON:       `{"id":"spec-trace-1"}`,
		InspectorJSON:   `{"protocolVersion":1}`,
		SpeculativeJSON: specJSON,
		CreatedAt:       42,
	}))

	got, err := q.GetAgencyGistTrace(ctx, "spec-trace-1")
	require.NoError(t, err)
	require.Equal(t, specJSON, got.SpeculativeJSON)

	rows, err := q.ListAgencyGistTracesByOffice(ctx, "org-S", 10)
	require.NoError(t, err)
	require.Len(t, rows, 1)
	require.Equal(t, specJSON, rows[0].SpeculativeJSON)
}

func TestAgencyGistTraceDefaultsSpeculativeJSON(t *testing.T) {
	t.Setenv("AGENCY_DB_PATH", filepath.Join(t.TempDir(), "agency.db"))
	conn, err := Connect()
	require.NoError(t, err)
	t.Cleanup(func() { _ = conn.Close() })

	q := New(conn)
	ctx := context.Background()

	// Empty SpeculativeJSON in params defaults to "{}" (NOT NULL satisfied).
	require.NoError(t, q.InsertAgencyGistTrace(ctx, InsertAgencyGistTraceParams{
		ID:        "spec-trace-default",
		OfficeID:  "org-S",
		AgentID:   "agent-S",
		Verdict:   "approved",
		TraceJSON: `{"id":"spec-trace-default"}`,
		CreatedAt: 7,
	}))
	got, err := q.GetAgencyGistTrace(ctx, "spec-trace-default")
	require.NoError(t, err)
	require.Equal(t, "{}", got.SpeculativeJSON)
}
