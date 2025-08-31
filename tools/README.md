# Tools模块开发文档

Tools模块是AIGO框架中的工具集合，为AI代理提供各种功能工具，包括知识管理、系统操作、推理分析等。本文档详细说明了如何开发和新增工具。

## 📋 现有工具概览

### 知识管理工具 (`knowledge_tool.go`)
- `load_documents` - 文档加载工具
- `search_documents` - 文档搜索工具
- `get_document` - 获取单个文档
- `update_document` - 更新文档
- `delete_document` - 删除文档
- `list_documents` - 列出文档

### 知识推理工具 (`knowledge_reasoning_tools.go`)
- `knowledge_think` - 知识推理思考工具
- `knowledge_search` - 知识库搜索工具
- `knowledge_analysis` - 知识分析工具

### 系统工具 (`shell_tool.go`)
- `shell_execute` - 命令执行工具
- `shell_system_info` - 系统信息工具
- `shell_processes` - 进程管理工具
- `shell_directory` - 目录操作工具

## 🛠️ 开发新工具的完整指南

### 1. 工具架构基础

所有工具必须实现Eino框架的接口：

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

### 2. 创建新工具的步骤

#### 第一步：定义工具结构体和参数

```go
package tools

import (
    "context"
    "encoding/json"
    "fmt"
    
    "github.com/cloudwego/eino/components/tool"
    "github.com/cloudwego/eino/schema"
    "github.com/eino-contrib/jsonschema"
)

// 工具结构体
type MyCustomTool struct {
    // 添加工具需要的依赖，例如：
    // database *sql.DB
    // config   *Config
}

// 参数结构体 - 必须包含jsonschema标签
type MyCustomParams struct {
    // required参数示例
    RequiredParam string `json:"requiredParam" jsonschema:"description=必需参数描述,required"`
    
    // 可选参数示例
    OptionalParam int `json:"optionalParam,omitempty" jsonschema:"description=可选参数描述,默认值为10"`
    
    // 枚举参数示例
    EnumParam string `json:"enumParam" jsonschema:"description=枚举参数,required,enum=option1,enum=option2,enum=option3"`
    
    // 数组参数示例
    ArrayParam []string `json:"arrayParam,omitempty" jsonschema:"description=数组参数"`
    
    // 嵌套对象参数示例
    NestedParam NestedObject `json:"nestedParam,omitempty" jsonschema:"description=嵌套对象参数"`
}

// 嵌套对象结构体
type NestedObject struct {
    Field1 string `json:"field1" jsonschema:"description=嵌套字段1"`
    Field2 int    `json:"field2" jsonschema:"description=嵌套字段2"`
}

// 结果结构体
type MyCustomResult struct {
    Operation string      `json:"operation"`
    Result    interface{} `json:"result"`
    Success   bool        `json:"success"`
    Error     string      `json:"error,omitempty"`
    Timestamp int64       `json:"timestamp"`
}
```

#### 第二步：实现构造函数

```go
// 构造函数
func NewMyCustomTool(/* 传入依赖 */) tool.InvokableTool {
    return &MyCustomTool{
        // 初始化依赖
    }
}

// 如果有多个相关工具，提供工具集合函数
func GetMyCustomTools(/* 共同依赖 */) []tool.BaseTool {
    return []tool.BaseTool{
        NewMyCustomTool(),
        // 其他相关工具...
    }
}
```

#### 第三步：实现Info方法

```go
// Info 实现 tool.BaseTool 接口
func (t *MyCustomTool) Info(ctx context.Context) (*schema.ToolInfo, error) {
    return &schema.ToolInfo{
        Name: "my_custom_tool",
        Desc: "这是我的自定义工具，用于执行特定功能。详细描述工具的作用和使用场景。",
        ParamsOneOf: schema.NewParamsOneOfByJSONSchema(
            jsonschema.Reflect(&MyCustomParams{}),
        ),
    }, nil
}
```

#### 第四步：实现InvokableRun方法

```go
// InvokableRun 实现 tool.InvokableTool 接口
func (t *MyCustomTool) InvokableRun(ctx context.Context, argumentsInJSON string, opts ...tool.Option) (string, error) {
    // 1. 解析参数
    var params MyCustomParams
    if err := json.Unmarshal([]byte(argumentsInJSON), &params); err != nil {
        return "", fmt.Errorf("解析参数失败: %w", err)
    }
    
    // 2. 参数验证
    if err := t.validateParams(params); err != nil {
        return "", fmt.Errorf("参数验证失败: %w", err)
    }
    
    // 3. 执行业务逻辑
    result, err := t.executeLogic(ctx, params)
    if err != nil {
        return "", err
    }
    
    // 4. 序列化结果
    resultJSON, err := json.Marshal(result)
    if err != nil {
        return "", fmt.Errorf("序列化结果失败: %w", err)
    }
    
    return string(resultJSON), nil
}
```

#### 第五步：实现业务逻辑方法

```go
// validateParams 参数验证
func (t *MyCustomTool) validateParams(params MyCustomParams) error {
    if params.RequiredParam == "" {
        return fmt.Errorf("必需参数不能为空")
    }
    
    // 添加其他验证逻辑
    
    return nil
}

// executeLogic 执行核心业务逻辑
func (t *MyCustomTool) executeLogic(ctx context.Context, params MyCustomParams) (*MyCustomResult, error) {
    // 实现具体的业务逻辑
    
    // 示例实现
    result := &MyCustomResult{
        Operation: "my_custom_operation",
        Success:   true,
        Timestamp: time.Now().Unix(),
    }
    
    // 根据参数执行不同逻辑
    switch params.EnumParam {
    case "option1":
        result.Result = "执行选项1的逻辑"
    case "option2":
        result.Result = "执行选项2的逻辑"
    case "option3":
        result.Result = "执行选项3的逻辑"
    default:
        return nil, fmt.Errorf("不支持的选项: %s", params.EnumParam)
    }
    
    return result, nil
}
```

### 3. 完整示例：文件操作工具

以下是一个完整的文件操作工具示例：

```go
package tools

import (
    "context"
    "encoding/json"
    "fmt"
    "io/ioutil"
    "os"
    "path/filepath"
    "time"
    
    "github.com/cloudwego/eino/components/tool"
    "github.com/cloudwego/eino/schema"
    "github.com/eino-contrib/jsonschema"
)

// FileOperationTool 文件操作工具
type FileOperationTool struct{}

// FileOperationParams 文件操作参数
type FileOperationParams struct {
    Operation string `json:"operation" jsonschema:"description=操作类型,required,enum=read,enum=write,enum=list,enum=delete"`
    FilePath  string `json:"filePath" jsonschema:"description=文件路径,required"`
    Content   string `json:"content,omitempty" jsonschema:"description=文件内容（写入操作时使用）"`
    Recursive bool   `json:"recursive,omitempty" jsonschema:"description=是否递归操作（列表操作时使用）"`
}

// FileOperationResult 文件操作结果
type FileOperationResult struct {
    Operation string      `json:"operation"`
    FilePath  string      `json:"filePath"`
    Result    interface{} `json:"result,omitempty"`
    Success   bool        `json:"success"`
    Error     string      `json:"error,omitempty"`
    Timestamp int64       `json:"timestamp"`
}

// NewFileOperationTool 创建文件操作工具
func NewFileOperationTool() tool.InvokableTool {
    return &FileOperationTool{}
}

// Info 实现 tool.BaseTool 接口
func (t *FileOperationTool) Info(ctx context.Context) (*schema.ToolInfo, error) {
    return &schema.ToolInfo{
        Name: "file_operation",
        Desc: "文件操作工具，支持读取、写入、列出和删除文件。提供基础的文件系统操作功能。",
        ParamsOneOf: schema.NewParamsOneOfByJSONSchema(
            jsonschema.Reflect(&FileOperationParams{}),
        ),
    }, nil
}

// InvokableRun 实现 tool.InvokableTool 接口
func (t *FileOperationTool) InvokableRun(ctx context.Context, argumentsInJSON string, opts ...tool.Option) (string, error) {
    var params FileOperationParams
    if err := json.Unmarshal([]byte(argumentsInJSON), &params); err != nil {
        return "", fmt.Errorf("解析参数失败: %w", err)
    }
    
    result, err := t.executeFileOperation(ctx, params)
    if err != nil {
        return "", err
    }
    
    resultJSON, err := json.Marshal(result)
    if err != nil {
        return "", fmt.Errorf("序列化结果失败: %w", err)
    }
    
    return string(resultJSON), nil
}

// executeFileOperation 执行文件操作
func (t *FileOperationTool) executeFileOperation(ctx context.Context, params FileOperationParams) (*FileOperationResult, error) {
    result := &FileOperationResult{
        Operation: params.Operation,
        FilePath:  params.FilePath,
        Timestamp: time.Now().Unix(),
    }
    
    switch params.Operation {
    case "read":
        content, err := ioutil.ReadFile(params.FilePath)
        if err != nil {
            result.Success = false
            result.Error = fmt.Sprintf("读取文件失败: %v", err)
        } else {
            result.Success = true
            result.Result = string(content)
        }
    
    case "write":
        err := ioutil.WriteFile(params.FilePath, []byte(params.Content), 0644)
        if err != nil {
            result.Success = false
            result.Error = fmt.Sprintf("写入文件失败: %v", err)
        } else {
            result.Success = true
            result.Result = "文件写入成功"
        }
    
    case "list":
        if params.Recursive {
            files, err := t.listFilesRecursive(params.FilePath)
            if err != nil {
                result.Success = false
                result.Error = fmt.Sprintf("递归列出文件失败: %v", err)
            } else {
                result.Success = true
                result.Result = files
            }
        } else {
            files, err := ioutil.ReadDir(params.FilePath)
            if err != nil {
                result.Success = false
                result.Error = fmt.Sprintf("列出文件失败: %v", err)
            } else {
                var fileNames []string
                for _, file := range files {
                    fileNames = append(fileNames, file.Name())
                }
                result.Success = true
                result.Result = fileNames
            }
        }
    
    case "delete":
        err := os.Remove(params.FilePath)
        if err != nil {
            result.Success = false
            result.Error = fmt.Sprintf("删除文件失败: %v", err)
        } else {
            result.Success = true
            result.Result = "文件删除成功"
        }
    
    default:
        result.Success = false
        result.Error = fmt.Sprintf("不支持的操作类型: %s", params.Operation)
    }
    
    return result, nil
}

// listFilesRecursive 递归列出文件
func (t *FileOperationTool) listFilesRecursive(root string) ([]string, error) {
    var files []string
    err := filepath.Walk(root, func(path string, info os.FileInfo, err error) error {
        if err != nil {
            return err
        }
        files = append(files, path)
        return nil
    })
    return files, err
}
```

### 4. 重要开发规范

#### JSON Schema标签规范

```go
type ParamsExample struct {
    // 必需参数
    Required string `json:"required" jsonschema:"description=参数描述,required"`
    
    // 可选参数
    Optional string `json:"optional,omitempty" jsonschema:"description=参数描述,默认值说明"`
    
    // 枚举参数
    Enum string `json:"enum" jsonschema:"description=参数描述,required,enum=value1,enum=value2"`
    
    // 数值范围参数
    Number int `json:"number" jsonschema:"description=参数描述,minimum=1,maximum=100"`
    
    // 数组参数
    Array []string `json:"array,omitempty" jsonschema:"description=数组参数描述"`
}
```

#### 错误处理规范

```go
func (t *MyTool) InvokableRun(ctx context.Context, argumentsInJSON string, opts ...tool.Option) (string, error) {
    // 参数解析错误
    var params MyParams
    if err := json.Unmarshal([]byte(argumentsInJSON), &params); err != nil {
        return "", fmt.Errorf("解析参数失败: %w", err)
    }
    
    // 业务逻辑错误
    result, err := t.executeLogic(ctx, params)
    if err != nil {
        return "", fmt.Errorf("执行失败: %w", err)
    }
    
    // 序列化错误
    resultJSON, err := json.Marshal(result)
    if err != nil {
        return "", fmt.Errorf("序列化结果失败: %w", err)
    }
    
    return string(resultJSON), nil
}
```

#### 结果结构规范

```go
type StandardResult struct {
    Operation string      `json:"operation"`     // 操作类型
    Success   bool        `json:"success"`       // 是否成功
    Result    interface{} `json:"result,omitempty"` // 结果数据
    Error     string      `json:"error,omitempty"`  // 错误信息
    Timestamp int64       `json:"timestamp"`     // 时间戳
    Duration  string      `json:"duration,omitempty"` // 执行时长（可选）
}
```

### 5. 工具集成到代理

#### 在代理中注册工具

```go
// 在agent包中使用工具
import "github.com/CoolBanHub/aggo/tools"

func createAgentWithCustomTools() *agent.Agent {
    // 获取工具集合
    knowledgeTools := tools.GetKnowledgeTools(knowledgeManager)
    shellTools := tools.GetSellTool()
    customTools := []tool.BaseTool{
        tools.NewFileOperationTool(),
        // 添加其他自定义工具
    }
    
    // 合并所有工具
    allTools := append(knowledgeTools, shellTools...)
    allTools = append(allTools, customTools...)
    
    // 创建代理时传入工具
    return agent.NewAgent(ctx, chatModel,
        agent.WithTools(allTools),
        // 其他配置...
    )
}
```

### 6. 测试工具

```go
package tools

import (
    "context"
    "testing"
    "encoding/json"
)

func TestMyCustomTool(t *testing.T) {
    tool := NewMyCustomTool()
    
    // 测试工具信息
    info, err := tool.Info(context.Background())
    if err != nil {
        t.Fatalf("获取工具信息失败: %v", err)
    }
    
    if info.Name != "my_custom_tool" {
        t.Errorf("工具名称不匹配: got %s, want my_custom_tool", info.Name)
    }
    
    // 测试工具执行
    params := MyCustomParams{
        RequiredParam: "test_value",
        EnumParam:     "option1",
    }
    
    paramsJSON, _ := json.Marshal(params)
    result, err := tool.InvokableRun(context.Background(), string(paramsJSON))
    if err != nil {
        t.Fatalf("工具执行失败: %v", err)
    }
    
    var resultObj MyCustomResult
    err = json.Unmarshal([]byte(result), &resultObj)
    if err != nil {
        t.Fatalf("结果解析失败: %v", err)
    }
    
    if !resultObj.Success {
        t.Errorf("工具执行失败: %s", resultObj.Error)
    }
}
```

### 7. 最佳实践

1. **参数验证**: 始终验证输入参数的合法性
2. **错误处理**: 提供清晰的错误信息
3. **文档化**: 在jsonschema标签中提供详细的参数描述
4. **幂等性**: 确保工具操作是幂等的（如果适用）
5. **资源管理**: 正确处理文件、网络连接等资源
6. **安全性**: 验证文件路径、命令参数等，防止安全漏洞
7. **性能**: 对于耗时操作，考虑超时机制
8. **日志记录**: 在关键操作点添加适当的日志

### 8. 常见问题解决

#### Q: JSONSchema标签不生效？
A: 确保导入了正确的jsonschema包：`github.com/eino-contrib/jsonschema`

#### Q: 工具执行时参数解析失败？
A: 检查struct的json标签是否正确，参数名是否匹配

#### Q: 如何处理可选参数？
A: 使用`omitempty`标签，并在业务逻辑中设置默认值

#### Q: 如何实现复杂的参数验证？
A: 在`validateParams`方法中实现自定义验证逻辑

通过遵循以上指南，您可以为AIGO框架开发出功能丰富、易用且可靠的工具。