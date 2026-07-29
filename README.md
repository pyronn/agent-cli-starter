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
