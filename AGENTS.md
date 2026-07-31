# Agent Telemetry

本仓库是唯一 `agent-telemetry` 插件及其多 Agent Adapter 的 Go 单一事实源。

## 核心约束

- 始终使用简体中文沟通。
- 目标运行时不依赖 Node.js、Python、Go toolchain 或外部 OTel SDK。
- 只发布一个 `agent-telemetry` 二进制和一个版本；产品差异是内置 Adapter，不是独立插件。
- 不提供按 Adapter 更新命令；重复安装升级共享运行时并保留已有配置。
- 公共能力放入 `internal/core`，产品差异放入 `internal/adapters/<product>`。
- 先归一化内部 Turn，再构建 Span；Metrics 必须从同批 Span 派生。
- 只上传 `completed` 或 `cancelled` 终态，跳过空白和中间态。
- `llm`、`tool:*`、`assistant` 直接挂到 `invoke_agent`；`skill:*` 只挂到有可靠证据的 `tool:*`。
- 默认 fail-open，采集和上传失败不能阻塞宿主 Agent。
- 状态必须分别记录 claim、traces、metrics 和 completed，部分成功可以恢复。
- 禁止记录凭据、敏感 header、未经裁剪的正文或真实用户 fixture。
- 安装和升级必须保留未知配置以及已有 endpoint、headers、enabled 和隐私选项。

## 验证

```bash
gofmt -w cmd internal
go test ./...
go vet ./...
go test -race ./...
```
