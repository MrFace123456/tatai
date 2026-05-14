package model

import "time"

// User 用户模型
type User struct {
	ID        int        `json:"id"`
	Username  string     `json:"username"`
	Password  string     `json:"-"` // 密码不返回给前端
	Role      string     `json:"role"`
	Status    int        `json:"status"` // 1:启用, 0:禁用
	CreatedAt time.Time  `json:"created_at"`
	UpdatedAt time.Time  `json:"updated_at"`
	LastLogin *time.Time `json:"last_login,omitempty"`
}

// LoginRequest 登录请求
type LoginRequest struct {
	Username string `json:"username"`
	Password string `json:"password"`
}

// LoginResponse 登录响应
type LoginResponse struct {
	Token    string `json:"token"`
	Username string `json:"username"`
	Role     string `json:"role"`
}

// UserInfoResponse 用户信息响应
type UserInfoResponse struct {
	ID       int    `json:"id"`
	Username string `json:"username"`
	Role     string `json:"role"`
}

// CreateUserRequest 创建用户请求
type CreateUserRequest struct {
	Username string `json:"username"`
	Password string `json:"password"`
}

// UpdateUserStatusRequest 更新用户状态请求
type UpdateUserStatusRequest struct {
	Status int `json:"status"` // 0禁用 1启用
}

// UpdatePasswordRequest 修改密码请求
type UpdatePasswordRequest struct {
	NewPassword string `json:"new_password"`
}

// UserListResponse 用户列表项（不含密码）
type UserListItem struct {
	ID        int        `json:"id"`
	Username  string     `json:"username"`
	Role      string     `json:"role"`
	Status    int        `json:"status"`
	LastLogin *time.Time `json:"last_login,omitempty"`
	CreatedAt time.Time  `json:"created_at"`
}
