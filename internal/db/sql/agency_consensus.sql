-- name: CreateAgencyConsensusVote :one
INSERT INTO agency_consensus_votes (
    id,
    office_id,
    ledger_entry_id,
    agent_id,
    quorum_key,
    decision,
    rationale,
    weight,
    metadata,
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
    strftime('%s', 'now')
) RETURNING *;

-- name: GetAgencyConsensusVoteByID :one
SELECT *
FROM agency_consensus_votes
WHERE id = ?
LIMIT 1;

-- name: ListAgencyConsensusVotesByLedgerEntry :many
SELECT *
FROM agency_consensus_votes
WHERE ledger_entry_id = ?
ORDER BY created_at ASC;

-- name: ListAgencyConsensusVotesByQuorum :many
SELECT *
FROM agency_consensus_votes
WHERE office_id = ?
  AND quorum_key = ?
ORDER BY created_at ASC;
