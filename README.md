# Agent Telemetry

`agent-telemetry` 是面向多种 AI Agent 的统一可观测插件。它由一个静态 Go
二进制提供安装、发现、状态管理、Hook 采集、OpenTelemetry Trace/Metrics
转换和上传能力。

Claude、Codex 等产品只是内置 Adapter，不再作为独立插件发布或升级。

## 当前支持

| Adapter | 采集方式 | 平台 | 运行时依赖 |
| --- | --- | --- | --- |
| Claude Code | `Stop` / `SessionEnd` Hook + transcript 回放 | Linux、macOS、Windows | 无 |
| Codex | `Stop` Hook + rollout 回放 | Linux、macOS、Windows | 无 |

后续 Qoder、WorkBuddy、OpenCode、OpenClaw 和 Hermes 接入同一个二进制，不再建立
新的插件仓库。

## 安装

从源码构建：

```bash
make build
./output/agent-telemetry version
```

安装唯一插件并自动接入本机已发现的 Agent：

```bash
./output/agent-telemetry install \
  --type gtrace \
  --endpoint https://llm-openway.guance.com \
  --x-token '<token>' \
  --enable
```

只接入指定 Adapter：

```bash
./output/agent-telemetry install codex \
  --type otlp \
  --endpoint http://127.0.0.1:4318 \
  --enable
```

重复执行 `install` 会替换共享二进制、校准 Hook，并保留没有显式覆盖的已有配置。
统一模型中没有 `update codex`：升级的是整个 `agent-telemetry`，不是某个独立插件。

从解压后的发布包安装：

```bash
./scripts/install.sh \
  --type gtrace \
  --endpoint https://llm-openway.guance.com \
  --x-token '<token>' \
  --enable
```

Windows：

```powershell
.\scripts\install.ps1 --type gtrace --endpoint https://llm-openway.guance.com --x-token '<token>' --enable
```

安装后共享运行时位于：

```text
~/.local/bin/agent-telemetry
```

## 管理

```bash
agent-telemetry discover
agent-telemetry status
agent-telemetry status codex
agent-telemetry enable codex
agent-telemetry disable codex
agent-telemetry uninstall codex
agent-telemetry uninstall --purge
```

- `discover` 只发现和显示状态，不修改系统。
- `uninstall codex` 只移除 Codex Adapter Hook，默认保留配置。
- `uninstall` 移除所有 Adapter Hook 和共享运行时。
- `--purge` 额外删除相应 Adapter 的配置和状态。

完整安装参数：

```bash
agent-telemetry install --help
```

隐私敏感环境可关闭内容采集：

```bash
agent-telemetry install \
  --type otlp \
  --endpoint http://127.0.0.1:4318 \
  --capture-content none \
  --enable
```

## Hook

安装器按检测结果注册：

```text
agent-telemetry hook claude
agent-telemetry hook codex
```

升级时会识别并替换旧 `gtrace-agent`、`claude-otel-plugin` 和
`codex-otel-plugin` Hook，其他 Hook 与未知配置保持不变。

## 配置

运行配置继续放在 Agent 自己的配置目录，避免 Hook 依赖管理工具常驻：

```text
~/.claude/gtrace.json
~/.codex/gtrace.json
```

最小配置：

```json
{
  "enabled": true,
  "endpoint": "http://127.0.0.1:4318",
  "tracePath": "v1/traces",
  "metricsPath": "v1/metrics",
  "captureContent": "preview"
}
```

`install --no-config` 只安装共享运行时并校准 Hook，不修改已有运行配置。

## 数据模型与可靠性

```text
Agent 生命周期 / transcript
          │
          ▼
internal/adapters/<agent>
          │
          ▼
internal/core/model
          │
          ├── semantic
          ├── metrics
          ├── privacy
          ├── state
          └── transport
```

根 Span 为 `invoke_agent`；`llm`、`tool:*`、`assistant` 直接挂根，
`skill:*` 只在有可靠证据时挂到对应工具。只上传终态 turn。

每个 turn 使用独立状态：

```text
claim -> traces -> metrics -> completed
```

Trace 成功而 Metrics 失败时，下次 Hook 只重试 Metrics。采集与上传默认
fail-open，不阻塞宿主 Agent。

## 发布

```bash
./scripts/build-release.sh
```

脚本生成 Linux、macOS、Windows 的 amd64/arm64 静态发布包。每个平台归档只包含
一个 `agent-telemetry` 二进制、一个统一 manifest 和一套安装脚本。

## 验证

```bash
go test ./...
go vet ./...
go test -race ./...
bash /home/liurui/.codex/skills/agent-otel-plugin/scripts/validate-plugin.sh .
```
