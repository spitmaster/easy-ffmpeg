// Package main is the desktop (Wails) entry of Easy FFmpeg. It boots the
// same shared backend the Web build uses, then loads a tiny HTML shell in
// the platform WebView. The shell receives the bound HTTP URL via a Wails
// event and navigates the WebView to it; from that point on the UI is
// byte-for-byte identical to the Web build.
//
// Design contract: this package is the *only* place in the repo allowed
// to import github.com/wailsapp/wails/v2/*. Everything else stays cgo-free
// so the Web build keeps its CGO_ENABLED=0 cross-compile property.
//
// See design/v0.4.0-architecture.md.
package main

import (
	"embed"
	"log"

	"github.com/wailsapp/wails/v2"
	"github.com/wailsapp/wails/v2/pkg/options"
	"github.com/wailsapp/wails/v2/pkg/options/assetserver"
)

//go:embed all:frontend/dist
var assets embed.FS

func main() {
	app := &App{}

	err := wails.Run(&options.App{
		Title:      "Easy FFmpeg",
		Width:      1280,
		Height:     800,
		MinWidth:   900,
		MinHeight:  600,
		OnStartup:  app.startup,
		OnDomReady: app.domReady,
		OnShutdown: app.shutdown,
		AssetServer: &assetserver.Options{
			Assets: assets,
		},
	})
	if err != nil {
		log.Fatalf("wails.Run: %v", err)
	}
}
