package handler

import (
	"database/sql"
	"errors"
	"net/http"
	"regexp"
	"strings"
	"time"
	"tix/internal/auth"
	"tix/internal/model"

	"github.com/google/uuid"
	"golang.org/x/crypto/bcrypt"
)

const sessionTTL = 7 * 24 * time.Hour

var usernamePattern = regexp.MustCompile(`^[a-zA-Z0-9._-]{3,32}$`)

func (h *Handler) Register(w http.ResponseWriter, r *http.Request) {
	total, err := h.db.CountUsers(r.Context())
	if err != nil {
		h.error(w, 500, "INTERNAL_ERROR", "Failed to check users")
		return
	}
	if total > 0 {
		h.error(w, 403, "BOOTSTRAP_CLOSED", "bootstrap is closed; use admin account to create users")
		return
	}

	user, err := h.createUser(r, "admin")
	if err != nil {
		h.writeAuthCreateError(w, err)
		return
	}

	resp, err := h.createSessionForUser(r, user)
	if err != nil {
		h.error(w, 500, "INTERNAL_ERROR", "Failed to create session")
		return
	}
	h.json(w, 201, resp)
}

func (h *Handler) BootstrapStatus(w http.ResponseWriter, r *http.Request) {
	total, err := h.db.CountUsers(r.Context())
	if err != nil {
		h.error(w, 500, "INTERNAL_ERROR", "Failed to check users")
		return
	}

	h.json(w, 200, map[string]any{
		"has_users":  total > 0,
		"user_count": total,
	})
}

func (h *Handler) CreateUserByAdmin(w http.ResponseWriter, r *http.Request) {
	if !auth.IsAdmin(r.Context()) {
		h.error(w, 403, "FORBIDDEN", "admin permission required")
		return
	}

	user, err := h.createUser(r, "")
	if err != nil {
		h.writeAuthCreateError(w, err)
		return
	}
	user.PasswordHash = ""
	h.json(w, 201, user)
}

func (h *Handler) ListUsers(w http.ResponseWriter, r *http.Request) {
	if !auth.IsAdmin(r.Context()) {
		h.error(w, 403, "FORBIDDEN", "admin permission required")
		return
	}

	users, err := h.db.ListUsers(r.Context())
	if err != nil {
		h.error(w, 500, "INTERNAL_ERROR", "Failed to load users")
		return
	}
	for i := range users {
		users[i].PasswordHash = ""
	}
	h.json(w, 200, map[string]any{"items": users})
}

func (h *Handler) Login(w http.ResponseWriter, r *http.Request) {
	var req model.LoginReq
	if err := h.decodeJSON(r, &req); err != nil {
		h.error(w, 400, "INVALID_JSON", "Invalid JSON body")
		return
	}

	username := strings.TrimSpace(req.Username)
	user, err := h.db.GetUserByUsername(r.Context(), username)
	if err != nil {
		if errors.Is(err, sql.ErrNoRows) {
			h.error(w, 401, "INVALID_CREDENTIALS", "invalid username or password")
			return
		}
		h.error(w, 500, "INTERNAL_ERROR", "Failed to load user")
		return
	}

	if bcrypt.CompareHashAndPassword([]byte(user.PasswordHash), []byte(req.Password)) != nil {
		h.error(w, 401, "INVALID_CREDENTIALS", "invalid username or password")
		return
	}

	resp, err := h.createSessionForUser(r, user)
	if err != nil {
		h.error(w, 500, "INTERNAL_ERROR", "Failed to create session")
		return
	}
	h.json(w, 200, resp)
}

func (h *Handler) Me(w http.ResponseWriter, r *http.Request) {
	userCtx, ok := auth.UserFromContext(r.Context())
	if !ok {
		h.error(w, 401, "UNAUTHORIZED", "not logged in")
		return
	}
	user, err := h.db.GetUserByID(r.Context(), userCtx.ID)
	if err != nil {
		h.error(w, 500, "INTERNAL_ERROR", "Failed to load current user")
		return
	}
	user.PasswordHash = ""
	h.json(w, 200, user)
}

func (h *Handler) Logout(w http.ResponseWriter, r *http.Request) {
	token := extractBearerToken(r.Header.Get("Authorization"))
	if token != "" {
		_ = h.db.DeleteSession(r.Context(), auth.HashToken(token))
	}
	h.json(w, 200, map[string]string{"message": "logged out"})
}

func (h *Handler) ChangePassword(w http.ResponseWriter, r *http.Request) {
	userCtx, ok := auth.UserFromContext(r.Context())
	if !ok {
		h.error(w, 401, "UNAUTHORIZED", "not logged in")
		return
	}

	var req model.ChangePasswordReq
	if err := h.decodeJSON(r, &req); err != nil {
		h.error(w, 400, "INVALID_JSON", "Invalid JSON body")
		return
	}

	if len(req.NewPassword) < 6 || len(req.NewPassword) > 72 {
		h.error(w, 400, "INVALID_PASSWORD", "password length must be 6-72")
		return
	}

	user, err := h.db.GetUserByID(r.Context(), userCtx.ID)
	if err != nil {
		h.error(w, 500, "INTERNAL_ERROR", "Failed to load current user")
		return
	}
	if bcrypt.CompareHashAndPassword([]byte(user.PasswordHash), []byte(req.CurrentPassword)) != nil {
		h.error(w, 400, "INVALID_PASSWORD", "current password is incorrect")
		return
	}

	hash, err := bcrypt.GenerateFromPassword([]byte(req.NewPassword), bcrypt.DefaultCost)
	if err != nil {
		h.error(w, 500, "INTERNAL_ERROR", "Failed to update password")
		return
	}
	if err := h.db.UpdateUserPassword(r.Context(), user.ID, string(hash)); err != nil {
		h.error(w, 500, "INTERNAL_ERROR", "Failed to update password")
		return
	}
	h.json(w, 200, map[string]string{"message": "password updated"})
}

func (h *Handler) DeleteUserByAdmin(w http.ResponseWriter, r *http.Request) {
	userCtx, ok := auth.UserFromContext(r.Context())
	if !ok {
		h.error(w, 401, "UNAUTHORIZED", "not logged in")
		return
	}

	id := strings.TrimSpace(r.PathValue("id"))
	if id == "" {
		h.error(w, 400, "INVALID_USER_ID", "user id is required")
		return
	}
	if id == userCtx.ID {
		h.error(w, 400, "INVALID_OPERATION", "cannot delete current user")
		return
	}

	if err := h.db.DeleteUser(r.Context(), id); err != nil {
		if errors.Is(err, sql.ErrNoRows) {
			h.error(w, 404, "NOT_FOUND", "user not found")
			return
		}
		h.error(w, 500, "INTERNAL_ERROR", "Failed to delete user")
		return
	}
	h.json(w, 200, map[string]string{"message": "deleted"})
}

func (h *Handler) createUser(r *http.Request, defaultRole string) (*model.User, error) {
	var req model.RegisterReq
	if err := h.decodeJSON(r, &req); err != nil {
		return nil, errInvalidJSON
	}

	username := strings.TrimSpace(req.Username)
	password := req.Password
	role := strings.TrimSpace(req.Role)
	if !usernamePattern.MatchString(username) {
		return nil, errInvalidUsername
	}
	if len(password) < 6 || len(password) > 72 {
		return nil, errInvalidPassword
	}
	if role == "" {
		role = defaultRole
	}
	if role != "admin" && role != "member" {
		return nil, errInvalidRole
	}

	hash, err := bcrypt.GenerateFromPassword([]byte(password), bcrypt.DefaultCost)
	if err != nil {
		return nil, err
	}

	user := &model.User{
		ID:           uuid.New().String()[:8],
		Username:     username,
		PasswordHash: string(hash),
		Role:         role,
		CreatedAt:    time.Now().UTC().Format(time.RFC3339),
	}

	if err := h.db.CreateUser(r.Context(), user); err != nil {
		if strings.Contains(strings.ToLower(err.Error()), "unique") {
			return nil, errDuplicateUsername
		}
		return nil, err
	}
	return user, nil
}

func (h *Handler) createSessionForUser(r *http.Request, user *model.User) (*model.AuthResponse, error) {
	now := time.Now().UTC()
	token, err := auth.GenerateToken()
	if err != nil {
		return nil, err
	}
	if err := h.db.CreateSession(
		r.Context(),
		auth.HashToken(token),
		user.ID,
		now.Format(time.RFC3339),
		now.Add(sessionTTL).Format(time.RFC3339),
	); err != nil {
		return nil, err
	}
	_ = h.db.UpdateUserLastLogin(r.Context(), user.ID, now.Format(time.RFC3339))

	respUser := *user
	respUser.PasswordHash = ""
	respUser.LastLoginAt = now.Format(time.RFC3339)
	return &model.AuthResponse{
		Token: token,
		User:  respUser,
	}, nil
}

var (
	errInvalidJSON       = errors.New("invalid json")
	errInvalidUsername   = errors.New("invalid username")
	errInvalidPassword   = errors.New("invalid password")
	errInvalidRole       = errors.New("invalid role")
	errDuplicateUsername = errors.New("duplicate username")
)

func (h *Handler) writeAuthCreateError(w http.ResponseWriter, err error) {
	switch err {
	case errInvalidJSON:
		h.error(w, 400, "INVALID_JSON", "Invalid JSON body")
	case errInvalidUsername:
		h.error(w, 400, "INVALID_USERNAME", "username must match [a-zA-Z0-9._-], length 3-32")
	case errInvalidPassword:
		h.error(w, 400, "INVALID_PASSWORD", "password length must be 6-72")
	case errInvalidRole:
		h.error(w, 400, "INVALID_ROLE", "role must be admin or member")
	case errDuplicateUsername:
		h.error(w, 409, "USERNAME_EXISTS", "username already exists")
	default:
		h.error(w, 500, "INTERNAL_ERROR", "Failed to create user")
	}
}

func extractBearerToken(v string) string {
	const prefix = "Bearer "
	if !strings.HasPrefix(v, prefix) {
		return ""
	}
	return strings.TrimSpace(strings.TrimPrefix(v, prefix))
}
