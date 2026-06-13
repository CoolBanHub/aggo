# AGGO工具系统

AGGO工具系统是一个基于Eino框架的智能代理工具集合，为AI代理提供知识管理、系统操作和推理分析等功能。所有工具都实现了Eino框架的标准接口，支持动态调用和参数验证。

## 🚀 核心特性

- **统一接口**: 基于Eino框架的`tool.InvokableTool`接口
- **自动推断**: 使用`utils.InferTool`进行工具信息自动推断
- **参数验证**: 基于JSON Schema的参数验证机制
- **结构化输出**: 统一的JSON格式返回结果
- **错误处理**: 完善的错误处理和异常捕获

## 📦 工具目录

### 知识管理工具 (`knowledge_tool.go`)

提供完整的文档知识库管理功能，支持多种文档来源和操作。

#### 可用工具

| 工具名称               | 描述     | 主要功能                  |
|--------------------|--------|-----------------------|
| `load_documents`   | 文档加载工具 | 支持文本文件、URL、目录、内存文档的加载 |
| `search_documents` | 文档搜索工具 | 基于向量相似度的文档搜索          |
| `get_document`     | 获取文档工具 | 根据ID获取单个文档详情          |
| `update_document`  | 更新文档工具 | 更新文档内容和元数据            |
| `delete_document`  | 删除文档工具 | 删除指定文档                |
| `list_documents`   | 列出文档工具 | 分页列出文档信息              |

#### 使用示例

```go
// 获取知识管理工具
knowledgeTools := tools.GetKnowledgeTools(knowledgeManager)

// 加载目录中的文档
loadParams := tools.LoadDocumentsParams{
SourceType:    "directory",
DirectoryPath: "/path/to/docs",
Extensions:    []string{".txt", ".md"},
Recursive:     true,
LoadOptions: tools.LoadOptionsInput{
EnableChunking: true,
ChunkSize:      1000,
ChunkOverlap:   200,
},
}

// 搜索文档
searchParams := tools.SearchParams{
Query:     "机器学习算法",
Limit:     10,
Threshold: 0.75,
}
```

### 知识推理工具 (`knowledge_reasoning_tools.go`)

提供知识推理和分析功能，支持思考链式推理过程。

#### 可用工具

| 工具名称                 | 描述     | 主要功能              |
|----------------------|--------|-------------------|
| `knowledge_think`    | 知识思考工具 | 内部推理和策略规划（对用户不可见） |
| `knowledge_search`   | 知识搜索工具 | 执行知识库搜索操作         |
| `knowledge_analysis` | 知识分析工具 | 分析搜索结果的质量和相关性     |

#### 推理工作流程

1. **思考阶段**: 使用`knowledge_think`进行问题分析和搜索策略制定
2. **搜索阶段**: 使用`knowledge_search`执行多轮搜索获取信息
3. **分析阶段**: 使用`knowledge_analysis`评估结果质量和完整性

#### 使用示例

```go
// 获取知识推理工具
reasoningTools := tools.GetKnowledgeReasoningTools(knowledgeManager)

// 思考策略（内部使用）
thinkParams := tools.ThinkParams{
Thought: "需要分析机器学习算法的优缺点，应该搜索相关技术文档",
}

// 执行搜索
searchParams := tools.KnowledgeSearchParams{
Query: "机器学习算法比较",
Limit: 10,
}

// 分析结果
analysisParams := tools.AnalysisParams{
Analysis: "搜索结果包含了深度学习和传统机器学习的对比信息，质量较高",
}
```

### 数据库工具

#### MySQL工具 (`mysql_tool.go`)

提供MySQL数据库操作功能，支持查询、更新、数据分析等。

| 工具名称            | 描述        | 主要功能                      |
|-----------------|-----------|---------------------------|
| `mysql_query`   | MySQL查询工具 | 执行SELECT查询操作              |
| `mysql_execute` | MySQL执行工具 | 执行INSERT、UPDATE、DELETE等操作 |
| `mysql_schema`  | MySQL架构工具 | 获取数据库结构信息                 |
| `mysql_analyze` | MySQL分析工具 | 数据分析和统计                   |

#### PostgreSQL工具 (`postgres_tool.go`)

提供PostgreSQL数据库操作功能，支持查询、更新、数据分析等。

| 工具名称               | 描述             | 主要功能                      |
|--------------------|----------------|---------------------------|
| `postgres_query`   | PostgreSQL查询工具 | 执行SELECT查询操作              |
| `postgres_execute` | PostgreSQL执行工具 | 执行INSERT、UPDATE、DELETE等操作 |
| `postgres_schema`  | PostgreSQL架构工具 | 获取数据库结构信息                 |
| `postgres_analyze` | PostgreSQL分析工具 | 数据分析和统计                   |

### 系统工具 (`shell_tool.go`)

提供系统级操作功能，支持命令执行、系统信息获取等。

#### 可用工具

| 工具名称                   | 描述     | 主要功能              |
|------------------------|--------|-------------------|
| `shell_execute`        | 命令执行工具 | 执行系统命令，支持超时、输出截断、工作目录限制和高危命令拦截 |
| `shell_system_info`    | 系统信息工具 | 获取OS、环境变量、内存等系统信息 |
| `shell_list_processes` | 进程管理工具 | 列出系统运行中的进程        |
| `shell_directory`      | 目录操作工具 | 获取和切换工作目录         |

#### 使用示例

```go
// 获取数据库工具
mysqlTools := tools.GetMySQLTools(mysqlConfig)
postgresTools := tools.GetPostgreSQLTools(postgresConfig)

// 获取系统工具。默认工作目录限制在当前进程启动目录下，并拒绝 rm、sudo 等高危命令。
shellTools := tools.GetShellTools()

// 如需进一步收紧权限，可以配置命令 allowlist。
shellTools = tools.GetShellTools(shell.WithAllowedCommands("ls", "pwd", "cat"))

// 执行命令
executeParams := tools.ExecuteParams{
Command:    "ls",
Args:       []string{"-la"},
WorkingDir: "/tmp",
Timeout:    30,
Shell:      false,
}

// 获取系统信息
systemParams := tools.SystemInfoParams{
InfoType: "memory", // os, env, path, user, disk, memory
}

// 目录操作
dirParams := tools.DirectoryParams{
Operation: "change", // get, change
Path:      "/new/working/directory",
}
```

## 🛠️ 工具开发指南

### 基础架构

所有工具都基于以下接口：

```go
// 基础接口
type tool.BaseTool interface {
Info(ctx context.Context) (*schema.ToolInfo, error)
}

// 可调用接口
type tool.InvokableTool interface {
tool.BaseTool
InvokableRun(ctx context.Context, argumentsInJSON string, opts ...tool.Option) (string, error)
}
```

### 开发新工具

#### 1. 定义工具结构体

```go
type MyTool struct {
// 工具依赖
manager *SomeManager
}

// 参数结构体
type MyParams struct {
Param1 string `json:"param1" jsonschema:"description=参数描述,required"`
Param2 int    `json:"param2,omitempty" jsonschema:"description=可选参数,默认值为10"`
}
```

#### 2. 实现构造函数

```go
func NewMyTool(manager *SomeManager) tool.InvokableTool {
this := &MyTool{manager: manager}
name := "my_tool"
desc := "工具功能描述"
t, _ := utils.InferTool(name, desc, this.execute)
return t
}
```

#### 3. 实现业务逻辑

```go
func (t *MyTool) execute(ctx context.Context, params MyParams) (interface{}, error) {
// 参数验证
if params.Param1 == "" {
return nil, fmt.Errorf("param1 is required")
}

// 业务逻辑实现
result := map[string]interface{}{
"operation": "my_operation",
"success":   true,
"result":    "执行结果",
"timestamp": time.Now().Unix(),
}

return result, nil
}
```

### JSON Schema标签规范

```go
type ExampleParams struct {
// 必需参数
Required string `json:"required" jsonschema:"description=必需参数描述,required"`

// 可选参数（带默认值说明）
Optional string `json:"optional,omitempty" jsonschema:"description=可选参数描述,默认值为xxx"`

// 枚举参数
Enum string `json:"enum" jsonschema:"description=枚举参数,required,enum=value1,enum=value2,enum=value3"`

// 数值范围
Number int `json:"number" jsonschema:"description=数值参数,minimum=1,maximum=100"`

// 数组参数
Array []string `json:"array,omitempty" jsonschema:"description=数组参数"`
}
```

## 🔧 工具集成

### 在代理中使用工具

```go
import "github.com/CoolBanHub/aggo/tools"

func createAgent(knowledgeManager *knowledge.KnowledgeManager) *agent.Agent {
// 获取各类工具
knowledgeTools := tools.GetKnowledgeTools(knowledgeManager)
reasoningTools := tools.GetKnowledgeReasoningTools(knowledgeManager)
mysqlTools := tools.GetMySQLTools(mysqlConfig) // 新增
postgresTools := tools.GetPostgreSQLTools(postgresConfig) // 新增
shellTools := tools.GetSellTool()

// 合并所有工具
allTools := append(knowledgeTools, reasoningTools...)
allTools = append(allTools, mysqlTools...) // 新增
allTools = append(allTools, postgresTools...) // 新增
allTools = append(allTools, shellTools...)

// 创建代理
return agent.NewAgent(ctx, chatModel,
agent.WithTools(allTools),
// 其他配置...
)
}
```

### 工具调用示例

```go
// 工具调用
toolResult, err := tool.InvokableRun(ctx, `{
    "query": "机器学习",
    "limit": 5,
    "threshold": 0.8
}`)
```

## 📊 返回结果格式

所有工具都遵循统一的结果格式：

```json
{
  "operation": "操作类型",
  "success": true,
  "result": "具体结果数据",
  "error": "错误信息（仅在失败时）",
  "timestamp": 1645123456,
  "duration": "执行时长（某些工具）"
}
```

## ⚡ 性能优化

- **输出截断**: 长输出自动截断防止token溢出
- **超时控制**: 命令执行支持超时设置
- **错误处理**: 完善的错误捕获和处理机制
- **资源管理**: 自动清理临时资源

## 🔒 安全考虑

- **命令验证**: Shell 工具默认拒绝高危命令；可通过 `shell.WithAllowedCommands(...)` 配置命令白名单
- **路径验证**: 文件路径操作防止目录遍历攻击
- **数据库保护**: `database_execute` 默认只允许只读查询；写操作需显式使用 `database.WithAllowWrite(true)`
- **权限控制**: 根据执行环境限制工具权限
- **输入清理**: 防止命令注入攻击

## 🧪 测试

每个工具都应包含相应的测试：

```go
func TestMyTool(t *testing.T) {
tool := NewMyTool(manager)

// 测试工具信息
info, err := tool.Info(context.Background())
require.NoError(t, err)
assert.Equal(t, "my_tool", info.Name)

// 测试工具执行
params := MyParams{Param1: "test"}
result, err := tool.execute(context.Background(), params)
require.NoError(t, err)
assert.True(t, result.Success)
}
```

## 📝 最佳实践

1. **命名规范**: 工具名使用下划线分隔，描述清晰准确
2. **参数设计**: 提供合理的默认值，必需参数明确标注
3. **错误处理**: 返回有意义的错误信息，避免暴露敏感信息
4. **文档完善**: JSON Schema描述详细，便于理解使用
5. **性能考虑**: 对耗时操作设置超时，大数据量结果进行分页
6. **版本兼容**: 新增功能保持向后兼容性

通过遵循以上指南，可以为AGGO框架开发出功能强大、安全可靠的智能代理工具。
