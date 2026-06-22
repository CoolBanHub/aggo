# AGGO 示例

本目录是独立 Go 模块：

```bash
cd example
go mod download
```

模块内使用：

```go
replace github.com/CoolBanHub/aggo => ../
```

这样示例会引用当前本地检出版本，而不是已发布的模块版本。

## 运行示例

除特别说明外，请在 `example/` 目录下运行命令：

```bash
go run ./simple_agent
go run ./mem_agent_test
go run ./mem0_agent_test
go run ./cron_test
go run ./sse
go run ./tool_test
go run ./callback_test
go run ./adk_test
go run ./vision_test
go run ./generate_img_test
go run ./skill_agent_test
```

知识库示例可以使用 PostgreSQL/pgvector 或 Milvus。请先启动需要的服务：

```bash
cd knowledge_agent_tool_test
./pg_docker.sh
# or
./milvus_docker.sh
cd ..
go run ./knowledge_agent_tool_test
```

`example/claw` 刻意隔离为独立模块：

```bash
cd example/claw
go mod download
go run .
```

## 环境变量

多数示例通过各自 `main.go` 读取环境变量，用于配置 OpenAI 兼容模型。常用变量包括：

```bash
BaseUrl=https://api.openai.com/v1
ApiKey=your-api-key
Model=gpt-4o-mini
EmbeddingModel=text-embedding-3-large
```

可选集成会使用自己的变量，例如 Langfuse 和 AILens360 凭证。

## 测试边界

根模块执行 `go test ./...` 时不会包含本目录，因为 `example/` 有自己的 `go.mod`。
若要验证示例可以编译，请运行：

```bash
cd example
go test ./...
cd claw
go test ./...
```
