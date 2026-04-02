-- name: CreateAgencySchedule :one
INSERT INTO agency_schedules (
    id,
    office_id,
    agent_id,
    name,
    timezone,
    cron_expr,
    wake_event,
    enabled,
    last_fired_at,
    next_fire_at,
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

-- name: GetAgencyScheduleByID :one
SELECT *
FROM agency_schedules
WHERE id = ?
LIMIT 1;

-- name: ListAgencySchedulesByOffice :many
SELECT *
FROM agency_schedules
WHERE office_id = ?
ORDER BY created_at ASC;

-- name: ListDueAgencySchedules :many
SELECT *
FROM agency_schedules
WHERE enabled = 1
  AND next_fire_at IS NOT NULL
  AND next_fire_at <= ?
ORDER BY next_fire_at ASC;

-- name: UpdateAgencyScheduleFireTimes :one
UPDATE agency_schedules
SET
    last_fired_at = ?,
    next_fire_at = ?,
    metadata = ?,
    updated_at = strftime('%s', 'now')
WHERE id = ?
RETURNING *;
