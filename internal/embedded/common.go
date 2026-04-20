package embedded

import (
	"bytes"
	"crypto/sha256"
	"encoding/hex"
	"fmt"
	"io"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"sync"
	"time"

	"github.com/bodgit/sevenzip"
)

const appDirName = ".easy-ffmpeg"

type Progress struct {
	State   string `json:"state"`   // "idle" | "extracting" | "ready" | "error"
	Percent int    `json:"percent"` // 0-100
	Current string `json:"current,omitempty"`
	Error   string `json:"error,omitempty"`
}

var (
	extractOnce sync.Once
	extractDir  string
	extractErr  error

	progressMu sync.Mutex
	progress   = Progress{State: "idle"}
	doneBytes  int64
	totalBytes int64
)

// GetProgress returns a snapshot of the extraction progress.
func GetProgress() Progress {
	progressMu.Lock()
	defer progressMu.Unlock()
	return progress
}

func setProgress(fn func(p *Progress)) {
	progressMu.Lock()
	defer progressMu.Unlock()
	fn(&progress)
}

func cacheDir() (string, error) {
	home, err := os.UserHomeDir()
	if err != nil {
		return "", err
	}
	sum := sha256.Sum256(archiveData)
	hash := hex.EncodeToString(sum[:4])
	return filepath.Join(home, appDirName, "bin-"+hash), nil
}

func ensureExtracted() (string, error) {
	extractOnce.Do(func() {
		extractDir, extractErr = extractArchive()
	})
	return extractDir, extractErr
}

func extractArchive() (string, error) {
	dir, err := cacheDir()
	if err != nil {
		setProgress(func(p *Progress) { p.State = "error"; p.Error = err.Error() })
		return "", err
	}

	marker := filepath.Join(dir, ".ok")
	if fileExists(marker) {
		setProgress(func(p *Progress) { p.State = "ready"; p.Percent = 100 })
		return dir, nil
	}

	if err := os.MkdirAll(dir, 0755); err != nil {
		setProgress(func(p *Progress) { p.State = "error"; p.Error = err.Error() })
		return "", err
	}

	fmt.Fprintf(os.Stderr, "  首次启动：正在解压 FFmpeg 到 %s ...\n", dir)
	start := time.Now()

	reader, err := sevenzip.NewReader(bytes.NewReader(archiveData), int64(len(archiveData)))
	if err != nil {
		setProgress(func(p *Progress) { p.State = "error"; p.Error = err.Error() })
		return "", fmt.Errorf("open embedded 7z: %w", err)
	}

	totalBytes = 0
	for _, f := range reader.File {
		totalBytes += int64(f.UncompressedSize)
	}
	doneBytes = 0
	setProgress(func(p *Progress) { p.State = "extracting"; p.Percent = 0 })

	printer := startProgressPrinter()
	defer printer.Stop()

	for _, f := range reader.File {
		setProgress(func(p *Progress) { p.Current = f.Name })
		if err := extractOne(f, dir); err != nil {
			setProgress(func(p *Progress) { p.State = "error"; p.Error = err.Error() })
			return "", fmt.Errorf("extract %s: %w", f.Name, err)
		}
	}

	if err := os.WriteFile(marker, nil, 0644); err != nil {
		setProgress(func(p *Progress) { p.State = "error"; p.Error = err.Error() })
		return "", err
	}

	setProgress(func(p *Progress) { p.State = "ready"; p.Percent = 100; p.Current = "" })
	printer.Stop()
	fmt.Fprintf(os.Stderr, "  解压完成 (%.1fs)\n", time.Since(start).Seconds())
	return dir, nil
}

type progressPrinter struct {
	stopCh  chan struct{}
	doneCh  chan struct{}
	stopped sync.Once
}

func (pp *progressPrinter) Stop() {
	pp.stopped.Do(func() {
		close(pp.stopCh)
		<-pp.doneCh
	})
}

func startProgressPrinter() *progressPrinter {
	pp := &progressPrinter{stopCh: make(chan struct{}), doneCh: make(chan struct{})}
	go func() {
		defer close(pp.doneCh)
		ticker := time.NewTicker(200 * time.Millisecond)
		defer ticker.Stop()
		draw := func() {
			p := GetProgress()
			const width = 30
			n := p.Percent * width / 100
			if n < 0 {
				n = 0
			}
			if n > width {
				n = width
			}
			bar := strings.Repeat("█", n) + strings.Repeat("░", width-n)
			fmt.Fprintf(os.Stderr, "\r  [%s] %3d%%  %-14s", bar, p.Percent, p.Current)
		}
		for {
			select {
			case <-pp.stopCh:
				draw()
				fmt.Fprintln(os.Stderr)
				return
			case <-ticker.C:
				draw()
			}
		}
	}()
	return pp
}

func extractOne(f *sevenzip.File, destDir string) error {
	dest := filepath.Join(destDir, f.Name)
	rc, err := f.Open()
	if err != nil {
		return err
	}
	defer rc.Close()
	out, err := os.Create(dest)
	if err != nil {
		return err
	}
	defer out.Close()

	counter := &progressWriter{w: out}
	if _, err := io.Copy(counter, rc); err != nil {
		return err
	}
	return os.Chmod(dest, 0755)
}

type progressWriter struct{ w io.Writer }

func (pw *progressWriter) Write(p []byte) (int, error) {
	n, err := pw.w.Write(p)
	if n > 0 {
		progressMu.Lock()
		doneBytes += int64(n)
		if totalBytes > 0 {
			progress.Percent = int(doneBytes * 100 / totalBytes)
		}
		progressMu.Unlock()
	}
	return n, err
}

func fileExists(path string) bool {
	_, err := os.Stat(path)
	return err == nil
}

func GetFFmpegBinary() (string, error) {
	dir, err := ensureExtracted()
	if err != nil {
		return "", err
	}
	return filepath.Join(dir, ffmpegBinaryName), nil
}

func GetFFprobeBinary() (string, error) {
	dir, err := ensureExtracted()
	if err != nil {
		return "", err
	}
	return filepath.Join(dir, ffprobeBinaryName), nil
}

func CheckEmbeddedFFmpeg() bool {
	path, err := GetFFmpegBinary()
	if err != nil {
		return false
	}
	return exec.Command(path, "-version").Run() == nil
}
