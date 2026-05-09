# Security Policy

Agency runs local daemons, talks to model providers, and can read local provider credentials, so security reports matter.

## Supported Version

The supported security target is the current `main` branch until tagged releases are introduced.

## Reporting A Vulnerability

Please do not open a public issue for a suspected credential leak, sandbox escape, command execution bug, or provider-auth vulnerability.

Use GitHub private vulnerability reporting if it is enabled for the repository. If it is not available, contact the maintainer through the GitHub profile and include:

- A concise description of the issue.
- Steps to reproduce.
- Affected OS and shell.
- Whether secrets, filesystem access, network access, or model-provider credentials are involved.

## Local Secret Hygiene

Agency intentionally ignores `.env`, `.tmp`, generated binaries, proof logs, and local agent state. Codex execution uses a read-only sandbox unless `AGENCY_CODEX_UNSANDBOXED=true` is explicitly set.
