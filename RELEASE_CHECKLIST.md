# Agency Release Checklist

Last updated: 2026-05-10

## Release Objective

Ship Agency V1 as the terminal-first local office, now with the V1.1 Director
agent as the local personal intake and monitoring layer. Voice, desktop, Docker
parity, and native remote clients remain V2/V3 product layers rather than
missing V1 requirements.

## Prompt-to-Artifact Audit

| User requirement | Artifact / command | Evidence | Status |
|------------------|--------------------|----------|--------|
| Frame voice correctly for V1/V2 | README V1 status, voice section, `scripts/install-voice`, macOS `say` fallback | Voice is documented as V1 optional fallback and V2 product-quality layer | Done |
| Do every release piece possible except custom voice | `scripts/release-smoke`, `scripts/live-release-proof`, `scripts/verify-release-proof`, docs | Static gates, proof wrapper, verifier, public docs, and checklist are in place | Done |
| Avoid Docker as default runtime tax | `internal/config/agency.go`, `./agency agency services --json`, README | Runtime config reports `docker.enabled=false`; Docker Compose is optional packaging only | Done |
| Preserve system architecture without Docker | `Procfile`, `scripts/build-daemons`, `scripts/release-smoke --with-overmind`, IPC smoke | Local Redis + Overmind + daemons + IPC path is canonical and repeatable | Done |
| Keep desktop as next path, not current blocker | README, HANDOFF, task plan | Desktop is explicitly next-lane work; release is terminal-first | Done |
| Provide deterministic completion state | `RELEASE_CHECKLIST.md`, `scripts/live-release-proof`, `scripts/verify-release-proof` | Checklist maps gates to evidence; proof command writes manifest/logs and auto-verifies | Done |
| Prove live Redis and Overmind runtime | `scripts/live-release-proof --log-dir .tmp/release-proof-terminal-attempt` from normal Terminal | Passed and verified; evidence in `.tmp/release-proof-terminal-attempt` | Done |
| Add personal Director agent | `agency agency director ...`, `agency-director-daemon`, `docs/DIRECTOR.md` | Director opens tickets, dispatches structured wake signals, monitors Agency, and serves a local portal | Done |
| Gate Director autonomy | `DirectorPolicy`, `agency agency director policy`, Director tests | Low-risk tickets may auto-dispatch; higher-risk tickets stay open for review | Done |
| Add 2026 provider profiles | `scripts/setup`, `internal/agency/provider_compatible.go` | OpenRouter, OpenCode, Zen, Go, LM Studio, LiteLLM, Mistral, xAI, Groq, and more profiles are configurable | Done |

## Decisions Locked

| Decision | Status | Evidence |
|----------|--------|----------|
| Voice is V1 optional / V2 product layer | Done | README marks voice as V1 optional fallback and V2 product-quality layer |
| Docker is V1 optional / V2 parity layer | Done | `./agency agency services --json` reports `docker.enabled=false`; README says Docker Compose is optional and V2 hardening/parity |
| Local runtime is canonical | Done | `Procfile`, `scripts/build-daemons`, and `scripts/release-smoke` define Redis + Overmind + daemons |
| Desktop is V2 companion surface | Done | README roadmap frames macOS desktop as V2 companion app |
| Unsafe Codex mode is not default | Done | Codex adapters use read-only sandbox by default; unsafe mode requires `AGENCY_CODEX_UNSANDBOXED=true` |
| Upstream attribution is preserved | Done | `LICENSE` retains upstream MIT notice; `NOTICE.md` documents Agency lineage and maintainer attribution |
| Generated artifacts are ignored | Done | `.gitignore` excludes local binary, `.tmp`, `dist`, and SwiftPM `.build` output |
| Local planning traces are not public by accident | Done | `.gitignore` excludes `.omx/` and root planning scratch files |
| Brand direction is explicit | Done | `BRAND.md`, README palette, setup wordmark, and splash wordmark define the public terminal face |
| One-command install is public | Done | Top-level `install` bootstraps dependencies, clones Agency, runs setup, and links `agency` into `~/.agency/bin` |
| Demo path is public | Done | `docs/DEMO.md` and `scripts/demo-local-office` show the local office loop without API keys |
| CI smoke path is public | Done | `.github/workflows/release-smoke.yml` runs static release smoke plus Redis IPC live smoke |
| Director is local-first | Done | Portal defaults to `127.0.0.1:8765`; remote exposure is opt-in and token-gated |
| OpenCode/Zen/Go are provider options | Done | Setup writes OPENCODE/ZEN/GO base URLs and model IDs as OpenAI-compatible provider profiles |

## Verified Gates

| Gate | Command / artifact | Status |
|------|--------------------|--------|
| Script syntax | `bash -n scripts/setup scripts/install-voice scripts/test-ipc scripts/release-smoke scripts/live-release-proof scripts/verify-release-proof` | Passed |
| Non-interactive setup mode | `scripts/setup --no-launch --skip-provider` | Passed |
| Full Go tests | `go test ./...` | Passed |
| IPC fanout integration test | `go test -run TestIPCServerFansOutOfficeEvents -v ./internal/agency` | Present; skipped in sandbox when Unix socket bind is unavailable |
| Main binary build | `go build -o agency .` | Passed |
| Daemon builds | `scripts/build-daemons` | Passed |
| Static release smoke | `scripts/release-smoke` | Passed |
| IPC smoke hygiene | `scripts/test-ipc` uses isolated temp/org IDs, readiness waits, existing-server mode, targeted cleanup, and skips redundant rebuilds in existing-server mode | Passed |
| Sandbox-safe Go build cache | `scripts/setup` and `scripts/release-smoke` default `GOCACHE` to `.tmp/go-build` | Passed |
| Docker default off | `./agency agency services --json` | Passed |
| Diff hygiene | `git diff --check` | Passed |
| Live-gate script help | `scripts/release-smoke --help` | Passed |
| One-command Terminal proof wrapper | `scripts/live-release-proof --help`; `scripts/live-release-proof --log-dir .tmp/release-proof-sandbox-check` | Help passed; manifest records invocation, expected files, git/tool state; static stage passed; failure-log path verified; successful live run auto-runs verifier |
| Proof verifier | `scripts/verify-release-proof --help`; `scripts/verify-release-proof .tmp/release-proof-sandbox-check`; synthetic `.tmp/release-proof-verifier-fixture` | Help passed; correctly rejects incomplete sandbox proof; correctly accepts complete proof fixture |
| Focused live rerun option | `scripts/release-smoke --with-overmind --skip-static` skips static gates and enters Overmind gate without requiring Go | Present; full Terminal proof passed through the one-command wrapper |
| Focused mode guardrail | `scripts/release-smoke --skip-static` fails unless a live gate is selected | Passed |
| Durable proof logging | `scripts/release-smoke --skip-static --log .tmp/release-smoke-test.log` | Passed; `.tmp` is ignored |
| Normal-Terminal live proof | `scripts/live-release-proof --log-dir .tmp/release-proof-terminal-attempt` | Passed; wrapper auto-ran verifier and exited `0` |
| Generated artifact hygiene | `git status --short --ignored -- .tmp dist agency teamcode desktop/Agency/.build .gitignore` | Generated outputs and local proof logs remain ignored; `.gitignore` is intentionally tracked |
| Public staging hygiene | `git status --short` | Internal traces should remain ignored; stage release files intentionally rather than using blind `git add .` |
| Brand polish | README, `BRAND.md`, setup banner, TUI splash | Wordmark and palette are present |
| Installer syntax | `bash -n install`; `./install --help` | Passed; installer exposes documented flags and parses cleanly |
| Local office demo | `scripts/demo-local-office` | Passed; shows office creation, Redis/IPC broadcast, approval, bulletin, and demo ledger transcript |
| Director unit tests | `go test ./internal/agency` | Passed |
| Director daemon build | `scripts/build-daemons` | Passed; includes `dist/agency-director-daemon` |
| Director CLI status | `./agency agency director status --json` | Passed; reports `personal-director` identity and current ledger state |
| Director policy tests | `go test ./internal/agency` | Passed; verifies allowed auto-dispatch, high-risk block, and monitor escalation |
| Director Overmind process | `scripts/release-smoke --with-overmind --skip-static` | Passed; Overmind status includes `director` running before IPC proof |

## Terminal Live Proof

The Codex sandbox cannot bind Redis TCP or Overmind Unix sockets, so the live
proof was launched through macOS Terminal. The final proof passed and the proof
folder verified successfully.

```bash
cd /Users/edwardellis/teamcode
scripts/live-release-proof --log-dir .tmp/release-proof-terminal-attempt
scripts/verify-release-proof .tmp/release-proof-terminal-attempt
```

Observed live proof:

- `manifest.txt`, `static.log`, `live-redis-ipc.log`, and `live-overmind.log`
  were written under `.tmp/release-proof-terminal-attempt`.
- `scripts/live-release-proof --log-dir .tmp/release-proof-terminal-attempt`
  auto-ran `scripts/verify-release-proof .tmp/release-proof-terminal-attempt`.
- Redis starts or is already reachable.
- `scripts/test-ipc` receives broadcast, approval, and bulletin messages.
- Overmind starts `redis`, `office`, `runtime`, `scheduler`, `ipc`, and `director`.
- Overmind status reaches all six process names with no stopped/exited/failed markers before IPC proof runs.
- The Overmind IPC server listens on a short repo-local smoke socket path
  `.tmp/om-*/.agency/ipc-*.sock` to stay below macOS Unix socket path limits.

## First Public Push Rule

The terminal-first release path can be called complete after the normal-Terminal
proof above. After staging and committing the final public release files, rerun
the proof so the manifest records the exact commit being published:

```bash
scripts/live-release-proof --log-dir .tmp/release-proof
```

## Completion Verdict

Complete for Agency V1: the terminal-first local office. Voice remains a V1
optional fallback and V2 product-quality layer, Docker remains optional V1
packaging with V2 parity hardening, and the Swift desktop is the V2 companion
surface.
