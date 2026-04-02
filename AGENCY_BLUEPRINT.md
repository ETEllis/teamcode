# Agency — Master Blueprint v3
> Binary: `teamcode` · Product: **Agency** · Last updated: 2026-04-02
> Incorporates: GIST per-agent cognitive layer · Model-agnostic routing layer · Cross-platform roadmap · GPT architectural review

---

## 1. What Agency Is

Agency is a **terminal-first AI working organization**. A team of autonomous agents with distinct voices, personalities, and roles — agents you can hear, watch build, and configure to run one-off tasks or operate indefinitely on a schedule. Whether you're spinning up a software project or running a business function on autopilot, Agency is the same thing: **your intent, multiplied**.

The double meaning is intentional. *Agency* is the firm you staff. *Agency* is also the personal power you get by having it.

---

## 2. Full Architecture Stack

```
┌──────────────────────────────────────────────────────────────┐
│  USER LAYER                                                   │
│  Genesis wizard · TUI bubbles · Voice · Bulletin board        │
│  Natural language intent → structured org config              │
└───────────────────────────┬──────────────────────────────────┘
                            │
┌───────────────────────────▼──────────────────────────────────┐
│  DETERMINISTIC LAYER  (Nested Temporal Cron Tree)             │
│  Rigid scheduling · Prompt injection · Layer config           │
│  Fires enriched WakeSignals down into reactive layer          │
└───────────────────────────┬──────────────────────────────────┘
                            │ enriched WakeSignal
┌───────────────────────────▼──────────────────────────────────┐
│  REACTIVE LAYER  (Stateful Agent Runtime)                     │
│  Event bus · Actor daemons · Ledger · Consensus               │
└───────────────────────────┬──────────────────────────────────┘
                            │
┌───────────────────────────▼──────────────────────────────────┐
│  PER-AGENT COGNITIVE LAYER  (GIST Core)                       │
│  Causal compression · Elastic stretch · Coherence field       │
│  Outputs: verdict + execution_intent                          │
└───────────────────────────┬──────────────────────────────────┘
                            │ ActionIntent
┌───────────────────────────▼──────────────────────────────────┐
│  EXECUTION & MODEL ROUTING LAYER  (NEW — first class)         │
│  ReasoningCore · ModelRouter · ProviderAdapter                │
│  CredentialBroker · ExecutionPolicy                           │
│  Deterministic: local → remote_api_key → remote_oauth         │
└───────────────────────────┬──────────────────────────────────┘
                            │ Result
┌───────────────────────────▼──────────────────────────────────┐
│  LEDGER  (single source of truth)                             │
│  ledger > DB > disk · append-only · CommitCertificate         │
└──────────────────────────────────────────────────────────────┘
```

**Request chain:**
```
WakeSignal → GIST/ReasoningCore → ActionIntent → ModelRouter → ProviderAdapter → Result → Ledger
```

---

## 3. Core Concepts

### 3.1 The Office
Runtime container. Has a name, a constitution, and a set of actors. Coordinates signal routing, consensus, the ledger, voice rooms, and schedules. You boot the office; agents wake inside it.

### 3.2 The Constitution
Defines the character and governance of the office:
- `governanceMode` — `hierarchical`, `peer`, `federated`, `flat`, `hybrid`
- `roles` — map of role archetypes: mission, personality, system prompt, allowed actions, tool bindings, peer routing, spawn permissions, **GIST complexity tier**, **default trust lane**, **routing policy**
- Constitutions are **versioned**. Bumping a version triggers genesis re-plan for affected roles only — not a full rebuild.

### 3.3 Actors (Agents)
Each actor is a running daemon with:
- `AgentIdentity` — ID, name, role, org, parent, avatar prompt
- `CapabilityPack` — skills, tools, action constraints, context scopes
- `ActorRuntimeSpec` persisted on disk
- Personal Redis channel for signals
- Personal GIST instance (cognitive core)
- Personal `CredentialHandle` set (routing credentials)

### 3.4 Action Model
Typed action taxonomy — agents produce structured actions, not free-form text:

| Action | Privilege | Default trust lane |
|--------|-----------|-------------------|
| `write_code` | Medium | advisory |
| `run_test` | Low | autonomous |
| `ping_peer` | Low | autonomous |
| `update_task` | Low | autonomous |
| `request_review` | Low | autonomous |
| `broadcast` | Lowest | autonomous |
| `spawn_agent` | High | supervised |
| `publish_artifact` | Medium | advisory |
| `handoff_shift` | Medium | advisory |

### 3.5 The Ledger
Append-only event log. **Single source of truth: ledger > DB > disk.** No component caches state that conflicts with ledger sequence. Secrets are never stored in the ledger — only `CredentialHandle` references and audit metadata.

Entry lifecycle: `proposed` → `pending` → `finalized` (or `rejected`)

### 3.6 Memory Topology

| Scope | What | Persistence |
|-------|------|-------------|
| `cell` | GIST lattice state — active working memory | Session |
| `role` | Shared role-level context (all agents of same role) | Office lifetime |
| `office` | Ledger snapshots — org-wide facts | Permanent |
| `ephemeral` | Signal payload — single wake cycle | Discarded after |

### 3.7 Event Bus
- `actor:{id}` — direct signal to one actor
- `org:{id}` — broadcast to all actors
- `bulletin:{orgId}` — performance bulletin (TUI subscribes)
- `approval:{orgId}` — advisory/supervised actions pending user action

### 3.8 Consensus
- `direct` — one approval (default)
- `quorum` — N of M eligible voters

Every finalized entry gets a `CommitCertificate`. Pending votes surface in the TUI log panel.

### 3.9 Trust Lanes

| Lane | Behavior |
|------|---------|
| `autonomous` | Agent acts freely within capability pack |
| `advisory` | Agent proposes, user sees in bulletin before commit |
| `supervised` | Agent proposes, human approves before finalize |
| `locked` | No agent action on this resource |

### 3.10 Failure Semantics

| Failure | Behavior |
|---------|---------|
| Timeout | Dead-letter queue → retry after next schedule tick |
| Kernel rejection | `CorrectionSignal` → agent, logged |
| Model error | Degrade to lower-tier provider → log → continue |
| Consensus timeout | Auto-finalize if `AutoFinalize: true`, else reject |
| Elastic stretch TTL | Monitor fires forced recall, partial result committed |
| No eligible route | Escalate to user via `approval:{orgId}`, log blocked action |

---

## 4. Boot Architecture

**Overmind + local Redis** — no Docker in the hot path.

```
Procfile:
  redis     → local Redis instance
  office    → office-coordinator daemon
  runtime   → runtime-daemon
  scheduler → scheduler-daemon
  (actor daemons spawned per-agent by runtime)
```

```bash
scripts/build-daemons && overmind start
```

Key env vars (provider-agnostic — set whichever you use):
```
# Runtime
AGENCY_REDIS_ADDR=localhost:6379
AGENCY_CONSTITUTION_NAME=default

# Model routing defaults
AGENCY_DEFAULT_PRIVACY=prefer_local
AGENCY_DEFAULT_COMPLEXITY=1
AGENCY_COST_CEILING_USD=0.01

# Provider credentials (any subset — router uses what's available)
ANTHROPIC_API_KEY=...
OPENAI_API_KEY=...
GEMINI_API_KEY=...
OLLAMA_BASE_URL=http://localhost:11434

# GIST
GIST_ELASTIC_RECALL_THRESHOLD=0.3
GIST_ELASTIC_MAX_TTL_MS=30000
```

---

## 5. Per-Agent GIST Cognitive Layer

Every actor daemon runs its own **GIST instance** as its reasoning engine. GIST replaces the flat `LLMActorProposer` — it structures the reasoning *before* any model sees it.

### 5.1 What GIST Does Per Agent

1. Receives incoming atoms (signal data, observation, role context)
2. Runs a **causal compression ladder** — triadic evidence gates synthesize atoms into verdicts using Pearl causal inference
3. Produces a **structured verdict** with confidence + causal chain + open questions
4. Also produces an **`execution_intent`** — what kind of cognition is needed (task type, complexity, privacy, tools required)
5. The `execution_intent` goes to the ModelRouter. The verdict goes to the LLM as system context.

GIST decides **what kind of cognition is needed**. The router decides **where to run it**. They never mix.

### 5.2 GISTAgentCore

```go
type GISTAgentCore struct {
    agentID     string
    gistProcess *exec.Cmd       // Python subprocess (same pattern as Kokoro TTS)
    stdin       io.WriteCloser
    stdout      io.ReadCloser
    budget      ElasticBudget
}

type ElasticBudget struct {
    Tier             int     // 0=leaf, 1=coalition, 2=meta
    MaxSubAgents     int     // 0 / 2-4 / unlimited
    MaxDepth         int     // 1 / 2 / unlimited
    RecallThreshold  float64 // coherence delta triggering recall (default 0.3)
    TTLMs            int     // max stretch before forced return (30000ms)
}
```

### 5.3 GIST Verdict Output (feeds ModelRouter)

```go
type GISTVerdict struct {
    Verdict         string          // causally grounded conclusion
    Confidence      float64
    CausalChain     []string
    OpenQuestions   []string
    ExecutionIntent ActionIntent    // passed to ModelRouter
}

type ActionIntent struct {
    TaskType        string   // "chat"|"planning"|"codegen"|"critique"|"classification"
    Complexity      int      // 0-3
    LatencyBudgetMs int
    PrivacyLevel    string   // "strict_local"|"prefer_local"|"remote_ok"
    CostCeilingUsd  float64
    RequiredTools   []string
    NeedsLongCtx    bool
}
```

### 5.4 Elastic Stretch

When GIST detects high-amplitude incoherence (insufficient atoms to resolve a directive):
1. Monitor sub-agent snapshots causal state, holds lattice position
2. Agent executes real work within `ElasticBudget` (code, research, analysis)
3. Monitor tracks `coherence delta` continuously
4. When `delta > RecallThreshold` OR TTL expires → recall signal fires
5. Agent returns with enriched atoms, reintegrates to lattice
6. Compression round runs with new evidence → verdict → route → LLM

### 5.5 Complexity Tiers

| Tier | Role examples | Max sub-agents | Default trust lane |
|------|--------------|----------------|-------------------|
| 0 (leaf) | scheduler, analyst | 0 | autonomous |
| 1 (coalition) | developer, reviewer | 2–4 | advisory |
| 2 (meta) | coordinator, architect | unlimited | supervised |

---

## 6. Execution & Model Routing Layer *(NEW — first class)*

GIST produces `ActionIntent`. The routing layer deterministically selects the model and provider. No mystical chooser — the algorithm is inspectable and loggable.

### 6.1 Component Chain

```
ActionIntent
    → CredentialBroker  (what credentials are available and valid?)
    → ExecutionPolicy   (apply privacy/budget/tool hard gates)
    → ModelRouter       (score eligible routes, pick highest)
    → ProviderAdapter   (handle auth + transport for selected provider)
    → Result
```

### 6.2 Provider Classes

| Class | Examples | Auth method |
|-------|---------|------------|
| `local` | Ollama, llama.cpp, MLX, vLLM | None (local socket) |
| `remote_api_key` | Anthropic, Gemini, OpenAI API, Together, Fireworks | API key in Keychain |
| `remote_user_oauth` | Providers with officially documented OAuth for native apps | OAuth token in Keychain |
| `remote_openai_linked` | OpenAI Codex-linked accounts | Provisional — treat as provider-specific |

### 6.3 CredentialHandle

```go
type CredentialHandle struct {
    Provider    string   // "anthropic"|"openai"|"google"|"local"|"together"|...
    AuthMode    string   // "local"|"api_key"|"oauth"|"linked_account"
    AccountID   string
    Scopes      []string
    Status      string   // "active"|"missing"|"expired"|"restricted"
}
```

**Secrets storage:**
- macOS/iOS clients: Keychain
- Server-side (future cloud): encrypted vault only
- Ledger: `CredentialHandle` references + audit metadata only. Never raw secrets.

### 6.4 InferenceRequest

Every routing decision starts with a structured request (produced from `ActionIntent`):

```go
type InferenceRequest struct {
    TaskType          string
    Complexity        int      // 0-3
    LatencyBudgetMs   int
    PrivacyLevel      string   // "strict_local"|"prefer_local"|"remote_ok"
    CostCeilingUsd    float64
    RequiredTools     []string
    ProviderAllowlist []string // optional override
    ProviderDenylist  []string // optional override
    NeedsLongCtx      bool
}
```

### 6.5 Deterministic Routing Algorithm

**Hard gates first (filter, not score):**
1. Filter by required capability (does this provider/model support the task type and tools?)
2. Filter by available + active credentials
3. Filter by privacy policy (`strict_local` → only `local` class survives)
4. Filter by tool compatibility
5. Filter by cost ceiling

**Soft scoring (applied after gates):**
```
route_score =
    capability_fit_score
  - latency_penalty
  - cost_penalty
  - credential_risk_penalty
  - data_sensitivity_penalty
  + locality_bonus
  + user_preference_bonus
```

**Outcome:**
- Highest scoring eligible route wins
- If no route qualifies → degrade (lower complexity tier) or escalate (fire `approval:{orgId}` signal for user to authorize)
- Every routing decision is logged: `"routed to local/qwen-coder: privacy=strict_local, complexity=1"` — debuggable and trustworthy

### 6.6 ProviderAdapter Interface

```go
type ProviderAdapter interface {
    Name()     string
    Supports(req InferenceRequest) bool
    Execute(ctx context.Context, req InferenceRequest, cred CredentialHandle) (InferenceResult, error)
}
```

Implementations: `AnthropicAdapter`, `OpenAIAdapter`, `GeminiAdapter`, `OllamaAdapter`, `TogetherAdapter`. Adding a new provider = one new adapter, nothing else changes.

### 6.7 ExecutionPolicy

Configurable per-office and per-role:
```go
type ExecutionPolicy struct {
    DefaultPrivacy      string
    DefaultComplexity   int
    MaxCostPerWakeUsd   float64
    LocalPreference     bool     // bias routing toward local when capability parity
    AllowedProviders    []string
    DeniedProviders     []string
    RequireApprovalOver float64  // cost threshold triggering supervised lane
}
```

---

## 7. Nested Temporal Orchestration Layer *(designed, not yet built)*

### 7.1 ScheduleNode Tree

```go
type ScheduleNode struct {
    ID             string
    ParentID       string
    Depth          int               // 0=strategic, 1=tactical, 2=operational, 3=execution
    Expression     string            // cron expression
    TargetAgentIDs []string
    PromptTemplate string            // directive injected at fire time
    LayerConfig    ScheduleLayerConfig
    Children       []string
}
```

Temporal layers map 1:1 to GIST broadcast frequencies:
```
Strategic  (@monthly)   → delta frequency    "Review last month. Adjust agent behaviors."
  Tactical (@weekly)    → theta frequency    "Review open work. Reprioritize."
    Ops    (@daily)     → alpha frequency    "Morning standup. What are you working on?"
      Exec (@every 4h)  → gamma frequency    "Check signals. Act if relevant."
```

When a `ScheduleNode` fires, it enriches `WakeSignal.Payload` with `prompt_injection`, `layer_depth`, `parent_fired_at`, `topology_mode`. The actor daemon feeds this as a high-weight atom into GIST before compression.

### 7.2 Performance Bulletin

```go
type PerformanceRecord struct {
    ID           string
    AgentID      string
    ScheduleID   string
    Directive    string     // injected prompt
    ActualOutput string     // what agent produced
    Score        float64    // GIST coherence delta as proxy score
    FedBack      bool
    CreatedAt    int64
}
```

Published to `bulletin:{orgId}` → TUI bulletin panel. GIST handles internal adaptation; this exists for human visibility.

---

## 8. Cross-Platform Roadmap *(post-terminal)*

Agency is local-runtime-heavy by design. The platform expansion sequence follows that nature — don't dilute it with premature cloud-first rewrites.

### Platform Order

**1. Terminal (current)**
Full office runtime. The canonical Agency experience.

**2. macOS Desktop (next)**
SwiftUI windowed app. Natural habitat for the local-first runtime: Overmind, Redis, daemon processes, voice, live office feed, approval lanes. All existing Go runtime stays — the Mac app is a native window into it.

**3. iPad/iPhone Companion**
Not full parity — companion first. Focus:
- Approval lane actions (advisory/supervised)
- Bulletin board
- Agent status + voice/social thread
- Quick triggers and emergency stop/resume
- Lightweight office chat

SwiftUI `WindowGroup` is designed for this: shared domain/UI core, macOS-first specialization, iPad adaptation without full rewrite.

**4. Web Control Plane**
Remote dashboard, not primary runtime host. Focus:
- Org/activity viewer
- Approval lane
- Schedule editor (visual cron tree)
- Ledger/log inspector
- Multi-office admin panel

Stack: React/Next or equivalent, thin client relative to runtime.

**5. Full Mobile Parity**
Only after routing, auth, live wake loop, Mac desktop, shared transport, and web control plane are solid.

### Shared Architecture Across Platforms

```
Core runtime:        Go daemons + Redis + ledger + scheduler + GIST subprocess
Local transport:     IPC (Unix socket) on Mac/desktop
Remote transport:    WebSocket + HTTP for iPad, iPhone, web clients
Client SDK:          Single event schema across all platforms:
                       WakeSignal · ActionProposal · LedgerEntry
                       PerformanceRecord · ApprovalRequest
Credentials:         Keychain on Apple clients · Encrypted vault (future cloud)
```

**Persistent context sync** — Agency maintains one continuous office state regardless of which surface the user engages from:
- Terminal and Mac desktop share the local runtime directly (IPC)
- iPad/iPhone and web subscribe to the same Redis event bus via WebSocket
- Every LedgerEntry is the ground truth — any client can reconstruct full state by replaying the ledger
- `ContextSnapshot` is published on connect so new clients catch up instantly
- Approval actions taken on mobile immediately reflect in terminal and desktop

This is the equivalent of what Claude Code achieves with dispatch and session sync — but Agency's ledger makes it more complete: the state is not just context, it's the full causal history of the office.

---

## 9. Voice

Each actor has a distinct Kokoro TTS voice mapped by role:

| Role | Voice |
|------|-------|
| coordinator | am_adam |
| architect | bm_daniel |
| developer | am_michael |
| analyst | bf_emma |
| scheduler | af_bella |
| reviewer | bm_george |
| default | af_heart |

Prosody adapts to signal kind. Fallback: macOS `say`. Voice events are first-class ledger entries.

---

## 10. Genesis

Manufactures a `GenesisPlan` from natural language intent:
- `OrgIntent` — canonical org description
- `[]GenesisRoleBundle` — roles with system prompts, GIST tier, trust lane, routing policy, schedules
- `SocialThread` — opening voice messages from each role
- `ManufacturingSignals` — bootstrap signal sequence

Conversation-driven TUI dialog: intent → roles → GIST tier → trust lane → routing policy → review → commit.

---

## 11. TUI Layer

- **Chat pane** — iMessage-style bubbles, per-role color/indent, voice on appearance
- **Sidebar** — social thread, live office feed
- **Agency Overview** — health, actor count, ledger sequence, schedule status
- **Bulletin panel** — directive → output → coherence score timeline
- **Approval panel** — advisory/supervised actions pending user action
- **Logs page** — full ledger table with commit certificates, vote status, routing audit log
- **Dialogs** — genesis wizard, agent editor, routing policy editor, trust lane config, theme

---

## 12. Data Model

| Table | Purpose |
|-------|---------|
| `agency_schedules` | Cron schedule records |
| `agency_agents` | Actor runtime specs |
| `agency_offices` | Office state + constitution |
| `agency_ledger` | Append-only event log |
| `agency_consensus` | Quorum vote records |
| `agency_constitutions` | Versioned constitution definitions |
| `agency_signals` | Signal history |
| `agency_snapshots` | Context snapshot history |
| *(planned)* `agency_schedule_nodes` | Nested temporal tree |
| *(planned)* `agency_performance_records` | Directive/output/score per agent |
| *(planned)* `agency_gist_state` | Persisted GIST lattice weights per agent |
| *(planned)* `agency_routing_log` | Audit log of routing decisions |
| *(planned)* `agency_credential_handles` | Provider credential references (no secrets) |

---

## 13. Build Status

### ✅ Built and Verified

| Component | Location |
|-----------|----------|
| Boot: Overmind + Redis | `Procfile`, `.env`, `scripts/build-daemons` |
| Office CLI (`status`, `boot`, `genesis`, `voice *`) | `cmd/agency.go` |
| Event bus (memory + Redis) | `internal/agency/bus.go` |
| Ledger (append, snapshot, replay) | `internal/agency/ledger.go` |
| Consensus (direct + quorum) | `internal/agency/consensus.go` |
| Kernel (observation validation) | `internal/agency/kernel.go` |
| Actor daemon (signal loop, propose, ledger write) | `internal/agency/daemon_actor.go` |
| LLM proposer — current (to be superseded by GIST + router) | `internal/agency/llm_actor.go` |
| Voice platform (Kokoro map, TTS detection, prosody) | `internal/agency/voice_platform.go` |
| Voice gateway (transcript, synthesis, projection) | `internal/agency/voice.go` |
| Genesis (role bundle manufacturing) | `internal/agency/genesis.go` |
| Flat scheduler (base cron layer) | `internal/agency/scheduler.go` |
| 4 daemon binaries | `internal/agency/cmd/*/main.go` |
| DB schema + migrations | `internal/db/migrations/` |
| Config system | `internal/config/agency.go` |
| TUI: bubbles, sidebar, overview, dialogs | `internal/tui/` |
| `go build ./...` + `go test ./...` | ✅ |

---

### 🔲 Outstanding — Execution Order

#### P0 — Live agents now

| # | Item | File(s) |
|---|------|---------|
| 1 | `scripts/install-voice` — pip kokoro-onnx, download model, write `kokoro-tts.py` | `scripts/install-voice` |
| 2 | `.env` additions — model env, provider keys | `.env` |
| 3 | Live scheduler wake loop — DB poll → Redis signal fire | `cmd/scheduler-daemon/main.go` |
| 4 | Actor broadcast → TUI bubble bridge | `internal/tui/components/chat/` |

#### P1 — GIST per-agent cognitive layer

| # | Item | File(s) |
|---|------|---------|
| 5 | `GISTAgentCore` — Python subprocess manager, stdin/stdout pipe | `internal/agency/gist_core.go` (new) |
| 6 | `ElasticBudget` + `GISTVerdict` + `ActionIntent` types | `internal/agency/types.go` |
| 7 | Atom builder — `ObservationSnapshot` + `WakeSignal` → GIST atoms | `internal/agency/gist_core.go` |
| 8 | Elastic stretch handler — snapshot, work, monitor, recall, reintegrate | `internal/agency/gist_core.go` |
| 9 | Wire `GISTAgentCore` into `daemon_actor.go` | `internal/agency/daemon_actor.go` |
| 10 | `agency_gist_state` DB table — persist lattice weights | `internal/db/migrations/` |

#### P2 — Model routing layer

| # | Item | File(s) |
|---|------|---------|
| 11 | `InferenceRequest` + `CredentialHandle` + `ActionIntent` types | `internal/agency/types.go` |
| 12 | `CredentialBroker` — load/validate handles, Keychain integration | `internal/agency/routing.go` (new) |
| 13 | `ExecutionPolicy` + `ModelRouter` — hard gates + soft scoring | `internal/agency/routing.go` |
| 14 | `ProviderAdapter` interface + `OllamaAdapter`, `AnthropicAdapter`, `OpenAIAdapter`, `GeminiAdapter` | `internal/agency/providers/` (new) |
| 15 | Wire router into `daemon_actor.go` — replace direct Anthropic call | `internal/agency/daemon_actor.go` |
| 16 | `agency_routing_log` DB table + `agency_credential_handles` table | `internal/db/migrations/` |
| 17 | Routing audit log visible in TUI logs page | `internal/tui/components/logs/` |

#### P3 — Core UX

| # | Item | File(s) |
|---|------|---------|
| 18 | TUI iMessage bubbles — per-role color, avatar, timestamp | `internal/tui/components/chat/message.go` |
| 19 | Voice playback in TUI — Kokoro synthesis on actor broadcast | `internal/agency/voice.go` + TUI |
| 20 | Approval panel — advisory/supervised actions pending user action | `internal/tui/components/chat/` (new) |
| 21 | Consensus / CommitCertificate in log panel | `internal/tui/components/logs/` |

#### P4 — Nested Temporal Orchestration

| # | Item | File(s) |
|---|------|---------|
| 22 | `ScheduleNode` type + tree | `internal/agency/types.go` |
| 23 | `NestedScheduler` — wraps flat scheduler, tree management, prompt injection | `internal/agency/nested_scheduler.go` (new) |
| 24 | Prompt injection → GIST atom in `daemon_actor.go` | `internal/agency/daemon_actor.go` |
| 25 | DB migration — `agency_schedule_nodes` | `internal/db/migrations/` |
| 26 | `PerformanceRecord` + `agency_performance_records` table | `internal/agency/types.go` + DB |
| 27 | Bulletin board pub + TUI bulletin panel | `internal/agency/performance.go` + TUI |

#### P5 — Cross-platform

| # | Item | What |
|---|------|------|
| 28 | IPC transport layer | Unix socket server exposing ledger events + approval actions to local clients |
| 29 | WebSocket transport layer | Remote client event stream — same schema as IPC |
| 30 | macOS SwiftUI app | Native window into the office: bubbles, bulletin, voice, approvals |
| 31 | iPad/iPhone companion | Approvals, bulletin, agent status, quick triggers |
| 32 | Web control plane | Dashboard, schedule editor, ledger viewer, multi-office admin |

#### P6 — Polish

| Item | What |
|------|------|
| Versioned constitution migrations | Bump version without full re-genesis |
| Adversarial reviewer agent | Challenges proposals before finalization |
| Agent spawning | Tier-2 actors create child actors at runtime |
| Timezone schedule windows | Honor business hours config |
| Replay harness | Simulate what agents would do given any historical ledger window |
| Multi-office federation | Multiple offices coordinating across Redis channels |

---

## 14. Where to Start Next Session

```bash
# Verify clean
go build ./... && go test ./...

# P0.1 — voice install script
# Write scripts/install-voice

# P0.3 — live scheduler wake loop
# Edit internal/agency/cmd/scheduler-daemon/main.go

# P1 — GISTAgentCore
# New file: internal/agency/gist_core.go
# Edit: internal/agency/types.go (ElasticBudget, GISTVerdict, ActionIntent)
# Edit: internal/agency/daemon_actor.go (wire GISTAgentCore)

# P2 — Model routing
# New file: internal/agency/routing.go
# New dir:  internal/agency/providers/
# Edit: internal/agency/daemon_actor.go (replace direct Anthropic call with router)
```

---

## 15. File Reference

```
teamcode/
├── AGENCY_BLUEPRINT.md              ← this file (v3 — final)
├── progress.md                      ← session log
├── Procfile / .env / .teamcode.json
├── .planning/config.json            ← disables context monitor hook for this project
├── scripts/
│   ├── build-daemons
│   └── install-voice                ← (TODO P0)
├── internal/agency/
│   ├── types.go                     ← all domain types
│   ├── bus.go                       ← EventBus
│   ├── ledger.go                    ← append-only log
│   ├── consensus.go                 ← quorum
│   ├── kernel.go                    ← observation validation
│   ├── daemon_actor.go              ← actor main loop (to be updated P1+P2)
│   ├── llm_actor.go                 ← current proposer (superseded by P1+P2)
│   ├── gist_core.go                 ← (TODO P1) GISTAgentCore
│   ├── routing.go                   ← (TODO P2) ModelRouter + CredentialBroker
│   ├── providers/                   ← (TODO P2) ProviderAdapter implementations
│   │   ├── ollama.go
│   │   ├── anthropic.go
│   │   ├── openai.go
│   │   └── gemini.go
│   ├── performance.go               ← (TODO P4) bulletin board
│   ├── nested_scheduler.go          ← (TODO P4) temporal hierarchy
│   ├── voice.go / voice_platform.go
│   ├── scheduler.go                 ← flat cron base
│   ├── genesis.go
│   └── cmd/                         ← 4 daemon binaries
├── internal/config/agency.go
├── internal/db/                     ← migrations + sqlc
└── internal/tui/
    ├── components/chat/
    │   ├── message.go               ← (TODO P3) per-role bubble styling
    │   ├── bulletin.go              ← (TODO P4) performance panel
    │   └── approval.go              ← (TODO P3) advisory/supervised lane UI
    ├── components/dialog/
    └── components/logs/             ← (TODO P3) consensus + routing audit
```
