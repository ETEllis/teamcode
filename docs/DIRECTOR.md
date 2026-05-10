# Agency Director

Director is the minimal personal agent that sits between you and the full
Agency office.

It is intentionally not a second Agency. Think of it as the calm daily
interface: it opens tickets, checks office health, asks for approvals, and
dispatches structured work into the local office when the intent is clear.

## What Runs

- `agency-director-daemon`: hosts the Director agent and local web portal.
- `agency agency director serve`: serves the same portal from the CLI.
- `agency agency director monitor`: runs one passive monitoring pass.
- `agency agency director submit --dispatch "..."`: opens and dispatches a
  ticket into Agency.

The default portal is local only:

```bash
agency agency director serve
```

The one-command installer writes:

```bash
AGENCY_DIRECTOR_ADDR=127.0.0.1:8765
AGENCY_DIRECTOR_TOKEN=<generated>
AGENCY_DIRECTOR_MONITOR_INTERVAL_SECONDS=300
```

## Remote Posture

Director is designed for mobile/web use, but the safe default is localhost. If
you expose it through Cloudflare Tunnel, Tailscale Funnel, ngrok, or another
reverse proxy, keep the app token enabled and add provider-side auth when
possible.

Remote access is a window into the local Agency office. It does not move local
execution into a cloud worker.

## Ticket Flow

```text
User request
  -> Director ticket
  -> Director triage
  -> WakeSignal(kind=director)
  -> Agency organization channel or target actor channel
  -> Ledger entry
  -> approval boundary before consequential work
```

Director writes its own `director/tickets.jsonl` and `director/events.jsonl`
under the Agency data directory, while Agency's append-only ledger remains the
source of truth for office execution.

## Provider Profiles

Agency's setup now recognizes these first-class provider/model paths:

- Codex ChatGPT OAuth
- Anthropic
- OpenAI
- Gemini
- Ollama
- OpenRouter
- OpenCode models
- Zen
- Go
- LM Studio
- LiteLLM
- Mistral
- xAI
- Groq
- Together
- Fireworks
- Perplexity
- Cerebras
- Z.ai / GLM
- Qwen / DashScope

OpenCode, Zen, and Go are configured as OpenAI-compatible profiles by default:

```bash
OPENCODE_BASE_URL=...
OPENCODE_MODEL=...
ZEN_BASE_URL=...
ZEN_MODEL=...
GO_BASE_URL=...
GO_MODEL=...
```
