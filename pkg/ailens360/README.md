# ailens360

为 [AILens360](https://github.com/CoolBanHub/ailens360) 反向代理写的 Go 客户端胶水层。给 Eino 风格的 OpenAI / Agentic OpenAI ChatModel 套上 BaseURL 改写和每请求遥测请求头注入。

## AILens360 是什么

AILens360 是开源、自部署、低代码侵入的 AI 应用可观测平台 —— "360° observability for every LLM call"。

核心做法：在你的应用和真实上游（OpenAI / Anthropic / Gemini / DeepSeek / vLLM / Ollama / ……）之间放一个反向代理。你只需要：

1. 把 `baseURL` 从 `https://api.openai.com/v1` 改成 `http(s)://<ailens360-host>/https://api.openai.com/v1` —— 代理 origin 后面 `/` **直接拼完整上游 URL**（不再有 `/p` 前缀）
2. 加一个请求头 `X-AILens-Project-Key: <控制台分配的项目密钥>`
3. 原本的 `Authorization` / `x-api-key` **透传给上游**，AILens360 不持有、不存储真实 API Key

之后所有调用都会被记录：请求/响应原文、流式 SSE 解析、token / cost 统计、TTFT / TTFB / TPS 等延迟指标、错误归因；按 Project / User / Session / Trace 维度聚合检索。

没有 SDK 强依赖，多数 OpenAI 兼容客户端只改一行 `baseURL` 就接入了。

## 这个包做什么

直接在 Go 里改两件事会很啰嗦——拼前缀 URL、给 `*http.Client.Transport` 套一层 RoundTripper、再让 RoundTripper 从 `req.Context()` 取 user/session/trace。本包把它们封成 4 个文件：

| 文件 | 内容 |
|---|---|
| `context.go` | ctx key + `WithUser` / `WithSession` / `WithTag` / `WithTraceID` / `WithTraceName` / `WithTrace`；`CurrentTrace(ctx)` 反查 |
| `transport.go` | `telemetryHeaders` RoundTripper，固定写入 `X-AILens-Project-Key`，按需从 ctx 写入 5 个 `X-AILens-*` |
| `decorator.go` | `Decorator{proxyPrefix, projectKey}`：`DecorateBaseURL(upstream)` 拼前缀；`HTTPClient(base)` 包出带 RoundTripper 的 `*http.Client`；`SetGlobal` / `Global` 进程级单例 |
| `apply.go` | `Decorator.Apply(*openai.ChatModelConfig)` / `Decorator.ApplyAgentic(*agenticopenai.ChatConfig)`：一行同时改写 `BaseURL` 和 `HTTPClient`，`nil` 时是 no-op；`ApplyGlobal` / `ApplyGlobalAgentic` 走全局单例 |

## 一分钟接入

```go
import (
    "context"

    "github.com/CoolBanHub/aggo/pkg/ailens360"
    "github.com/cloudwego/eino-ext/components/model/agenticopenai"
    "github.com/cloudwego/eino/schema"
)

func main() {
    // 1) 配置缺一即 nil；nil 时 Apply 直接 no-op，应用照常跑、不走代理。
    dec := ailens360.NewDecorator(
        "https://ailens360.example.com", // 控制台给的 proxy_prefix（proxy 进程 origin，无 /p 后缀）
        "<64-char project_key>",
    )

    // 2) 像平时一样构造 Agentic OpenAI ChatModel，ApplyAgentic 之后 BaseURL 被改成
    //    "<proxy>/<upstream>"，HTTPClient 自动注入遥测请求头。
    cfg := &agenticopenai.ChatConfig{
        APIKey:  "sk-real-upstream-key", // 透传给上游
        BaseURL: "https://api.openai.com/v1",
        Model:   "gpt-4o-mini",
    }
    dec.ApplyAgentic(cfg)

    chatModel, _ := agenticopenai.NewChatModel(context.Background(), cfg)

    // 3) 调用前把 user / session / trace 塞进 ctx；
    //    同一个 chatModel 实例可服务任意调用方，不用每次重建。
    ctx := ailens360.WithTrace(context.Background(), ailens360.TraceConfig{
        ID:        "trace_xxx", // 想要稳定 trace 就自己生成（建议 SHA1(turn 关键参数)）
        Name:      "customer_agent_turn",
        UserID:    "user_alice_42",
        SessionID: "group_1:user_alice_42",
        Tag:       "prod,channel=htsy",
    })

    _, _ = chatModel.Generate(ctx, []*schema.AgenticMessage{
        schema.UserAgenticMessage("hello"),
    })
}
```

完整示例见 [`example/ailens360_test/`](../../example/ailens360_test)，里面是一个使用 ReAct 工具调用和 AILens360 trace 的端到端例子。

## ctx 请求头对照表

`telemetryHeaders.RoundTrip` 每次出站请求都会读 `req.Context()`，按下表把非空值写入请求头：

| ctx 写入 | 发出的请求头 | AILens360 控制台用途 |
|---|---|---|
| `WithUser(ctx, v)` | `X-AILens-User` | 按用户聚合 / 检索 |
| `WithSession(ctx, v)` | `X-AILens-Session` | 把同一会话/对话内的多次调用串起来 |
| `WithTag(ctx, v)` | `X-AILens-Tag` | 自由标签，逗号分隔（env / channel / 实验组） |
| `WithTraceID(ctx, v)` | `X-AILens-Trace-Id` | 同一业务 turn 多次模型调用共享，trace 视图合并显示 |
| `WithTraceName(ctx, v)` | `X-AILens-Trace-Name` | trace 的展示名 |

`WithTrace(ctx, TraceConfig{…})` 是上面 5 个的一次性聚合写法。

固定不动的 `X-AILens-Project-Key` 由 `NewDecorator` 时传入，每个请求都会自动加，**和 ctx 无关**。

## 全局与局部

```go
// 进程级单例：服务启动时设一次，业务代码不感知。
ailens360.SetGlobal(dec)

// 任意 agenticopenai.ChatConfig 在构造前调一下：
ailens360.ApplyGlobalAgentic(cfg) // 返回 true 表示全局 Decorator 生效
```

适合在大型项目里把"是否启用 AILens360"做成开关：env 没配 → `SetGlobal(nil)` → `ApplyGlobalAgentic` / `ApplyGlobal` 全是 no-op → 应用代码零分支。

如果使用非 Agentic 的 `github.com/cloudwego/eino-ext/components/model/openai.ChatModelConfig`，
对应入口是 `dec.Apply(cfg)` 和 `ailens360.ApplyGlobal(cfg)`。

## 设计取舍

- **单次调用遥测走 ctx 而不是参数**：保持 `*openai.ChatModel` 是单例、可在 goroutine 间共享；调用方只关心"这次请求是谁/什么会话"，不关心 transport 细节。
- **`NewDecorator` 容忍空配置**：`proxyPrefix` 或 `projectKey` 任一空就返回 `nil`，调用方不用写 `if dec != nil` 分支；`Apply` / `DecorateBaseURL` / `HTTPClient` 在 `nil` receiver 上都是 no-op。
- **不预设上游**：`DecorateBaseURL` 只做字符串拼接，OpenAI / Anthropic / Gemini / DeepSeek / vLLM 都一样用。
- **真实上游 Key 不经过这个包**：`Authorization` 由 `openai.ChatModel` 内部按 `APIKey` 字段塞，透传到上游；AILens360 也只是中转，不落库。

## 相关链接

- AILens360 项目：https://github.com/CoolBanHub/ailens360
- AILens360 接入说明（baseURL / project_key 协议）：见上游仓库 `docs/api-design.md`
