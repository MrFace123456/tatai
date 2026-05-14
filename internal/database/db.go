package database

import (
	"database/sql"
	"fmt"
	_ "strings"
	"tatai/internal/model"
	"tatai/internal/notify"

	_ "modernc.org/sqlite"
)

func InitDB(dbPath string) (*sql.DB, error) {
	db, err := sql.Open("sqlite", dbPath)
	if err != nil {
		return nil, err
	}

	// 开启 WAL 模式
	if _, err := db.Exec("PRAGMA journal_mode=WAL;"); err != nil {
		return nil, err
	}

	// 创建应用表（基础结构）
	query := `
	CREATE TABLE IF NOT EXISTS apps (
		id INTEGER PRIMARY KEY AUTOINCREMENT,
		name TEXT,
		command TEXT,
		auto_restart INTEGER DEFAULT 0
	);
	`
	if _, err := db.Exec(query); err != nil {
		return nil, err
	}

	// 迁移：添加缺失的字段
	if err := migrateColumns(db); err != nil {
		return nil, err
	}

	// 初始化通知模块的表
	if err := notify.InitWebhookTable(db); err != nil {
		return nil, err
	}

	// 初始化用户表
	if err := InitUserTable(db); err != nil {
		return nil, err
	}
	return db, nil
}

// AddApp 添加应用
func AddApp(db *sql.DB, app model.App) (int64, error) {
	var maxOrder int
	db.QueryRow("SELECT COALESCE(MAX(sort_order), -1) FROM apps").Scan(&maxOrder)

	query := `INSERT INTO apps (name, command, auto_restart, type, jdk_key, jar_path, memory, sort_order, ports) 
              VALUES (?, ?, ?, ?, ?, ?, ?, ?, ?)`
	result, err := db.Exec(query,
		app.Name,
		app.Command,
		app.AutoRestart,
		app.Type,
		app.JDKKey,
		app.JarPath,
		app.Memory,
		maxOrder+1,
		app.Ports, // 如果为空，默认前端传 "[]"
	)
	if err != nil {
		return 0, err
	}
	return result.LastInsertId()
}

// GetAllApps 获取所有应用（按排序顺序）
func GetAllApps(db *sql.DB, appType string, keyword string) ([]model.App, error) {
	query := `
        SELECT id, name, command, auto_restart, type, jdk_key, jar_path, memory,
               docker_name, nginx_path, remark, ports, pid
        FROM apps WHERE 1=1
    `
	var args []interface{}

	if appType != "" && appType != "null" {
		query += " AND type = ?"
		args = append(args, appType)
	}
	if keyword != "" {
		query += " AND name LIKE ?"
		args = append(args, "%"+keyword+"%")
	}
	query += " ORDER BY sort_order ASC"

	rows, err := db.Query(query, args...)
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	var apps []model.App
	for rows.Next() {
		var a model.App
		var autoRestartInt int
		var jdkKey, jarPath, memory, cmd sql.NullString
		var appTypeSQL, dockerName, nginxPath, remark, ports sql.NullString
		var pid sql.NullInt64
		if err := rows.Scan(&a.ID, &a.Name, &cmd, &autoRestartInt, &appTypeSQL, &jdkKey, &jarPath, &memory,
			&dockerName, &nginxPath, &remark, &ports, &pid); err != nil {
			return nil, err
		}
		a.Command = cmd.String
		a.AutoRestart = autoRestartInt == 1
		a.Type = appTypeSQL.String
		a.JDKKey = jdkKey.String
		a.JarPath = jarPath.String
		a.Memory = memory.String
		a.DockerName = dockerName.String
		a.NginxPath = nginxPath.String
		a.Remark = remark.String
		a.Ports = ports.String
		if a.Ports == "" {
			a.Ports = "[]"
		}
		a.PID = int(pid.Int64)
		apps = append(apps, a)
	}
	return apps, nil
}

// UpdateAppPID 更新应用的 PID
func UpdateAppPID(db *sql.DB, appID int, pid int) error {
	_, err := db.Exec("UPDATE apps SET pid = ? WHERE id = ?", pid, appID)
	return err
}

// ClearAppPID 清空应用的 PID（停止时调用）
func ClearAppPID(db *sql.DB, appID int) error {
	_, err := db.Exec("UPDATE apps SET pid = 0 WHERE id = ?", appID)
	return err
}

// UpdateApp 更新应用信息
func UpdateApp(db *sql.DB, app model.App) error {
	query := `UPDATE apps SET 
        name = ?,
        command = ?,
        auto_restart = ?,
        type = ?,
        jdk_key = ?,
        jar_path = ?,
        memory = ?,
        docker_name = ?,
        nginx_path = ?,
        remark = ?,
        ports = ?,
        is_daemon = ?
    WHERE id = ?`

	autoRestart := 0
	if app.AutoRestart {
		autoRestart = 1
	}
	isDaemon := 0
	if app.IsDaemon {
		isDaemon = 1
	}

	_, err := db.Exec(query,
		app.Name, app.Command, autoRestart, app.Type,
		app.JDKKey, app.JarPath, app.Memory,
		app.DockerName, app.NginxPath, app.Remark,
		app.Ports, isDaemon, app.ID)
	return err
}

// GetAppsWithValidPID 获取所有 pid > 0 的应用（启动时恢复用）
func GetAppsWithValidPID(db *sql.DB) ([]model.App, error) {
	query := `
        SELECT id, name, command, auto_restart, type, jdk_key, jar_path, memory,
               docker_name, nginx_path, remark, ports, pid
        FROM apps WHERE pid > 0
    `
	rows, err := db.Query(query)
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	var apps []model.App
	for rows.Next() {
		var a model.App
		var autoRestartInt int
		var jdkKey, jarPath, memory, cmd sql.NullString
		var appTypeSQL, dockerName, nginxPath, remark, ports sql.NullString
		var pid sql.NullInt64

		if err := rows.Scan(&a.ID, &a.Name, &cmd, &autoRestartInt, &appTypeSQL, &jdkKey, &jarPath, &memory,
			&dockerName, &nginxPath, &remark, &ports, &pid); err != nil {
			return nil, err
		}
		a.Command = cmd.String
		a.AutoRestart = autoRestartInt == 1
		a.Type = appTypeSQL.String
		a.JDKKey = jdkKey.String
		a.JarPath = jarPath.String
		a.Memory = memory.String
		a.DockerName = dockerName.String
		a.NginxPath = nginxPath.String
		a.Remark = remark.String
		a.Ports = ports.String
		if a.Ports == "" {
			a.Ports = "[]"
		}
		a.PID = int(pid.Int64)
		apps = append(apps, a)
	}
	return apps, nil
}

// DeleteApp 删除应用
func DeleteApp(db *sql.DB, appID int) error {
	result, err := db.Exec("DELETE FROM apps WHERE id = ?", appID)
	if err != nil {
		return err
	}
	rowsAffected, _ := result.RowsAffected()
	if rowsAffected == 0 {
		return fmt.Errorf("应用不存在")
	}
	return nil
}

// UpdateAppsOrder 批量更新排序
func UpdateAppsOrder(db *sql.DB, ids []int) error {
	tx, err := db.Begin()
	if err != nil {
		return err
	}
	defer tx.Rollback()

	for index, id := range ids {
		if _, err := tx.Exec("UPDATE apps SET sort_order = ? WHERE id = ?", index, id); err != nil {
			return err
		}
	}

	return tx.Commit()
}

// CountApps 统计应用总数
func CountApps(db *sql.DB) (int, error) {
	var count int
	err := db.QueryRow("SELECT COUNT(*) FROM apps").Scan(&count)
	return count, err
}

// CountDaemonApps 统计开启守护的应用数
func CountDaemonApps(db *sql.DB) (int, error) {
	var count int
	// is_daemon 字段存储为 0/1，使用 fallback 兼容旧数据
	err := db.QueryRow("SELECT COUNT(*) FROM apps WHERE is_daemon = 1 OR auto_restart = 1").Scan(&count)
	return count, err
}

// migrateColumns 添加表结构中缺失的字段
func migrateColumns(db *sql.DB) error {
	// 获取表结构信息
	rows, err := db.Query("PRAGMA table_info(apps)")
	if err != nil {
		return err
	}
	defer rows.Close()

	// 记录已存在的字段
	existingColumns := make(map[string]bool)
	for rows.Next() {
		var cid int
		var name, ctype string
		var notnull, pk int
		var dfltValue interface{}
		if err := rows.Scan(&cid, &name, &ctype, &notnull, &dfltValue, &pk); err != nil {
			return err
		}
		existingColumns[name] = true
	}

	// 添加缺失的字段
	if !existingColumns["type"] {
		db.Exec("ALTER TABLE apps ADD COLUMN type TEXT DEFAULT ''")
		fmt.Println("数据库迁移: 已添加字段 type")
	}
	if !existingColumns["jdk_key"] {
		db.Exec("ALTER TABLE apps ADD COLUMN jdk_key TEXT DEFAULT ''")
		fmt.Println("数据库迁移: 已添加字段 jdk_key")
	}
	if !existingColumns["jar_path"] {
		db.Exec("ALTER TABLE apps ADD COLUMN jar_path TEXT DEFAULT ''")
		fmt.Println("数据库迁移: 已添加字段 jar_path")
	}
	if !existingColumns["memory"] {
		db.Exec("ALTER TABLE apps ADD COLUMN memory TEXT DEFAULT ''")
		fmt.Println("数据库迁移: 已添加字段 memory")
	}
	if !existingColumns["sort_order"] {
		db.Exec("ALTER TABLE apps ADD COLUMN sort_order INTEGER DEFAULT 0")
		fmt.Println("数据库迁移: 已添加字段 sort_order")
	}
	if !existingColumns["docker_name"] {
		db.Exec("ALTER TABLE apps ADD COLUMN docker_name TEXT DEFAULT ''")
		fmt.Println("数据库迁移: 已添加字段 docker_name")
	}
	if !existingColumns["nginx_path"] {
		db.Exec("ALTER TABLE apps ADD COLUMN nginx_path TEXT DEFAULT ''")
		fmt.Println("数据库迁移: 已添加字段 nginx_path")
	}
	if !existingColumns["remark"] {
		db.Exec("ALTER TABLE apps ADD COLUMN remark TEXT DEFAULT ''")
		fmt.Println("数据库迁移: 已添加字段 remark")
	}
	if !existingColumns["remark"] {
		db.Exec("ALTER TABLE apps ADD COLUMN is_daemon  TEXT DEFAULT ''")
		fmt.Println("数据库迁移: 已添加字段 is_daemon ")
	}
	if !existingColumns["ports"] {
		db.Exec("ALTER TABLE apps ADD COLUMN ports TEXT DEFAULT '[]'")
		fmt.Println("数据库迁移: 已添加字段 ports")
	}
	if !existingColumns["pid"] {
		db.Exec("ALTER TABLE apps ADD COLUMN pid INTEGER DEFAULT 0")
		fmt.Println("数据库迁移: 已添加字段 pid")
	}
	return nil
}
