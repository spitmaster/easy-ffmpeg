package server

import (
	"context"
	"easy-ffmpeg/internal/job"
	"embed"
	"io/fs"
	"log"
	"net"
	"net/http"
	"strings"
	"time"
)

//go:embed web
var webRoot embed.FS

type Server struct {
	httpSrv *http.Server
	jobs    *job.Manager
	quit    chan struct{}
}

func New() *Server {
	s := &Server{
		jobs: job.New(),
		quit: make(chan struct{}),
	}
	mux := http.NewServeMux()
	s.routes(mux)
	s.httpSrv = &http.Server{Handler: logMiddleware(mux)}
	return s
}

// Endpoints that are polled frequently and would flood the console.
var silentPaths = map[string]bool{
	"/api/prepare/status": true,
}

func logMiddleware(next http.Handler) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		start := time.Now()
		next.ServeHTTP(w, r)
		if !strings.HasPrefix(r.URL.Path, "/api/") {
			return
		}
		if silentPaths[r.URL.Path] {
			return
		}
		log.Printf("%s %s (%s) %s", r.Method, r.URL.Path, r.RemoteAddr, time.Since(start))
	})
}

func (s *Server) routes(mux *http.ServeMux) {
	sub, _ := fs.Sub(webRoot, "web")
	mux.Handle("/", http.FileServer(http.FS(sub)))

	mux.HandleFunc("/api/ffmpeg/status", s.handleFFmpegStatus)
	mux.HandleFunc("/api/ffmpeg/reveal", s.handleFFmpegReveal)
	mux.HandleFunc("/api/prepare/status", s.handlePrepareStatus)
	mux.HandleFunc("/api/fs/list", s.handleFsList)
	mux.HandleFunc("/api/fs/home", s.handleFsHome)
	mux.HandleFunc("/api/fs/reveal", s.handleFsReveal)
	mux.HandleFunc("/api/config/dirs", s.handleConfigDirs)
	mux.HandleFunc("/api/convert/start", s.handleConvertStart)
	mux.HandleFunc("/api/convert/cancel", s.handleConvertCancel)
	mux.HandleFunc("/api/convert/stream", s.handleConvertStream)
	mux.HandleFunc("/api/audio/probe", s.handleAudioProbe)
	mux.HandleFunc("/api/audio/start", s.handleAudioStart)
	mux.HandleFunc("/api/audio/cancel", s.handleAudioCancel)
	mux.HandleFunc("/api/version", s.handleVersion)
	mux.HandleFunc("/api/quit", s.handleQuit)

	// Video editor module — registers /api/editor/* routes.
	// Failure here is non-fatal: the rest of the app keeps working.
	if mod, dataDir, err := s.buildEditorModule(); err != nil {
		log.Printf("editor: disabled (%v)", err)
	} else {
		mod.Register(mux, "/api/editor")
		log.Printf("editor: mounted at /api/editor (data: %s)", dataDir)
	}
}

// Listen binds to the given host:port and returns the actual address.
// Pass ":0" to get a random port.
func (s *Server) Listen(addr string) (string, error) {
	ln, err := net.Listen("tcp", addr)
	if err != nil {
		return "", err
	}
	go func() {
		_ = s.httpSrv.Serve(ln)
	}()
	return ln.Addr().String(), nil
}

// Wait blocks until a shutdown is requested via /api/quit.
func (s *Server) Wait() {
	<-s.quit
	ctx, cancel := context.WithTimeout(context.Background(), 3*time.Second)
	defer cancel()
	_ = s.httpSrv.Shutdown(ctx)
}

// RequestShutdown signals Wait to return. Safe to call multiple times.
func (s *Server) RequestShutdown() {
	select {
	case <-s.quit:
	default:
		close(s.quit)
	}
}

// Quit returns a channel that is closed once RequestShutdown has been
// called. Lets callers observe the shutdown signal without going through
// Wait's synchronous httpSrv.Shutdown — whose 3s graceful timeout
// dominates the perceived close latency when long-lived SSE streams are
// still open. The desktop entry uses this to close its window the
// instant the user clicks "退出"; the Go process exiting takes care of
// the HTTP listener regardless.
func (s *Server) Quit() <-chan struct{} { return s.quit }
