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

Developers with Go may still use `go install`, but it is not required for normal users.
