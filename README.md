# Agent CLI Starter

一个面向 AI Agent 调用场景的 Go CLI 快速启动模板。它既适合人类在终端中使用，也提供稳定的 JSON 输入输出约定，便于 Claude Code、Codex、自动化脚本或未来的 MCP Server 调用。

模板已实现：

- Cobra 命令树与可测试的业务层
- `命令行参数 > 环境变量 > 配置文件 > 默认值` 的明确配置优先级
- 类似 GitHub CLI 的 `config list/get/set/unset/path`
- Windows、Linux、macOS 原生配置目录
- `--json` / `--output json` 结构化成功与错误输出
- 类型安全的配置校验、来源追踪和无敏感信息的 `doctor`
- 构建版本注入、跨平台 CI 和单元/集成测试

## 快速开始

需要 Go 1.26 或更高版本：

```shell
go mod download
go test ./...
go run ./cmd/agentctl version
go run ./cmd/agentctl example echo "hello agent" --upper
```

Windows 也可以运行构建脚本：

```powershell
.\scripts\build.ps1 -Version 0.1.0
.\dist\agentctl.exe version
```

## 初始化为自己的 CLI

从 GitHub 模板创建仓库或复制本项目后，先运行初始化工具。它会统一修改：

- CLI 命令名和 `cmd/<name>` 目录
- Go module 路径及所有内部 import
- 环境变量前缀
- 跨平台配置目录名
- README 标题和项目描述
- PowerShell 构建产物名
- GitHub Actions 六平台产物名

Windows PowerShell：

```powershell
.\scripts\init.ps1 `
  --name acmectl `
  --module github.com/acme/acmectl `
  --description "Manage Acme resources" `
  --yes
```

Linux 或 macOS：

```shell
bash scripts/init.sh \
  --name acmectl \
  --module github.com/acme/acmectl \
  --description "Manage Acme resources" \
  --yes
```

也可以直接运行跨平台的 Go 初始化工具：

```shell
go run ./tools/init --name acmectl --module github.com/acme/acmectl --yes
```

如果不提供 `--name` 或 `--module`，工具会进入交互模式。环境变量前缀默认根据命令名生成，例如 `acme-agent` 会得到 `ACME_AGENT`；也可以用 `--env-prefix` 显式指定。

初始化工具默认要求 Git 工作区干净，修改完成后会执行 `go mod tidy` 和 `go test ./...`。常用的安全选项：

```shell
# 只预览文件和目录变更
go run ./tools/init --name acmectl --module github.com/acme/acmectl --dry-run

# 跳过初始化后的 Go 验证
go run ./tools/init --name acmectl --module github.com/acme/acmectl --no-verify --yes

# 明确允许修改存在未提交内容的工作区
go run ./tools/init --name acmectl --module github.com/acme/acmectl --force --yes
```

建议在开始编写业务代码前运行一次初始化，并在初始化成功后立即提交结果。

## 安装 CLI

### 下载 GitHub Release（推荐）

普通用户不需要安装 Go。进入仓库的 `Releases` 页面，下载与操作系统和 CPU 架构匹配的压缩包：

| 系统 | x86-64 / amd64 | ARM64 |
|---|---|---|
| Windows | `agentctl-vX.Y.Z-windows-amd64.zip` | `agentctl-vX.Y.Z-windows-arm64.zip` |
| Linux | `agentctl-vX.Y.Z-linux-amd64.tar.gz` | `agentctl-vX.Y.Z-linux-arm64.tar.gz` |
| macOS | `agentctl-vX.Y.Z-darwin-amd64.tar.gz` | `agentctl-vX.Y.Z-darwin-arm64.tar.gz` |

每个 Release 还会提供 `SHA256SUMS`。Linux 可以在下载目录验证：

```shell
sha256sum --ignore-missing -c SHA256SUMS
```

Windows PowerShell 可以把输出与 `SHA256SUMS` 中对应文件的值比较：

```powershell
Get-FileHash .\agentctl-v1.2.3-windows-amd64.zip -Algorithm SHA256
```

### 从源码安装

克隆项目后，在仓库根目录执行：

```shell
go install ./cmd/agentctl
agentctl version
agentctl doctor
```

`go install` 会把可执行文件放入 `go env GOBIN`；如果 `GOBIN` 为空，则放入 `$(go env GOPATH)/bin`。请确保该目录已经加入系统的 `PATH`。

模板替换为自己的模块路径并发布到 GitHub 后，也可以直接安装指定版本：

```shell
go install github.com/your-org/your-repo/cmd/agentctl@latest
```

建议生产环境使用明确版本，避免安装结果随 `latest` 变化：

```shell
go install github.com/your-org/your-repo/cmd/agentctl@v1.2.3
```

### 下载 GitHub Actions 构建产物

CI 会在测试通过后生成以下 6 个产物：

| 系统 | x86-64 / amd64 | ARM64 |
|---|---|---|
| Windows | `agentctl-windows-amd64.exe` | `agentctl-windows-arm64.exe` |
| Linux | `agentctl-linux-amd64` | `agentctl-linux-arm64` |
| macOS | `agentctl-darwin-amd64` | `agentctl-darwin-arm64` |

在 GitHub 仓库中打开 `Actions` → 选择一次成功的 `ci` 运行 → 在 `Artifacts` 区域下载对应系统和 CPU 架构的压缩包。可以用以下命令确认本机架构：

```shell
go env GOOS GOARCH
```

Windows PowerShell 安装示例：

```powershell
$UserBin = Join-Path $HOME "bin"
New-Item -ItemType Directory -Force $UserBin
Move-Item .\agentctl-windows-amd64.exe (Join-Path $UserBin "agentctl.exe")
```

把 `%USERPROFILE%\bin` 加入用户 `PATH`，重新打开终端后验证：

```powershell
agentctl version
agentctl doctor --json
```

Linux amd64 安装示例：

```shell
mkdir -p ~/.local/bin
install -m 0755 agentctl-linux-amd64 ~/.local/bin/agentctl
agentctl version
```

macOS Apple Silicon 安装示例：

```shell
mkdir -p ~/.local/bin
install -m 0755 agentctl-darwin-arm64 ~/.local/bin/agentctl
agentctl version
```

Linux 和 macOS 需要确保 `~/.local/bin` 位于 `PATH` 中：

```shell
export PATH="$HOME/.local/bin:$PATH"
```

当前 Release 和 CI 产物没有代码签名。macOS 或企业 Windows 环境可能会提示来源未知；正式发布时应配置平台签名，不建议要求用户长期关闭系统安全检查。

## 发布版本

Release 工作流只接受带 `v` 前缀的语义化版本标签，例如 `v1.2.3` 或 `v1.2.3-rc.1`。确认 `main` 分支测试通过后创建并推送标签：

```shell
git switch main
git pull --ff-only
git tag -a v1.2.3 -m "Release v1.2.3"
git push origin v1.2.3
```

推送后，[release 工作流](.github/workflows/release.yml) 会自动：

1. 校验 SemVer 标签并运行 `go test`、`go vet`。
2. 构建 Windows、Linux、macOS 的 amd64/arm64 二进制。
3. 将 Windows 产物打包为 ZIP，将 Linux/macOS 产物打包为 `tar.gz`。
4. 汇总产物并生成 `SHA256SUMS`。
5. 使用标签创建 GitHub Release、生成 release notes 并上传全部附件。

包含连字符的版本（例如 `v1.2.3-rc.1`）会自动标记为 Pre-release。发布任务使用仓库自带的 `GITHUB_TOKEN`，所需权限限定为发布任务的 `contents: write`。

## 配置命令

```shell
agentctl config path
agentctl config list
agentctl config set endpoint https://api.example.com
agentctl config set timeout 45s
agentctl config set output json
agentctl config get endpoint
agentctl config unset timeout
```

只看配置文件中实际保存的值：

```shell
agentctl config list
```

查看最终生效值和每个值的来源：

```shell
agentctl config list --effective
agentctl config list --effective --json
```

默认配置文件位置由 `os.UserConfigDir()` 决定：

| 平台 | 典型路径 |
|---|---|
| Windows | `%AppData%\agentctl\config.yaml` |
| macOS | `~/Library/Application Support/agentctl/config.yaml` |
| Linux | `~/.config/agentctl/config.yaml` |

测试、CI 或多实例场景可以显式指定：

```shell
agentctl --config ./config.dev.yaml config list
```

## 配置优先级

从高到低：

1. 命令行参数：`--endpoint`、`--timeout`、`--output`
2. 环境变量：`AGENTCTL_ENDPOINT`、`AGENTCTL_TIMEOUT`、`AGENTCTL_OUTPUT`
3. CLI 自己维护的配置文件
4. 代码内默认值

例如：

```powershell
agentctl config set timeout 30s
$env:AGENTCTL_TIMEOUT = "45s"
agentctl --timeout 1m config list --effective
```

最终 `timeout` 是 `1m0s`，来源为 `flag`。

认证令牌不属于普通配置项。模板约定从 `AGENTCTL_TOKEN` 读取，避免 `config list` 或配置文件意外泄漏凭据。生产项目可把它替换为操作系统 Keyring、OAuth 登录或云厂商凭据链。

## Agent 友好的输出协议

成功响应统一放在 `data` 中：

```shell
agentctl example echo hello --json
```

```json
{
  "data": {
    "message": "hello",
    "length": 5
  }
}
```

错误写入 stderr，并使用非零退出码：

```json
{
  "error": {
    "code": "usage_error",
    "message": "unknown configuration key \"missing\""
  }
}
```

约定的退出码：

| 退出码 | 含义 |
|---|---|
| `0` | 成功 |
| `1` | 运行失败或资源不存在 |
| `2` | 参数或配置错误 |

Agent 应优先判断退出码，再解析 stdout 或 stderr 的 JSON，不要依赖自然语言文本。

## 项目结构

```text
.
├── cmd/agentctl/          # 最薄的可执行程序入口
├── internal/buildinfo/    # 版本和构建元数据
├── internal/cli/          # Cobra 命令、输出选择和退出码
├── internal/config/       # 配置文件、校验、优先级和来源追踪
├── internal/output/       # 稳定的 JSON envelope
├── internal/service/      # 与 CLI 框架解耦的业务逻辑
├── scripts/build.ps1      # Windows 本地构建
└── .github/workflows/     # 三平台 CI
```

## 基于模板开发自己的 CLI

1. 全局替换模块路径 `github.com/example/agent-cli-starter`。
2. 修改 `appName`、配置目录名和 `AGENTCTL_` 环境变量前缀。
3. 在 `internal/config` 中添加允许的配置项和校验规则。
4. 用你的领域命令替换 `example echo`，把业务逻辑放在 `internal/service`。
5. HTTP 客户端只接收已经解析好的 `config.Values`，不要在业务层再次读取环境变量。
6. 保持 JSON 字段和错误码向后兼容；破坏性变更应提升主版本号。

新增命令时推荐保持以下依赖方向：

```text
Cobra command -> service interface -> API / filesystem / database
       |
       +-> effective config
       +-> output envelope
```

这样将来增加 MCP 入口时，可以直接复用 service，而不需要从 CLI 命令中抽取业务逻辑。

## 发布跨平台单文件

Go 可以直接交叉编译：

```powershell
$env:GOOS = "linux";   $env:GOARCH = "amd64"; go build -o dist/agentctl-linux-amd64 ./cmd/agentctl
$env:GOOS = "darwin";  $env:GOARCH = "arm64"; go build -o dist/agentctl-darwin-arm64 ./cmd/agentctl
$env:GOOS = "windows"; $env:GOARCH = "amd64"; go build -o dist/agentctl-windows-amd64.exe ./cmd/agentctl
```

在 SSH 自动化中建议执行单次、非交互命令：

```shell
ssh windows-node 'agentctl doctor --json'
ssh windows-node 'agentctl example echo hello --json'
```

生产发布还应增加校验和、签名、SBOM，并把版本、提交哈希和构建时间通过 `-ldflags` 注入。
