# Agency — Session Handoff
> Written at context limit. Start a new session, read this file, say "go".

---

## What This Project Is

**Agency** — terminal-first AI working organization. Binary: `teamcode`. Product: **Agency**. "Your intent, multiplied."

GitHub: `https://github.com/ETEllis/agency` (renamed from teamcode)
Go module: `github.com/ETEllis/teamcode` — do NOT change import paths.
Repo: `/Users/edwardellis/teamcode/`

```bash
cd /Users/edwardellis/teamcode
export PATH=$PATH:/usr/local/go/bin:/opt/homebrew/bin
go build ./...   # ✅
go test ./...    # ✅
```

---

## Architecture (LOCKED — do not redesign)

Full spec: `AGENCY_BLUEPRINT.md v3`

Request chain: `WakeSignal → GISTAgentCore → ActionIntent → ModelRouter → ProviderAdapter → Result → Ledger`

---

## Stage Progress — Implemented Surface

| Stage | Summary |
|-------|---------|
| 1 | Optional voice, DB scheduler, broadcast→TUI |
| 2 | GIST cognitive layer |
| 3 | ModelRouter, CredentialBroker, provider adapters |
| 4 | TUI: iMessage bubbles, optional TTS, ApprovalCmp |
| 5 | NestedScheduler, bulletin timeline, directive→GIST |

---

## THIS SESSION — What Was Built (2026-04-02)

### Critical Bug Fixed: genesis→actor-spawn bridge was missing
After `/agency genesis`, `GenesisRoleBundle[]` were saved to `office-state.json` but never converted into `ActorRuntimeSpec` files. The runtime daemon found nothing to spawn. This was the core V1 blocker.

**Fix implemented:**
- `internal/app/agency.go` — added `writeActorSpecsLocked()` + `ManifestActors()`. `StartGenesis` now auto-calls manifest after saving state. Writes actor spec JSON to `{baseDir}/runtime/actors/{id}.json`. Added `ManifestCount int` to `AgencyGenesisResult`.
- `cmd/agency.go` — added `/agency bootstrap` as the canonical re-manifest command (legacy `/office bootstrap` alias preserved). Updated genesis output to show actor count + "Start the runtime" instructions.
- `scripts/build-daemons` — now builds `dist/agency-ipc-server`
- `Procfile` — added `ipc: ./dist/agency-ipc-server`
- `scripts/test-ipc` — smoke test: builds ipc-server, starts it, connects nc client, publishes 3 Redis events, verifies all 3 message types received. Requires Redis.

### First-Run UX Overhaul
- `internal/tui/components/chat/splash.go` — provider status panel on splash showing all 5 providers with check/cross/status. Codex shows auth.json state. OpenAI shows active OPENAI_MODEL. Warning shown if nothing configured.
- `internal/tui/page/chat.go` + `internal/tui/tui.go` — error messages now actionable with specific env vars + "run: scripts/setup"
- `internal/agency/daemon_actor.go` — routing failure now broadcasts `"[no provider] ..."` instead of silent `"actor ready"`

### Codex CLI OAuth Adapter (NEW)
- `internal/agency/provider_codex.go` — `CodexCLIAdapter`: checks `codex` binary + `~/.codex/auth.json`, runs `codex exec --json --sandbox read-only "<prompt>"` by default, parses OpenAI Responses API JSONL event stream. Unsafe unsandboxed developer mode requires explicit `AGENCY_CODEX_UNSANDBOXED=true`.
- `internal/agency/adapters.go` — Codex added first in `defaultProviderAdapters()` (highest priority when available)
- `internal/agency/routing.go` — "codex" added to credential broker probes (empty envKey, like Ollama — `Available()` does real check)

### Setup Script
- `scripts/setup` — interactive first-run: checks/installs deps (go, redis, overmind), 6 provider options:
  1. **Codex** — installs `@openai/codex` npm, runs `codex login` (browser OAuth)
  2. Anthropic — saves `ANTHROPIC_API_KEY` to `.env`
  3. OpenAI — saves `OPENAI_API_KEY` + `OPENAI_MODEL` to `.env`
  4. Gemini — saves `GEMINI_API_KEY` to `.env`
  5. Ollama — checks install, optionally pulls llama3.2
  6. Skip
  Builds all binaries. Prints exact launch commands.

---

## Current State: Terminal Release Candidate

The default release path is local Redis + Overmind + daemons, not Docker.
Docker Compose remains optional packaging/parity work. Voice is optional and
must not block the terminal release.

Verified in the release loop:

- `scripts/release-smoke` static gates pass.
- `go test ./...` passes.
- `go build -o agency .` passes.
- `scripts/build-daemons` builds office, runtime, scheduler, actor, and IPC binaries.
- `./agency agency services --json` reports Docker disabled by default.
- Codex CLI uses read-only sandbox args by default; unsafe bypass requires `AGENCY_CODEX_UNSANDBOXED=true`.

Live gates passed from a normal Terminal session:

```bash
scripts/live-release-proof --log-dir .tmp/release-proof-terminal-attempt
scripts/verify-release-proof .tmp/release-proof-terminal-attempt
```

For a fresh evidence bundle, rerun `scripts/live-release-proof --log-dir
.tmp/release-proof`.

---

## After Terminal Runtime Is Confirmed → Swift Desktop

Three views remain to implement. The desktop lane is intentionally next-phase
work and should not be treated as release-blocking until these views compile.

**Location:** `desktop/Agency/Sources/Agency/Views/`

### 1. `BubbleListView.swift`
- `ScrollViewReader` auto-scroll on `connection.broadcasts` change
- Avatar: colored `Circle` (28pt) with 2-letter initials from `.initials`
- Left accent bar (3pt) in `roleColor` → SwiftUI Color map: `.blue/.purple/.teal/.green/.orange/.pink`
- Actor name + relative timestamp header row
- Empty state: `ContentUnavailableView("Waiting for signals", systemImage: "bubble.left")`
- Data: `connection.broadcasts: [BroadcastMessage]` — `.actorID`, `.message`, `.createdAt`, `.initials`, `.roleColor`

### 2. `ApprovalView.swift`
- `List` with keyboard focus for `return`=approve, `delete`=reject
- `.swipeActions(edge: .trailing)` — green Approve + red Reject
- Calls `connection.sendVote(proposalID: item.proposalID, approved: true/false)`
- Empty state: `ContentUnavailableView("No Pending Approvals", systemImage: "checkmark.shield")`
- Data: `connection.approvals: [ApprovalItem]` — `.proposalID`, `.actorID`, `.actionType`, `.target`, `.createdAt`

### 3. `BulletinView.swift`
- Actor name + provider/model tag right-aligned + timestamp
- Directive in italic `.caption` with left accent
- Output `.lineLimit(2).truncationMode(.tail)`
- Score badge: `"%.0f%%" * entry.score * 100`, color from `entry.scoreColor` (`.good`→green, `.fair`→yellow, `.poor`→red)
- Empty state: `ContentUnavailableView("No Performance Records", systemImage: "chart.bar.doc.horizontal")`
- Data: `connection.bulletins: [BulletinEntry]` — `.actorID`, `.directive`, `.output`, `.score`, `.provider`, `.modelID`, `.createdAt`, `.scoreColor`

### Swift compile check
```bash
cd desktop/Agency && swift build
```

### Desktop wiring after views are done
1. `AgencyApp.swift` already opens secondary Approvals window when `!connection.approvals.isEmpty`
2. Set env vars before launching:
   ```bash
   export AGENCY_ORG_ID=<org-id>
   export AGENCY_BASE_DIR=/Users/edwardellis/teamcode
   ```
3. Socket path: `{AGENCY_BASE_DIR}/.agency/ipc-{AGENCY_ORG_ID}.sock`

---

## After Swift Desktop → WebSocket Transport → iPad

Per blueprint: item 29 = WebSocket transport, item 31 = iPad companion app.

---

## Key Files Reference

| File | Role |
|------|------|
| `internal/agency/daemon_actor.go` | Actor main loop — full pipeline |
| `internal/agency/ipc.go` | Unix socket IPC server |
| `internal/agency/routing.go` | ModelRouter + CredentialBroker (5 providers incl. Codex) |
| `internal/agency/provider_codex.go` | Codex CLI OAuth adapter — NEW |
| `internal/agency/adapters.go` | defaultProviderAdapters() list |
| `internal/app/agency.go` | AgencyService — genesis, ManifestActors, subscriptions |
| `internal/tui/components/chat/splash.go` | Provider status panel on splash |
| `cmd/agency.go` | /agency genesis, /agency bootstrap commands (legacy /office alias hidden) |
| `scripts/setup` | First-run setup script — deps + provider + build |
| `scripts/test-ipc` | IPC smoke test |
| `scripts/build-daemons` | Builds all 5 daemons incl. ipc-server |
| `Procfile` | 5 processes: redis, office, runtime, scheduler, ipc |
| `desktop/Agency/Sources/Agency/Services/AgencyConnection.swift` | macOS IPC client |
| `desktop/Agency/Sources/Agency/Views/OfficeView.swift` | Main window + sidebar |
| `.planning/config.json` | Disables context monitor hook — do not delete |

---

## Notes for Next Claude

- `defaultMode: "dontAsk"` in `~/.claude/settings.json` — Write/Edit/Bash all auto-approved.
- `.planning/config.json` disables context monitor hook. Do not delete.
- Go module path is `github.com/ETEllis/teamcode` — do NOT change imports.
- Desktop uses `NWConnection` (Network.framework) for Unix socket — no third-party deps.
- `RoleColor` in `Models.swift` maps to SwiftUI colors in `BubbleListView` — use same deterministic hash as Go's `actorColor()` in `list.go`.
- Codex adapter reads `~/.codex/auth.json` — token refresh is handled automatically by Codex CLI on next `codex exec` call.
- The `.env` file in project root is auto-loaded by overmind. For `./teamcode` (TUI), user must `source .env` or add to shell profile.
- Do NOT redesign. Blueprint v3 is final. Execute the plan.
- **Next session start:** Verify V1 terminal works end-to-end before touching Swift.
