# TeamCode

⌬ Terminal-based AI coding team assistant with multi-agent collaboration support.

## Overview

TeamCode is a fork of OpenCode focused on first-class multi-agent collaboration. It keeps the terminal-native workflow, model selection, and coding tools from OpenCode, while adding persistent team coordination and formal subagent execution. It enables:

- **Team Context**: Shared charter, roles, goals, and working agreements
- **Task Board**: Shared backlog, in-progress, blocked, and done state
- **Handoff Protocol**: Explicit task transitions between agents
- **Private Inboxes**: Each agent has their own message inbox
- **Direct Chatter**: Teammates can message specific peers
- **Global Broadcasts**: Team-wide updates and coordination signals
- **Persistent Teammates**: Named worker sessions with role identity
- **Subagents**: Bounded worker sessions for focused delegated execution
- **Nested Delegation**: Teammates can spawn their own subagents

## Architecture

```
TeamCode (Go)
    │
    ├── internal/team/             # Shared team state and messaging
    │       ├── team_context.json
    │       ├── task_board.json
    │       ├── handoffs.json
    │       ├── members.json
    │       └── inboxes/<agent>.json
    │
    └── internal/orchestration/    # Persistent teammate/subagent runtime
            └── worker manager + child sessions
```

Team state is now native Go and file-backed under `.teamcode/teams` or the configured TeamCode data directory. The old Python bridge is no longer part of the runtime.

## Building

```bash
go build -o teamcode .
```

## Usage

```bash
# Interactive mode
./teamcode

# Non-interactive mode
./teamcode -p "Explain this codebase"

# With debug logging
./teamcode -d
```

## Collaboration Tools

The main coder agent now has first-class collaboration tools:

- `team_create_context` - Create team with charter and roles
- `team_add_role` - Add role definitions
- `team_assign_role` - Assign agents to roles
- `task_create` - Add tasks to the board
- `task_move` - Move tasks between columns
- `handoff_create` - Create task handoffs
- `handoff_accept` - Accept pending handoffs
- `inbox_read` - Read your messages
- `team_message_send` - Send a direct teammate message
- `team_broadcast` - Send a team-wide broadcast
- `team_status` - Inspect team, task, member, handoff, and worker state
- `teammate_spawn` / `teammate_wait` - Launch and monitor persistent teammates
- `subagent_spawn` / `subagent_wait` - Launch and monitor bounded subagents

## Compatibility

TeamCode prefers `.teamcode`, `TeamCode.md`, `teamcode.db`, and `teamcode` theme names, but still reads legacy OpenCode config and memory files so existing installs can migrate without breaking.

## License

MIT
