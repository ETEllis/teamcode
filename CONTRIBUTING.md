# Contributing To Agency

Agency is early, public, and release-minded. Contributions should preserve the terminal-first local runtime while improving the product surface, reliability, provider support, or documentation.

## Good First Areas

- Provider adapters and routing tests.
- TUI polish, command discoverability, and accessibility.
- `scripts/release-smoke` and live proof hardening.
- Docs that make installation, boot, and troubleshooting clearer.
- Runtime tests for Redis, IPC, scheduling, ledger, and approvals.

## Local Checks

Run the static release gate before opening a pull request:

```bash
go test ./...
scripts/release-smoke
```

For changes touching Redis, Overmind, IPC, daemon orchestration, setup, or install scripts, also run:

```bash
scripts/live-release-proof --log-dir .tmp/release-proof
```

## Release Hygiene

Do not commit `.env`, `.tmp`, `dist`, local binaries, `.omx`, `.teamcode`, `.opencode`, or local agent planning files. These are ignored by default; keep public commits intentional and reproducible.
