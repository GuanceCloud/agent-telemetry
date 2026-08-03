# obs-agent-connector 融合说明

`obs-agent-connector` 的控制面能力正在收敛到唯一的 `agent-telemetry` 插件模型。
当前第一阶段已经完成 Claude 和 Codex；Hermes、OpenCode、OpenClaw、Qoder 和
WorkBuddy 在对应 Adapter 迁入前仍由旧 Connector 管理。

## 概念映射

| 原 Connector 概念 | 统一后的概念 |
| --- | --- |
| 独立 Agent Plugin | 内置 Adapter |
| `install codex` 下载 Codex 插件 | 注册共享运行时的 Codex Adapter |
| `update codex` | 删除；重复 `install` 升级共享运行时 |
| Plugin cache/marketplace marker | Agent Hook 注册状态 |
| 多个插件 release | 一个 `agent-telemetry` release |

## 命令迁移

```text
obs-agent-connector discover       -> agent-telemetry discover
obs-agent-connector status codex   -> agent-telemetry status codex
obs-agent-connector install codex  -> agent-telemetry install codex
obs-agent-connector enable codex   -> agent-telemetry enable codex
obs-agent-connector disable codex  -> agent-telemetry disable codex
obs-agent-connector remove codex   -> agent-telemetry uninstall codex
obs-agent-connector update codex   -> agent-telemetry install
```

统一安装器不会打印 X-Token。Claude 和 Codex 不再通过远程下载并执行独立安装
脚本，其注册与配置由同一进程内 Go 安装库完成。

## 兼容策略

- 重装时识别并替换旧 `gtrace-agent`、Claude Python Hook 和旧 Codex Hook；
- 重装时仍保留 `gtrace.json`、endpoint、headers、enabled 和隐私设置；
- 默认卸载会删除托管 Hook 和对应 `gtrace.json`，`--purge` 再删除状态目录；
- 旧逐 turn 状态目录暂时保留，避免升级后重复上报历史 turn。
