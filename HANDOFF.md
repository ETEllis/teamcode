# Agency — Session Handoff
> Written at ~90% context. Start a new Claude Code session, paste this file, say "go".

---

## What This Project Is

**Agency** — terminal-first AI working organization. Binary: `teamcode`. Product name: Agency (drop "The"). Your intent, multiplied. Each agent has a voice, an iMessage-style bubble, a distinct personality, and runs indefinitely on a schedule or one-off. Full architecture is locked in `AGENCY_BLUEPRINT.md v3`.

---

## Repository

```
/Users/edwardellis/teamcode/
```

Git repo, builds clean:
```bash
go build ./...   # ✅
go test ./...    # ✅
```

Boot (do NOT use Docker — disabled):
```bash
scripts/build-daemons && overmind start
```

---

## Architecture (locked — do not redesign)

Full spec: `/Users/edwardellis/teamcode/AGENCY_BLUEPRINT.md`

Stack top to bottom:
1. **User Layer** — TUI (Bubbletea), iMessage bubbles, voice, bulletin board
2. **Deterministic Layer** — Nested temporal cron tree, prompt injection into signals
3. **Reactive Layer** — Event bus (Redis), actor daemons, ledger, consensus
4. **GIST Cognitive Layer** — Per-agent GIST Python subprocess, causal compression, elastic stretch
5. **Model Routing Layer** — ModelRouter, ProviderAdapter, CredentialBroker, deterministic hard-gate scoring
6. **Ledger** — Append-only, single source of truth

Request chain:
```
WakeSignal → GISTAgentCore → ActionIntent → ModelRouter → ProviderAdapter → Result → Ledger
```

---

## What's Built (all verified ✅)

### Infrastructure
- Overmind boot (Procfile, .env, scripts/build-daemons)
- Office CLI (status, boot, genesis, voice)
- Event bus — memory + Redis (`internal/agency/bus.go`)
- Ledger + consensus + kernel (`internal/agency/ledger.go`, `consensus.go`, `kernel.go`)
- DB schema + migrations (`internal/db/migrations/`)
- Config system (`internal/config/agency.go`)
- `.planning/config.json` — disables context monitor hook (keeps tool calls fast)

### Stage 1 — Live Agent Foundation ✅
- **Voice install**: `scripts/install-voice` (kokoro-onnx), `scripts/kokoro-tts.py`
- **Env vars**: `.env` has `AGENCY_LLM_MODEL`, `AGENCY_LLM_MAX_TOKENS`, `GIST_ELASTIC_RECALL_THRESHOLD`, `GIST_ELASTIC_MAX_TTL_MS`
- **DB poll scheduler**: `internal/agency/cmd/scheduler-daemon/main.go` — `runDBPollScheduler` goroutine polls `agency_schedules` every 5s, fires `WakeSignal`s, updates `next_fire_at`
- **Broadcast → TUI bubble pipeline**:
  - `daemon_actor.go` publishes `ActionBroadcast` proposals to org channel as `SignalBroadcast`
  - `app.AgencyService.SubscribeBroadcasts()` — Redis subscriber for org channel
  - `chat.AgencyBroadcastMsg` tea message type
  - `messagesCmp.broadcasts []app.BroadcastMsg` — renders as labeled bubbles in viewport
  - `chatPage.Init()` subscribes + chains `waitForBroadcast` cmd

### Stage 2 — GIST Cognitive Layer ✅
- **Types**: `ElasticBudget`, `GISTVerdict`, `ActionIntent` in `types.go`
- **Migration**: `20260402000001_add_gist_state.sql` — `agency_gist_state(agent_id, lattice_json, updated_at)`
- **`gist_core.go`**: `GISTAgentCore` — `BuildAtoms`, `Compress` (Python subprocess via stdin JSON → stdout JSON), `ElasticStretch`, `SetLattice/LatticeJSON`. Degrades to `proceed_with_caution` (confidence 0.1) if subprocess unavailable
- **`LatticeStore` interface** — `GetLattice`, `SetLattice`
- **DB**: `internal/db/agency_gist_state.go` — `GetAgencyGistLattice`, `UpsertAgencyGistLattice` on `*Queries`
- **`actor-daemon/main.go`**: `dbLatticeStore` wired; DB unavailable → graceful degrade
- **`daemon_actor.go` wiring**: per-wake load lattice → `BuildAtoms` → `Compress` → `ElasticStretch` → persist lattice → `verdict.Verdict` as LLM system prefix → `executionIntent` on proposal payload

### Stage 4 — Core TUI Experience ✅
- **iMessage bubbles**: `renderBroadcastBubble()` in `list.go` — role color from deterministic actor hash, 2-char avatar initials, timestamp. Uses `theme.Primary/Secondary/Accent/Success/Warning/Info` palette via `roleColorIndex`.
- **Voice TTS**: `chatPage.playTTS()` in `page/chat.go` — fires on every `AgencyBroadcastMsg`, calls `agency.PlatformTTSCommand(config.WorkingDirectory())`, runs in goroutine (fire-and-forget). Voice ID from `agency.VoiceIDForRole(actorID)`.
- **Approval channel**: `ApprovalChannel(orgID)` added to `runtime.go` = `"agency.approval." + orgID`. `daemon_actor.go` now publishes every proposal (not just broadcasts) to this channel as `SignalReview`.
- **`approval.go`**: `ApprovalCmp` — subscribes to approval channel, lists pending proposals, `a`=approve / `r`=reject / `↑↓`=navigate. Sends vote via `AgencyService.SendApprovalVote`. Panel auto-shows in right rail when proposals arrive, auto-clears when queue is empty.
- **`app/agency.go`**: `ProposalMsg`, `SubscribeApprovals`, `SendApprovalVote` added.

### Stage 3 — Model Routing Layer ✅
- **Types**: `CredentialHandle`, `InferenceRequest`, `InferenceResult`, `ExecutionPolicy` in `types.go`
- **Migration**: `20260402000002_add_routing_log.sql` — `agency_routing_log`, `agency_credential_handles`
- **`routing.go`**: `ProviderAdapter` interface, `CredentialBroker` (probes env for all 4 providers), `ModelRouter.Route()` — 5 hard gates (capability → auth → privacy → tools → budget) + soft scoring (local-first, latency alignment, provider preference order)
- **Provider adapters** (all in `internal/agency/` package):
  - `provider_anthropic.go` — Anthropic Messages API, token tracking
  - `provider_ollama.go` — Ollama `/api/chat`, `Available()` pings `/api/tags`
  - `provider_openai.go` — OpenAI `/v1/chat/completions`
  - `provider_gemini.go` — Google Generative Language API
  - `adapters.go` — `defaultProviderAdapters()` (Ollama first)
- **`RoutingLog` interface** + `ActorDaemonConfig.RoutingLog`, `ExecutionPolicy`
- **DB**: `internal/db/agency_routing_log.go` — `InsertAgencyRoutingLog`
- **`actor-daemon/main.go`**: `dbRoutingLog` wired
- **`daemon_actor.go` wiring**: `ExecutionIntent` → `ActionIntent` → `InferenceRequest` → `router.Route()` → `adapter.Execute()` → proposal. Graceful degrade to "actor ready" if routing fails. Logs every decision.

---

## Execution Plan (approved)

Full plan: `/Users/edwardellis/.claude/plans/compiled-snuggling-boole.md`

5 stages, sequential baton passes. **Stages 1–4 complete. Start at Stage 5.**

---

### STAGE 4 — P3: Core TUI Experience
**Goal:** Office looks and feels alive. Per-role bubbles, voice playback, approval lane, consensus display.

#### Track A: iMessage bubbles + voice
- `internal/tui/components/chat/message.go` — role→color map, avatar label, timestamp
  - Currently: broadcast bubbles in `list.go` render as simple `labelStyle + bubbleStyle`
  - Needed: proper iMessage-style bubble (role color from role name, avatar initials, timestamp from `CreatedAt`)
- Wire voice: on `AgencyBroadcastMsg` → `PlatformTTSCommand` → play audio subprocess in background

#### Track B: Approval panel + logs
- New: `internal/tui/components/chat/approval.go`
  - Subscribe to `approval:{orgId}` channel on init
  - List pending `ActionProposal` items
  - `a` key → approve, `r` key → reject
  - Sends approval signal back via bus
- `internal/tui/components/logs/` — add routing audit tab + CommitCertificate display

---

### STAGE 5 — P4: Nested Temporal Orchestration + Performance
**Goal:** Schedule tree with prompt injection. Bulletin board showing directive→output→score.

#### Track A: NestedScheduler + DB
- `internal/agency/types.go` — add `ScheduleNode`, `ScheduleLayerConfig`
- New migration: `agency_schedule_nodes`
- New: `internal/agency/nested_scheduler.go` — wraps `Scheduler`, enriches WakeSignal payload with `prompt_injection` key

#### Track B: Performance tracker + bulletin
- `internal/agency/performance.go` — `PerformanceRecord`, publish to `bulletin:{orgId}`
- `internal/tui/components/chat/bulletin.go` — directive→output→score timeline

#### After A+B:
- `daemon_actor.go` — read `signal.Payload["prompt_injection"]` → high-weight GIST atom

---

## Key Files (quick reference)

| File | Role |
|------|------|
| `internal/agency/daemon_actor.go` | Actor main loop — full GIST+router pipeline |
| `internal/agency/types.go` | All domain types |
| `internal/agency/llm_actor.go` | Prompt building (BuildSystemPrompt, BuildUserMessage, SetGISTContext) |
| `internal/agency/gist_core.go` | GIST subprocess manager |
| `internal/agency/routing.go` | ModelRouter, CredentialBroker, ProviderAdapter interface |
| `internal/agency/provider_*.go` | Anthropic, Ollama, OpenAI, Gemini adapters |
| `internal/agency/adapters.go` | defaultProviderAdapters() factory |
| `internal/tui/components/chat/list.go` | messagesCmp — has broadcasts field + AgencyBroadcastMsg handler |
| `internal/tui/components/chat/message.go` | uiMessage rendering — extend for role colors + avatar |
| `internal/tui/page/chat.go` | chatPage — has broadcastCh + waitForBroadcast subscription |
| `internal/app/agency.go` | AgencyService — has SubscribeBroadcasts, BroadcastMsg |
| `internal/db/migrations/` | All migrations (5 total) |
| `internal/db/agency_gist_state.go` | Hand-written GIST lattice DB methods |
| `internal/db/agency_routing_log.go` | Hand-written routing log DB methods |
| `AGENCY_BLUEPRINT.md` | Full architecture reference |
| `.planning/config.json` | Disables context monitor hook (keeps tool calls fast) |

---

## DO FIRST in new session

```bash
cd /Users/edwardellis/teamcode
export PATH=$PATH:/usr/local/go/bin:/opt/homebrew/bin
go build ./... && go test ./...   # confirm still clean
```

Then execute Stage 4, Track A and Track B in parallel.

---

## Notes for next Claude

- `defaultMode: "dontAsk"` in `~/.claude/settings.json` — `Write`, `Edit`, `Bash` are in the allow list, should work.
- `.planning/config.json` disables the context monitor hook that was causing 10s freezes between tool calls. Do not delete it.
- Do not re-architect. Blueprint v3 is final. Execute the plan.
- **Stage 4 Track A detail**: `AgencyBroadcastMsg.BroadcastMsg` has `ActorID`, `Message`, `CreatedAt int64`. For role→color, derive from `ActorID` suffix or role string. Voice playback: call `agency.PlatformTTSCommand(baseDir)` in a background goroutine via `exec.Command` — fire and forget, don't block TUI.
- **Stage 4 Track B detail**: Approval channel name is `agency.approval.{orgId}` (follow `OrganizationChannel` naming pattern). The `Kernel.ValidateAction` already exists — use it on the receiving side.
- **Stage 5**: `daemon_actor.go` reads `signal.Payload["prompt_injection"]` and adds a `gistAtom{Kind: "directive", Content: injection, Weight: 1.5}` to the atom list before `Compress()`.
- Provider adapters live in `internal/agency/` (NOT a subpackage) to avoid circular import: `agency` → `providers` → `agency`.
- `llm_actor.go` stays in place — it provides `BuildSystemPrompt`, `BuildUserMessage`, `SetGISTContext`. Don't delete it.
- GIST Python subprocess at `scripts/gist_subprocess.py` doesn't exist yet — `gist_core.go` degrades gracefully. Don't block on it.
- Hand-written DB methods (`agency_gist_state.go`, `agency_routing_log.go`) use `q.db.ExecContext` / `q.db.QueryRowContext` directly — NOT the generated helpers. That's intentional.
