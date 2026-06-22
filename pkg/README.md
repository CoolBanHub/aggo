# 公共集成包

`pkg/` 存放受支持的公共集成包，面向核心 `agent`、`memory`、`tools`、
`model`、`cron`、`database` 等领域包之外的复用场景。

以下导入路径属于项目公共 API 边界：

| 包 | 用途 |
| --- | --- |
| `github.com/CoolBanHub/aggo/pkg/adapter` | 将 Eino 智能体消息转换为 OpenAI 兼容响应结构。 |
| `github.com/CoolBanHub/aggo/pkg/ailens360` | 为受支持的模型配置接入 AILens360 代理和遥测请求头。 |
| `github.com/CoolBanHub/aggo/pkg/langfuse` | Langfuse 客户端和回调处理器集成。 |
| `github.com/CoolBanHub/aggo/pkg/sse` | 用于 HTTP 流式响应的 SSE 事件和写入器工具。 |

不要把仅供内部使用的辅助代码放到本目录。下游项目不应导入的代码应放入
`internal/`；如果功能属于 `memory`、`tools`、`database`、`cron` 等核心领域，
应直接放在对应领域包内。
