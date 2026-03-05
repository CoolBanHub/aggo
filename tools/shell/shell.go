package shell

import (
	"bytes"
	"context"
	"errors"
	"fmt"
	"os"
	"os/exec"
	"runtime"
	"time"

	"github.com/cloudwego/eino/components/tool"
	"github.com/cloudwego/eino/components/tool/utils"
)

// GetTools 获取全部 Shell 工具
func GetTools() []tool.BaseTool {
	this := &ShellTool{}
	return []tool.BaseTool{
		this.newShellExecuteTool(),
	}
}

// GetExecuteTools 获取命令执行工具
func GetExecuteTools() []tool.BaseTool {
	this := &ShellTool{}
	return []tool.BaseTool{
		this.newShellExecuteTool(),
	}
}

type ShellTool struct{}

// ExecuteParams 表示命令执行的参数
type ExecuteParams struct {
	Command    string `json:"command" jsonschema:"description=要执行的shell命令,required"`
	WorkingDir string `json:"workingDir,omitempty" jsonschema:"description=命令执行的工作目录"`
	Timeout    int    `json:"timeout,omitempty" jsonschema:"description=超时时间（秒），默认为60秒"`
}

// ToolResult 表示命令执行的可读结果
type ToolResult struct {
	Output  string `json:"output"`  // 命令输出的结果
	IsError bool   `json:"isError"` // 是否发生错误
}

// newShellExecuteTool 创建一个新的 ShellExecuteTool 实例
func (this *ShellTool) newShellExecuteTool() tool.InvokableTool {
	name := "shell_execute"
	desc := "Execute a shell command and return its output. Supports both Unix and Windows systems."
	t, _ := utils.InferTool(name, desc, this.executeCommand)
	return t
}

// executeCommand 在系统shell中运行命令
func (t *ShellTool) executeCommand(ctx context.Context, params ExecuteParams) (*ToolResult, error) {
	if params.Command == "" {
		return &ToolResult{Output: "command is required", IsError: true}, nil
	}

	cwd := params.WorkingDir
	if cwd == "" {
		if wd, err := os.Getwd(); err == nil {
			cwd = wd
		}
	}

	timeout := 60 * time.Second
	if params.Timeout > 0 {
		timeout = time.Duration(params.Timeout) * time.Second
	}

	var cmdCtx context.Context
	var cancel context.CancelFunc
	if timeout > 0 {
		cmdCtx, cancel = context.WithTimeout(ctx, timeout)
	} else {
		cmdCtx, cancel = context.WithCancel(ctx)
	}
	defer cancel()

	var cmd *exec.Cmd
	if runtime.GOOS == "windows" {
		cmd = exec.CommandContext(cmdCtx, "powershell", "-NoProfile", "-NonInteractive", "-Command", params.Command)
	} else {
		cmd = exec.CommandContext(cmdCtx, "sh", "-c", params.Command)
	}

	if cwd != "" {
		cmd.Dir = cwd
	}

	prepareCommandForTermination(cmd)

	var stdout, stderr bytes.Buffer
	cmd.Stdout = &stdout
	cmd.Stderr = &stderr

	if err := cmd.Start(); err != nil {
		return &ToolResult{Output: fmt.Sprintf("failed to start command: %v", err), IsError: true}, nil
	}

	done := make(chan error, 1)
	go func() {
		done <- cmd.Wait()
	}()

	var err error
	select {
	case err = <-done:
	case <-cmdCtx.Done():
		_ = terminateProcessTree(cmd)
		select {
		case err = <-done:
		case <-time.After(2 * time.Second):
			if cmd.Process != nil {
				_ = cmd.Process.Kill()
			}
			err = <-done
		}
	}

	output := stdout.String()
	if stderr.Len() > 0 {
		// Only add STDERR if there's actual error output to make it cleaner when empty
		if output != "" {
			output += "\nSTDERR:\n"
		} else {
			output = "STDERR:\n"
		}
		output += stderr.String()
	}

	if err != nil {
		if errors.Is(cmdCtx.Err(), context.DeadlineExceeded) {
			return &ToolResult{Output: fmt.Sprintf("Command timed out after %v", timeout), IsError: true}, nil
		}
		output += fmt.Sprintf("\nExit code: %v", err)
	}

	if output == "" {
		output = "(no output)"
	}

	maxLen := 10000
	if len(output) > maxLen {
		output = output[:maxLen] + fmt.Sprintf("\n... (truncated, %d more chars)", len(output)-maxLen)
	}

	return &ToolResult{
		Output:  output,
		IsError: err != nil,
	}, nil
}
