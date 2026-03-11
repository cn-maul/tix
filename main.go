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
	"os/exec"
	"runtime"
	"strings"
)

//go:embed static
var staticFS embed.FS

func main() {
	// 加载配置
	cfg, err := config.Load()
	if err != nil {
		log.Fatalf("❌ 加载配置失败: %v", err)
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
	svc := service.NewTicketService(db, cfg.Categories, cfg)
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

	// 自动打开浏览器
	go openBrowser(url)

	// 启动服务
	server := &http.Server{Handler: mux}
	if err := server.Serve(listener); err != nil {
		log.Fatalf("❌ 服务错误: %v", err)
	}
}

func printBanner(url string, port int) {
	// 计算终端显示宽度（中文占2，ASCII占1，其他符号占1）
	displayWidth := func(s string) int {
		width := 0
		for _, r := range s {
			if r >= 0x4E00 && r <= 0x9FFF {
				width += 2 // CJK中文字符
			} else {
				width += 1 // ASCII和其他符号
			}
		}
		return width
	}

	// 填充到指定宽度
	pad := func(s string, totalWidth int) string {
		currentWidth := displayWidth(s)
		if currentWidth >= totalWidth {
			return s
		}
		return s + strings.Repeat(" ", totalWidth-currentWidth)
	}

	boxWidth := 48 // 内容区宽度

	lines := []string{
		"  IT 工单管理系统 v1.0",
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

func openBrowser(url string) {
	var cmd *exec.Cmd
	switch runtime.GOOS {
	case "linux":
		cmd = exec.Command("xdg-open", url)
	case "windows":
		cmd = exec.Command("rundll32", "url.dll,FileProtocolHandler", url)
	case "darwin":
		cmd = exec.Command("open", url)
	default:
		return
	}
	cmd.Start()
}
