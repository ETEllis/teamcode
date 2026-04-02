-- name: CreateAgencyOffice :one
INSERT INTO agency_offices (
    id,
    name,
    slug,
    mode,
    status,
    bus_driver,
    consensus_mode,
    workspace_path,
    shared_volume_path,
    redis_addr,
    ledger_driver,
    metadata,
    updated_at,
    created_at
) VALUES (
    ?,
    ?,
    ?,
    ?,
    ?,
    ?,
    ?,
    ?,
    ?,
    ?,
    ?,
    ?,
    strftime('%s', 'now'),
    strftime('%s', 'now')
) RETURNING *;

-- name: GetAgencyOfficeByID :one
SELECT *
FROM agency_offices
WHERE id = ?
LIMIT 1;

-- name: GetAgencyOfficeBySlug :one
SELECT *
FROM agency_offices
WHERE slug = ?
LIMIT 1;

-- name: ListAgencyOffices :many
SELECT *
FROM agency_offices
ORDER BY created_at DESC;

-- name: ListAgencyOfficesByStatus :many
SELECT *
FROM agency_offices
WHERE status = ?
ORDER BY updated_at DESC;

-- name: UpdateAgencyOfficeStatus :one
UPDATE agency_offices
SET
    status = ?,
    metadata = ?,
    updated_at = strftime('%s', 'now')
WHERE id = ?
RETURNING *;
