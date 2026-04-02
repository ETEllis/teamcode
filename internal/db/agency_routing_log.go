package db

import (
	"context"
	"time"
)

// InsertRoutingLogParams holds a single routing decision to persist.
type InsertRoutingLogParams struct {
	ID              string
	AgentID         string
	OrgID           string
	Provider        string
	ModelID         string
	ExecutionIntent string
	LatencyMs       int64
	TokensUsed      int
	GateReason      string
}

// InsertAgencyRoutingLog appends a routing decision to the log table.
func (q *Queries) InsertAgencyRoutingLog(ctx context.Context, p InsertRoutingLogParams) error {
	_, err := q.db.ExecContext(ctx,
		`INSERT INTO agency_routing_log
		 (id, agent_id, org_id, provider, model_id, execution_intent, latency_ms, tokens_used, gate_reason, created_at)
		 VALUES (?, ?, ?, ?, ?, ?, ?, ?, ?, ?)`,
		p.ID, p.AgentID, p.OrgID, p.Provider, p.ModelID,
		p.ExecutionIntent, p.LatencyMs, p.TokensUsed, p.GateReason,
		time.Now().Unix(),
	)
	return err
}
