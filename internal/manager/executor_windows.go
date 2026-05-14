//go:build windows

package manager

import (
	"fmt"
	"log"
	"os/exec"
	"regexp"
	"runtime"
	"strconv"
	"strings"
	"syscall"
)

// GetCommand 针对 Windows 的命令包装，并脱离控制台
func GetCommand(command string) *exec.Cmd {
	cmd := exec.Command("cmd", "/C", command)
	if cmd.SysProcAttr == nil {
		cmd.SysProcAttr = &syscall.SysProcAttr{}
	}
	// DETACHED_PROCESS = 0x8，使子进程不继承控制台
	cmd.SysProcAttr.CreationFlags = 0x00000008
	return cmd
}

// NewCommand 使用参数数组创建命令（推荐用于 Java 启动）
func NewCommand(name string, args ...string) *exec.Cmd {
	cmd := exec.Command(name, args...)
	if cmd.SysProcAttr == nil {
		cmd.SysProcAttr = &syscall.SysProcAttr{}
	}
	cmd.SysProcAttr.CreationFlags = 0x00000008
	return cmd
}

// IsProcessAlive 检查进程是否存活（Windows 实现）
func IsProcessAlive(pid int) bool {
	// 打开进程，只需要 PROCESS_QUERY_INFORMATION 和 SYNCHRONIZE 权限
	handle, err := syscall.OpenProcess(syscall.PROCESS_QUERY_INFORMATION|syscall.SYNCHRONIZE, false, uint32(pid))
	if err != nil {
		return false
	}
	defer syscall.CloseHandle(handle)

	var exitCode uint32
	// 获取退出代码
	err = syscall.GetExitCodeProcess(handle, &exitCode)
	if err != nil {
		return false
	}
	// STILL_ACTIVE = 259
	return exitCode == 259
}

// KillProcess 强制终止指定 PID 的进程（Windows 实现）
func KillProcess(pid int) error {
	handle, err := syscall.OpenProcess(syscall.PROCESS_TERMINATE, false, uint32(pid))
	if err != nil {
		return fmt.Errorf("OpenProcess failed: %v", err)
	}
	defer syscall.CloseHandle(handle)
	err = syscall.TerminateProcess(handle, 1)
	if err != nil {
		return fmt.Errorf("TerminateProcess failed: %v", err)
	}
	return nil
}

// GetProcessPorts Windows 平台获取进程端口（精确按 PID 查询）
func GetProcessPorts(pid int) ([]int, error) {
	log.Printf("[DEBUG] GetProcessPorts 被调用, pid=%d, GOOS=%s", pid, runtime.GOOS)

	// 使用 netstat -ano 并过滤 PID（不区分边界，因为端口号占用 PID 值的概率极低）
	cmd := exec.Command("cmd", "/C", "netstat -ano | findstr "+strconv.Itoa(pid))
	out, err := cmd.Output()
	if err != nil {
		// 没有输出也算成功，返回空切片
		log.Printf("[DEBUG] netstat 过滤无结果: %v", err)
		return []int{}, nil
	}
	return parseNetstatPorts(string(out), pid), nil
}

// parseNetstatPorts 从 netstat 输出中提取该 PID 的 LISTENING 端口
func parseNetstatPorts(output string, pid int) []int {
	ports := make(map[int]bool)
	lines := strings.Split(output, "\n")
	pidStr := strconv.Itoa(pid)

	for _, line := range lines {
		// 必须包含 LISTENING 且 PID 匹配
		if !strings.Contains(line, "LISTENING") {
			continue
		}
		// 确保 PID 以空格或行尾分隔
		if !strings.Contains(line, " "+pidStr) && !strings.HasSuffix(line, pidStr) {
			continue
		}
		// 提取端口：匹配 IP:端口 或 [::]:端口
		re := regexp.MustCompile(`\d+\.\d+\.\d+\.\d+:(\d+)|\[.*\]:(\d+)`)
		matches := re.FindAllStringSubmatch(line, -1)
		for _, match := range matches {
			portStr := match[1]
			if portStr == "" {
				portStr = match[2]
			}
			if portStr == "" {
				continue
			}
			port, err := strconv.Atoi(portStr)
			if err != nil {
				continue
			}
			// 过滤掉端口 0（0.0.0.0:0 这种无效端口）
			if port == 0 {
				continue
			}
			ports[port] = true
		}
	}

	result := []int{}
	for p := range ports {
		result = append(result, p)
	}
	log.Printf("[DEBUG] parseNetstatPorts result: %v", result)
	return result
}
