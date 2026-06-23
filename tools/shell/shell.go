package shell

import (
	"bytes"
	"context"
	"errors"
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"runtime"
	"strings"
	"time"
	"unicode"

	"github.com/cloudwego/eino/components/tool"
	"github.com/cloudwego/eino/components/tool/utils"
)

const (
	defaultTimeoutSeconds = 60
	defaultMaxTimeout     = 2 * time.Minute
	defaultMaxOutputBytes = 10_000
)

var defaultDeniedCommands = []string{
	"rm", "rmdir", "dd", "mkfs", "mount", "umount",
	"shutdown", "reboot", "halt", "poweroff",
	"sudo", "su", "chmod", "chown", "chgrp",
}

// GetTools 获取全部 Shell 工具
func GetTools(opts ...Option) []tool.BaseTool {
	shellTool := newShellTool(opts...)
	return []tool.BaseTool{
		shellTool.newShellExecuteTool(),
	}
}

// GetExecuteTools 获取命令执行工具
func GetExecuteTools(opts ...Option) []tool.BaseTool {
	shellTool := newShellTool(opts...)
	return []tool.BaseTool{
		shellTool.newShellExecuteTool(),
	}
}

// Option configures shell tool execution policy.
type Option func(*ShellTool)

type ShellTool struct {
	workingDirRoot string
	allowed        map[string]struct{}
	denied         map[string]struct{}
	defaultTimeout time.Duration
	maxTimeout     time.Duration
	maxOutputBytes int
}

func newShellTool(opts ...Option) *ShellTool {
	root, _ := os.Getwd()
	root, _ = filepath.Abs(root)

	t := &ShellTool{
		workingDirRoot: root,
		denied:         commandSet(defaultDeniedCommands),
		defaultTimeout: defaultTimeoutSeconds * time.Second,
		maxTimeout:     defaultMaxTimeout,
		maxOutputBytes: defaultMaxOutputBytes,
	}
	for _, opt := range opts {
		if opt != nil {
			opt(t)
		}
	}
	return t
}

// WithWorkingDirRoot restricts WorkingDir to root or one of its descendants.
func WithWorkingDirRoot(root string) Option {
	return func(t *ShellTool) {
		if strings.TrimSpace(root) == "" {
			return
		}
		if abs, err := filepath.Abs(root); err == nil {
			t.workingDirRoot = filepath.Clean(abs)
		}
	}
}

// WithUnrestrictedWorkingDir disables WorkingDir root validation.
func WithUnrestrictedWorkingDir() Option {
	return func(t *ShellTool) {
		t.workingDirRoot = ""
	}
}

// WithAllowedCommands restricts execution to simple commands whose executable
// name is in the allowlist. Shell operators are rejected when this is set.
func WithAllowedCommands(commands ...string) Option {
	return func(t *ShellTool) {
		t.allowed = commandSet(commands)
	}
}

// WithDeniedCommands adds executable names that should never be run.
func WithDeniedCommands(commands ...string) Option {
	return func(t *ShellTool) {
		if t.denied == nil {
			t.denied = map[string]struct{}{}
		}
		for cmd := range commandSet(commands) {
			t.denied[cmd] = struct{}{}
		}
	}
}

// WithMaxTimeout caps caller-provided command timeouts.
func WithMaxTimeout(timeout time.Duration) Option {
	return func(t *ShellTool) {
		if timeout > 0 {
			t.maxTimeout = timeout
		}
	}
}

// WithDefaultTimeout sets the timeout used when params.Timeout is omitted.
func WithDefaultTimeout(timeout time.Duration) Option {
	return func(t *ShellTool) {
		if timeout > 0 {
			t.defaultTimeout = timeout
		}
	}
}

// WithMaxOutputBytes caps captured stdout/stderr to avoid unbounded memory use.
func WithMaxOutputBytes(n int) Option {
	return func(t *ShellTool) {
		if n > 0 {
			t.maxOutputBytes = n
		}
	}
}

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
func (t *ShellTool) newShellExecuteTool() tool.InvokableTool {
	name := "shell_execute"
	desc := "Execute a shell command and return its output. Supports both Unix and Windows systems."
	inferred, _ := utils.InferTool(name, desc, t.executeCommand)
	return inferred
}

// executeCommand 在系统shell中运行命令
func (t *ShellTool) executeCommand(ctx context.Context, params ExecuteParams) (*ToolResult, error) {
	params.Command = strings.TrimSpace(params.Command)
	if params.Command == "" {
		return &ToolResult{Output: "command is required", IsError: true}, nil
	}
	if err := t.validateCommand(params.Command); err != nil {
		return &ToolResult{Output: err.Error(), IsError: true}, nil
	}

	cwd, err := t.resolveWorkingDir(params.WorkingDir)
	if err != nil {
		return &ToolResult{Output: err.Error(), IsError: true}, nil
	}

	timeout := t.defaultTimeout
	if params.Timeout > 0 {
		timeout = time.Duration(params.Timeout) * time.Second
	}
	if t.maxTimeout > 0 && timeout > t.maxTimeout {
		timeout = t.maxTimeout
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
		psCommand := "[Console]::OutputEncoding=[System.Text.Encoding]::UTF8; " + params.Command
		cmd = exec.CommandContext(cmdCtx, "powershell", "-NoProfile", "-NonInteractive", "-Command", psCommand)
	} else {
		cmd = exec.CommandContext(cmdCtx, "sh", "-c", params.Command)
	}

	if cwd != "" {
		cmd.Dir = cwd
	}

	prepareCommandForTermination(cmd)

	stdout := newLimitedBuffer(t.maxOutputBytes)
	stderr := newLimitedBuffer(t.maxOutputBytes)
	cmd.Stdout = &stdout
	cmd.Stderr = &stderr

	if err := cmd.Start(); err != nil {
		return &ToolResult{Output: fmt.Sprintf("failed to start command: %v", err), IsError: true}, nil
	}

	done := make(chan error, 1)
	go func() {
		done <- cmd.Wait()
	}()

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

	if stdout.Truncated() > 0 || stderr.Truncated() > 0 {
		output += fmt.Sprintf("\n... (output truncated, stdout %d bytes, stderr %d bytes omitted)", stdout.Truncated(), stderr.Truncated())
	}

	maxLen := t.maxOutputBytes
	if len(output) > maxLen {
		output = output[:maxLen] + fmt.Sprintf("\n... (truncated, %d more chars)", len(output)-maxLen)
	}

	return &ToolResult{
		Output:  output,
		IsError: err != nil,
	}, nil
}

func (t *ShellTool) resolveWorkingDir(raw string) (string, error) {
	cwd := strings.TrimSpace(raw)
	if cwd == "" {
		if t.workingDirRoot != "" {
			return t.workingDirRoot, nil
		}
		wd, err := os.Getwd()
		if err != nil {
			return "", fmt.Errorf("failed to resolve working directory: %w", err)
		}
		return wd, nil
	}

	if !filepath.IsAbs(cwd) {
		base := t.workingDirRoot
		if base == "" {
			wd, err := os.Getwd()
			if err != nil {
				return "", fmt.Errorf("failed to resolve working directory: %w", err)
			}
			base = wd
		}
		cwd = filepath.Join(base, cwd)
	}

	abs, err := filepath.Abs(cwd)
	if err != nil {
		return "", fmt.Errorf("invalid working directory: %w", err)
	}
	abs = filepath.Clean(abs)
	if t.workingDirRoot != "" && !isPathWithin(abs, t.workingDirRoot) {
		return "", fmt.Errorf("workingDir must stay under %s", t.workingDirRoot)
	}
	return abs, nil
}

func (t *ShellTool) validateCommand(command string) error {
	commands := commandNames(command)
	for _, name := range commands {
		if _, denied := t.denied[name]; denied {
			return fmt.Errorf("command %q is denied by shell policy", name)
		}
	}

	if len(t.allowed) == 0 {
		return nil
	}
	if hasShellOperator(command) {
		return errors.New("shell operators are not allowed when allowedCommands is configured")
	}
	if len(commands) == 0 {
		return errors.New("command is required")
	}
	if _, ok := t.allowed[commands[0]]; !ok {
		return fmt.Errorf("command %q is not in allowedCommands", commands[0])
	}
	return nil
}

func commandSet(commands []string) map[string]struct{} {
	out := make(map[string]struct{}, len(commands))
	for _, cmd := range commands {
		cmd = normalizeCommandName(cmd)
		if cmd != "" {
			out[cmd] = struct{}{}
		}
	}
	return out
}

func normalizeCommandName(cmd string) string {
	cmd = strings.Trim(strings.TrimSpace(cmd), `"'`)
	cmd = filepath.Base(cmd)
	return strings.ToLower(cmd)
}

func commandTokens(command string) []string {
	fields := strings.FieldsFunc(command, func(r rune) bool {
		return unicode.IsSpace(r) || strings.ContainsRune(`;&|()<>`+"`", r)
	})
	tokens := make([]string, 0, len(fields))
	for _, field := range fields {
		token := normalizeCommandName(field)
		if token != "" {
			tokens = append(tokens, token)
		}
	}
	return tokens
}

func commandNames(command string) []string {
	segments := strings.FieldsFunc(command, func(r rune) bool {
		return strings.ContainsRune(`;&|()`+"`", r)
	})
	commands := make([]string, 0, len(segments))
	for _, segment := range segments {
		tokens := commandTokens(segment)
		if len(tokens) > 0 {
			commands = append(commands, tokens[0])
		}
	}
	return commands
}

func hasShellOperator(command string) bool {
	return strings.ContainsAny(command, `;&|<>`+"`")
}

func isPathWithin(path, root string) bool {
	rel, err := filepath.Rel(root, path)
	if err != nil {
		return false
	}
	return rel == "." || (rel != ".." && !strings.HasPrefix(rel, ".."+string(os.PathSeparator)))
}

type limitedBuffer struct {
	buf       bytes.Buffer
	limit     int
	truncated int
}

func newLimitedBuffer(limit int) limitedBuffer {
	if limit <= 0 {
		limit = defaultMaxOutputBytes
	}
	return limitedBuffer{limit: limit}
}

func (b *limitedBuffer) Write(p []byte) (int, error) {
	if b.limit <= 0 {
		b.truncated += len(p)
		return len(p), nil
	}
	remaining := b.limit - b.buf.Len()
	if remaining <= 0 {
		b.truncated += len(p)
		return len(p), nil
	}
	if len(p) > remaining {
		_, _ = b.buf.Write(p[:remaining])
		b.truncated += len(p) - remaining
		return len(p), nil
	}
	_, _ = b.buf.Write(p)
	return len(p), nil
}

func (b *limitedBuffer) String() string {
	return b.buf.String()
}

func (b *limitedBuffer) Len() int {
	return b.buf.Len()
}

func (b *limitedBuffer) Truncated() int {
	return b.truncated
}
