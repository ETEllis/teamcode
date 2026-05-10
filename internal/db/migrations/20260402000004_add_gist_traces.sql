-- +goose Up
-- +goose StatementBegin
CREATE TABLE IF NOT EXISTS agency_gist_traces (
    id TEXT PRIMARY KEY,
    office_id TEXT NOT NULL,
    agent_id TEXT NOT NULL,
    verdict TEXT NOT NULL,
    risk_level TEXT NOT NULL DEFAULT '',
    confidence REAL NOT NULL DEFAULT 0,
    trace_json TEXT NOT NULL,
    proof_json TEXT NOT NULL DEFAULT '{}',
    lattice_json TEXT NOT NULL DEFAULT '{}',
    input_hash TEXT NOT NULL DEFAULT '',
    next_lattice_hash TEXT NOT NULL DEFAULT '',
    created_at INTEGER NOT NULL
);

CREATE INDEX IF NOT EXISTS idx_agency_gist_traces_office_created
ON agency_gist_traces (office_id, created_at DESC);

CREATE INDEX IF NOT EXISTS idx_agency_gist_traces_agent_created
ON agency_gist_traces (agent_id, created_at DESC);
-- +goose StatementEnd

-- +goose Down
-- +goose StatementBegin
DROP TABLE IF EXISTS agency_gist_traces;
-- +goose StatementEnd
