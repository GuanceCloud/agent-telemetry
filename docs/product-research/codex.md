# Codex OTEL 接入调研

## 1. 产品范围

- 产品名称：Codex CLI
- 产品版本：已验证 fixture 中包含 `0.145.0`；精确兼容边界仍需真实版本矩阵确认
- 支持平台：Linux、macOS、Windows
- 目标仓库：`agent-telemetry`
- 调研日期：2026-07-31

## 2. 插件与 Hook 能力

| 项目 | 结论 | 证据 |
| --- | --- | --- |
| Hook | 用户级 Stop Hook | 旧 `codex-otel-plugin` 安装器和 `~/.codex/hooks.json` 写入逻辑 |
| 输入 | stdin JSON，核心字段为 `transcript_path` | `internal/adapters/codex/hook` |
| Transcript | rollout JSONL | parser、collector 和去敏测试 fixture |
| Hook 信任 | `codex app-server` 的 `hooks/list` + `config/batchWrite` | `internal/install/codex_trust.go` |
| 重复与并发 | Stop 可重复/并发触发 | rollout lock、turn claim 和并发测试 |

## 3. 标识、生命周期和数据

| 概念 | 来源 | 降级 |
| --- | --- | --- |
| Session ID | `session_meta.id` | 缺失时保留空值，不生成伪 ID |
| Turn ID | `task_started.turn_id` | 缺失时不写完成 marker |
| LLM 边界 | step、response item、token_count | 时间不足时保持正 duration，不延长到 Tool 结束 |
| Tool Call ID | function/tool call `call_id` | 无 ID 时不推测关联 |
| Subagent | thread ID 与子 rollout | 仅在能找到对应 rollout 时建立父子关系 |
| 完成 | `task_complete` | 非终态不上传 |
| 取消 | abort 事件 | `final_status=cancelled` |
| Token | 每 step 的 `last_token_usage` | root 汇总，LLM 保留单次 usage |

## 4. 架构决策

- 架构：Stop Hook + rollout 全量重放
- OTLP：Go 最小 HTTP/protobuf 编码器
- 内部产物：`invoke_agent`、`llm`、`tool:*`、`skill:*`、`assistant`
- Metrics：仅从同批 Span 派生四个标准指标
- 去重：rollout 文件锁 + `(session_id, turn_id)` 状态 claim
- 部分成功：

```text
claim
  -> traces marker
  -> metrics marker
  -> completed
  -> legacy rollout .gtrace marker
```

旧 `.gtrace` marker 继续用于升级兼容，但只能在 Trace 和 Metrics 都成功后写入。

## 5. 安装与配置

| 文件 | 用途 |
| --- | --- |
| `~/.codex/hooks.json` | Stop Hook |
| `~/.codex/gtrace.json` | endpoint、headers、隐私和资源属性 |
| `~/.codex/state/gtrace-agent` | 兼容旧版的逐 turn 上传状态目录 |
| `~/.local/bin/agent-telemetry` | 所有 Adapter 共享的静态二进制 |

安装器通过 `agent-telemetry install codex` 执行，配置最后写入。升级保留未知 JSON
字段、已有 endpoint、headers、resource attributes 和 enabled；`--no-config`
不修改 `gtrace.json`。

## 6. Fixture 覆盖

- [x] 普通问答
- [x] 多 LLM 与 token
- [x] Tool 与 Skill
- [x] Subagent
- [x] Cancelled
- [x] 非终态
- [x] 重复与并发 Stop
- [x] Trace 成功、Metrics 失败恢复

## 7. 未知项与风险

| 问题 | 临时降级 | 后续验证 |
| --- | --- | --- |
| Codex Hook schema 和信任协议可能随版本变化 | 安装失败不写新启用配置，Hook 运行保持 fail-open | 在目标版本执行安装和升级冒烟 |
| 精确最低支持版本未知 | 文档标为 RC | 建立 Codex 版本矩阵 |
| Codex 运行中可能缓存旧 Hook | 安装后提示重启 | 验证 reload 行为 |
| release 尚未从远程 GitHub 重新下载验证 | 本地 archive + checksum 验证 | 发布 RC 后执行远程安装 |
