# The Agency One-Sheet

## What The Product Is Now

The product is now **Agency**.

Under the hood, the current implementation still runs on the same compatibility lineage: a heavily modified OpenCode fork with native Go team services, persistent shared team state, teammates, subagents, task flow, messaging, handoffs, and a terminal UI.

So the honest answer is:

- the product is Agency
- the binary and module lineage still use `teamcode` for compatibility
- solo OpenCode-like use remains a first-class mode inside the same product

## The Core Thesis

The Agency is not "an agent that uses tools inside an environment."

It is an environment that agents inhabit.

That is the important distinction:

- most products orchestrate disposable workers
- The Agency is trying to become a persistent organizational runtime

Even before the deeper runtime lands, the current product already points in that direction by making team state explicit instead of faking collaboration through one giant prompt.

## What Already Exists

Today the product already supports:

- terminal-native chat and session flow
- model/provider switching
- custom commands and skill indexing
- file-backed shared team state
- team bootstrap and config-backed templates
- role-aware teammates
- bounded subagents
- direct messages and broadcasts
- task board and handoffs
- sidebar org visibility
- Agency-triggered copy and commands in the TUI

That means the product already works as more than a solo shell, even if the deeper runtime is not finished.

## What Is Changing

The Agency reframes the existing system in a bigger way:

- `solo` remains the clean coding-office mode
- `team` becomes one constitution of the same system
- the product language shifts from "team features" to "staffed office"
- the TUI exposes an Agency trigger path without breaking the solo shell

The current front end now treats compatibility lineage as implementation detail and Agency as the product layer.

## What Is Not Implemented Yet

The full Agency architecture still needs to be built:

- persistent shared office sandbox
- event-reactive daemonized agent runtimes
- append-only ledger as shared truth
- kernel validation of typed actions
- distributed consensus for committed state
- schedules, office hours, and long-running staffed orgs

So this is not being misrepresented as finished. The product frame is ahead of the backend, by design and explicitly.

## What Makes It Different

Compared with OpenCode:

- OpenCode is solo-first
- The Agency keeps that speed, but adds shared state and coordinated execution

Compared with Claude Code:

- Claude Code mainly treats collaboration as an orchestration pattern
- The Agency is aiming to treat collaboration as the environment itself

That is the real architectural bet.

## Simple Explanation

The Agency is a terminal-native product for standing up AI working organizations.

Right now it runs on the existing compatibility/runtime layer, which already supports shared state, roles, teammates, subagents, messaging, and task flow. The current UI keeps solo coding fast, while exposing an Agency path for bootstrapping a staffed office. The long-term build completes that vision with a persistent sandbox, ledgered truth, event-reactive agents, and scheduled organizational continuity.
