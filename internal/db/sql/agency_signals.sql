-- name: CreateAgencyWakeSignal :one
INSERT INTO agency_wake_signals (
    id,
    office_id,
    agent_id,
    schedule_id,
    signal_type,
    channel,
    payload,
    status,
    available_at,
    delivered_at,
    acknowledged_at,
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
    strftime('%s', 'now')
) RETURNING *;

-- name: GetAgencyWakeSignalByID :one
SELECT *
FROM agency_wake_signals
WHERE id = ?
LIMIT 1;

-- name: ListPendingAgencyWakeSignalsByOffice :many
SELECT *
FROM agency_wake_signals
WHERE office_id = ?
  AND status = 'pending'
  AND available_at <= ?
ORDER BY available_at ASC, created_at ASC;

-- name: ListPendingAgencyWakeSignalsByAgent :many
SELECT *
FROM agency_wake_signals
WHERE agent_id = ?
  AND status = 'pending'
  AND available_at <= ?
ORDER BY available_at ASC, created_at ASC;

-- name: MarkAgencyWakeSignalDelivered :one
UPDATE agency_wake_signals
SET
    status = 'delivered',
    delivered_at = ?,
    acknowledged_at = acknowledged_at
WHERE id = ?
RETURNING *;

-- name: AcknowledgeAgencyWakeSignal :one
UPDATE agency_wake_signals
SET
    status = 'acknowledged',
    acknowledged_at = ?,
    delivered_at = COALESCE(delivered_at, ?)
WHERE id = ?
RETURNING *;
