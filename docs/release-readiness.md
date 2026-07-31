# v0.3.0-rc.1 发布验收

- [x] 单一 `agent-telemetry` manifest、二进制和发布版本
- [x] Claude/Codex 作为内置 Adapter，不再生成独立插件包
- [x] `discover`、`status`、`enable`、`disable` 和 `uninstall`
- [x] 重复 `install` 升级共享运行时并保留 Adapter 配置
- [x] 旧 `gtrace-agent` 与独立插件 Hook 可识别和迁移
- [x] `go test ./...`
- [x] `go vet ./...`
- [x] `go test -race ./...`
- [x] Claude 配置优先级和显式关闭
- [x] transcript 不完整尾行恢复
- [x] assistant 快照合并
- [x] Tool 成功、失败和 recorded duration
- [x] Skill 只由高置信度产品工具生成
- [x] root / LLM token 语义
- [x] `captureContent=none`
- [x] 终态过滤与未完成 Tool 跳过
- [x] 并发 claim 去重
- [x] Trace 成功、Metrics 失败后的单信号重试
- [x] OTLP Trace/Metrics protobuf 编解码互操作测试
- [x] Linux/macOS/Windows amd64/arm64 静态交叉编译
- [x] 发布包 SHA256 校验
- [x] 统一 manifest 和安装脚本不含 Python、pip、venv、Node.js
- [x] 安装器幂等并保留未知 Claude 设置
- [x] Codex Stop Hook 幂等注册与 app-server 信任握手
- [x] Codex `--no-config`、enable/disable、headers 和 resource attributes 合并
- [x] Codex Trace 成功、Metrics 失败后只重试 Metrics
- [x] Codex 旧 `.gtrace` 标记仅在双信号成功后写入
- [x] 无明显硬编码凭据

发布前仍需在真实 Claude Code 和 Codex 环境做一次统一安装、终态 Hook、旧版迁移
和卸载冒烟。
