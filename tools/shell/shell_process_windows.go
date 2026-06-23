//go:build windows

package shell

import (
	"os/exec"
	"strconv"
	"syscall"
)

func prepareCommandForTermination(cmd *exec.Cmd) {
	if cmd == nil {
		return
	}
	cmd.SysProcAttr = &syscall.SysProcAttr{HideWindow: true}
}

func terminateProcessTree(cmd *exec.Cmd) error {
	if cmd == nil || cmd.Process == nil {
		return nil
	}

	pid := cmd.Process.Pid
	if pid <= 0 {
		return nil
	}

	taskkill := exec.Command("taskkill", "/T", "/F", "/PID", strconv.Itoa(pid))
	taskkill.SysProcAttr = &syscall.SysProcAttr{HideWindow: true}
	_ = taskkill.Run()
	_ = cmd.Process.Kill()
	return nil
}
