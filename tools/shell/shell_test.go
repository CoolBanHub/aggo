package shell

import (
	"testing"
)

func TestValidateCommandRejectsDeniedCommand(t *testing.T) {
	tool := newShellTool()

	if err := tool.validateCommand("rm -rf /tmp/example"); err == nil {
		t.Fatal("expected denied command error")
	}
}

func TestValidateCommandAllowsDeniedWordAsArgument(t *testing.T) {
	tool := newShellTool(WithAllowedCommands("echo"))

	if err := tool.validateCommand("echo rm"); err != nil {
		t.Fatalf("validateCommand returned error: %v", err)
	}
}

func TestResolveWorkingDirRejectsPathOutsideRoot(t *testing.T) {
	tool := newShellTool(WithWorkingDirRoot("/tmp/root"))

	if _, err := tool.resolveWorkingDir("/tmp/other"); err == nil {
		t.Fatal("expected workingDir outside root to be rejected")
	}
}
