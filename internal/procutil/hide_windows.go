//go:build windows

package procutil

import (
	"os/exec"
	"syscall"
)

// HideWindow prevents the child process from flashing a console window on Windows.
func HideWindow(cmd *exec.Cmd) {
	cmd.SysProcAttr = &syscall.SysProcAttr{
		HideWindow:    true,
		CreationFlags: 0x08000000, // CREATE_NO_WINDOW
	}
}
