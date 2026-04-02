-- name: CreateAgencyContextSnapshot :one
INSERT INTO agency_context_snapshots (
    id,
    office_id,
    source_entry_id,
    snapshot_kind,
    payload,
    created_at
) VALUES (
    ?,
    ?,
    ?,
    ?,
    ?,
    strftime('%s', 'now')
) RETURNING *;

-- name: GetAgencyContextSnapshotByID :one
SELECT *
FROM agency_context_snapshots
WHERE id = ?
LIMIT 1;

-- name: GetLatestAgencyContextSnapshotByOffice :one
SELECT *
FROM agency_context_snapshots
WHERE office_id = ?
ORDER BY created_at DESC
LIMIT 1;

-- name: ListAgencyContextSnapshotsByOffice :many
SELECT *
FROM agency_context_snapshots
WHERE office_id = ?
ORDER BY created_at DESC;
