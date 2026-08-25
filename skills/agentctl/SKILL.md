---
name: agentctl
description: Use agentctl to inspect its installation and effective configuration, read or persist supported settings, diagnose setup problems, or run its domain commands. Use when the user asks to operate or troubleshoot this CLI; do not use for developing the Go template itself.
metadata:
  requires:
    bins: ["agentctl"]
---

# agentctl

Operate `agentctl` through its installed binary. Treat the current binary's help as the source of truth because a user's installed version may differ from this skill.

## Workflow

1. Confirm availability with `agentctl version --json`. If the executable is missing, stop and explain how to install it; do not silently build or download software.
2. Before using an unfamiliar command or flag, run `agentctl --help` and then the relevant subcommand's `--help`. Never invent commands from the user's wording.
3. Prefer `--json` for automation. Judge success by exit code before parsing stdout or stderr.
4. Use `agentctl doctor --json` for setup failures and `agentctl config list --effective --json` when configuration precedence matters.
5. Read the relevant reference before executing:
   - Command selection and mutation behavior: [references/commands.md](references/commands.md)
   - JSON envelopes, streams, and exit codes: [references/output-contract.md](references/output-contract.md)
   - Configuration precedence, persistence, and secrets: [references/configuration.md](references/configuration.md)

## Constraints

- `config set` and `config unset` persist user configuration. Execute them only when the user asked to change configuration, and report the changed key without exposing secrets.
- Never print `AGENTCTL_TOKEN` or include its value in commands, logs, or replies. This starter reads it only for credential presence checks.
- Pass `--config` only when the user specifies an alternate file or the task clearly targets an isolated configuration. Otherwise use the platform-native default.
- `example echo` is demonstration scaffolding, not a general-purpose domain capability.
- If help output and these files disagree, follow help output and mention the version mismatch.
