package main

import (
	"easy-ffmpeg/internal/browser"
	"easy-ffmpeg/server"
	"easy-ffmpeg/service"
	"fmt"
	"log"
	"os"
	"os/signal"
	"strings"
	"syscall"
)

func main() {
	addr := envOr("EASY_FFMPEG_ADDR", "127.0.0.1:0")

	srv := server.New()
	bound, err := srv.Listen(addr)
	if err != nil {
		log.Fatalf("listen failed: %v", err)
	}
	url := toURL(bound)

	fmt.Println()
	fmt.Println("  Easy FFmpeg 已启动")
	fmt.Println("  访问地址:", url)
	fmt.Println("  关闭服务: Ctrl+C  或  在网页右上角点击「退出」")
	fmt.Println()

	// Run extraction in background so the browser can open immediately
	// and show a progress UI polling /api/prepare/status.
	go func() {
		if err := service.Prepare(); err != nil {
			fmt.Println("  警告：嵌入 FFmpeg 准备失败，将尝试系统 PATH 中的 ffmpeg。原因:", err)
		}
	}()

	if err := browser.Open(url); err != nil {
		fmt.Println("  未能自动打开浏览器，请手动访问上面的地址。")
	}

	go func() {
		sig := make(chan os.Signal, 1)
		signal.Notify(sig, os.Interrupt, syscall.SIGTERM)
		<-sig
		srv.RequestShutdown()
	}()

	srv.Wait()
	fmt.Println("已退出。")
}

func envOr(key, def string) string {
	if v, ok := os.LookupEnv(key); ok && v != "" {
		return v
	}
	return def
}

func toURL(addr string) string {
	// addr like "127.0.0.1:54321" or "[::1]:54321"
	host := addr
	if strings.HasPrefix(addr, "0.0.0.0") || strings.HasPrefix(addr, "[::]:") {
		// bound to all interfaces; use localhost for the URL
		_, port, ok := strings.Cut(addr, ":")
		if ok {
			host = "127.0.0.1:" + port
		}
	}
	return "http://" + host + "/"
}
