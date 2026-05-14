package notify

// AlertLevel 告警级别
type AlertLevel string

const (
	Info    AlertLevel = "info"
	Warning AlertLevel = "warning"
	Error   AlertLevel = "error"
)

// EventType 事件类型
type EventType string

const (
	// 磁盘相关
	EventDiskWarning    EventType = "disk_warning"    // 磁盘空间不足
	EventDiskSuggestion EventType = "disk_suggestion" // 磁盘清理建议
	EventDiskRecover    EventType = "disk_recover"    // 磁盘恢复（可选）

	// 内存相关
	EventMemoryWarning EventType = "memory_warning" // 内存使用率过高
	EventSwapWarning   EventType = "swap_warning"   // 交换分区过高

	// 应用相关
	EventAppStartFail EventType = "app_start_fail" // 应用启动失败
	EventAppCrash     EventType = "app_crash"      // 应用异常退出
	EventAppStop      EventType = "app_stop"       // 应用停止
	EventAppStart     EventType = "app_start"      // 应用启动成功
)

// AlertMessage 统一告警消息结构
type AlertMessage struct {
	EventType EventType         `json:"event_type"`      // 事件类型
	Title     string            `json:"title"`           // 简短标题
	Content   string            `json:"content"`         // 详细内容
	Level     AlertLevel        `json:"level"`           // 告警级别
	Timestamp int64             `json:"timestamp"`       // 时间戳（Unix秒）
	Extra     map[string]string `json:"extra,omitempty"` // 额外字段
}

// WebhookConfig Webhook配置结构（数据库用）
type WebhookConfig struct {
	ID         int      `json:"id"`
	Name       string   `json:"name"`        // 配置名称
	URL        string   `json:"url"`         // Webhook地址
	Enabled    bool     `json:"enabled"`     // 是否启用
	EventTypes []string `json:"event_types"` // 监听的事件类型（空=全部）
}

// WebhookConfigResponse 前端响应结构
type WebhookConfigResponse struct {
	URL    string   `json:"url"`
	Events []string `json:"events"`
}
