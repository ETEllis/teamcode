# Agency

**Your intent, multiplied.**

```txt
    _    ____ _____ _   _  ______   __
   / \  / ___| ____| \ | |/ ___\ \ / /
  / _ \| |  _|  _| |  \| | |    \ V /
 / ___ \ |_| | |___| |\  | |___  | |
/_/   \_\____|_____|_| \_|\____| |_|
```

Agency is a terminal-first AI working organization. Staff a persistent office of autonomous agents, each with a distinct role and optional local voice, then let that office wake on schedules, reason through work, route to the right model, and write every consequential step to a shared ledger.

It is built for developers who want more than a chatbot tab: live daemons, a TUI command center, provider routing, approvals, durable evidence, and a local-first runtime you can inspect end to end.

```bash
curl -fsSL https://raw.githubusercontent.com/ETEllis/agency/main/install | bash
```

### What You Get

- A terminal command center for a staffed AI office.
- Local Redis + Overmind runtime with office, scheduler, actor, runtime, and IPC daemons.
- Provider routing across Codex, Anthropic, OpenAI, Gemini, and Ollama.
- Human approval lanes for proposed actions.
- A bulletin board and append-only ledger for auditability.
- Optional voice, optional Docker packaging, and a macOS desktop companion in progress.

### Why Agency

Agency treats autonomous work as an organization, not a prompt. Schedules create wake signals, agents compress context into GIST state, model routing chooses the safest available provider, approvals gate consequential action, and the ledger preserves what happened. The result is a local command center that can become a daily working surface today and a larger orchestration substrate tomorrow.

---

## What's Built

The core terminal runtime is present in this repo and is the first release target.
Voice, Docker, and the macOS desktop app are intentionally non-blocking for the
first public push.

### Status

| Area | Status |
|------|--------|
| Terminal CLI/TUI | Release candidate |
| Local runtime | Redis + Overmind + daemons, verified by `scripts/live-release-proof` |
| Voice | Optional; macOS `say` fallback, Kokoro install path available |
| Docker Compose | Optional/experimental packaging path |
| macOS desktop | In progress |

For release evidence, rerun the live local-process gates from a normal Terminal
session to create a fresh proof bundle:

```bash
scripts/live-release-proof --log-dir .tmp/release-proof
```

For targeted reruns, use `scripts/release-smoke --live` or
`scripts/release-smoke --with-overmind --skip-static`.

The tracked release audit is [RELEASE_CHECKLIST.md](RELEASE_CHECKLIST.md).

### Brand

Agency uses a command-center palette: Ledger Ink `#101114`, Signal Gold `#E2B76D`, Relay Cyan `#5EB7C7`, Ledger Green `#7FB069`, and Parchment text `#E8E3D6`. The full public brand note is [BRAND.md](BRAND.md).

### Architecture

```
┌─────────────────────────────────────────────────────────┐
│  USER LAYER                                              │
│  TUI iMessage bubbles · Optional Voice · Approval        │
│  lane · Bulletin board · Genesis wizard                  │
└────────────────────────┬────────────────────────────────┘
                         │
┌────────────────────────▼────────────────────────────────┐
│  DETERMINISTIC LAYER  (Nested Temporal Cron Tree)        │
│  ScheduleNode tree · prompt_injection per node           │
│  Fires enriched WakeSignals into reactive layer          │
└────────────────────────┬────────────────────────────────┘
                         │ enriched WakeSignal
┌────────────────────────▼────────────────────────────────┐
│  REACTIVE LAYER  (Stateful Agent Runtime)                │
│  Redis event bus · Actor daemons · Ledger · Consensus    │
└────────────────────────┬────────────────────────────────┘
                         │
┌────────────────────────▼────────────────────────────────┐
│  GIST COGNITIVE LAYER  (per-agent)                       │
│  Causal compression · ElasticStretch · Lattice state     │
│  Outputs: GISTVerdict + execution_intent                 │
└────────────────────────┬────────────────────────────────┘
                         │ ActionIntent
┌────────────────────────▼────────────────────────────────┐
│  MODEL ROUTING LAYER                                     │
│  ModelRouter · CredentialBroker · 5 provider adapters    │
│  5 hard gates (capability/auth/privacy/tools/budget)     │
│  Ollama-first soft scoring                               │
└────────────────────────┬────────────────────────────────┘
                         │ Result
┌────────────────────────▼────────────────────────────────┐
│  LEDGER  (append-only, single source of truth)           │
│  CommitCertificate · Quorum consensus · Snapshots        │
└─────────────────────────────────────────────────────────┘
```

**Full request chain:**
```
WakeSignal → GIST/ReasoningCore → ActionIntent → ModelRouter → ProviderAdapter → Result → Ledger
```

### Completed Stages

| Stage | What shipped |
|-------|-------------|
| **1 — Live Agent Foundation** | DB poll scheduler, broadcast→TUI pipeline, env config, optional voice via Kokoro or macOS `say` fallback |
| **2 — GIST Cognitive Layer** | `GISTAgentCore`, causal compression, `ElasticStretch`, `LatticeStore`, per-wake lattice persistence |
| **3 — Model Routing Layer** | `ModelRouter` (5 hard gates + soft scoring), `CredentialBroker`, Codex/Anthropic/Ollama/OpenAI/Gemini adapters, routing audit log |
| **4 — Core TUI Experience** | iMessage-style bubbles (per-actor color + avatar + timestamp), TTS voice on broadcast, `ApprovalCmp` panel (a/r keys, auto right-rail), approval channel + vote relay |
| **5 — Nested Temporal Orchestration** | `ScheduleNode` tree with `prompt_injection`, `NestedScheduler`, `PerformanceRecord`, bulletin timeline (directive→output→score), daemon wired: directive → 1.5-weight GIST atom + performance publish |

---

## Quick Start

### One-command install

On macOS, this installs or verifies Go, Redis, and Overmind through Homebrew,
clones Agency, builds the CLI and daemons, and adds `agency` to your shell PATH:

```bash
curl -fsSL https://raw.githubusercontent.com/ETEllis/agency/main/install | bash
```

For a non-interactive build-only install:

```bash
curl -fsSL https://raw.githubusercontent.com/ETEllis/agency/main/install | bash -s -- --yes --skip-provider --no-launch
```

The installer uses `~/.agency` by default. Override with `AGENCY_INSTALL_DIR`.
Linux is supported when `git`, `go`, `redis-server`, and `overmind` are already
available, or when Homebrew/Linuxbrew is installed.

### Repository Contents

| Path | What it is |
|------|------------|
| `cmd/` | CLI commands, runtime commands, and schema generation |
| `internal/agency/` | Core office runtime: schedules, agents, bus, routing, ledger, IPC |
| `internal/tui/` | Terminal command-center UI, splash, approval lane, and themes |
| `scripts/` | Setup, daemon build, smoke tests, live proof, and verifier scripts |
| `Procfile` | Local Redis + office + runtime + scheduler + IPC process graph |
| `Dockerfile.agency`, `docker-compose.agency.yml` | Optional packaging path, not required for the default local install |
| `AGENCY_BLUEPRINT.md` | Architecture reference |
| `RELEASE_CHECKLIST.md` | Public release gates and evidence trail |
| `CONTRIBUTING.md`, `SECURITY.md` | Public contribution and vulnerability reporting guidance |

### Prerequisites

- Go 1.22+
- Redis (for the local multi-process office runtime)
- Overmind (for running the local `Procfile`)
- Python 3.9+ with `kokoro-onnx` only if you want higher-quality local voice; macOS `say` is the no-extra-dependency fallback
- Codex CLI authenticated with `codex login`, an API key for at least one hosted provider (Anthropic, OpenAI, Gemini), **or** Ollama running locally
- Docker is not required for the default terminal release path

### Voice (optional)

Agency can ship and run without a custom voice model. On macOS it can use the built-in `say` command as the lightweight fallback. Install Kokoro only when you want better local TTS:

```bash
scripts/install-voice
```

### Boot the office

```bash
# Build and configure without auto-launching the TUI
scripts/setup --no-launch --skip-provider

# Build all daemons
scripts/build-daemons

# Connect at least one provider
codex login                       # ChatGPT OAuth, no API key
# or:
export ANTHROPIC_API_KEY=sk-...   # OPENAI_API_KEY, GEMINI_API_KEY, OLLAMA_API_BASE also work

# Start the local office runtime (Redis + daemons + IPC)
overmind start
```

### Release smoke

Run static/build gates anywhere:

```bash
scripts/release-smoke
```

Run live Redis/IPC proof from a normal Terminal session:

```bash
scripts/release-smoke --live
```

Run the full local-process proof:

```bash
scripts/release-smoke --with-overmind
```

Run the full terminal release proof with durable logs and automatic verification:

```bash
scripts/live-release-proof
```

To choose the evidence directory:

```bash
scripts/live-release-proof --log-dir .tmp/release-proof
```

The proof directory includes `manifest.txt`, `static.log`,
`live-redis-ipc.log`, and `live-overmind.log`.

Re-check a completed proof directory:

```bash
scripts/verify-release-proof .tmp/release-proof
```

### TUI only (no daemons)

```bash
go build -o agency .
./agency
```

### Key commands inside the TUI

```
/agency genesis   — natural-language intent → structured org config
/agency bootstrap — boot a staffed office from a constitution
/agency status    — inspect running office, actors, schedules
/agency stop      — graceful shutdown
```

### Approval lane

When agents propose actions, the approval panel appears in the right rail automatically. Press `a` to approve, `r` to reject, `↑↓` to navigate.

### Bulletin board

Performance records (directive→output→score) stream into the messages viewport as agents complete inference cycles. Color-coded score badges shift green→yellow→red.

---

## Configuration

Primary config: `~/.agency.json` or `.agency.json` in your project root.

```jsonc
{
  "agency": {
    "productName": "Agency",
    "office": {
      "mode": "staffed",
      "sharedWorkplace": ".agency/workplace",
      "autoBoot": true
    },
    "redis": {
      "enabled": true,
      "address": "localhost:6379"
    },
    "schedules": {
      "defaultCadence": "@every 5m",
      "timezone": "America/New_York"
    },
    "currentConstitution": "coding-office"
  }
}
```

Legacy `.teamcode.json` / `.opencode.json` config files are still read as fallbacks for existing installs.

---

## Architecture — Key Files

| File | Role |
|------|------|
| `internal/agency/daemon_actor.go` | Actor main loop: GIST -> routing -> proposals -> ledger -> bus |
| `internal/agency/types.go` | All domain types including `ScheduleNode`, `ActionProposal`, `WakeSignal` |
| `internal/agency/gist_core.go` | GIST subprocess manager + elastic stretch |
| `internal/agency/nested_scheduler.go` | Cron tree with prompt injection |
| `internal/agency/routing.go` | `ModelRouter`, `CredentialBroker`, 5-gate scoring |
| `internal/agency/performance.go` | `PerformanceRecord`, `BulletinChannel`, `PublishPerformance` |
| `internal/agency/runtime.go` | `RuntimeManager`, channel helpers |
| `internal/tui/components/chat/approval.go` | Approval panel component |
| `internal/tui/components/chat/bulletin.go` | Bulletin timeline renderer |
| `internal/app/agency.go` | `AgencyService` — subscriptions, votes, genesis |
| `internal/db/migrations/` | 6 migrations (schema + agency runtime) |
| `AGENCY_BLUEPRINT.md` | Full architecture reference (canonical) |

---

## What's Next

### IPC Transport
Unix socket server exposing the live office event stream to local clients (desktop app, CLI tools). Same `WakeSignal` / `LedgerEntry` schema as Redis but over a local socket.

### macOS Desktop App (in progress)
Native SwiftUI companion. Real-time office view: iMessage bubbles, bulletin board, approval lanes, agent status. Connects to the Go runtime via IPC socket. macOS-first, iPad companion after.

### Docker Compose (optional)
The public terminal path is local Redis + Overmind + daemons. Docker Compose remains an experimental packaging/parity path and is not required to run Agency.

### WebSocket Transport
Remote client event stream — same schema as IPC, enabling web dashboard and mobile companion.

---

## Contributing

Agency is early but intentionally public. Good first contributions are provider adapters, release-smoke hardening, TUI polish, docs, and runtime tests. Before opening changes, run:

```bash
go test ./...
scripts/release-smoke
```

For changes touching Redis, Overmind, IPC, or daemon orchestration, also run:

```bash
scripts/live-release-proof --log-dir .tmp/release-proof
```

See [CONTRIBUTING.md](CONTRIBUTING.md) for the public contribution path.

## Security

Do not commit `.env`, `.codex`, local proof logs, generated binaries, or agent scratch state. The repo ignores those paths by default. Codex execution uses a read-only sandbox unless `AGENCY_CODEX_UNSANDBOXED=true` is explicitly set.

See [SECURITY.md](SECURITY.md) for vulnerability reporting.

---

## Providers

Agency routes to whichever provider passes its gates. The easiest hosted path is Codex CLI with ChatGPT OAuth:

```bash
npm install -g @openai/codex
codex login
```

You can also set any subset of these:

```bash
export ANTHROPIC_API_KEY=...
export OPENAI_API_KEY=...
export GEMINI_API_KEY=...
export OLLAMA_API_BASE=http://localhost:11434   # Ollama preferred first (local-first)
```

The setup script exposes the same provider choices interactively:

```bash
scripts/setup
```

Codex execution uses a read-only sandbox by default. The unsafe unsandboxed developer mode is opt-in only via `AGENCY_CODEX_UNSANDBOXED=true`.

---

## License And Attribution

MIT

Agency preserves upstream MIT attribution from the OpenCode / TeamCode lineage.
See [NOTICE.md](NOTICE.md).
