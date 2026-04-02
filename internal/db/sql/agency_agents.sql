-- name: CreateAgencyAgent :one
INSERT INTO agency_agents (
    id,
    office_id,
    constitution_id,
    parent_agent_id,
    identity_json,
    role_name,
    runtime_status,
    lifecycle_phase,
    workspace_path,
    inbox_channel,
    last_snapshot_id,
    last_wake_signal_id,
    capabilities_json,
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
    ?,
    ?,
    strftime('%s', 'now'),
    strftime('%s', 'now')
) RETURNING *;

-- name: GetAgencyAgentByID :one
SELECT *
FROM agency_agents
WHERE id = ?
LIMIT 1;

-- name: ListAgencyAgentsByOffice :many
SELECT *
FROM agency_agents
WHERE office_id = ?
ORDER BY created_at ASC;

-- name: ListAgencyChildAgents :many
SELECT *
FROM agency_agents
WHERE parent_agent_id = ?
ORDER BY created_at ASC;

-- name: UpdateAgencyAgentRuntime :one
UPDATE agency_agents
SET
    runtime_status = ?,
    lifecycle_phase = ?,
    last_snapshot_id = ?,
    last_wake_signal_id = ?,
    metadata = ?,
    updated_at = strftime('%s', 'now')
WHERE id = ?
RETURNING *;
