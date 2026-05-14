package manager

import (
	"fmt"
	"os"
	"path/filepath"
	"sort"
	"strings"
	"time"

	"github.com/shirou/gopsutil/v3/cpu"
	"github.com/shirou/gopsutil/v3/disk"
	"github.com/shirou/gopsutil/v3/mem"
	"github.com/shirou/gopsutil/v3/process"
)

type SysStats struct {
	CPUPercent   float64       `json:"cpu_percent"`
	MemPercent   float64       `json:"mem_percent"`
	DiskPercent  float64       `json:"disk_percent"`
	CPUUsageRank []ProcessStat `json:"cpu_rank"`
	MemUsageRank []ProcessStat `json:"mem_rank"`
}

// TopProcess 单个进程信息（用于前端展示）
type TopProcess struct {
	PID      int32   `json:"pid"`
	Name     string  `json:"name"`
	Usage    float64 `json:"usage"`     // CPU 为百分比，内存为 MB
	UsageStr string  `json:"usage_str"` // 格式化后的字符串，如 "45.2%" 或 "1.2 GB"
}

type ProcessStat struct {
	Name    string  `json:"name"`
	Percent float64 `json:"percent"`
}

func GetSystemStats() (*SysStats, error) {
	// 1. 基础仪表盘数据
	cpuP, _ := cpu.Percent(time.Second, false)
	vm, _ := mem.VirtualMemory()
	d, _ := disk.Usage("/")

	stats := &SysStats{
		CPUPercent:  cpuP[0],
		MemPercent:  vm.UsedPercent,
		DiskPercent: d.UsedPercent,
	}

	// 2. 获取进程排行榜
	procs, _ := process.Processes()
	var procStats []struct {
		pid  int32
		name string
		cpu  float64
		mem  float32
	}

	for _, p := range procs {
		n, _ := p.Name()
		c, _ := p.CPUPercent()
		m, _ := p.MemoryPercent()
		if n != "" {
			procStats = append(procStats, struct {
				pid  int32
				name string
				cpu  float64
				mem  float32
			}{p.Pid, n, c, m})
		}
	}

	// CPU 排序 (前5)
	sort.Slice(procStats, func(i, j int) bool { return procStats[i].cpu > procStats[j].cpu })
	for i := 0; i < 5 && i < len(procStats); i++ {
		stats.CPUUsageRank = append(stats.CPUUsageRank, ProcessStat{procStats[i].name, procStats[i].cpu})
	}

	// 内存排序 (前5)
	sort.Slice(procStats, func(i, j int) bool { return procStats[i].mem > procStats[j].mem })
	for i := 0; i < 5 && i < len(procStats); i++ {
		stats.MemUsageRank = append(stats.MemUsageRank, ProcessStat{procStats[i].name, float64(procStats[i].mem)})
	}

	return stats, nil
}

// GetTopProcesses 根据类型获取 Top 5 进程
// processType: "cpu" 或 "mem"
func GetTopProcesses(processType string) ([]TopProcess, error) {
	procs, err := process.Processes()
	if err != nil {
		return nil, err
	}

	type procInfo struct {
		pid      int32
		name     string
		cpu      float64
		mem      float32
		memBytes uint64
	}

	var allProcs []procInfo

	for _, p := range procs {
		name, _ := p.Name()
		if name == "" {
			continue
		}

		cpuPercent, _ := p.CPUPercent()
		memPercent, _ := p.MemoryPercent()
		memInfo, _ := p.MemoryInfo()

		memBytes := uint64(0)
		if memInfo != nil {
			memBytes = memInfo.RSS
		}

		allProcs = append(allProcs, procInfo{
			pid:      p.Pid,
			name:     name,
			cpu:      cpuPercent,
			mem:      memPercent,
			memBytes: memBytes,
		})
	}

	var result []TopProcess

	switch processType {
	case "cpu":
		// 按 CPU 使用率排序
		sort.Slice(allProcs, func(i, j int) bool {
			return allProcs[i].cpu > allProcs[j].cpu
		})
		for i := 0; i < 5 && i < len(allProcs); i++ {
			p := allProcs[i]
			result = append(result, TopProcess{
				PID:      p.pid,
				Name:     p.name,
				Usage:    p.cpu,
				UsageStr: formatCPUPercent(p.cpu),
			})
		}

	case "mem":
		// 按内存使用量排序
		sort.Slice(allProcs, func(i, j int) bool {
			return allProcs[i].memBytes > allProcs[j].memBytes
		})
		for i := 0; i < 5 && i < len(allProcs); i++ {
			p := allProcs[i]
			result = append(result, TopProcess{
				PID:      p.pid,
				Name:     p.name,
				Usage:    float64(p.memBytes) / 1024 / 1024, // 转换为 MB
				UsageStr: formatMemorySize(p.memBytes),
			})
		}
	}

	return result, nil
}

// formatCPUPercent 格式化 CPU 百分比
func formatCPUPercent(percent float64) string {
	return fmt.Sprintf("%.1f%%", percent)
}

// formatMemorySize 格式化内存大小
func formatMemorySize(bytes uint64) string {
	const (
		KB = 1024
		MB = KB * 1024
		GB = MB * 1024
	)

	switch {
	case bytes >= GB:
		return fmt.Sprintf("%.2f GB", float64(bytes)/float64(GB))
	case bytes >= MB:
		return fmt.Sprintf("%.2f MB", float64(bytes)/float64(MB))
	case bytes >= KB:
		return fmt.Sprintf("%.2f KB", float64(bytes)/float64(KB))
	default:
		return fmt.Sprintf("%d B", bytes)
	}
}

// DiskSuggestion 磁盘清理建议
type DiskSuggestion struct {
	Path     string `json:"path"`
	Size     int64  `json:"size"`
	SizeStr  string `json:"size_str"`
	Modified string `json:"modified"`
	Reason   string `json:"reason"`
}

// GetDiskSuggestions 获取磁盘清理建议
func GetDiskSuggestions() ([]DiskSuggestion, error) {
	var suggestions []DiskSuggestion

	// 定义需要扫描的常见大文件/日志目录
	scanPaths := []string{
		"/var/log",       // Linux 系统日志
		"/tmp",           // 临时文件目录
		"/opt/apps/logs", // 应用日志目录
		"./logs",         // 当前程序日志目录
		"./data",         // 数据目录
	}

	// 额外检查当前工作目录下的日志文件
	workDir, _ := os.Getwd()
	scanPaths = append(scanPaths, workDir)

	// 定义需要过滤的文件扩展名（日志和临时文件）
	logExts := map[string]bool{
		".log":   true,
		".out":   true,
		".tmp":   true,
		".temp":  true,
		".cache": true,
	}

	// 定义文件名的关键字匹配
	keywords := []string{
		"log", "out", "nohup", "syslog", "debug", "error",
		"tmp", "temp", "cache", "old", "backup",
	}

	for _, path := range scanPaths {
		// 检查路径是否存在
		info, err := os.Stat(path)
		if err != nil {
			continue
		}

		if info.IsDir() {
			// 遍历目录
			err := filepath.Walk(path, func(filePath string, fileInfo os.FileInfo, err error) error {
				if err != nil {
					return nil // 忽略无法访问的文件
				}

				// 跳过目录本身
				if fileInfo.IsDir() {
					// 跳过特定子目录避免扫描过深
					skipDirs := []string{"node_modules", ".git", "vendor", "Library", "System"}
					for _, skip := range skipDirs {
						if strings.Contains(filePath, skip) {
							return filepath.SkipDir
						}
					}
					return nil
				}

				// 检查文件大小（只关注大于 50MB 的文件）
				size := fileInfo.Size()
				if size < 50*1024*1024 {
					return nil
				}

				// 检查是否为日志/临时文件
				isTarget := false
				ext := strings.ToLower(filepath.Ext(filePath))
				if logExts[ext] {
					isTarget = true
				} else {
					// 检查文件名是否包含关键字
					lowerName := strings.ToLower(fileInfo.Name())
					for _, kw := range keywords {
						if strings.Contains(lowerName, kw) {
							isTarget = true
							break
						}
					}
				}

				if isTarget {
					suggestions = append(suggestions, DiskSuggestion{
						Path:     filePath,
						Size:     size,
						SizeStr:  formatMemorySize(uint64(size)),
						Modified: fileInfo.ModTime().Format("2006-01-02 15:04:05"),
						Reason:   getCleanReason(filePath, fileInfo),
					})
				}
				return nil
			})
			if err != nil {
				continue
			}
		} else {
			// 如果是文件，直接检查
			size := info.Size()
			if size >= 50*1024*1024 {
				ext := strings.ToLower(filepath.Ext(path))
				if logExts[ext] || containsKeywords(strings.ToLower(info.Name()), keywords) {
					suggestions = append(suggestions, DiskSuggestion{
						Path:     path,
						Size:     size,
						SizeStr:  formatMemorySize(uint64(size)),
						Modified: info.ModTime().Format("2006-01-02 15:04:05"),
						Reason:   getCleanReason(path, info),
					})
				}
			}
		}
	}

	// 按文件大小降序排序
	sort.Slice(suggestions, func(i, j int) bool {
		return suggestions[i].Size > suggestions[j].Size
	})

	// 限制返回前 20 条建议
	if len(suggestions) > 20 {
		suggestions = suggestions[:20]
	}

	return suggestions, nil
}

// containsKeywords 检查文件名是否包含关键字
func containsKeywords(name string, keywords []string) bool {
	for _, kw := range keywords {
		if strings.Contains(name, kw) {
			return true
		}
	}
	return false
}

// getCleanReason 生成清理理由
func getCleanReason(path string, info os.FileInfo) string {
	// 根据文件路径和类型生成清理建议理由
	path = strings.ToLower(path)
	modifiedDays := int(time.Since(info.ModTime()).Hours() / 24)

	switch {
	case strings.Contains(path, "nohup.out"):
		return "nohup 输出文件，通常可定期清理或重定向到 /dev/null"
	case strings.Contains(path, "syslog"):
		return "系统日志文件，如果已轮转可安全删除"
	case strings.Contains(path, ".log"):
		if modifiedDays > 30 {
			return fmt.Sprintf("日志文件，已超过 %d 天未修改，可考虑删除或归档", modifiedDays)
		}
		return "日志文件，如果应用已正常运行可考虑清空"
	case strings.Contains(path, "tmp") || strings.Contains(path, "temp"):
		return "临时文件目录，建议清理过期临时文件"
	case strings.Contains(path, "cache"):
		return "缓存文件，如果应用未在使用可安全删除"
	default:
		return "大文件，请确认是否为必要文件"
	}
}
