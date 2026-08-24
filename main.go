package main

import (
	"context"
	"embed"
	"flag"
	"html"
	"log"
	"net/http"
	"os"
	"os/signal"
	"regexp"
	"strings"
	"syscall"
	"time"

	_ "time/tzdata" // 内嵌时区数据：容器内设置 TZ 即可按本地时间显示
)

const defaultPassword = "admin123"

//go:embed web/dist
var webAssets embed.FS

func main() {
	port := os.Getenv("PORT")
	if port == "" {
		port = "8881"
	}
	addr := flag.String("addr", ":"+port, "监听地址（默认 :8881，可用 PORT 环境变量覆盖）")
	dbPath := flag.String("db", "tix.db", "SQLite 数据库文件路径")
	password := flag.String("password", defaultPassword, "管理端访问密码（可用 TIX_PASSWORD 环境变量覆盖，/submit 公开免密）")
	trustProxy := flag.Bool("trust-proxy", false, "信任 X-Forwarded-For / X-Real-IP 头获取客户端真实 IP（仅在反向代理之后开启，否则限流可被伪造头绕过）")
	flag.Parse()

	db, err := openDB(*dbPath)
	if err != nil {
		log.Fatalf("打开数据库失败: %v", err)
	}
	defer db.Close()
	if err := initDB(db); err != nil {
		log.Fatalf("初始化数据库失败: %v", err)
	}
	if err := migrateDB(db); err != nil {
		log.Fatalf("数据库迁移失败: %v", err)
	}

	pw := *password
	if env := os.Getenv("TIX_PASSWORD"); env != "" {
		pw = env
	}
	// 初始化默认管理员账户
	if err := setupDefaultAdmin(db, pw); err != nil {
		log.Fatalf("初始化默认管理员失败: %v", err)
	}
	a := &app{
		db:            db,
		auth:          newAuthStore(),
		notify:        newNotifier(),
		trustProxy:    *trustProxy,
		loginLimiter:  newRateLimiter(10, time.Minute),
		submitLimiter: newRateLimiter(10, time.Minute),
	}
	srv := &http.Server{
		Addr:              *addr,
		Handler:           securityHeaders(a.authMiddleware(a.routes())),
		ReadHeaderTimeout: 5 * time.Second,
		ReadTimeout:       15 * time.Second,
		WriteTimeout:      30 * time.Second,
		IdleTimeout:       60 * time.Second,
	}
	log.Printf("tix 工单系统启动，监听 %s（数据库: %s）", *addr, *dbPath)

	go func() {
		if err := srv.ListenAndServe(); err != nil && err != http.ErrServerClosed {
			log.Fatalf("HTTP 服务异常退出: %v", err)
		}
	}()

	sig := make(chan os.Signal, 1)
	signal.Notify(sig, os.Interrupt, syscall.SIGTERM)
	<-sig
	log.Println("收到退出信号，正在优雅关闭…")
	ctx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
	defer cancel()
	if err := srv.Shutdown(ctx); err != nil {
		log.Printf("优雅关闭失败: %v", err)
	}
	log.Println("已退出")
}

// routes 注册 /api JSON 路由 + SPA 回退。除公开接口外均需密码会话。
func (a *app) routes() *http.ServeMux {
	mux := http.NewServeMux()

	// JSON REST API
	mux.HandleFunc("GET /api/health", a.apiHealth)
	mux.HandleFunc("POST /api/login", a.apiLogin)
	mux.HandleFunc("POST /api/logout", a.apiLogout)
	mux.HandleFunc("GET /api/auth/status", a.apiAuthStatus)
	mux.HandleFunc("GET /api/stats", a.apiStats)
	mux.HandleFunc("GET /api/tickets", a.apiTicketList)
	mux.HandleFunc("POST /api/tickets", a.apiTicketCreate)
	mux.HandleFunc("/api/tickets/{id}", a.apiTicketByID) // GET / PUT
	mux.HandleFunc("POST /api/tickets/{id}/done", a.apiTicketDone)
	mux.HandleFunc("POST /api/tickets/{id}/delete", a.apiTicketDelete)
	mux.HandleFunc("POST /api/tickets/{id}/assign", a.apiTicketAssign)       // 指派/取消负责人
	mux.HandleFunc("POST /api/tickets/batch-done", a.apiTicketBatchDone)     // 批量标记已处理
	mux.HandleFunc("POST /api/tickets/batch-delete", a.apiTicketBatchDelete) // 批量删除（含备注）
	mux.HandleFunc("/api/tickets/{id}/comments", a.apiTicketComments)        // GET / POST
	mux.HandleFunc("GET /api/categories", a.apiCategoryList)
	mux.HandleFunc("POST /api/categories", a.apiCategoryCreate)
	mux.HandleFunc("/api/categories/{id}", a.apiCategoryByID)           // PUT / DELETE
	mux.HandleFunc("POST /api/submit", a.apiSubmitCompat)               // 表单兼容别名（公开）
	mux.HandleFunc("GET /api/submit/categories", a.apiSubmitCategories) // 提交页分类（公开）
	mux.HandleFunc("GET /api/my/tickets", a.apiMyTickets)               // 游客进度查询：列表（公开）
	mux.HandleFunc("GET /api/my/tickets/{id}", a.apiMyTicketDetail)     // 游客进度查询：详情+处理记录（公开）
	mux.HandleFunc("GET /api/export/csv", a.apiExportCSV)               // CSV 导出
	mux.HandleFunc("GET /api/users", a.apiUserList)                     // 用户列表（登录用户，供指派取人）
	mux.HandleFunc("POST /api/users", a.apiUserCreate)                  // 创建用户（管理员）
	mux.HandleFunc("PUT /api/users/{id}", a.apiUserUpdate)              // 更新用户（管理员）
	mux.HandleFunc("DELETE /api/users/{id}", a.apiUserDelete)           // 删除用户（管理员）
	mux.HandleFunc("PUT /api/profile/password", a.apiProfilePassword)   // 自助改密（登录用户）
	mux.HandleFunc("GET /api/settings", a.apiSettingsGet)               // 获取设置（公开，仅白名单键）
	mux.HandleFunc("PUT /api/settings", a.apiSettingsUpdate)            // 更新设置（管理员）
	mux.HandleFunc("GET /api/notify/config", a.apiNotifyConfigGet)      // 推送配置（管理员，Token 脱敏）
	mux.HandleFunc("PUT /api/notify/config", a.apiNotifyConfigUpdate)   // 更新推送配置（管理员）
	mux.HandleFunc("POST /api/notify/test", a.apiNotifyTest)            // 发送测试推送（管理员）
	// 未注册的 /api 路径统一返回 404 JSON（避免落入 SPA 回退）
	mux.HandleFunc("/api/", a.apiNotFound)

	// SPA 回退：所有未匹配路径返回 index.html，交给前端路由。
	mux.HandleFunc("/", a.spaFallback)
	return mux
}

// apiNotFound 未注册的 API 路径统一返回 404 JSON。
func (a *app) apiNotFound(w http.ResponseWriter, r *http.Request) {
	jsonError(w, http.StatusNotFound, "接口不存在")
}

// securityHeaders 为所有响应附加基础安全头（含 401/404 等错误响应）。
// CSP 约束资源同源加载：Vite 产物均为 /assets 下的自托管资源，
// style-src 'unsafe-inline' 用于 Radix/ECharts/sonner 的行内样式。
func securityHeaders(next http.Handler) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("X-Content-Type-Options", "nosniff")
		w.Header().Set("X-Frame-Options", "DENY")
		w.Header().Set("Referrer-Policy", "same-origin")
		w.Header().Set("Content-Security-Policy",
			"default-src 'self'; script-src 'self'; style-src 'self' 'unsafe-inline'; "+
				"img-src 'self' data: blob:; font-src 'self' data:; connect-src 'self'; "+
				"frame-ancestors 'none'; base-uri 'self'; form-action 'self'")
		next.ServeHTTP(w, r)
	})
}

// publicAPI 无需密码即可访问的接口。
func publicAPI(p string) bool {
	// 游客进度查询：列表与 /api/my/tickets/{id} 详情（详情按前缀放行）
	if p == "/api/my/tickets" || strings.HasPrefix(p, "/api/my/tickets/") {
		return true
	}
	switch p {
	case "/api/health", "/api/login", "/api/logout", "/api/auth/status", "/api/submit", "/api/submit/categories", "/api/settings":
		return true
	}
	return false
}

// authMiddleware 保护除公开接口外的所有 /api 路径。
func (a *app) authMiddleware(next http.Handler) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if strings.HasPrefix(r.URL.Path, "/api/") && !publicAPI(r.URL.Path) {
			if a.requireAuth(w, r) == nil {
				return
			}
		}
		next.ServeHTTP(w, r)
	})
}

// spaFallback 提供静态文件服务，支持 Brotli/Gzip 预压缩与长缓存策略。
// hashed 资源（assets/）使用 immutable 长缓存，index.html 使用 no-cache 保证更新。
func (a *app) spaFallback(w http.ResponseWriter, r *http.Request) {
	if r.URL.Path == "/api" || strings.HasPrefix(r.URL.Path, "/api/") {
		jsonError(w, http.StatusNotFound, "接口不存在")
		return
	}

	w.Header().Set("Vary", "Accept-Encoding")

	rel := strings.TrimLeft(r.URL.Path, "/")
	if rel == "" {
		rel = "index.html"
	}
	full := "web/dist/" + rel

	// index.html 不走预压缩分支：需要按「系统设置-站点名称」动态注入 <title>，
	// 保证浏览器标签页/首屏标题跟随改名（文件极小，压缩收益可忽略）
	if rel == "index.html" {
		data, err := webAssets.ReadFile(full)
		if err != nil {
			http.Error(w, "前端资源未构建", http.StatusInternalServerError)
			return
		}
		w.Header().Set("Content-Type", "text/html; charset=utf-8")
		setCache(w, rel)
		_, _ = w.Write(a.injectSiteTitle(data))
		return
	}

	// 优先返回预压缩版本（构建时生成 .br / .gz，零 CPU 开销）
	if data, ct, ok := servePrecompressed(w, r, full); ok {
		w.Header().Set("Content-Type", ct)
		setCache(w, rel)
		_, _ = w.Write(data)
		return
	}

	// 返回原始文件
	data, err := webAssets.ReadFile(full)
	if err == nil {
		ext := ""
		if i := strings.LastIndex(full, "."); i >= 0 {
			ext = full[i:]
		}
		w.Header().Set("Content-Type", contentType(ext))
		setCache(w, rel)
		_, _ = w.Write(data)
		return
	}

	// SPA 回退：所有未命中路径返回 index.html
	data, err = webAssets.ReadFile("web/dist/index.html")
	if err != nil {
		http.Error(w, "前端资源未构建", http.StatusInternalServerError)
		return
	}
	w.Header().Set("Content-Type", "text/html; charset=utf-8")
	w.Header().Set("Cache-Control", "no-cache")
	_, _ = w.Write(a.injectSiteTitle(data))
}

// titleRe 匹配 <title>…</title>，用于注入站点名。
var titleRe = regexp.MustCompile(`<title>[^<]*</title>`)

// injectSiteTitle 用设置中的站点名替换 index.html 的 <title>；
// 站点名为空（未配置）时原样返回。
func (a *app) injectSiteTitle(page []byte) []byte {
	name, err := getSetting(a.db, "site_name")
	if err != nil || name == "" {
		return page
	}
	return titleRe.ReplaceAll(page, []byte("<title>"+html.EscapeString(name)+"</title>"))
}

// servePrecompressed 根据 Accept-Encoding 返回预压缩文件（Brotli 优先）。
func servePrecompressed(w http.ResponseWriter, r *http.Request, origPath string) ([]byte, string, bool) {
	accept := r.Header.Get("Accept-Encoding")
	ext := ""
	if i := strings.LastIndex(origPath, "."); i >= 0 {
		ext = origPath[i:]
	}

	if strings.Contains(accept, "br") {
		if data, err := webAssets.ReadFile(origPath + ".br"); err == nil {
			w.Header().Set("Content-Encoding", "br")
			return data, contentType(ext), true
		}
	}
	if strings.Contains(accept, "gzip") {
		if data, err := webAssets.ReadFile(origPath + ".gz"); err == nil {
			w.Header().Set("Content-Encoding", "gzip")
			return data, contentType(ext), true
		}
	}
	return nil, "", false
}

// setCache 根据文件类型设置缓存策略。
func setCache(w http.ResponseWriter, rel string) {
	if strings.HasPrefix(rel, "assets/") && strings.Contains(rel, ".") {
		w.Header().Set("Cache-Control", "public, max-age=31536000, immutable")
	} else {
		w.Header().Set("Cache-Control", "no-cache")
	}
}

// contentType 按扩展名给出常见 MIME 类型。
func contentType(ext string) string {
	switch strings.ToLower(ext) {
	case ".html":
		return "text/html; charset=utf-8"
	case ".js":
		return "application/javascript; charset=utf-8"
	case ".css":
		return "text/css; charset=utf-8"
	case ".png":
		return "image/png"
	case ".jpg", ".jpeg":
		return "image/jpeg"
	case ".svg":
		return "image/svg+xml"
	case ".ico":
		return "image/x-icon"
	case ".json":
		return "application/json"
	case ".wasm":
		return "application/wasm"
	default:
		return "application/octet-stream"
	}
}
