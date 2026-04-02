-- +goose Up
-- +goose StatementBegin
CREATE TABLE IF NOT EXISTS agency_routing_log (
    id TEXT PRIMARY KEY,
    agent_id TEXT NOT NULL,
    org_id TEXT NOT NULL,
    provider TEXT NOT NULL,
    model_id TEXT NOT NULL,
    execution_intent TEXT NOT NULL DEFAULT '',
    latency_ms INTEGER NOT NULL DEFAULT 0,
    tokens_used INTEGER NOT NULL DEFAULT 0,
    gate_reason TEXT NOT NULL DEFAULT '',
    created_at INTEGER NOT NULL
);

CREATE INDEX IF NOT EXISTS idx_agency_routing_log_agent_id ON agency_routing_log (agent_id);
CREATE INDEX IF NOT EXISTS idx_agency_routing_log_created_at ON agency_routing_log (created_at);

CREATE TABLE IF NOT EXISTS agency_credential_handles (
    provider TEXT PRIMARY KEY,
    key_ref TEXT NOT NULL,
    status TEXT NOT NULL DEFAULT 'missing',
    model_id TEXT NOT NULL DEFAULT '',
    updated_at INTEGER NOT NULL
);
-- +goose StatementEnd

-- +goose Down
-- +goose StatementBegin
DROP TABLE IF EXISTS agency_credential_handles;
DROP TABLE IF EXISTS agency_routing_log;
-- +goose StatementEnd
