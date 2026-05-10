package db

import (
	"context"
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
}
