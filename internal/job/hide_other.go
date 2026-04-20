//go:build !windows

package job

import "os/exec"

func hideWindow(cmd *exec.Cmd) {}
