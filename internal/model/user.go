package model

type User struct {
	ID           string `json:"id"`
	Username     string `json:"username"`
	PasswordHash string `json:"-"`
	Role         string `json:"role"`
	CreatedAt    string `json:"created_at"`
	LastLoginAt  string `json:"last_login_at,omitempty"`
}

type RegisterReq struct {
	Username string `json:"username"`
	Password string `json:"password"`
	Role     string `json:"role,omitempty"`
}

type LoginReq struct {
	Username string `json:"username"`
	Password string `json:"password"`
}

type ChangePasswordReq struct {
	CurrentPassword string `json:"current_password"`
	NewPassword     string `json:"new_password"`
}

type AuthResponse struct {
	Token string `json:"token"`
	User  User   `json:"user"`
}
