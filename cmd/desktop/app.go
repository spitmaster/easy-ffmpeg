package main

import (
	"context"
	"log"

	"easy-ffmpeg/server"
	"easy-ffmpeg/service"

	wruntime "github.com/wailsapp/wails/v2/pkg/runtime"
)

// App holds the desktop process state. It is intentionally thin: all
// business logic lives in the shared *server.Server, identical to the Web
// build. This struct only owns "when to start" and "when to stop".
//
// See design/v0.4.0-architecture.md §4.1.
type App struct {
	ctx context.Context
	srv *server.Server
	url string
}

// startup is fired by Wails after the main window has been created but
// before the first frame is rendered. We boot the same backend the Web
// build uses, then remember the bound URL so domReady can hand it to the
// shell page.
func (a *App) startup(ctx context.Context) {
	a.ctx = ctx

	a.srv = server.New()
	bound, err := a.srv.Listen("127.0.0.1:0")
	if err != nil {
		log.Fatalf("desktop: listen failed: %v", err)
	}
	a.url = "http://" + bound + "/"
	log.Printf("desktop: backend bound at %s", a.url)

	go func() {
		if err := service.Prepare(); err != nil {
			log.Printf("desktop: ffmpeg prepare failed: %v", err)
		}
	}()
}

// domReady fires after the shell page's DOM is parsed and its
// EventsOn('backend-ready', ...) listener is attached. Emitting the URL
// here guarantees the listener exists when the event arrives, which is
// the contract the P2 fallback path relies on (architecture §4.2).
func (a *App) domReady(ctx context.Context) {
	wruntime.EventsEmit(ctx, "backend-ready", a.url)
}

// shutdown is invoked when the user closes the main window. We reuse the
// same graceful path as the Web build's Ctrl+C / /api/quit handler.
func (a *App) shutdown(ctx context.Context) {
	if a.srv != nil {
		a.srv.RequestShutdown()
	}
}
