# Command guide

Read this file when selecting or executing an `agentctl` command. Confirm flags against the installed version's `--help` before execution.

## Inspection

| Intent | Command |
|---|---|
| Show build metadata | `agentctl version --json` |
| Check for a newer release without installing it | `agentctl update --check --json` |
| Diagnose platform, configuration, endpoint, and credential presence | `agentctl doctor --json` |
| Show the active configuration path | `agentctl config path --json` |
| Show persisted values only | `agentctl config list --json` |
| Show effective values and their sources | `agentctl config list --effective --json` |
| Read a persisted value | `agentctl config get KEY --json` |
| Read the effective value | `agentctl config get KEY --effective --json` |

Supported configuration keys in this starter are `endpoint`, `timeout`, and `output`.

## Updates

`agentctl update` downloads the release archive for the current operating system and CPU architecture, verifies it against `SHA256SUMS`, and replaces the installed executable in place.

```text
agentctl update --check --json
agentctl update --json
agentctl update --version v1.2.3 --json
```

- Prefer `update --check --json` to report availability; it changes nothing.
- `update` without flags installs the latest release and reports `updated: false` when the installed version is already current.
- `update --version` installs an exact tag and is the only way to request a downgrade or a repair.
- Installing modifies the user's machine, so run it only when the user asked for an update.
- The text mode notice printed after other commands is informational; do not treat it as an error and do not rely on it in automation.

## Persistent mutations

These commands modify the CLI-owned YAML configuration file:

```text
agentctl config set KEY VALUE --json
agentctl config unset KEY --json
```

Validate values through the CLI rather than pre-normalizing them:

- `endpoint`: absolute HTTP or HTTPS URL
- `timeout`: positive Go duration such as `30s` or `2m`
- `output`: `text` or `json`

When a mutation fails, do not edit the YAML file directly as a fallback. Return the structured error and suggest the valid form.

## Demonstration command

```text
agentctl example echo MESSAGE [--upper] --json
```

Use this only to verify starter behavior or demonstrate how a domain command is shaped. Projects derived from the template are expected to replace it.
