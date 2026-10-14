package processexec

import (
	"context"
	"testing"
)

func TestNoConsole(t *testing.T) {
	cmd := CommandContext(context.Background(), "cmd.exe", "/c", "exit", "0")
	if cmd.SysProcAttr == nil || !cmd.SysProcAttr.HideWindow || cmd.SysProcAttr.CreationFlags&0x08000000 == 0 {
		t.Fatal("background child could allocate a visible console")
	}
	if err := cmd.Run(); err != nil {
		t.Fatal(err)
	}
}
