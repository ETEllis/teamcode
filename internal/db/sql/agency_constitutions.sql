-- name: CreateAgencyConstitution :one
INSERT INTO agency_constitutions (
    id,
    office_id,
    name,
    kind,
    is_active,
    org_intent,
    governance,
    role_specs,
    capability_packs,
    schedule_policy,
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
    strftime('%s', 'now'),
    strftime('%s', 'now')
) RETURNING *;

-- name: GetAgencyConstitutionByID :one
SELECT *
FROM agency_constitutions
WHERE id = ?
LIMIT 1;

-- name: GetActiveAgencyConstitutionByOffice :one
SELECT *
FROM agency_constitutions
WHERE office_id = ? AND is_active = 1
ORDER BY updated_at DESC
LIMIT 1;

-- name: ListAgencyConstitutionsByOffice :many
SELECT *
FROM agency_constitutions
WHERE office_id = ?
ORDER BY created_at DESC;

-- name: DeactivateAgencyConstitutionsByOffice :exec
UPDATE agency_constitutions
SET
    is_active = 0,
    updated_at = strftime('%s', 'now')
WHERE office_id = ?;

-- name: ActivateAgencyConstitution :one
UPDATE agency_constitutions
SET
    is_active = 1,
    updated_at = strftime('%s', 'now')
WHERE id = ?
RETURNING *;
