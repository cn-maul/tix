package main

import (
	"bytes"
	"context"
	"embed"
	"fmt"
	"io/fs"
	"log"
	"mime"
	"net"
	"net/http"
	"os"
	"path"
	"strings"
	"time"
	"tix/internal/config"
	"tix/internal/database"
	"tix/internal/handler"
	"tix/internal/middleware"
	"tix/internal/service"

	_ "modernc.org/sqlite" // SQLite driver
)

//go:embed static
var staticFS embed.FS

func main() {
	// 加载配置
	cfg, err := config.Load()
	if err != nil {
		log.Fatalf("❌ 加载配置失败: %v", err)
	}

	// 首次运行时保存默认配置
	if _, err := os.Stat("config.yaml"); os.IsNotExist(err) {
		if err := config.Save(cfg); err != nil {
			log.Printf("⚠️ 无法保存默认配置: %v", err)
		} else {
			log.Printf("✓ 已创建默认配置: config.yaml")
		}
	}

	// 初始化数据库
	db, err := database.Open(cfg.Database.Filename)
	if err != nil {
		log.Fatalf("❌ 打开数据库失败: %v", err)
	}
	defer db.Close()

	if err := db.InitSchema(context.Background()); err != nil {
		log.Fatalf("❌ 初始化数据库失败: %v", err)
	}

	// 创建服务
	svc := service.NewTicketService(db)
	aiSvc := service.NewAIService(&cfg.AI, nil)
	svc.SetAI(aiSvc)

	h := handler.NewHandler(db, svc, cfg.Categories, cfg)

	mux := http.NewServeMux()
	h.RegisterRoutes(mux)

	// 静态文件服务
	staticContent, _ := fs.Sub(staticFS, "static")
	registerStaticRoutes(mux, staticContent)

	// 端口自适应
	actualPort := cfg.Server.Port
	maxPort := cfg.Server.Port + 100

	var listener net.Listener
	for port := cfg.Server.Port; port <= maxPort; port++ {
		addr := fmt.Sprintf("%s:%d", cfg.Server.Host, port)
		listener, err = net.Listen("tcp", addr)
		if err == nil {
			actualPort = port
			break
		}
	}

	if listener == nil {
		log.Fatalf("❌ 无法绑定端口 %d-%d", cfg.Server.Port, maxPort)
	}

	// 输出启动信息
	url := fmt.Sprintf("http://127.0.0.1:%d", actualPort)
	printBanner(url, actualPort)

	// 启动服务
	server := &http.Server{
		Handler: middleware.Chain(mux, middleware.Recover, middleware.Logger, middleware.CORS, middleware.Auth(db)),
	}
	if err := server.Serve(listener); err != nil {
		log.Fatalf("❌ 服务错误: %v", err)
	}
}

func registerStaticRoutes(mux *http.ServeMux, staticContent fs.FS) {
	fileServer := http.FileServer(http.FS(staticContent))
	indexHandler := serveStaticFile(staticContent, "index.html")
	loginHandler := serveStaticFile(staticContent, "login.html")
	publicHandler := serveStaticFile(staticContent, "public.html")

	mux.HandleFunc("/{$}", indexHandler)
	mux.HandleFunc("/index.html", indexHandler)
	mux.HandleFunc("/login.html", loginHandler)
	mux.HandleFunc("/public.html", publicHandler)
	mux.Handle("/public", http.RedirectHandler("/public.html", http.StatusTemporaryRedirect))
	mux.Handle("/", fileServer)
}

func serveStaticFile(staticContent fs.FS, name string) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		data, err := fs.ReadFile(staticContent, name)
		if err != nil {
			http.NotFound(w, r)
			return
		}

		if contentType := mime.TypeByExtension(path.Ext(name)); contentType != "" {
			w.Header().Set("Content-Type", contentType)
		}

		if path.Ext(name) == ".html" {
			w.Header().Set("Cache-Control", "no-store, no-cache, must-revalidate")
			w.Header().Set("Pragma", "no-cache")
			w.Header().Set("Expires", "0")
		}

		http.ServeContent(w, r, name, time.Time{}, bytes.NewReader(data))
	}
}

func printBanner(url string, port int) {
	displayWidth := func(s string) int {
		width := 0
		for _, r := range s {
			if r >= 0x4E00 && r <= 0x9FFF {
				width += 2
			} else {
				width += 1
			}
		}
		return width
	}

	pad := func(s string, totalWidth int) string {
		currentWidth := displayWidth(s)
		if currentWidth >= totalWidth {
			return s
		}
		return s + strings.Repeat(" ", totalWidth-currentWidth)
	}

	boxWidth := 48

	lines := []string{
		"  Tix v3.1.0 - 工单管理系统",
		fmt.Sprintf("  状态: ✓ 运行中 (端口 %d)", port),
		fmt.Sprintf("  地址: %s", url),
		"  按 Ctrl+C 停止服务",
	}

	fmt.Println()
	fmt.Println("╔" + strings.Repeat("═", boxWidth) + "╗")
	for i, line := range lines {
		fmt.Println("║" + pad(line, boxWidth) + "║")
		if i == 0 || i == 2 {
			fmt.Println("╠" + strings.Repeat("═", boxWidth) + "╣")
		}
	}
	fmt.Println("╚" + strings.Repeat("═", boxWidth) + "╝")
	fmt.Println()
}
