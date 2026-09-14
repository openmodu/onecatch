//go:build windows

package command

import (
	"os"

	"golang.org/x/sys/windows"
)

// A GUI-subsystem executable has no console by default. Attach to its caller
// for worker help/logs, preserving handles supplied by pipes and services.
func attachConsole() {
	attach := windows.NewLazySystemDLL("kernel32.dll").NewProc("AttachConsole")
	_, _, _ = attach.Call(uintptr(^uint32(0)))
	for _, stream := range []struct {
		id   uint32
		file **os.File
		name string
	}{
		{windows.STD_INPUT_HANDLE, &os.Stdin, "stdin"},
		{windows.STD_OUTPUT_HANDLE, &os.Stdout, "stdout"},
		{windows.STD_ERROR_HANDLE, &os.Stderr, "stderr"},
	} {
		if _, err := (*stream.file).Stat(); err == nil {
			continue
		}
		handle, err := windows.GetStdHandle(stream.id)
		if err == nil && handle != 0 && handle != windows.InvalidHandle {
			*stream.file = os.NewFile(uintptr(handle), stream.name)
		}
	}
}
