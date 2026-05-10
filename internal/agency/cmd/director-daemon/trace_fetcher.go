package main

import (
	"context"
	"errors"

	"github.com/ETEllis/teamcode/internal/agency"
	"github.com/ETEllis/teamcode/internal/db"
)

// dbGISTTraceFetcher implements agency.GISTTraceFetcher backed by the
// SQLite agency_gist_traces table. It is the read-only counterpart to
// the actor-daemon's dbGISTTraceStore: that side writes
// inspector_json, this side reads it for the lattice inspector route.
type dbGISTTraceFetcher struct {
	q *db.Queries
}

func newDBGISTTraceFetcher(q *db.Queries) *dbGISTTraceFetcher {
	return &dbGISTTraceFetcher{q: q}
}

// GetInspectorTrace returns a hydrated InspectorTraceView for the given
// trace id. It prefers the persisted inspector_json envelope, but falls
// back to a best-effort hydration from trace_json for traces that
// pre-date Phase 3 storage.
func (f *dbGISTTraceFetcher) GetInspectorTrace(ctx context.Context, id string) (*agency.InspectorTraceView, error) {
	row, err := f.q.GetAgencyGistTrace(ctx, id)
	if err != nil {
		if errors.Is(err, db.ErrAgencyGistTraceNotFound) {
			return nil, agency.ErrTraceNotFound
		}
		return nil, err
	}

	view := &agency.InspectorTraceView{
		Summary:       agency.InspectorTraceSummary{
			ID:         row.ID,
			OfficeID:   row.OfficeID,
			AgentID:    row.AgentID,
			Verdict:    row.Verdict,
			RiskLevel:  row.RiskLevel,
			Confidence: row.Confidence,
			CreatedAt:  row.CreatedAt,
		},
		LatticeJSON:   row.LatticeJSON,
		TraceJSON:     row.TraceJSON,
		ProofJSON:     row.ProofJSON,
		InspectorJSON: row.InspectorJSON,
	}

	bundle, parseErr := agency.ParseInspectorBundle(row.InspectorJSON)
	switch {
	case parseErr != nil:
		// Persisted JSON is malformed - report it but keep going so
		// the rest of the trace still renders.
		view.BundleSource = "inspector_json:parse_error:" + parseErr.Error()
	case bundle != nil:
		view.Bundle = bundle
		view.Summary.HasBundle = true
		view.BundleSource = "inspector_json"
	default:
		// No (or empty) inspector envelope. Fall back to best-effort
		// hydration from the legacy trace blob.
		legacy := agency.HydrateInspectorBundleFromLegacy(row.TraceJSON)
		if legacy != nil {
			view.Bundle = legacy
			view.Summary.HasBundle = false
			view.BundleSource = "legacy_hydrated"
		} else {
			view.BundleSource = "none"
		}
	}
	return view, nil
}

// ListInspectorTraces returns the most-recent N trace summaries for the
// given office. HasBundle is set so the index page can show which rows
// have rich data vs. which fall back to legacy hydration.
func (f *dbGISTTraceFetcher) ListInspectorTraces(ctx context.Context, officeID string, limit int) ([]agency.InspectorTraceSummary, error) {
	rows, err := f.q.ListAgencyGistTracesByOffice(ctx, officeID, limit)
	if err != nil {
		return nil, err
	}
	out := make([]agency.InspectorTraceSummary, 0, len(rows))
	for _, row := range rows {
		hasBundle, _ := agency.ParseInspectorBundle(row.InspectorJSON)
		out = append(out, agency.InspectorTraceSummary{
			ID:         row.ID,
			OfficeID:   row.OfficeID,
			AgentID:    row.AgentID,
			Verdict:    row.Verdict,
			RiskLevel:  row.RiskLevel,
			Confidence: row.Confidence,
			CreatedAt:  row.CreatedAt,
			HasBundle:  hasBundle != nil,
		})
	}
	return out, nil
}
