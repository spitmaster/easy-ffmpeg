package job

import (
	"bufio"
	"bytes"
	"fmt"
	"io"
	"os/exec"
	"strings"
	"sync"
	"time"
)

const progressInterval = 100 * time.Millisecond

func isProgressLine(s string) bool {
	return strings.HasPrefix(s, "frame=") || strings.HasPrefix(s, "size=")
}

// scanLinesOrCR splits on either \n or \r so that ffmpeg's in-place
// progress updates (which use lone \r) become separate tokens instead
// of accumulating in the buffer until the next real newline.
func scanLinesOrCR(data []byte, atEOF bool) (advance int, token []byte, err error) {
	if atEOF && len(data) == 0 {
		return 0, nil, nil
	}
	if i := bytes.IndexAny(data, "\r\n"); i >= 0 {
		advance = i + 1
		if data[i] == '\r' && i+1 < len(data) && data[i+1] == '\n' {
			advance = i + 2
		}
		return advance, data[:i], nil
	}
	if atEOF {
		return len(data), data, nil
	}
	return 0, nil, nil
}

type Event struct {
	Type    string `json:"type"` // "state" | "log" | "done" | "error" | "cancelled"
	Line    string `json:"line,omitempty"`
	Message string `json:"message,omitempty"`
	Running bool   `json:"running,omitempty"`
}

type Manager struct {
	mu          sync.Mutex
	cmd         *exec.Cmd
	cancelled   bool
	subscribers map[chan Event]struct{}
	running     bool
}

func New() *Manager {
	return &Manager{subscribers: make(map[chan Event]struct{})}
}

func (m *Manager) Running() bool {
	m.mu.Lock()
	defer m.mu.Unlock()
	return m.running
}

// Start launches ffmpeg asynchronously.
// Returns an error if another job is already running.
func (m *Manager) Start(binary string, args []string) error {
	m.mu.Lock()
	if m.running {
		m.mu.Unlock()
		return fmt.Errorf("another job is running")
	}
	cmd := exec.Command(binary, args...)
	hideWindow(cmd)
	stderr, err := cmd.StderrPipe()
	if err != nil {
		m.mu.Unlock()
		return err
	}
	if err := cmd.Start(); err != nil {
		m.mu.Unlock()
		return err
	}
	m.cmd = cmd
	m.cancelled = false
	m.running = true
	m.mu.Unlock()

	go m.pump(cmd, stderr)
	return nil
}

func (m *Manager) pump(cmd *exec.Cmd, stderr io.ReadCloser) {
	scanner := bufio.NewScanner(stderr)
	scanner.Buffer(make([]byte, 64*1024), 1024*1024)
	scanner.Split(scanLinesOrCR)

	var lastEmit time.Time
	var pendingProgress string
	for scanner.Scan() {
		line := scanner.Text()
		if line == "" {
			continue
		}
		if isProgressLine(line) {
			pendingProgress = line
			if time.Since(lastEmit) < progressInterval {
				continue
			}
			m.broadcast(Event{Type: "log", Line: line})
			pendingProgress = ""
			lastEmit = time.Now()
			continue
		}
		// Flush any pending throttled progress line before a non-progress
		// line, otherwise fast transcodes (e.g. speed=45x) lose the final
		// "frame= ..." summary to the next summary line.
		if pendingProgress != "" {
			m.broadcast(Event{Type: "log", Line: pendingProgress})
			pendingProgress = ""
		}
		m.broadcast(Event{Type: "log", Line: line})
		lastEmit = time.Now()
	}
	if pendingProgress != "" {
		m.broadcast(Event{Type: "log", Line: pendingProgress})
	}
	waitErr := cmd.Wait()

	m.mu.Lock()
	cancelled := m.cancelled
	m.cmd = nil
	m.running = false
	m.cancelled = false
	m.mu.Unlock()

	switch {
	case cancelled:
		m.broadcast(Event{Type: "cancelled"})
	case waitErr != nil:
		m.broadcast(Event{Type: "error", Message: waitErr.Error()})
	default:
		m.broadcast(Event{Type: "done"})
	}
}

func (m *Manager) broadcast(ev Event) {
	m.mu.Lock()
	defer m.mu.Unlock()
	for ch := range m.subscribers {
		select {
		case ch <- ev:
		default:
			// drop if subscriber is slow; don't block ffmpeg progress
		}
	}
}

// Cancel kills the current job if any.
func (m *Manager) Cancel() {
	m.mu.Lock()
	cmd := m.cmd
	if cmd != nil {
		m.cancelled = true
	}
	m.mu.Unlock()
	if cmd != nil && cmd.Process != nil {
		_ = cmd.Process.Kill()
	}
}

// Subscribe returns a channel of events plus an unsubscribe func.
// An initial "state" event is delivered immediately.
func (m *Manager) Subscribe() (<-chan Event, func()) {
	ch := make(chan Event, 256)
	m.mu.Lock()
	m.subscribers[ch] = struct{}{}
	running := m.running
	m.mu.Unlock()

	ch <- Event{Type: "state", Running: running}

	unsub := func() {
		m.mu.Lock()
		if _, ok := m.subscribers[ch]; ok {
			delete(m.subscribers, ch)
			close(ch)
		}
		m.mu.Unlock()
	}
	return ch, unsub
}
