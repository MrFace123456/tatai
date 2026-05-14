package notify

import (
	"database/sql"
	"strings"
)

// InitWebhookTable 初始化webhook配置表（在数据库初始化时调用）
func InitWebhookTable(db *sql.DB) error {
	query := `
	CREATE TABLE IF NOT EXISTS webhook_config (
		id INTEGER PRIMARY KEY AUTOINCREMENT,
		name TEXT NOT NULL,
		url TEXT NOT NULL,
		enabled INTEGER DEFAULT 1,
		event_types TEXT DEFAULT '',
		created_at DATETIME DEFAULT CURRENT_TIMESTAMP,
		updated_at DATETIME DEFAULT CURRENT_TIMESTAMP
	);
	`
	_, err := db.Exec(query)
	return err
}

// SaveWebhookConfig 保存webhook配置（单条模式，匹配前端格式）
func SaveWebhookConfig(db *sql.DB, url string, events []string) error {
	tx, err := db.Begin()
	if err != nil {
		return err
	}
	defer tx.Rollback()

	// 清空现有配置
	if _, err := tx.Exec("DELETE FROM webhook_config"); err != nil {
		return err
	}

	// 直接保存前端传来的 events 值，不做特殊处理
	// 如果包含 "all" 或为空数组，都保存为 "all"
	eventTypesStr := ""
	if len(events) > 0 {
		// 检查是否包含 "all"
		hasAll := false
		filteredEvents := []string{}
		for _, e := range events {
			if e == "all" {
				hasAll = true
				break
			}
			// 将前端事件名映射为后台事件名（如果有映射）
			mapped := mapFrontendEvent(e)
			filteredEvents = append(filteredEvents, mapped)
		}
		if hasAll {
			// 包含 "all" 时，保存为 "all"
			eventTypesStr = "all"
		} else if len(filteredEvents) > 0 {
			eventTypesStr = strings.Join(filteredEvents, ",")
		}
	}

	// 插入新配置
	_, err = tx.Exec(`
		INSERT INTO webhook_config (name, url, enabled, event_types)
		VALUES (?, ?, ?, ?)
	`, "default", url, 1, eventTypesStr)
	if err != nil {
		return err
	}

	return tx.Commit()
}

// GetWebhookConfig 获取webhook配置（返回前端需要的格式）
// GetWebhookConfig 获取webhook配置（返回前端需要的格式）
func GetWebhookConfig(db *sql.DB) (*WebhookConfigResponse, error) {
	var url, eventTypesStr string
	var enabledInt int

	err := db.QueryRow(`
		SELECT url, enabled, COALESCE(event_types, '')
		FROM webhook_config 
		WHERE enabled = 1
		LIMIT 1
	`).Scan(&url, &enabledInt, &eventTypesStr)
	if err != nil {
		if err == sql.ErrNoRows {
			return nil, nil
		}
		return nil, err
	}

	// 解析event_types
	var events []string
	if eventTypesStr == "all" {
		// 如果是 "all"，返回 ["all"]
		events = []string{"all"}
	} else if eventTypesStr != "" {
		// 将后台事件名映射回前端事件名
		backendEvents := strings.Split(eventTypesStr, ",")
		for _, be := range backendEvents {
			events = append(events, mapBackendEvent(be))
		}
	} else {
		// 空字符串也表示接收所有事件，返回 ["all"]
		events = []string{"all"}
	}

	return &WebhookConfigResponse{
		URL:    url,
		Events: events,
	}, nil
}

// GetEnabledWebhooks 获取所有启用的webhook配置（广播模式用）
func GetEnabledWebhooks(db *sql.DB) ([]WebhookConfig, error) {
	rows, err := db.Query(`
		SELECT id, name, url, enabled, COALESCE(event_types, '')
		FROM webhook_config 
		WHERE enabled = 1
	`)
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	var configs []WebhookConfig
	for rows.Next() {
		var cfg WebhookConfig
		var eventTypesStr string
		var enabledInt int

		if err := rows.Scan(&cfg.ID, &cfg.Name, &cfg.URL, &enabledInt, &eventTypesStr); err != nil {
			return nil, err
		}
		cfg.Enabled = enabledInt == 1
		if eventTypesStr != "" {
			cfg.EventTypes = strings.Split(eventTypesStr, ",")
		} else {
			cfg.EventTypes = []string{}
		}
		configs = append(configs, cfg)
	}
	return configs, nil
}

// GetEnabledWebhooksForEvent 根据事件类型获取匹配的webhook
func GetEnabledWebhooksForEvent(db *sql.DB, eventType EventType) ([]WebhookConfig, error) {
	rows, err := db.Query(`
		SELECT id, name, url, enabled, COALESCE(event_types, '')
		FROM webhook_config 
		WHERE enabled = 1
	`)
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	var configs []WebhookConfig
	for rows.Next() {
		var cfg WebhookConfig
		var eventTypesStr string
		var enabledInt int

		if err := rows.Scan(&cfg.ID, &cfg.Name, &cfg.URL, &enabledInt, &eventTypesStr); err != nil {
			return nil, err
		}
		cfg.Enabled = enabledInt == 1

		// 解析事件类型
		if eventTypesStr != "" {
			cfg.EventTypes = strings.Split(eventTypesStr, ",")
		} else {
			cfg.EventTypes = []string{} // 空表示接收所有
		}

		// 检查是否匹配
		if shouldReceiveEvent(cfg, eventType) {
			configs = append(configs, cfg)
		}
	}
	return configs, nil
}

// shouldReceiveEvent 判断配置是否应该接收该事件
func shouldReceiveEvent(cfg WebhookConfig, eventType EventType) bool {
	// 如果配置的事件类型包含 "all" 或为空数组，接收所有事件
	if len(cfg.EventTypes) == 0 {
		return true
	}
	// 检查是否包含 "all"
	for _, et := range cfg.EventTypes {
		if et == "all" {
			return true
		}
	}
	// 检查是否精确匹配
	eventTypeStr := string(eventType)
	for _, et := range cfg.EventTypes {
		if et == eventTypeStr {
			return true
		}
	}
	return false
}

// 前端事件名 → 后台事件名映射
var frontendToBackend = map[string]string{
	"crash":      string(EventAppCrash),
	"disk":       string(EventDiskWarning),
	"start_fail": string(EventAppStartFail),
	"start":      string(EventAppStart),
	"stop":       string(EventAppStop),
}

// 后台事件名 → 前端事件名映射
var backendToFrontend = map[string]string{
	string(EventAppCrash):     "crash",
	string(EventDiskWarning):  "disk",
	string(EventAppStartFail): "start_fail",
	string(EventAppStart):     "start",
	string(EventAppStop):      "stop",
}

// mapFrontendEvent 将前端事件名映射为后台事件名
func mapFrontendEvent(frontendEvent string) string {
	if mapped, ok := frontendToBackend[frontendEvent]; ok {
		return mapped
	}
	return frontendEvent
}

// mapBackendEvent 将后台事件名映射为前端事件名
func mapBackendEvent(backendEvent string) string {
	if mapped, ok := backendToFrontend[backendEvent]; ok {
		return mapped
	}
	return backendEvent
}
