package ui

import (
	"os/exec"
	"sync"
)

// 当前运行的ffmpeg进程
var currentCmd *exec.Cmd
var cmdMutex sync.Mutex
