# AGGO 工具系统

`tools/` 提供可直接注册到 Eino Agent 的工具集合。顶层
`github.com/CoolBanHub/aggo/tools` 包是便捷入口；生产代码也可以直接导入
子包获得更细的配置能力。

## 工具包

| 包 | 工具 | 说明 |
| --- | --- | --- |
| `tools/knowledge` | `load_documents`, `search_documents` | 文档加载与知识库检索。 |
| `tools/knowledge` | `knowledge_think`, `knowledge_search`, `knowledge_analysis` | 面向多步知识检索的推理辅助工具。 |
| `tools/database` | `database_execute` | 通用 GORM SQL 工具，默认只读。 |
| `tools/shell` | `shell_execute` | Shell 命令执行工具，默认限制工作目录、拒绝高危命令并截断长输出。 |
| `tools/cron` | `cron` | 定时任务添加、查看、删除、启用和禁用。 |
| `tools/memory` | `search_user_memory` | 支持事件检索的记忆 provider 可注册该工具。 |

## 使用示例

```go
import (
    "time"

    cronPkg "github.com/CoolBanHub/aggo/cron"
    aggotools "github.com/CoolBanHub/aggo/tools"
    "github.com/CoolBanHub/aggo/tools/database"
    "github.com/CoolBanHub/aggo/tools/shell"
    "github.com/cloudwego/eino/components/tool"
)

knowledgeTools := aggotools.GetKnowledgeTools(indexer, retriever, retrieverOptions)
reasoningTools := aggotools.GetKnowledgeReasoningTools(retriever, retrieverOptionsList)

// database_execute 默认只允许只读查询。
dbTools := aggotools.GetDatabaseTools(gormDB)

// 写 SQL 需要由工具创建方显式开启。
writeDBTools := aggotools.GetDatabaseTools(gormDB, database.WithAllowWrite(true))

// 可限制查询行数和执行超时。
dbTools = aggotools.GetDatabaseTools(
    gormDB,
    database.WithMaxResultRows(200),
    database.WithTimeout(3*time.Second),
)

// shell_execute 默认拒绝 rm、sudo 等高危命令，并将 workingDir 限制在进程启动目录内。
shellTools := aggotools.GetShellTools(
    shell.WithAllowedCommands("ls", "pwd", "cat"),
    shell.WithMaxOutputBytes(8_000),
    shell.WithMaxTimeout(30*time.Second),
)

service := cronPkg.NewCronService(cronPkg.NewFileStore("cron_jobs.json"), nil)
cronTools := aggotools.GetCronTools(service)

allTools := append([]tool.BaseTool{}, knowledgeTools...)
allTools = append(allTools, reasoningTools...)
allTools = append(allTools, dbTools...)
allTools = append(allTools, shellTools...)
allTools = append(allTools, cronTools...)
```

## 安全边界

- `database_execute` 默认只允许 `SELECT`、`SHOW`、`DESCRIBE`、`EXPLAIN`、`PRAGMA`、`WITH` 等只读语句。需要写操作时使用 `database.WithAllowWrite(true)`。
- `database_execute` 可以用 `database.WithMaxResultRows(...)` 和 `database.WithTimeout(...)` 限制结果规模和执行时间。
- `shell_execute` 默认工作目录根为当前进程启动目录。需要修改根目录时使用 `shell.WithWorkingDirRoot(...)`；确需关闭限制时使用 `shell.WithUnrestrictedWorkingDir()`。
- `shell_execute` 默认拒绝高危命令，并可用 `shell.WithAllowedCommands(...)` 将可执行命令收敛到白名单。
- `shell_execute` 可以用 `shell.WithMaxOutputBytes(...)`、`shell.WithDefaultTimeout(...)`、`shell.WithMaxTimeout(...)` 限制输出和运行时间。
- 对会产生大量结果的工具，应配置行数、输出长度或检索数量上限，避免把过多数据送入模型上下文。

## 开发约定

- 新工具应放在对应领域子包内，再按需通过 `tools/tools.go` 暴露便捷入口。
- 工具参数结构体应提供清晰的 `jsonschema` 描述，便于模型正确调用。
- 有外部副作用的工具必须有默认安全策略，并提供显式选项放宽限制。
- 新增工具时至少补一组参数校验或安全策略测试。
