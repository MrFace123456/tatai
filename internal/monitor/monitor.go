package monitor

import (
	"fmt"
	"log"
	"tatai/internal/config"
	"tatai/internal/notify"
	"time"

	"github.com/shirou/gopsutil/v3/disk"
	"github.com/shirou/gopsutil/v3/mem"
)

// Config 监控配置
type Config struct {
	Enabled            bool
	Interval           time.Duration
	DiskWarningPercent float64
	MemWarningPercent  float64
}

// LoadFromAppConfig 从应用配置加载监控配置
func LoadFromAppConfig(appCfg *config.Config) *Config {
	if appCfg == nil {
		return DefaultConfig()
	}

	return &Config{
		Enabled:            appCfg.Monitor.IsEnabled(),
		Interval:           appCfg.Monitor.GetInterval(),
		DiskWarningPercent: appCfg.Monitor.DiskThreshold,
		MemWarningPercent:  appCfg.Monitor.MemoryThreshold,
	}
}

// DefaultConfig 默认配置
func DefaultConfig() *Config {
	return &Config{
		Enabled:            true,
		Interval:           5 * time.Minute,
		DiskWarningPercent: 85.0,
		MemWarningPercent:  90.0,
	}
}

// Monitor 监控器
type Monitor struct {
	config *Config
	stopCh chan struct{}

	// 防重复告警
	lastDiskAlert bool
	lastMemAlert  bool
}

// NewMonitor 创建监控器
func NewMonitor(config *Config) *Monitor {
	if config == nil {
		config = DefaultConfig()
	}
	return &Monitor{
		config: config,
		stopCh: make(chan struct{}),
	}
}

// Start 启动监控
func (m *Monitor) Start() {
	if !m.config.Enabled {
		log.Println("[Monitor] 监控模块未启用")
		return
	}

	log.Printf("[Monitor] 启动监控模块，检测间隔: %v, 磁盘阈值: %.0f%%, 内存阈值: %.0f%%",
		m.config.Interval, m.config.DiskWarningPercent, m.config.MemWarningPercent)
	go m.run()
}

// Stop 停止监控
func (m *Monitor) Stop() {
	close(m.stopCh)
	log.Println("[Monitor] 监控模块已停止")
}

// run 运行监控循环
func (m *Monitor) run() {
	// 启动后立即检测一次
	m.check()

	ticker := time.NewTicker(m.config.Interval)
	defer ticker.Stop()

	for {
		select {
		case <-ticker.C:
			m.check()
		case <-m.stopCh:
			return
		}
	}
}

// check 执行一次检测
func (m *Monitor) check() {
	m.checkDisk()
	m.checkMemory()
}

// checkDisk 检测磁盘
func (m *Monitor) checkDisk() {
	usage, err := disk.Usage("/")
	if err != nil {
		log.Printf("[Monitor] 获取磁盘信息失败: %v", err)
		return
	}

	usedPercent := usage.UsedPercent

	// 调试日志（可删除）
	log.Printf("[Monitor] 磁盘检测: 使用率 %.2f%%, 阈值 %.0f%%, lastAlert=%v",
		usedPercent, m.config.DiskWarningPercent, m.lastDiskAlert)

	if usedPercent >= m.config.DiskWarningPercent {
		if !m.lastDiskAlert {
			m.lastDiskAlert = true
			log.Printf("[Monitor] 触发磁盘告警! 使用率: %.2f%%", usedPercent)
			notify.SendDiskWarning(usage.Path, usedPercent, formatSize(usage.Free))
		}
	} else {
		if m.lastDiskAlert {
			m.lastDiskAlert = false
			log.Printf("[Monitor] 磁盘已恢复: 使用率 %.2f%%", usedPercent)
		}
	}
}

// checkMemory 检测内存
func (m *Monitor) checkMemory() {
	vm, err := mem.VirtualMemory()
	if err != nil {
		log.Printf("[Monitor] 获取内存信息失败: %v", err)
		return
	}

	usedPercent := vm.UsedPercent

	// 调试日志
	log.Printf("[Monitor] 内存检测: 使用率 %.2f%%, 阈值 %.0f%%, lastAlert=%v",
		usedPercent, m.config.MemWarningPercent, m.lastMemAlert)

	if usedPercent >= m.config.MemWarningPercent {
		if !m.lastMemAlert {
			m.lastMemAlert = true
			log.Printf("[Monitor] 触发内存告警! 使用率: %.2f%%", usedPercent)

			usedGB := float64(vm.Used) / (1024 * 1024 * 1024)
			totalGB := float64(vm.Total) / (1024 * 1024 * 1024)

			notify.SendAlert(notify.AlertMessage{
				EventType: notify.EventMemoryWarning,
				Title:     "内存使用率过高",
				Content: fmt.Sprintf("系统内存使用率已达 %.1f%%，已用 %.1f GB / 总计 %.1f GB",
					usedPercent, usedGB, totalGB),
				Level: notify.Warning,
				Extra: map[string]string{
					"used_percent": fmt.Sprintf("%.1f%%", usedPercent),
					"used":         fmt.Sprintf("%.1f GB", usedGB),
					"total":        fmt.Sprintf("%.1f GB", totalGB),
				},
			})
		}
	} else {
		if m.lastMemAlert {
			m.lastMemAlert = false
			log.Printf("[Monitor] 内存已恢复: 使用率 %.2f%%", usedPercent)
		}
	}
}

// formatSize 格式化字节大小
func formatSize(bytes uint64) string {
	const (
		KB = 1024
		MB = KB * 1024
		GB = MB * 1024
	)

	switch {
	case bytes >= GB:
		return fmt.Sprintf("%.1f GB", float64(bytes)/float64(GB))
	case bytes >= MB:
		return fmt.Sprintf("%.1f MB", float64(bytes)/float64(MB))
	case bytes >= KB:
		return fmt.Sprintf("%.1f KB", float64(bytes)/float64(KB))
	default:
		return fmt.Sprintf("%d B", bytes)
	}
}
