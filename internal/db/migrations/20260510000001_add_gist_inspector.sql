-- +goose Up
-- +goose StatementBegin
ALTER TABLE agency_gist_traces
    ADD COLUMN inspector_json TEXT NOT NULL DEFAULT '{}';
-- +goose StatementEnd

-- +goose Down
-- +goose StatementBegin
-- SQLite < 3.35 cannot DROP COLUMN. To roll back, recreate the table
-- without the inspector_json column. We deliberately keep this no-op
-- because rolling back would lose data; downgrade the binary instead.
SELECT 1;
-- +goose StatementEnd
