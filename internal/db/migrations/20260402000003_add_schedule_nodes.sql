-- +goose Up
-- +goose StatementBegin
CREATE TABLE IF NOT EXISTS agency_schedule_nodes (
    id TEXT PRIMARY KEY,
    organization_id TEXT NOT NULL,
    actor_id TEXT NOT NULL,
    parent_id TEXT,
    expression TEXT NOT NULL,
    prompt_injection TEXT NOT NULL DEFAULT '',
    layer INTEGER NOT NULL DEFAULT 0,
    enabled INTEGER NOT NULL DEFAULT 1,
    metadata TEXT,
    created_at INTEGER NOT NULL,
    updated_at INTEGER NOT NULL
);

CREATE INDEX IF NOT EXISTS idx_agency_schedule_nodes_org    ON agency_schedule_nodes(organization_id);
CREATE INDEX IF NOT EXISTS idx_agency_schedule_nodes_actor  ON agency_schedule_nodes(actor_id);
CREATE INDEX IF NOT EXISTS idx_agency_schedule_nodes_parent ON agency_schedule_nodes(parent_id);
-- +goose StatementEnd

-- +goose Down
-- +goose StatementBegin
DROP TABLE IF EXISTS agency_schedule_nodes;
-- +goose StatementEnd
