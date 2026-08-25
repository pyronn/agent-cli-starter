# Configuration

Read this file when diagnosing effective values or changing persistent settings.

## Precedence

Effective configuration is resolved from highest to lowest priority:

1. Command flags: `--endpoint`, `--timeout`, `--output`
2. Environment: `AGENTCTL_ENDPOINT`, `AGENTCTL_TIMEOUT`, `AGENTCTL_OUTPUT`
3. CLI-owned configuration file
4. Built-in defaults

Use the following command to avoid guessing which layer won:

```text
agentctl config list --effective --json
```

Each item includes a `source` such as `flag`, `environment`, `config`, or `default`.

## Persistence

`agentctl config set` writes only the explicitly supplied key. It must not copy temporary flags or environment values into the file. `config unset` removes a persisted key, allowing lower-priority defaults to become effective again.

The default path comes from the operating system's user configuration directory. Use `agentctl config path --json` instead of constructing a path manually.

An explicit `--config PATH` selects a different file for that invocation. Preserve that flag across related reads and writes in the same task.

## Credentials

`AGENTCTL_TOKEN` is intentionally separate from ordinary configuration and must never be stored with `config set`. `doctor` reports only whether it is present and redacts the value.
