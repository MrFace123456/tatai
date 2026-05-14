package guard

import (
	"fmt"
	"log"
	"sync"
	"tatai/internal/model"
	"tatai/internal/notify"
	"time"
)

// RestartRecord 重启记录
type RestartRecord struct {
	AppID      int       `json:"app_id"`
	Timestamp  time.Time `json:"timestamp"`
	Reason     string    `json:"reason"`
	BackoffSec int       `json:"backoff_sec"`
}

// GuardedApp 被守护的应用信息
type GuardedApp struct {
	App    *model.App `json:"app"`
	Policy *Policy    `json:"policy"`

	// 运行时状态
	mu             sync.Mutex
	restartHistory []RestartRecord
	isRestarting   bool
	stopCh         chan struct{}
}

// GetRestartHistory 获取重启历史（导出方法）
func (g *GuardedApp) GetRestartHistory(limit int) []RestartRecord {
	g.mu.Lock()
	defer g.mu.Unlock()

	if limit <= 0 || limit > len(g.restartHistory) {
		limit = len(g.restartHistory)
	}

	result := make([]RestartRecord, limit)
	for i := 0; i < limit; i++ {
		result[i] = g.restartHistory[i]
	}
	return result
}

// GetAllRestartHistory 获取所有重启历史
func (g *GuardedApp) GetAllRestartHistory() []RestartRecord {
	g.mu.Lock()
	defer g.mu.Unlock()

	result := make([]RestartRecord, len(g.restartHistory))
	copy(result, g.restartHistory)
	return result
}

// addRestartRecord 添加重启记录
func (g *GuardedApp) addRestartRecord(record RestartRecord) {
	g.mu.Lock()
	defer g.mu.Unlock()

	g.restartHistory = append([]RestartRecord{record}, g.restartHistory...)

	if len(g.restartHistory) > 100 {
		g.restartHistory = g.restartHistory[:100]
	}
}

// cleanExpiredHistory 清理过期的重启记录
func (g *GuardedApp) cleanExpiredHistory(window time.Duration) {
	g.mu.Lock()
	defer g.mu.Unlock()

	cutoff := time.Now().Add(-window)
	newHistory := make([]RestartRecord, 0)

	for _, record := range g.restartHistory {
		if record.Timestamp.After(cutoff) {
			newHistory = append(newHistory, record)
		}
	}
	g.restartHistory = newHistory
}

// countRecentRestarts 统计窗口内的重启次数
func (g *GuardedApp) countRecentRestarts(window time.Duration) int {
	g.mu.Lock()
	defer g.mu.Unlock()

	cutoff := time.Now().Add(-window)
	count := 0
	for _, record := range g.restartHistory {
		if record.Timestamp.After(cutoff) {
			count++
		}
	}
	return count
}

// stop 停止守护
func (g *GuardedApp) stop() {
	close(g.stopCh)
}

// AppRunner 定义应用运行接口（由 manager 实现）
type AppRunner interface {
	IsRunning(appID int) bool
	StartApp(app *model.App) error
	GetAppByID(appID int) (*model.App, error)
}

// ProcessGuard 进程守护器
type ProcessGuard struct {
	mu          sync.RWMutex
	guardedApps map[int]*GuardedApp
	stopCh      chan struct{}
	started     bool

	// 外部依赖
	appRunner AppRunner
}

// NewProcessGuard 创建进程守护器
func NewProcessGuard(runner AppRunner) *ProcessGuard {
	return &ProcessGuard{
		guardedApps: make(map[int]*GuardedApp),
		appRunner:   runner,
		stopCh:      make(chan struct{}),
	}
}

// Start 启动守护器
func (g *ProcessGuard) Start() {
	g.mu.Lock()
	defer g.mu.Unlock()

	if g.started {
		return
	}
	g.started = true

	go g.runLoop()
	log.Println("[Guard] 进程守护器已启动")
}

// Stop 停止守护器
func (g *ProcessGuard) Stop() {
	g.mu.Lock()
	defer g.mu.Unlock()

	if !g.started {
		return
	}

	// 通知所有被守护的应用停止
	for _, guarded := range g.guardedApps {
		guarded.stop()
	}

	close(g.stopCh)
	g.started = false
	log.Println("[Guard] 进程守护器已停止")
}

// runLoop 主循环
func (g *ProcessGuard) runLoop() {
	ticker := time.NewTicker(5 * time.Second)
	defer ticker.Stop()

	for {
		select {
		case <-ticker.C:
			g.checkAll()
		case <-g.stopCh:
			return
		}
	}
}

// checkAll 检查所有被守护的应用
func (g *ProcessGuard) checkAll() {
	g.mu.RLock()
	apps := make([]*GuardedApp, 0, len(g.guardedApps))
	for _, guarded := range g.guardedApps {
		apps = append(apps, guarded)
	}
	g.mu.RUnlock()

	for _, guarded := range apps {
		g.checkAndRestart(guarded)
	}
}

// checkAndRestart 检查单个应用并尝试重启
func (g *ProcessGuard) checkAndRestart(guarded *GuardedApp) {
	guarded.mu.Lock()
	if guarded.isRestarting {
		guarded.mu.Unlock()
		return
	}
	guarded.mu.Unlock()

	// 检查进程是否存活
	isRunning := g.appRunner.IsRunning(guarded.App.ID)
	if isRunning {
		return
	}

	// 进程已死，执行重启逻辑
	log.Printf("[Guard] 应用 %s (ID:%d) 进程已退出，开始恢复", guarded.App.Name, guarded.App.ID)

	guarded.mu.Lock()
	guarded.isRestarting = true
	guarded.mu.Unlock()

	go func() {
		defer func() {
			guarded.mu.Lock()
			guarded.isRestarting = false
			guarded.mu.Unlock()
		}()

		g.doRestart(guarded)
	}()
}

// doRestart 执行重启
func (g *ProcessGuard) doRestart(guarded *GuardedApp) {
	policy := guarded.Policy

	// 1. 清理过期的重启记录
	guarded.cleanExpiredHistory(policy.WindowDuration)

	// 2. 检查窗口内的重启次数
	recentCount := guarded.countRecentRestarts(policy.WindowDuration)

	if recentCount >= policy.MaxRetries {
		log.Printf("[Guard] 应用 %s (ID:%d) 在 %v 内重启 %d 次，已达到最大限制，暂停守护",
			guarded.App.Name, guarded.App.ID, policy.WindowDuration, recentCount)

		notify.SendAlert(notify.AlertMessage{
			EventType: notify.EventAppCrash,
			Title:     "应用守护失败（超过重启限制）",
			Content: fmt.Sprintf("应用 %s (ID:%d) 在 %v 内连续崩溃 %d 次，已停止自动重启",
				guarded.App.Name, guarded.App.ID, policy.WindowDuration, recentCount),
			Level: notify.Error,
			Extra: map[string]string{
				"app_id":   fmt.Sprintf("%d", guarded.App.ID),
				"app_name": guarded.App.Name,
			},
		})
		return
	}

	// 3. 计算退避等待时间
	waitTime := policy.GetBackoffDuration(recentCount + 1)
	if waitTime > 0 {
		log.Printf("[Guard] 应用 %s (ID:%d) 将在 %v 后尝试重启",
			guarded.App.Name, guarded.App.ID, waitTime)
		time.Sleep(waitTime)
	}

	// 4. 再次检查进程
	if g.appRunner.IsRunning(guarded.App.ID) {
		log.Printf("[Guard] 应用 %s (ID:%d) 在等待期间已恢复，跳过重启", guarded.App.Name, guarded.App.ID)
		return
	}

	// 5. 获取最新配置并启动
	latestApp, err := g.appRunner.GetAppByID(guarded.App.ID)
	if err != nil {
		log.Printf("[Guard] 获取应用配置失败 (ID:%d): %v", guarded.App.ID, err)
		latestApp = guarded.App
	}

	err = g.appRunner.StartApp(latestApp)
	if err != nil {
		log.Printf("[Guard] 重启应用 %s (ID:%d) 失败: %v", guarded.App.Name, guarded.App.ID, err)

		guarded.addRestartRecord(RestartRecord{
			AppID:      guarded.App.ID,
			Timestamp:  time.Now(),
			Reason:     "restart_failed: " + err.Error(),
			BackoffSec: int(waitTime.Seconds()),
		})

		notify.SendAppStartFail(guarded.App.ID, guarded.App.Name, err.Error())
		return
	}

	// 记录重启成功
	guarded.addRestartRecord(RestartRecord{
		AppID:      guarded.App.ID,
		Timestamp:  time.Now(),
		Reason:     "auto_restart",
		BackoffSec: int(waitTime.Seconds()),
	})

	log.Printf("[Guard] 应用 %s (ID:%d) 已自动重启成功", guarded.App.Name, guarded.App.ID)

	notify.SendAlert(notify.AlertMessage{
		EventType: notify.EventAppStart,
		Title:     "应用已自动恢复",
		Content:   fmt.Sprintf("应用 %s (ID:%d) 已自动重启", guarded.App.Name, guarded.App.ID),
		Level:     notify.Info,
		Extra: map[string]string{
			"app_id":   fmt.Sprintf("%d", guarded.App.ID),
			"app_name": guarded.App.Name,
		},
	})
}

// EnableGuard 为应用开启守护
func (g *ProcessGuard) EnableGuard(app *model.App, policy *Policy) error {
	if policy == nil {
		policy = DefaultPolicy()
	}
	policy.Validate()

	g.mu.Lock()
	defer g.mu.Unlock()

	if _, exists := g.guardedApps[app.ID]; exists {
		return fmt.Errorf("应用 %d 已经开启了守护", app.ID)
	}

	guarded := &GuardedApp{
		App:            app,
		Policy:         policy,
		restartHistory: make([]RestartRecord, 0),
		stopCh:         make(chan struct{}),
	}

	g.guardedApps[app.ID] = guarded

	log.Printf("[Guard] 已为应用 %s (ID:%d) 开启守护", app.Name, app.ID)
	return nil
}

// DisableGuard 关闭应用守护
func (g *ProcessGuard) DisableGuard(appID int) error {
	g.mu.Lock()
	defer g.mu.Unlock()

	guarded, exists := g.guardedApps[appID]
	if !exists {
		return fmt.Errorf("应用 %d 未开启守护", appID)
	}

	guarded.stop()
	delete(g.guardedApps, appID)

	log.Printf("[Guard] 已关闭应用 %d 的守护", appID)
	return nil
}

// IsGuarded 检查应用是否被守护
func (g *ProcessGuard) IsGuarded(appID int) bool {
	g.mu.RLock()
	defer g.mu.RUnlock()
	_, exists := g.guardedApps[appID]
	return exists
}

// GetGuardInfo 获取守护信息
func (g *ProcessGuard) GetGuardInfo(appID int) *GuardedApp {
	g.mu.RLock()
	defer g.mu.RUnlock()

	if guarded, exists := g.guardedApps[appID]; exists {
		guarded.mu.Lock()
		defer guarded.mu.Unlock()

		historyCopy := make([]RestartRecord, len(guarded.restartHistory))
		copy(historyCopy, guarded.restartHistory)

		return &GuardedApp{
			App:            guarded.App,
			Policy:         guarded.Policy,
			restartHistory: historyCopy,
		}
	}
	return nil
}

// GetAllGuardedApps 获取所有被守护的应用
func (g *ProcessGuard) GetAllGuardedApps() []*GuardedApp {
	g.mu.RLock()
	defer g.mu.RUnlock()

	result := make([]*GuardedApp, 0, len(g.guardedApps))
	for _, guarded := range g.guardedApps {
		guarded.mu.Lock()
		historyCopy := make([]RestartRecord, len(guarded.restartHistory))
		copy(historyCopy, guarded.restartHistory)

		result = append(result, &GuardedApp{
			App:            guarded.App,
			Policy:         guarded.Policy,
			restartHistory: historyCopy,
		})
		guarded.mu.Unlock()
	}
	return result
}
