-- +goose Up
-- +goose StatementBegin
CREATE TABLE IF NOT EXISTS agency_offices (
    id TEXT PRIMARY KEY,
    name TEXT NOT NULL,
    slug TEXT NOT NULL UNIQUE,
    mode TEXT NOT NULL,
    status TEXT NOT NULL,
    bus_driver TEXT NOT NULL DEFAULT 'redis',
    consensus_mode TEXT NOT NULL DEFAULT 'quorum',
    workspace_path TEXT NOT NULL,
    shared_volume_path TEXT NOT NULL,
    redis_addr TEXT NOT NULL DEFAULT '',
    ledger_driver TEXT NOT NULL DEFAULT 'sqlite',
    metadata TEXT NOT NULL DEFAULT '{}',
    updated_at INTEGER NOT NULL,
    created_at INTEGER NOT NULL
);

CREATE INDEX IF NOT EXISTS idx_agency_offices_slug ON agency_offices (slug);

CREATE TABLE IF NOT EXISTS agency_constitutions (
    id TEXT PRIMARY KEY,
    office_id TEXT NOT NULL,
    name TEXT NOT NULL,
    kind TEXT NOT NULL,
    is_active BOOLEAN NOT NULL DEFAULT FALSE,
    org_intent TEXT NOT NULL DEFAULT '{}',
    governance TEXT NOT NULL DEFAULT '{}',
    role_specs TEXT NOT NULL DEFAULT '[]',
    capability_packs TEXT NOT NULL DEFAULT '[]',
    schedule_policy TEXT NOT NULL DEFAULT '{}',
    metadata TEXT NOT NULL DEFAULT '{}',
    updated_at INTEGER NOT NULL,
    created_at INTEGER NOT NULL,
    FOREIGN KEY (office_id) REFERENCES agency_offices (id) ON DELETE CASCADE
);

CREATE INDEX IF NOT EXISTS idx_agency_constitutions_office_id ON agency_constitutions (office_id);
CREATE INDEX IF NOT EXISTS idx_agency_constitutions_office_active ON agency_constitutions (office_id, is_active);

CREATE TABLE IF NOT EXISTS agency_agents (
    id TEXT PRIMARY KEY,
    office_id TEXT NOT NULL,
    constitution_id TEXT,
    parent_agent_id TEXT,
    identity_json TEXT NOT NULL DEFAULT '{}',
    role_name TEXT NOT NULL,
    runtime_status TEXT NOT NULL,
    lifecycle_phase TEXT NOT NULL DEFAULT 'waiting',
    workspace_path TEXT NOT NULL,
    inbox_channel TEXT NOT NULL DEFAULT '',
    last_snapshot_id TEXT,
    last_wake_signal_id TEXT,
    capabilities_json TEXT NOT NULL DEFAULT '{}',
    metadata TEXT NOT NULL DEFAULT '{}',
    updated_at INTEGER NOT NULL,
    created_at INTEGER NOT NULL,
    FOREIGN KEY (office_id) REFERENCES agency_offices (id) ON DELETE CASCADE,
    FOREIGN KEY (constitution_id) REFERENCES agency_constitutions (id) ON DELETE SET NULL,
    FOREIGN KEY (parent_agent_id) REFERENCES agency_agents (id) ON DELETE SET NULL
);

CREATE INDEX IF NOT EXISTS idx_agency_agents_office_id ON agency_agents (office_id);
CREATE INDEX IF NOT EXISTS idx_agency_agents_parent_agent_id ON agency_agents (parent_agent_id);
CREATE INDEX IF NOT EXISTS idx_agency_agents_role_name ON agency_agents (role_name);
CREATE INDEX IF NOT EXISTS idx_agency_agents_runtime_status ON agency_agents (runtime_status);

CREATE TABLE IF NOT EXISTS agency_schedules (
    id TEXT PRIMARY KEY,
    office_id TEXT NOT NULL,
    agent_id TEXT,
    name TEXT NOT NULL,
    timezone TEXT NOT NULL,
    cron_expr TEXT NOT NULL,
    wake_event TEXT NOT NULL,
    enabled BOOLEAN NOT NULL DEFAULT TRUE,
    last_fired_at INTEGER,
    next_fire_at INTEGER,
    metadata TEXT NOT NULL DEFAULT '{}',
    updated_at INTEGER NOT NULL,
    created_at INTEGER NOT NULL,
    FOREIGN KEY (office_id) REFERENCES agency_offices (id) ON DELETE CASCADE,
    FOREIGN KEY (agent_id) REFERENCES agency_agents (id) ON DELETE CASCADE
);

CREATE INDEX IF NOT EXISTS idx_agency_schedules_office_id ON agency_schedules (office_id);
CREATE INDEX IF NOT EXISTS idx_agency_schedules_agent_id ON agency_schedules (agent_id);
CREATE INDEX IF NOT EXISTS idx_agency_schedules_due ON agency_schedules (enabled, next_fire_at);

CREATE TABLE IF NOT EXISTS agency_ledger_entries (
    id TEXT PRIMARY KEY,
    office_id TEXT NOT NULL,
    agent_id TEXT,
    entry_type TEXT NOT NULL,
    proposal_kind TEXT NOT NULL,
    snapshot_id TEXT,
    parent_entry_id TEXT,
    status TEXT NOT NULL,
    quorum_key TEXT NOT NULL DEFAULT '',
    quorum_state TEXT NOT NULL DEFAULT '{}',
    action_payload TEXT NOT NULL DEFAULT '{}',
    observation_payload TEXT NOT NULL DEFAULT '{}',
    commit_certificate TEXT NOT NULL DEFAULT '{}',
    metadata TEXT NOT NULL DEFAULT '{}',
    committed_at INTEGER,
    created_at INTEGER NOT NULL,
    FOREIGN KEY (office_id) REFERENCES agency_offices (id) ON DELETE CASCADE,
    FOREIGN KEY (agent_id) REFERENCES agency_agents (id) ON DELETE SET NULL,
    FOREIGN KEY (parent_entry_id) REFERENCES agency_ledger_entries (id) ON DELETE SET NULL
);

CREATE INDEX IF NOT EXISTS idx_agency_ledger_entries_office_id ON agency_ledger_entries (office_id);
CREATE INDEX IF NOT EXISTS idx_agency_ledger_entries_agent_id ON agency_ledger_entries (agent_id);
CREATE INDEX IF NOT EXISTS idx_agency_ledger_entries_status ON agency_ledger_entries (status);
CREATE INDEX IF NOT EXISTS idx_agency_ledger_entries_created_at ON agency_ledger_entries (created_at);

CREATE TABLE IF NOT EXISTS agency_consensus_votes (
    id TEXT PRIMARY KEY,
    office_id TEXT NOT NULL,
    ledger_entry_id TEXT NOT NULL,
    agent_id TEXT NOT NULL,
    quorum_key TEXT NOT NULL DEFAULT '',
    decision TEXT NOT NULL,
    rationale TEXT NOT NULL DEFAULT '',
    weight INTEGER NOT NULL DEFAULT 1,
    metadata TEXT NOT NULL DEFAULT '{}',
    created_at INTEGER NOT NULL,
    FOREIGN KEY (office_id) REFERENCES agency_offices (id) ON DELETE CASCADE,
    FOREIGN KEY (ledger_entry_id) REFERENCES agency_ledger_entries (id) ON DELETE CASCADE,
    FOREIGN KEY (agent_id) REFERENCES agency_agents (id) ON DELETE CASCADE,
    UNIQUE (ledger_entry_id, agent_id)
);

CREATE INDEX IF NOT EXISTS idx_agency_consensus_votes_office_id ON agency_consensus_votes (office_id);
CREATE INDEX IF NOT EXISTS idx_agency_consensus_votes_ledger_entry_id ON agency_consensus_votes (ledger_entry_id);
CREATE INDEX IF NOT EXISTS idx_agency_consensus_votes_quorum_key ON agency_consensus_votes (quorum_key);

CREATE TABLE IF NOT EXISTS agency_context_snapshots (
    id TEXT PRIMARY KEY,
    office_id TEXT NOT NULL,
    source_entry_id TEXT,
    snapshot_kind TEXT NOT NULL,
    payload TEXT NOT NULL DEFAULT '{}',
    created_at INTEGER NOT NULL,
    FOREIGN KEY (office_id) REFERENCES agency_offices (id) ON DELETE CASCADE,
    FOREIGN KEY (source_entry_id) REFERENCES agency_ledger_entries (id) ON DELETE SET NULL
);

CREATE INDEX IF NOT EXISTS idx_agency_context_snapshots_office_id ON agency_context_snapshots (office_id);
CREATE INDEX IF NOT EXISTS idx_agency_context_snapshots_created_at ON agency_context_snapshots (created_at);

CREATE TABLE IF NOT EXISTS agency_wake_signals (
    id TEXT PRIMARY KEY,
    office_id TEXT NOT NULL,
    agent_id TEXT,
    schedule_id TEXT,
    signal_type TEXT NOT NULL,
    channel TEXT NOT NULL DEFAULT '',
    payload TEXT NOT NULL DEFAULT '{}',
    status TEXT NOT NULL DEFAULT 'pending',
    available_at INTEGER NOT NULL,
    delivered_at INTEGER,
    acknowledged_at INTEGER,
    created_at INTEGER NOT NULL,
    FOREIGN KEY (office_id) REFERENCES agency_offices (id) ON DELETE CASCADE,
    FOREIGN KEY (agent_id) REFERENCES agency_agents (id) ON DELETE CASCADE,
    FOREIGN KEY (schedule_id) REFERENCES agency_schedules (id) ON DELETE SET NULL
);

CREATE INDEX IF NOT EXISTS idx_agency_wake_signals_office_id ON agency_wake_signals (office_id);
CREATE INDEX IF NOT EXISTS idx_agency_wake_signals_agent_id ON agency_wake_signals (agent_id);
CREATE INDEX IF NOT EXISTS idx_agency_wake_signals_schedule_id ON agency_wake_signals (schedule_id);
CREATE INDEX IF NOT EXISTS idx_agency_wake_signals_delivery ON agency_wake_signals (status, available_at);
-- +goose StatementEnd

-- +goose Down
-- +goose StatementBegin
DROP TABLE IF EXISTS agency_wake_signals;
DROP TABLE IF EXISTS agency_context_snapshots;
DROP TABLE IF EXISTS agency_consensus_votes;
DROP TABLE IF EXISTS agency_ledger_entries;
DROP TABLE IF EXISTS agency_schedules;
DROP TABLE IF EXISTS agency_agents;
DROP TABLE IF EXISTS agency_constitutions;
DROP TABLE IF EXISTS agency_offices;
-- +goose StatementEnd
