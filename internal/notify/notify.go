package notify

import (
	"bytes"
	"database/sql"
	"encoding/json"
	"fmt"
	"log"
	"net/http"
	"time"
)

var (
	dbInstance *sql.DB
	httpClient = &http.Client{
		Timeout: 5 * time.Second, // 5秒超时
	}
	serverName = "unknown" // 新增：全局服务器标识
)

// Init 初始化通知模块，传入服务器名称
func Init(db *sql.DB, name string) error {
	dbInstance = db
	serverName = name
	return InitWebhookTable(db)
}

// SendAlert 发送告警通知（异步，不阻塞）
func SendAlert(msg AlertMessage) {
	if dbInstance == nil {
		log.Println("[Notify] 通知模块未初始化，跳过发送")
		return
	}

	// 异步发送，不阻塞主流程
	go doSendAlert(msg)
}

// doSendAlert 实际发送逻辑
func doSendAlert(msg AlertMessage) {
	// 设置时间戳（如果未设置）
	if msg.Timestamp == 0 {
		msg.Timestamp = time.Now().Unix()
	}

	// 获取匹配该事件类型的webhook配置
	configs, err := GetEnabledWebhooksForEvent(dbInstance, msg.EventType)
	if err != nil {
		log.Printf("[Notify] 获取webhook配置失败: %v", err)
		return
	}

	if len(configs) == 0 {
		// 没有配置匹配的webhook，静默跳过
		return
	}

	// 构建企业微信格式的消息内容
	webhookMsg := buildWeChatMessage(msg)

	// 向每个匹配的webhook发送
	for _, cfg := range configs {
		sendToWebhook(cfg, webhookMsg, msg)
	}
}

// sendToWebhook 发送到单个webhook
func sendToWebhook(cfg WebhookConfig, payload []byte, msg AlertMessage) {
	req, err := http.NewRequest("POST", cfg.URL, bytes.NewReader(payload))
	if err != nil {
		log.Printf("[Notify] 创建请求失败 [%s]: %v", cfg.Name, err)
		return
	}
	req.Header.Set("Content-Type", "application/json")

	resp, err := httpClient.Do(req)
	if err != nil {
		log.Printf("[Notify] 发送失败 [%s]: %v", cfg.Name, err)
		return
	}
	defer resp.Body.Close()

	// 检查HTTP状态码
	if resp.StatusCode < 200 || resp.StatusCode >= 300 {
		log.Printf("[Notify] 收到非成功状态码 [%s]: %d", cfg.Name, resp.StatusCode)
		return
	}

	log.Printf("[Notify] 发送成功 [%s] 事件: %s", cfg.Name, msg.EventType)
}

// buildWeChatMessage 构建企业微信机器人消息格式，自动添加服务器标识
func buildWeChatMessage(msg AlertMessage) []byte {
	var levelIcon string
	switch msg.Level {
	case Warning:
		levelIcon = "⚠️"
	case Error:
		levelIcon = "🚨"
	default:
		levelIcon = "📢"
	}

	// 构建主体内容
	content := fmt.Sprintf(`%s 【%s】
服务器: %s
标题: %s
详情: %s
时间: %s`,
		levelIcon,
		string(msg.EventType),
		serverName, // 自动附加服务器标识
		msg.Title,
		msg.Content,
		time.Unix(msg.Timestamp, 0).Format("2006-01-02 15:04:05"),
	)

	// 附加额外字段
	if len(msg.Extra) > 0 {
		content += "\n附加信息:"
		for k, v := range msg.Extra {
			content += fmt.Sprintf("\n  %s: %s", k, v)
		}
	}

	wechatMsg := map[string]interface{}{
		"msgtype": "text",
		"text": map[string]string{
			"content": content,
		},
	}

	data, _ := json.Marshal(wechatMsg)
	return data
}

// 以下是一些便捷方法，供其他模块调用

// SendDiskWarning 发送磁盘空间不足告警
func SendDiskWarning(path string, usedPercent float64, available string) {
	SendAlert(AlertMessage{
		EventType: EventDiskWarning,
		Title:     "磁盘空间不足",
		Content:   fmt.Sprintf("路径 %s 使用率已达 %.1f%%，剩余空间 %s", path, usedPercent, available),
		Level:     Warning,
		Extra: map[string]string{
			"path":         path,
			"used_percent": fmt.Sprintf("%.1f%%", usedPercent),
			"available":    available,
		},
	})
}

// SendAppStartFail 发送应用启动失败告警
func SendAppStartFail(appID int, appName string, errMsg string) {
	SendAlert(AlertMessage{
		EventType: EventAppStartFail,
		Title:     "应用启动失败",
		Content:   fmt.Sprintf("应用 %s (ID:%d) 启动失败: %s", appName, appID, errMsg),
		Level:     Error,
		Extra: map[string]string{
			"app_id":   fmt.Sprintf("%d", appID),
			"app_name": appName,
		},
	})
}

// SendAppCrash 发送应用异常退出告警
func SendAppCrash(appID int, appName string) {
	SendAlert(AlertMessage{
		EventType: EventAppCrash,
		Title:     "应用异常退出",
		Content:   fmt.Sprintf("应用 %s (ID:%d) 异常退出，请检查日志", appName, appID),
		Level:     Error,
		Extra: map[string]string{
			"app_id":   fmt.Sprintf("%d", appID),
			"app_name": appName,
		},
	})
}

// SendAppStart 发送应用启动成功通知
func SendAppStart(appID int, appName string, pid int) {
	SendAlert(AlertMessage{
		EventType: EventAppStart,
		Title:     "应用启动成功",
		Content:   fmt.Sprintf("应用 %s (ID:%d) 已启动，PID: %d", appName, appID, pid),
		Level:     Info,
		Extra: map[string]string{
			"app_id": fmt.Sprintf("%d", appID),
			"pid":    fmt.Sprintf("%d", pid),
		},
	})
}

// SendAppStop 发送应用停止通知
func SendAppStop(appID int, appName string) {
	SendAlert(AlertMessage{
		EventType: EventAppStop,
		Title:     "应用已停止",
		Content:   fmt.Sprintf("应用 %s (ID:%d) 已停止", appName, appID),
		Level:     Info,
		Extra: map[string]string{
			"app_id":   fmt.Sprintf("%d", appID),
			"app_name": appName,
		},
	})
}
