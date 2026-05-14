package manager

import (
	"fmt"
	"os"
	"os/exec"
	"regexp"
	"strconv"
	"strings"
	"syscall"
)

// GetCommand 针对 Linux/Unix 的命令包装，并创建新会话脱离终端
func GetCommand(command string) *exec.Cmd {
	cmd := exec.Command("sh", "-c", command)
	if cmd.SysProcAttr == nil {
		cmd.SysProcAttr = &syscall.SysProcAttr{}
	}
	// 创建新会话，使子进程独立于父进程终端，避免 SIGHUP
	cmd.SysProcAttr.Setsid = true
	return cmd
}

// NewCommand 使用参数数组创建命令
func NewCommand(name string, args ...string) *exec.Cmd {
	cmd := exec.Command(name, args...)
	if cmd.SysProcAttr == nil {
		cmd.SysProcAttr = &syscall.SysProcAttr{}
	}
	cmd.SysProcAttr.Setsid = true
	return cmd
}

// GetProcessPorts Linux 平台获取进程端口
func GetProcessPorts(pid int) ([]int, error) {
	// 尝试 lsof
	lsofCmd := exec.Command("lsof", "-p", strconv.Itoa(pid), "-i", "-P", "-n")
	out, err := lsofCmd.Output()
	if err == nil {
		return parseLsofPorts(string(out)), nil
	}
	// 降级 netstat -tlnp
	netstatCmd := exec.Command("netstat", "-tlnp")
	out, err = netstatCmd.Output()
	if err != nil {
		return nil, fmt.Errorf("无法获取端口信息: %v", err)
	}
	return parseNetstatPorts(string(out), pid), nil
}

// IsProcessAlive 检查进程是否存活（Linux 实现）
func IsProcessAlive(pid int) bool {
	process, err := os.FindProcess(pid)
	if err != nil {
		return false
	}
	// 发送信号 0 测试进程是否存在
	err = process.Signal(syscall.Signal(0))
	return err == nil
}

// KillProcess 强制终止指定 PID 的进程（Linux 实现）
func KillProcess(pid int) error {
	process, err := os.FindProcess(pid)
	if err != nil {
		return err
	}
	return process.Kill()
}
func parseLsofPorts(output string) []int {
	ports := make(map[int]bool)
	lines := strings.Split(output, "\n")
	for _, line := range lines {
		if strings.Contains(line, "LISTEN") {
			re := regexp.MustCompile(`[0-9]+\.[0-9]+\.[0-9]+\.[0-9]+:([0-9]+)`)
			matches := re.FindStringSubmatch(line)
			if len(matches) == 2 {
				port, _ := strconv.Atoi(matches[1])
				ports[port] = true
			}
		}
	}
	result := []int{}
	for p := range ports {
		result = append(result, p)
	}
	return result
}

func parseNetstatPorts(output string, pid int) []int {
	ports := make(map[int]bool)
	lines := strings.Split(output, "\n")
	for _, line := range lines {
		if strings.Contains(line, "LISTEN") && strings.Contains(line, fmt.Sprintf("%d/", pid)) {
			fields := strings.Fields(line)
			if len(fields) >= 4 {
				localAddr := fields[3]
				parts := strings.Split(localAddr, ":")
				if len(parts) >= 2 {
					port, _ := strconv.Atoi(parts[len(parts)-1])
					ports[port] = true
				}
			}
		}
	}
	result := []int{}
	for p := range ports {
		result = append(result, p)
	}
	return result
}
