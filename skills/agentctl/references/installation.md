# Native installation

Read this file when `agentctl` is missing or the user asks how to install or update it.

The supported end-user installation uses the native executable published in GitHub Releases. It does not require Go, Node.js, npm, Python, or a package manager.

Linux or macOS:

```sh
curl -fsSL https://raw.githubusercontent.com/pyronn/agent-cli-starter/main/scripts/install.sh | sh
```

Windows PowerShell:

```powershell
irm https://raw.githubusercontent.com/pyronn/agent-cli-starter/main/scripts/install.ps1 | iex
```

The installers detect OS and CPU architecture, download the corresponding Release archive, verify it against `SHA256SUMS`, install it in a user-owned directory, and report the installed version. The PowerShell installer adds its user installation directory to `PATH`; the shell installer prints the required PATH command when necessary.

Do not execute either installer without user authorization. If the user prefers to inspect downloaded code before execution, direct them to download the script first and run it locally.

## Updating an installation

An installed executable updates itself without the install scripts:

```text
agentctl update --check --json
agentctl update --json
agentctl update --version v1.2.3 --json
```

`update` resolves the latest released tag, downloads the archive for the current platform, verifies `SHA256SUMS`, and replaces the running executable. It needs write access to the directory that holds the executable; when that is missing it exits with code `1` and the underlying error, and the user should fall back to reinstalling with the native installer.

Replacements are explicit user actions. Report `update --check` first unless the user already asked to upgrade, and never perform a downgrade without confirmation.

Developers with Go may still use `go install`, but it is not required for normal users.
