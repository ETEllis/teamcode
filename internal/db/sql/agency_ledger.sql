-- name: AppendAgencyLedgerEntry :one
INSERT INTO agency_ledger_entries (
    id,
    office_id,
    agent_id,
    entry_type,
    proposal_kind,
    snapshot_id,
    parent_entry_id,
    status,
    quorum_key,
    quorum_state,
    action_payload,
    observation_payload,
    commit_certificate,
    metadata,
    committed_at,
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
    ?,
    strftime('%s', 'now')
) RETURNING *;

-- name: GetAgencyLedgerEntryByID :one
SELECT *
FROM agency_ledger_entries
WHERE id = ?
LIMIT 1;

-- name: ListAgencyLedgerEntriesByOffice :many
SELECT *
FROM agency_ledger_entries
WHERE office_id = ?
ORDER BY created_at ASC;

-- name: ListPendingAgencyLedgerEntries :many
SELECT *
FROM agency_ledger_entries
WHERE office_id = ?
  AND status IN ('proposed', 'pending', 'review')
ORDER BY created_at ASC;

-- name: CommitAgencyLedgerEntry :one
UPDATE agency_ledger_entries
SET
    status = ?,
    quorum_state = ?,
    commit_certificate = ?,
    metadata = ?,
    committed_at = ? 
WHERE id = ?
RETURNING *;

-- name: RejectAgencyLedgerEntry :one
UPDATE agency_ledger_entries
SET
    status = 'rejected',
    quorum_state = ?,
    metadata = ?
WHERE id = ?
RETURNING *;
