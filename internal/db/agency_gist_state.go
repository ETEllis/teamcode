package db

import (
	"context"
	"database/sql"
	"time"
)

// GetAgencyGistLattice returns the persisted lattice JSON for the given agent.
// Returns empty string (not an error) when no row exists yet.
func (q *Queries) GetAgencyGistLattice(ctx context.Context, agentID string) (string, error) {
	row := q.db.QueryRowContext(ctx,
		`SELECT lattice_json FROM agency_gist_state WHERE agent_id = ?`, agentID)
	var lattice string
	if err := row.Scan(&lattice); err != nil {
		if err == sql.ErrNoRows {
			return "{}", nil
		}
		return "{}", err
	}
	return lattice, nil
}

// UpsertAgencyGistLattice inserts or replaces the lattice JSON for the given agent.
func (q *Queries) UpsertAgencyGistLattice(ctx context.Context, agentID, latticeJSON string) error {
	if latticeJSON == "" {
		latticeJSON = "{}"
	}
	_, err := q.db.ExecContext(ctx,
		`INSERT INTO agency_gist_state (agent_id, lattice_json, updated_at)
		 VALUES (?, ?, ?)
		 ON CONFLICT(agent_id) DO UPDATE SET
		   lattice_json = excluded.lattice_json,
		   updated_at   = excluded.updated_at`,
		agentID, latticeJSON, time.Now().Unix(),
	)
	return err
}
