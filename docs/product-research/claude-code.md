# Claude Code OTEL 插件接入调研

## 1. 产品范围

- 产品名称：Claude Code
- 产品版本：transcript payload 可提供 `version`；当前未锁定最低/最高兼容版本
- 支持平台：Linux、macOS、Windows
- 目标插件仓库：`agent-telemetry`
- 调研日期：2026-07-31

## 2. 插件能力

| 项目 | 结论 | 证据 |
| --- | --- | --- |
| 原生插件/扩展机制 | Claude Plugin Hook | 旧仓库 `.claude-plugin/plugin.json`、`hooks/hooks.json` |
| Hook 列表 | `Stop`、`SessionEnd` | 旧仓库 fixture 与本仓库安装器测试 |
| Hook 输入方式 | stdin JSON，含 session、transcript、cwd、event | 旧 Python Hook 测试与 `internal/adapters/claude/hook` |
| Hook 超时和失败行为 | manifest 为 60 秒；采集器 fail-open | Hook manifest、`RunCLI` 始终返回 0 |
| 并发/重复触发 | 允许按重复触发设计，必须竞争 claim | `internal/core/state` 与并发 Hook 测试 |
| 重放行为 | 每次读取完整 transcript，再按稳定 turn ID 去重 | `internal/adapters/claude/parse`、`hook` |

## 3. 数据源

| 数据源 | 路径或入口 | 格式 | 生命周期 | 敏感性 |
| --- | --- | --- | --- | --- |
| Hook | stdin | JSON | Stop / SessionEnd | 中 |
| Transcript | `transcript_path` | JSONL | 会话内持续追加 | 高 |
| Session snapshot | 未使用 | - | - | - |
| SQLite/其他 | 未使用 | - | - | - |

## 4. 标识与关联

| 概念 | 原始字段 | 稳定性 | 回退策略 |
| --- | --- | --- | --- |
| Session ID | Hook `session_id` / `session.id` | 高 | 缺失则不上传 |
| Turn/Request ID | 用户消息 `message.id` 或 `uuid` | 中高 | session、序号、时间、正文的哈希 |
| Message ID | `message.id` / `uuid` | 高 | assistant 使用 turn 内序号 |
| LLM Call ID | assistant `message.id` | 高 | turn ID + LLM 序号 |
| Tool Call ID | `tool_use.id` | 高 | 无 ID 时仍解析但不建立结果关联 |
| Parent/Subagent ID | 当前 transcript 未证明 | 未知 | 不生成推测的 subagent 关系 |

## 5. 生命周期

- 用户 turn 开始证据：非 meta、非 tool result 的 `user` 消息
- 完成证据：下一用户 turn、`turn_duration`，或 Stop/SessionEnd 下的最终文本/完成 stop reason
- 取消证据：当前样本未证明可靠字段
- 错误证据：`isApiErrorMessage`、`apiErrorStatus`、`API Error:` 文本、tool result `is_error`
- 内部 title/summary/heartbeat/review 的识别方式：`isMeta=true` 不作为新 turn
- transcript 写入与 Hook 的先后关系：Hook 触发后等待文件稳定；不完整 JSONL 尾行忽略，下次全量回放恢复

```text
user
  └─ assistant/LLM (0..n)
       ├─ tool_use ─ tool_result
       └─ final text
            └─ turn_duration / Stop / SessionEnd
                   └─ normalize -> claim -> traces -> metrics -> completed
```

## 6. LLM 与 Token

| 字段 | 来源 | 单次/累计 | 可用性与限制 |
| --- | --- | --- | --- |
| Provider | 固定 `anthropic` | 单次 | transcript 无独立 provider 字段 |
| Request model | assistant `message.model` | 单次 | 可用 |
| Response model | assistant `message.model` | 单次 | 当前与 request 相同 |
| Input token | `usage.input_tokens` + 两类 cache input | 单次 | turn 汇总为各 LLM 相加 |
| Output token | `usage.output_tokens` | 单次 | 可用 |
| Cache read token | `usage.cache_read_input_tokens` | 单次 | 可用时上报 |
| Reasoning token | 未证明 | - | 不上报 |
| Finish reason | `message.stop_reason` | 单次 | 可用 |
| Start/end | user、assistant、tool result 时间 | 推断 | LLM 开始时间不是原生 API 时钟 |
| TTFT | 无 | - | 不推测 |

## 7. Tool、Skill 与 Subagent

- Tool before/after：assistant `tool_use` 与后续 user `tool_result`
- Tool result/error：`tool_result.content`、`tool_result.is_error`
- Command 提取：tool input 的 `cmd` 或 `command`
- Skill 高置信度证据：产品级 `Skill` 工具及其 `input.skill`
- Subagent 模型：当前数据未证明
- 可用父子关联：Tool 保存触发它的 LLM call ID；Skill 挂对应 `Skill` Tool

## 8. 安装与配置

| 平台 | 产品 HOME | Config root | 插件目录 | 重载/重启 |
| --- | --- | --- | --- | --- |
| Linux | 用户 HOME | `~/.claude` | Claude 插件目录或用户 Hook | 新会话验证 |
| macOS | 用户 HOME | `~/.claude` | Claude 插件目录或用户 Hook | 新会话验证 |
| Windows | UserProfile | `.claude` | Claude 插件目录或用户 Hook | 新会话验证 |

- 官方安装 CLI：本阶段不假设具体 marketplace 命令；统一发布包提供 `agent-telemetry install claude`
- Marketplace/Registry：旧仓库为本地 Claude plugin manifest
- 产品运行时配置写回：幂等更新 `.claude/settings.json`，保留未知字段与非 GTrace Hook
- 原生 OTEL/旧插件冲突：安装时替换旧 Python/GTrace Claude Hook，避免重复
- 敏感配置存储：沿用环境变量或 `.claude/gtrace.json`；日志和状态不写入 headers/token

## 9. 架构决策

- 选择：终态 Hook + transcript 全量重放
- OTLP：Go 最小 protobuf 编码器
- 主要参考仓库：`claude-otel-plugin`、`codex-otel-plugin`
- 选择理由：彻底移除 venv/pip/OpenTelemetry Python SDK，并统一语义和恢复机制
- 缺失事件降级：终态不可靠则跳过；Stop 下仅在无未完成工具且有最终证据时推断完成
- 去重主键：session ID + turn ID 的哈希目录
- 部分信号恢复：独立记录 `traces.json`、`metrics.json`，全部成功后写 `completed.json`

## 10. 字段映射

| 产品字段/事件 | 内部模型 | OTEL Span/Attribute | 备注 |
| --- | --- | --- | --- |
| Hook `session_id` | Turn.SessionID | `gen_ai.conversation.id`、`session_id` | |
| 用户消息 | Turn input | `invoke_agent` / `gen_ai.input.messages` | 可关闭内容采集 |
| assistant message | LLMCall | `llm` | 同 ID 快照合并 |
| assistant text | AssistantOutput | `assistant` | 不携带 token |
| `tool_use` / `tool_result` | ToolCall | `tool:<name>` | Tool 直挂根 |
| `Skill` tool | SkillUse | `skill:<name>` | Skill 挂 Tool |
| message usage | Usage | `gen_ai.usage.*` | root 为各 LLM 合计 |
| `turn_duration` | Turn end | `gen_ai.workflow.duration` | recorded timing |

## 11. Fixture

所有 fixture 均为人工去敏数据。

- [x] 普通问答
- [x] 多 LLM
- [x] Tool 成功
- [x] Tool 失败
- [x] Error
- [x] Incomplete transcript
- [x] Skill
- [ ] Subagent
- [x] 重复 Hook

## 12. 未知项与风险

| 问题 | 影响 | 临时降级 | 后续验证 |
| --- | --- | --- | --- |
| Claude Code 版本兼容边界未锁定 | transcript schema 变化可能漏采 | 宽松字段别名、未知行忽略 | 建立多版本去敏 fixture |
| 可靠取消字段未证明 | cancelled turn 可能不上传 | 不把未知终态猜成 cancelled | 采集真实取消样本 |
| Subagent 关联未证明 | 无子 Agent 树 | 不生成推测关系 | 调研 Agent/Subagent Hook |
| LLM 原生开始时间和 TTFT 不可得 | LLM duration 为推断 | 标记 `timing.source=inferred` | 若产品新增生命周期则切换 |
| 用户 settings Hook schema 的跨版本兼容 | 直装入口可能随产品变化 | 同时保留标准 plugin 包 | 在发布矩阵验证 Claude 版本 |
