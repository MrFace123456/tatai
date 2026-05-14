package database

import (
	"database/sql"
	"fmt"
	"strings"
	"tatai/internal/model"
	"time"

	"golang.org/x/crypto/bcrypt"
)

// InitUserTable 初始化用户表
func InitUserTable(db *sql.DB) error {
	query := `
	CREATE TABLE IF NOT EXISTS users (
		id INTEGER PRIMARY KEY AUTOINCREMENT,
		username TEXT UNIQUE NOT NULL,
		password TEXT NOT NULL,
		role TEXT DEFAULT 'admin',
		created_at DATETIME DEFAULT CURRENT_TIMESTAMP,
		updated_at DATETIME DEFAULT CURRENT_TIMESTAMP,
		last_login DATETIME
	);
	`
	if _, err := db.Exec(query); err != nil {
		return err
	}
	// 迁移：添加 status 字段（兼容旧表）
	_, err := db.Exec("ALTER TABLE users ADD COLUMN status INTEGER DEFAULT 1")
	if err != nil && !strings.Contains(err.Error(), "duplicate column name") {
		return err
	}
	// 创建默认admin用户（如果不存在）
	return createDefaultAdmin(db)
}

// createDefaultAdmin 创建默认admin用户
func createDefaultAdmin(db *sql.DB) error {
	var count int
	err := db.QueryRow("SELECT COUNT(*) FROM users WHERE username = 'admin'").Scan(&count)
	if err != nil {
		return err
	}
	if count == 0 {
		hashedPassword, err := bcrypt.GenerateFromPassword([]byte("123456"), bcrypt.DefaultCost)
		if err != nil {
			return err
		}
		_, err = db.Exec(`
			INSERT INTO users (username, password, role, status, created_at, updated_at)
			VALUES (?, ?, ?, ?, ?, ?)
		`, "admin", string(hashedPassword), "admin", 1, time.Now(), time.Now())
		if err != nil {
			return err
		}
		fmt.Println("[Database] 默认admin用户已创建 (用户名: admin, 密码: 123456)")
	}
	return nil
}

// GetUserByUsername 根据用户名获取用户
func GetUserByUsername(db *sql.DB, username string) (*model.User, error) {
	var user model.User
	var lastLogin sql.NullTime

	query := `SELECT id, username, password, role, status, created_at, updated_at, last_login 
              FROM users WHERE username = ?`
	err := db.QueryRow(query, username).Scan(
		&user.ID, &user.Username, &user.Password, &user.Role, &user.Status,
		&user.CreatedAt, &user.UpdatedAt, &lastLogin,
	)
	if err != nil {
		if err == sql.ErrNoRows {
			return nil, nil
		}
		return nil, err
	}

	if lastLogin.Valid {
		user.LastLogin = &lastLogin.Time
	}

	return &user, nil
}

// GetUserByID 根据ID获取用户（不返回密码）
func GetUserByID(db *sql.DB, userID int) (*model.User, error) {
	var user model.User
	var lastLogin sql.NullTime

	query := `SELECT id, username, role, created_at, updated_at, last_login 
	          FROM users WHERE id = ?`
	err := db.QueryRow(query, userID).Scan(
		&user.ID, &user.Username, &user.Role,
		&user.CreatedAt, &user.UpdatedAt, &lastLogin,
	)
	if err != nil {
		if err == sql.ErrNoRows {
			return nil, nil
		}
		return nil, err
	}

	if lastLogin.Valid {
		user.LastLogin = &lastLogin.Time
	}

	return &user, nil
}

// UpdateLastLogin 更新最后登录时间
func UpdateLastLogin(db *sql.DB, userID int) error {
	_, err := db.Exec("UPDATE users SET last_login = ? WHERE id = ?", time.Now(), userID)
	return err
}

// VerifyPassword 验证密码
func VerifyPassword(hashedPassword, password string) bool {
	err := bcrypt.CompareHashAndPassword([]byte(hashedPassword), []byte(password))
	return err == nil
}

// GetAllUsers 获取所有用户（不含密码）
func GetAllUsers(db *sql.DB) ([]model.UserListItem, error) {
	rows, err := db.Query(`
        SELECT id, username, role, status, last_login, created_at 
        FROM users ORDER BY id
    `)
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	var users []model.UserListItem
	for rows.Next() {
		var u model.UserListItem
		var lastLogin sql.NullTime
		err := rows.Scan(&u.ID, &u.Username, &u.Role, &u.Status, &lastLogin, &u.CreatedAt)
		if err != nil {
			return nil, err
		}
		if lastLogin.Valid {
			u.LastLogin = &lastLogin.Time
		}
		users = append(users, u)
	}
	return users, nil
}

// CreateUser 创建新用户（密码已哈希）
func CreateUser(db *sql.DB, username, hashedPassword, role string, status int) (int64, error) {
	result, err := db.Exec(`
        INSERT INTO users (username, password, role, status, created_at, updated_at)
        VALUES (?, ?, ?, ?, ?, ?)
    `, username, hashedPassword, role, status, time.Now(), time.Now())
	if err != nil {
		return 0, err
	}
	return result.LastInsertId()
}

// UpdateUserStatus 更新用户状态（禁止修改admin）
func UpdateUserStatus(db *sql.DB, userID int, status int) error {
	// 检查是否是 admin 用户
	var isAdmin bool
	err := db.QueryRow("SELECT username = 'admin' FROM users WHERE id = ?", userID).Scan(&isAdmin)
	if err != nil {
		return err
	}
	if isAdmin {
		return fmt.Errorf("不能修改 admin 用户的状态")
	}
	_, err = db.Exec("UPDATE users SET status = ?, updated_at = ? WHERE id = ?", status, time.Now(), userID)
	return err
}

// ResetUserPassword 重置用户密码（管理员调用，不检查旧密码）
func ResetUserPassword(db *sql.DB, userID int, hashedPassword string) error {
	_, err := db.Exec("UPDATE users SET password = ?, updated_at = ? WHERE id = ?", hashedPassword, time.Now(), userID)
	return err
}

// UpdatePasswordSelf 当前用户修改自己的密码
func UpdatePasswordSelf(db *sql.DB, userID int, hashedPassword string) error {
	_, err := db.Exec("UPDATE users SET password = ?, updated_at = ? WHERE id = ?", hashedPassword, time.Now(), userID)
	return err
}

// IsAdmin 判断用户是否为 admin
func IsAdmin(db *sql.DB, userID int) (bool, error) {
	var username string
	err := db.QueryRow("SELECT username FROM users WHERE id = ?", userID).Scan(&username)
	if err != nil {
		return false, err
	}
	return username == "admin", nil
}
