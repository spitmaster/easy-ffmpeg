//go:build !windows

package procutil

import "os/exec"

// HideWindow is a no-op on non-Windows platforms.
func HideWindow(cmd *exec.Cmd) {}

// HideWindowLowPriority is also a no-op here. On macOS/Linux, ffmpeg
// already cooperates with the desktop scheduler and rarely freezes the
// UI; if that turns out to be wrong, we can introduce nice(2) here.
func HideWindowLowPriority(cmd *exec.Cmd) {}
