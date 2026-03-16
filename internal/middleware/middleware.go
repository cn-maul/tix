package middleware

import (
	"database/sql"
	"log"
	"net/http"
	"strings"
	"time"
	"tix/internal/auth"
	"tix/internal/database"
)

// Logger 请求日志中间件
func Logger(next http.Handler) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		start := time.Now()

		// 包装 ResponseWriter 以获取状态码
		wrapped := &responseWriter{ResponseWriter: w, statusCode: 200}

		next.ServeHTTP(wrapped, r)

		duration := time.Since(start)
		log.Printf("[%s] %s %s %d %v", r.Method, r.URL.Path, r.RemoteAddr, wrapped.statusCode, duration)
	})
}

type responseWriter struct {
	http.ResponseWriter
	statusCode int
}

func (rw *responseWriter) WriteHeader(code int) {
	rw.statusCode = code
	rw.ResponseWriter.WriteHeader(code)
}

// CORS 跨域支持
func CORS(next http.Handler) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Access-Control-Allow-Origin", "*")
		w.Header().Set("Access-Control-Allow-Methods", "GET, POST, PATCH, DELETE, OPTIONS")
		w.Header().Set("Access-Control-Allow-Headers", "Content-Type, Authorization")

		if r.Method == "OPTIONS" {
			w.WriteHeader(204)
			return
		}

		next.ServeHTTP(w, r)
	})
}

// Recover 错误恢复
func Recover(next http.Handler) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		defer func() {
			if err := recover(); err != nil {
				log.Printf("Panic recovered: %v", err)
				http.Error(w, `{"error":{"code":"INTERNAL_ERROR","message":"Internal server error"}}`, 500)
			}
		}()
		next.ServeHTTP(w, r)
	})
}

// Chain 中间件链
func Chain(h http.Handler, middlewares ...func(http.Handler) http.Handler) http.Handler {
	for i := len(middlewares) - 1; i >= 0; i-- {
		h = middlewares[i](h)
	}
	return h
}

// Auth 认证和基础授权中间件
func Auth(db *database.DB) func(http.Handler) http.Handler {
	publicPaths := map[string]struct{}{
		"/v1/auth/bootstrap-status": {},
		"/v1/auth/login":            {},
		"/v1/auth/register":         {},
		"/v1/public/categories":     {},
		"/v1/public/sms/code":       {},
		"/v1/public/tickets":        {},
	}

	return func(next http.Handler) http.Handler {
		return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			path := r.URL.Path
			if !strings.HasPrefix(path, "/v1/") {
				next.ServeHTTP(w, r)
				return
			}

			if _, ok := publicPaths[path]; ok {
				next.ServeHTTP(w, r)
				return
			}

			token := extractBearerToken(r.Header.Get("Authorization"))
			if token == "" {
				writeAuthError(w, http.StatusUnauthorized, "UNAUTHORIZED", "missing bearer token")
				return
			}

			now := time.Now().UTC().Format(time.RFC3339)
			_ = db.DeleteExpiredSessions(r.Context(), now)
			tokenHash := auth.HashToken(token)
			user, err := db.GetUserBySessionHash(r.Context(), tokenHash, now)
			if err != nil {
				if err == sql.ErrNoRows {
					writeAuthError(w, http.StatusUnauthorized, "UNAUTHORIZED", "invalid or expired token")
					return
				}
				writeAuthError(w, http.StatusInternalServerError, "INTERNAL_ERROR", "auth check failed")
				return
			}

			ctx := auth.WithUser(r.Context(), &auth.UserContext{
				ID:       user.ID,
				Username: user.Username,
				Role:     user.Role,
			})

			if strings.HasPrefix(path, "/v1/config") || strings.HasPrefix(path, "/v1/system") || strings.HasPrefix(path, "/v1/users") {
				if user.Role != "admin" {
					writeAuthError(w, http.StatusForbidden, "FORBIDDEN", "admin permission required")
					return
				}
			}

			next.ServeHTTP(w, r.WithContext(ctx))
		})
	}
}

func extractBearerToken(v string) string {
	const prefix = "Bearer "
	if !strings.HasPrefix(v, prefix) {
		return ""
	}
	return strings.TrimSpace(strings.TrimPrefix(v, prefix))
}

func writeAuthError(w http.ResponseWriter, status int, code, message string) {
	w.Header().Set("Content-Type", "application/json")
	w.WriteHeader(status)
	_, _ = w.Write([]byte(`{"error":{"code":"` + code + `","message":"` + message + `"}}`))
}
