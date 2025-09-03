package tools

import (
	"context"
	"encoding/json"
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"runtime"
	"strings"
	"time"

	"github.com/cloudwego/eino/components/tool"
	"github.com/cloudwego/eino/components/tool/utils"
	"github.com/cloudwego/eino/schema"
)

func GetSellTool() []tool.BaseTool {
	this := &ShellTool{}
	return []tool.BaseTool{
		this.newShellExecuteTool(),
		this.newShellSystemInfoTool(),
		this.newShellProcessesTool(),
		this.newShellDirectoryTool(),
	}
}

func GetSysInfoTool() []tool.BaseTool {
	this := &ShellTool{}
	return []tool.BaseTool{
		this.newShellSystemInfoTool(),
	}
}

func GetExecuteTool() []tool.BaseTool {
	this := &ShellTool{}
	return []tool.BaseTool{
		this.newShellExecuteTool(),
	}
}

type ShellTool struct{}

// ShellResult 表示命令执行的结果
type ShellResult struct {
	Command    string `json:"command"`         // 执行的命令
	ExitCode   int    `json:"exitCode"`        // 退出码
	Stdout     string `json:"stdout"`          // 标准输出
	Stderr     string `json:"stderr"`          // 标准错误输出
	Success    bool   `json:"success"`         // 是否执行成功
	Error      string `json:"error,omitempty"` // 错误信息
	Duration   string `json:"duration"`        // 执行时长
	WorkingDir string `json:"workingDir"`      // 工作目录
	Operation  string `json:"operation"`       // 操作类型
}

// ExecuteParams 表示命令执行的参数
type ExecuteParams struct {
	Command    string   `json:"command" jsonschema:"description=要执行的命令,required"`
	Args       []string `json:"args,omitempty" jsonschema:"description=命令参数"`
	WorkingDir string   `json:"workingDir,omitempty" jsonschema:"description=命令执行的工作目录"`
	Timeout    int      `json:"timeout,omitempty" jsonschema:"description=超时时间（秒），默认为30秒"`
	Shell      bool     `json:"shell,omitempty" jsonschema:"description=是否在shell环境中执行，默认为false"`
}

// SystemInfoParams 表示系统信息查询的参数
type SystemInfoParams struct {
	InfoType string `json:"infoType" jsonschema:"description=系统信息类型: os\, env\, path\, user\, disk\, memory,required,enum=os,enum=env,enum=path,enum=user,enum=disk,enum=memory"`
}

// DirectoryParams 表示目录操作的参数
type DirectoryParams struct {
	Operation string `json:"operation" jsonschema:"description=要执行的操作: get获取当前目录\, change切换目录,required,enum=get,enum=change"`
	Path      string `json:"path,omitempty" jsonschema:"description=要切换到的目录路径（当operation为change时必需）"`
}

// newShellExecuteTool 创建一个新的 ShellExecuteTool 实例
func (this *ShellTool) newShellExecuteTool() tool.InvokableTool {
	name := "shell_execute"
	desc := "执行系统命令，支持超时处理和错误处理。支持直接命令执行和shell环境执行。"
	t, _ := utils.InferTool(name, desc, this.executeCommand)
	return t
}

// newShellSystemInfoTool 创建一个新的 ShellSystemInfoTool 实例
func (this *ShellTool) newShellSystemInfoTool() tool.InvokableTool {
	name := "shell_system_info"
	desc := "获取各种系统信息，包括操作系统详情、环境变量、路径信息、用户详情、磁盘使用情况和内存统计。"
	t, _ := utils.InferTool(name, desc, this.getSystemInfo)
	return t
}

// newShellProcessesTool 创建一个新的 ShellProcessesTool 实例
func (this *ShellTool) newShellProcessesTool() tool.InvokableTool {
	name := "shell_list_processes"
	desc := "列出系统上运行的进程。以平台特定的格式返回进程信息（Unix系统上使用ps aux，Windows上使用tasklist）。"

	t := utils.NewTool(&schema.ToolInfo{
		Name: name,
		Desc: desc,
		ParamsOneOf: schema.NewParamsOneOfByParams(
			map[string]*schema.ParameterInfo{},
		),
	}, this.listProcesses)

	return t
}

// newShellDirectoryTool 创建一个新的 ShellDirectoryTool 实例
func (this *ShellTool) newShellDirectoryTool() tool.InvokableTool {
	name := "shell_directory"
	desc := "获取当前工作目录或切换到新目录。通过不同的参数集支持两种操作。"
	t, _ := utils.InferTool(name, desc, this.getDirectory)
	return t
}

// executeCommand 在系统shell中运行命令
func (t *ShellTool) executeCommand(ctx context.Context, params ExecuteParams) (interface{}, error) {
	if params.Command == "" {
		return nil, fmt.Errorf("command is required")
	}

	// Set default timeout
	if params.Timeout <= 0 {
		params.Timeout = 30
	}

	// Set working directory
	workingDir := params.WorkingDir
	if workingDir == "" {
		var err error
		workingDir, err = os.Getwd()
		if err != nil {
			workingDir = "unknown"
		}
	}

	// Create context with timeout
	ctx, cancel := context.WithTimeout(context.Background(), time.Duration(params.Timeout)*time.Second)
	defer cancel()

	start := time.Now()
	var cmd *exec.Cmd

	// Prepare command based on shell flag and platform
	if params.Shell {
		// Execute in shell
		switch runtime.GOOS {
		case "windows":
			fullCommand := params.Command
			if len(params.Args) > 0 {
				fullCommand += " " + strings.Join(params.Args, " ")
			}
			cmd = exec.CommandContext(ctx, "cmd", "/C", fullCommand)
		default:
			fullCommand := params.Command
			if len(params.Args) > 0 {
				fullCommand += " " + strings.Join(params.Args, " ")
			}
			cmd = exec.CommandContext(ctx, "sh", "-c", fullCommand)
		}
	} else {
		// Execute directly
		if len(params.Args) > 0 {
			cmd = exec.CommandContext(ctx, params.Command, params.Args...)
		} else {
			cmd = exec.CommandContext(ctx, params.Command)
		}
	}

	// Set working directory
	if params.WorkingDir != "" {
		cmd.Dir = params.WorkingDir
	}

	// Execute command
	stdout, err := cmd.Output()
	duration := time.Since(start)

	result := ShellResult{
		Command:    params.Command,
		WorkingDir: workingDir,
		Duration:   duration.String(),
		Operation:  "Execute",
	}

	if err != nil {
		// Handle different types of errors
		if exitError, ok := err.(*exec.ExitError); ok {
			result.ExitCode = exitError.ExitCode()
			result.Stderr = string(exitError.Stderr)
			result.Success = false
		} else {
			result.Error = fmt.Sprintf("execution failed: %v", err)
			result.Success = false
			result.ExitCode = -1
		}
	} else {
		result.ExitCode = 0
		result.Success = true
	}

	result.Stdout = string(stdout)

	// Truncate output if too long to avoid token overflow
	if len(result.Stdout) > 5000 {
		result.Stdout = result.Stdout[:5000] + "\n[... output truncated ...]"
	}
	if len(result.Stderr) > 2000 {
		result.Stderr = result.Stderr[:2000] + "\n[... error output truncated ...]"
	}

	return result, nil
}

// GetSystemInfo 获取各种系统信息
func (t *ShellTool) getSystemInfo(ctx context.Context, params SystemInfoParams) (interface{}, error) {
	if params.InfoType == "" {
		return nil, fmt.Errorf("info_type is required")
	}

	infoType := strings.ToLower(params.InfoType)

	switch infoType {
	case "os":
		return map[string]interface{}{
			"os":         runtime.GOOS,
			"arch":       runtime.GOARCH,
			"num_cpu":    runtime.NumCPU(),
			"go_version": runtime.Version(),
			"operation":  "GetSystemInfo",
			"info_type":  "os",
		}, nil

	case "env":
		env := make(map[string]string)
		for _, e := range os.Environ() {
			parts := strings.SplitN(e, "=", 2)
			if len(parts) == 2 {
				env[parts[0]] = parts[1]
			}
		}
		return map[string]interface{}{
			"environment": env,
			"operation":   "GetSystemInfo",
			"info_type":   "env",
		}, nil

	case "path":
		pathVar := os.Getenv("PATH")
		var paths []string
		if runtime.GOOS == "windows" {
			paths = strings.Split(pathVar, ";")
		} else {
			paths = strings.Split(pathVar, ":")
		}
		return map[string]interface{}{
			"path_variable": pathVar,
			"paths":         paths,
			"operation":     "GetSystemInfo",
			"info_type":     "path",
		}, nil

	case "user":
		homeDir, _ := os.UserHomeDir()
		currentDir, _ := os.Getwd()
		return map[string]interface{}{
			"home_directory":    homeDir,
			"current_directory": currentDir,
			"user":              os.Getenv("USER"),
			"username":          os.Getenv("USERNAME"), // Windows
			"operation":         "GetSystemInfo",
			"info_type":         "user",
		}, nil

	case "disk":
		currentDir, _ := os.Getwd()
		// Get disk usage for current directory
		var diskInfo map[string]interface{}

		if runtime.GOOS == "windows" {
			// For Windows, we'd need to use syscalls for accurate disk info
			diskInfo = map[string]interface{}{
				"current_directory": currentDir,
				"note":              "Detailed disk information requires platform-specific implementation",
			}
		} else {
			// For Unix-like systems, we could use statvfs syscall
			diskInfo = map[string]interface{}{
				"current_directory": currentDir,
				"note":              "Detailed disk information requires platform-specific implementation",
			}
		}

		diskInfo["operation"] = "GetSystemInfo"
		diskInfo["info_type"] = "disk"
		return diskInfo, nil

	case "memory":
		var m runtime.MemStats
		runtime.ReadMemStats(&m)

		return map[string]interface{}{
			"alloc_mb":       bToMb(m.Alloc),
			"total_alloc_mb": bToMb(m.TotalAlloc),
			"sys_mb":         bToMb(m.Sys),
			"num_gc":         m.NumGC,
			"goroutines":     runtime.NumGoroutine(),
			"operation":      "GetSystemInfo",
			"info_type":      "memory",
		}, nil

	default:
		return nil, fmt.Errorf("unsupported info_type: %s. Supported: os, env, path, user, disk, memory", infoType)
	}
}

// ListProcesses 列出运行中的进程（简化版本）
func (t *ShellTool) listProcesses(ctx context.Context, params any) (interface{}, error) {
	var cmd *exec.Cmd

	switch runtime.GOOS {
	case "windows":
		cmd = exec.Command("tasklist", "/FO", "CSV")
	case "darwin", "linux":
		cmd = exec.Command("ps", "aux")
	default:
		return nil, fmt.Errorf("process listing not supported on %s", runtime.GOOS)
	}

	output, err := cmd.Output()
	if err != nil {
		return ShellResult{
			Command:   "ps/tasklist",
			Success:   false,
			Error:     fmt.Sprintf("failed to list processes: %v", err),
			Operation: "ListProcesses",
		}, nil
	}

	outputStr := string(output)
	// Truncate if too long
	if len(outputStr) > 8000 {
		outputStr = outputStr[:8000] + "\n[... output truncated ...]"
	}

	return map[string]interface{}{
		"processes": outputStr,
		"operation": "ListProcesses",
		"platform":  runtime.GOOS,
	}, nil
}

func (t *ShellTool) getDirectory(ctx context.Context, params DirectoryParams) (string, error) {

	var result interface{}
	var err error

	switch params.Operation {
	case "get":
		result, err = t.GetCurrentDirectory()
	case "change":
		if params.Path == "" {
			return "", fmt.Errorf("path is required for change operation")
		}
		result, err = t.ChangeDirectory(params.Path)
	default:
		return "", fmt.Errorf("unsupported operation: %s", params.Operation)
	}

	if err != nil {
		return "", err
	}

	resultJSON, err := json.Marshal(result)
	if err != nil {
		return "", fmt.Errorf("failed to marshal result: %w", err)
	}

	return string(resultJSON), nil
}

// GetCurrentDirectory 获取当前工作目录
func (t *ShellTool) GetCurrentDirectory() (interface{}, error) {
	currentDir, err := os.Getwd()
	if err != nil {
		return ShellResult{
			Success:   false,
			Error:     fmt.Sprintf("failed to get current directory: %v", err),
			Operation: "GetCurrentDirectory",
		}, nil
	}

	return map[string]interface{}{
		"current_directory": currentDir,
		"absolute_path":     filepath.IsAbs(currentDir),
		"operation":         "GetCurrentDirectory",
	}, nil
}

// ChangeDirectory 切换当前工作目录
func (t *ShellTool) ChangeDirectory(path string) (interface{}, error) {
	if path == "" {
		return nil, fmt.Errorf("path is required")
	}

	// Get current directory before change
	oldDir, _ := os.Getwd()

	err := os.Chdir(path)
	if err != nil {
		return ShellResult{
			Success:   false,
			Error:     fmt.Sprintf("failed to change directory: %v", err),
			Operation: "ChangeDirectory",
		}, nil
	}

	// Get new directory after change
	newDir, _ := os.Getwd()

	return map[string]interface{}{
		"old_directory":  oldDir,
		"new_directory":  newDir,
		"requested_path": path,
		"operation":      "ChangeDirectory",
		"success":        true,
	}, nil
}

// bToMb 辅助函数，将字节转换为兆字节
func bToMb(b uint64) uint64 {
	return b / 1024 / 1024
}
