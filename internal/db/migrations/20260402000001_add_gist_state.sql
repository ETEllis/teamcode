-- +goose Up
-- +goose StatementBegin
CREATE TABLE IF NOT EXISTS agency_gist_state (
    agent_id TEXT PRIMARY KEY,
    lattice_json TEXT NOT NULL DEFAULT '{}',
    updated_at INTEGER NOT NULL
);
-- +goose StatementEnd

-- +goose Down
-- +goose StatementBegin
DROP TABLE IF EXISTS agency_gist_state;
-- +goose StatementEnd
