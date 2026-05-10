package db

import (
	"context"
	"database/sql"
	"errors"
	"time"
)

type InsertAgencyGistTraceParams struct {
	ID               string
	OfficeID         string
	AgentID          string
	Verdict          string
	RiskLevel        string
	Confidence       float64
	TraceJSON        string
	ProofJSON        string
	LatticeJSON      string
	InspectorJSON    string
	SpeculativeJSON  string
	InputHash        string
	NextLatticeHash  string
	CreatedAt        int64
}

type AgencyGistTrace struct {
	ID              string  `json:"id"`
	OfficeID        string  `json:"office_id"`
	AgentID         string  `json:"agent_id"`
	Verdict         string  `json:"verdict"`
	RiskLevel       string  `json:"risk_level"`
	Confidence      float64 `json:"confidence"`
	TraceJSON       string  `json:"trace_json"`
	ProofJSON       string  `json:"proof_json"`
	LatticeJSON     string  `json:"lattice_json"`
	InspectorJSON   string  `json:"inspector_json"`
	SpeculativeJSON string  `json:"speculative_json"`
	InputHash       string  `json:"input_hash"`
	NextLatticeHash string  `json:"next_lattice_hash"`
	CreatedAt       int64   `json:"created_at"`
}

// ErrAgencyGistTraceNotFound is returned by GetAgencyGistTrace when no row
// matches the supplied id. Callers may check via errors.Is.
var ErrAgencyGistTraceNotFound = errors.New("agency gist trace not found")

func (q *Queries) InsertAgencyGistTrace(ctx context.Context, arg InsertAgencyGistTraceParams) error {
	if arg.CreatedAt == 0 {
		arg.CreatedAt = time.Now().UnixMilli()
	}
	if arg.ProofJSON == "" {
		arg.ProofJSON = "{}"
	}
	if arg.LatticeJSON == "" {
		arg.LatticeJSON = "{}"
	}
	if arg.InspectorJSON == "" {
		arg.InspectorJSON = "{}"
	}
	if arg.SpeculativeJSON == "" {
		arg.SpeculativeJSON = "{}"
	}
	_, err := q.db.ExecContext(ctx,
		`INSERT INTO agency_gist_traces (
		   id, office_id, agent_id, verdict, risk_level, confidence, trace_json,
		   proof_json, lattice_json, inspector_json, speculative_json,
		   input_hash, next_lattice_hash, created_at
		 ) VALUES (?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?)
		 ON CONFLICT(id) DO UPDATE SET
		   verdict = excluded.verdict,
		   risk_level = excluded.risk_level,
		   confidence = excluded.confidence,
		   trace_json = excluded.trace_json,
		   proof_json = excluded.proof_json,
		   lattice_json = excluded.lattice_json,
		   inspector_json = excluded.inspector_json,
		   speculative_json = excluded.speculative_json,
		   input_hash = excluded.input_hash,
		   next_lattice_hash = excluded.next_lattice_hash,
		   created_at = excluded.created_at`,
		arg.ID,
		arg.OfficeID,
		arg.AgentID,
		arg.Verdict,
		arg.RiskLevel,
		arg.Confidence,
		arg.TraceJSON,
		arg.ProofJSON,
		arg.LatticeJSON,
		arg.InspectorJSON,
		arg.SpeculativeJSON,
		arg.InputHash,
		arg.NextLatticeHash,
		arg.CreatedAt,
	)
	return err
}

// GetAgencyGistTrace returns the single trace row matching id, or
// ErrAgencyGistTraceNotFound if no row exists.
func (q *Queries) GetAgencyGistTrace(ctx context.Context, id string) (AgencyGistTrace, error) {
	row := q.db.QueryRowContext(ctx,
		`SELECT id, office_id, agent_id, verdict, risk_level, confidence, trace_json,
		        proof_json, lattice_json, inspector_json, speculative_json,
		        input_hash, next_lattice_hash, created_at
		   FROM agency_gist_traces
		  WHERE id = ?`,
		id,
	)
	var item AgencyGistTrace
	err := row.Scan(
		&item.ID,
		&item.OfficeID,
		&item.AgentID,
		&item.Verdict,
		&item.RiskLevel,
		&item.Confidence,
		&item.TraceJSON,
		&item.ProofJSON,
		&item.LatticeJSON,
		&item.InspectorJSON,
		&item.SpeculativeJSON,
		&item.InputHash,
		&item.NextLatticeHash,
		&item.CreatedAt,
	)
	if errors.Is(err, sql.ErrNoRows) {
		return AgencyGistTrace{}, ErrAgencyGistTraceNotFound
	}
	if err != nil {
		return AgencyGistTrace{}, err
	}
	return item, nil
}

func (q *Queries) ListAgencyGistTracesByOffice(ctx context.Context, officeID string, limit int) ([]AgencyGistTrace, error) {
	if limit <= 0 {
		limit = 25
	}
	rows, err := q.db.QueryContext(ctx,
		`SELECT id, office_id, agent_id, verdict, risk_level, confidence, trace_json,
		        proof_json, lattice_json, inspector_json, speculative_json,
		        input_hash, next_lattice_hash, created_at
		   FROM agency_gist_traces
		  WHERE office_id = ?
		  ORDER BY created_at DESC
		  LIMIT ?`,
		officeID,
		limit,
	)
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	out := []AgencyGistTrace{}
	for rows.Next() {
		var item AgencyGistTrace
		if err := rows.Scan(
			&item.ID,
			&item.OfficeID,
			&item.AgentID,
			&item.Verdict,
			&item.RiskLevel,
			&item.Confidence,
			&item.TraceJSON,
			&item.ProofJSON,
			&item.LatticeJSON,
			&item.InspectorJSON,
			&item.SpeculativeJSON,
			&item.InputHash,
			&item.NextLatticeHash,
			&item.CreatedAt,
		); err != nil {
			return nil, err
		}
		out = append(out, item)
	}
	return out, rows.Err()
}
