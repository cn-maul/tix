package main

import (
	"embed"
	"fmt"
	"tix/internal/config"
	"tix/internal/database"
	"tix/internal/handler"
	"tix/internal/service"
	"io/fs"
	"log"
	"net"
	"net/http"
	"os"
	"path/filepath"
	"strings"

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
	home, _ := os.UserHomeDir()
	configPath := filepath.Join(home, ".tix", "config.yaml")
	if _, err := os.Stat(configPath); os.IsNotExist(err) {
		if err := config.Save(cfg); err != nil {
			log.Printf("⚠️ 无法保存默认配置: %v", err)
		} else {
			log.Printf("✓ 已创建默认配置: %s", configPath)
		}
	}

	// 初始化数据库
	db, err := database.Open(cfg.Database.Filename)
	if err != nil {
		log.Fatalf("❌ 打开数据库失败: %v", err)
	}
	defer db.Close()

	if err := db.InitSchema(); err != nil {
		log.Fatalf("❌ 初始化数据库失败: %v", err)
	}

	// 创建服务
	svc := service.NewTicketService(db)
	aiSvc := service.NewAIService(&cfg.AI)
	svc.SetAI(aiSvc)

	h := handler.NewHandler(svc, cfg.Categories, cfg)

	mux := http.NewServeMux()
	h.RegisterRoutes(mux)

	// 静态文件服务
	staticContent, _ := fs.Sub(staticFS, "static")
	mux.Handle("/", http.FileServer(http.FS(staticContent)))

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
	server := &http.Server{Handler: mux}
	if err := server.Serve(listener); err != nil {
		log.Fatalf("❌ 服务错误: %v", err)
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
		"  Tix v2.0 - 工单管理系统",
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
