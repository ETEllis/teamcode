# Agency Demo

Agency is easiest to understand when you see the local office breathe.

The fastest path is:

```bash
scripts/demo-local-office
```

The demo uses the real Redis-backed IPC path and a mock provider event, so it does not require API keys. It shows the V1 causal loop:

```txt
WakeSignal -> GIST/ReasoningCore -> ActionIntent -> ModelRouter -> ProviderAdapter -> Result -> Ledger
```

## What The Demo Shows

1. Creates an isolated demo office id.
2. Starts or reuses local Redis.
3. Builds and starts the IPC server.
4. Connects a local client through the Unix socket.
5. Publishes a wake-style broadcast.
6. Publishes a mock approval request for consequential action.
7. Publishes a bulletin result from the mock provider.
8. Writes a demo ledger transcript under `.tmp/demo-local-office/ledger.jsonl`.

The IPC proof is real. The provider response is intentionally mocked so a first-time visitor can see the mechanism without configuring Anthropic, OpenAI, Gemini, Codex, or Ollama first.

## Full Runtime Proof

For the complete V1 release proof, including static gates, Redis IPC, Overmind process status, and proof verification:

```bash
scripts/live-release-proof --log-dir .tmp/release-proof
```

The proof bundle contains:

- `manifest.txt`
- `static.log`
- `live-redis-ipc.log`
- `live-overmind.log`

## Why This Matters

Agency is not merely a multi-agent chat surface. It treats autonomous work as an inspectable local organization with schedules, roles, trust lanes, routing policy, approvals, and a ledgered causal history.

That is the product spine: not agents talking, but work becoming inspectable.
