# Output contract

Read this file when scripting the CLI or interpreting command results.

## Success

With `--json`, successful commands write one JSON document to stdout and exit `0`:

```json
{
  "data": {}
}
```

The shape inside `data` depends on the command. Do not infer success from a domain field; use the process exit code.

## Failure

JSON failures write one error document to stderr and leave stdout empty:

```json
{
  "error": {
    "code": "usage_error",
    "message": "human-readable detail"
  }
}
```

Exit codes:

| Code | Meaning | Agent behavior |
|---|---|---|
| `0` | Success | Parse stdout |
| `1` | Runtime failure or missing resource/value | Parse stderr and report the actionable detail |
| `2` | Invalid arguments or configuration | Correct only when the intended value is unambiguous; otherwise ask the user |

Keep stdout and stderr separate. Do not combine them before JSON parsing.

## Update notices

In text mode, a successful command may append a short update notice to stderr when a newer release exists:

```text
A new version of agentctl is available: v1.2.3 (current v1.2.0).
Run "agentctl update" to install it.
```

It is not an error: judge success by the exit code. `--json` never emits this notice or any download progress, so structured streams are never mixed with human text. Set `AGENTCTL_NO_UPDATE_CHECK=1` or pass `--no-update-check` to suppress the notice entirely.

## Output selection

`--json` is shorthand for `--output=json`. Do not combine `--json` with `--output=text`. For agent calls, prefer `--json` even if the user's stored default is text.
