//go:build windows

package procutil

import (
	"os/exec"
	"syscall"
)

const (
	createNoWindow           = 0x08000000 // CREATE_NO_WINDOW
	belowNormalPriorityClass = 0x00004000 // BELOW_NORMAL_PRIORITY_CLASS
)

// HideWindow prevents the child process from flashing a console window on Windows.
func HideWindow(cmd *exec.Cmd) {
	cmd.SysProcAttr = &syscall.SysProcAttr{
		HideWindow:    true,
		CreationFlags: createNoWindow,
	}
}

// HideWindowLowPriority is HideWindow plus BELOW_NORMAL_PRIORITY_CLASS so a
// long-running ffmpeg encode doesn't starve the UI thread of CPU. Without
// this the Windows scheduler treats ffmpeg's threads-pinned-to-all-cores
// at the same priority as the WebView/Wails compositor, and the mouse
// cursor visibly stutters under sustained encoding load.
func HideWindowLowPriority(cmd *exec.Cmd) {
	cmd.SysProcAttr = &syscall.SysProcAttr{
		HideWindow:    true,
		CreationFlags: createNoWindow | belowNormalPriorityClass,
	}
}
