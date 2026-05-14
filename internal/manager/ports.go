package manager

import (
	"encoding/json"
	"fmt"
	"os"
	"os/exec"
	"regexp"
	"strconv"
	"strings"
)

type PortEntry struct {
	Port     int    `json:"port"`
	Protocol string `json:"protocol,omitempty"`
	HostPort int    `json:"hostPort,omitempty"`
	HostIP   string `json:"hostIP,omitempty"`
}

// GetDockerPorts 获取 Docker 容器的端口映射
func GetDockerPorts(containerName string) ([]PortEntry, error) {
	cmd := exec.Command("docker", "inspect", "--format='{{json .NetworkSettings.Ports}}'", containerName)
	out, err := cmd.Output()
	if err != nil {
		return nil, fmt.Errorf("docker inspect 失败: %v", err)
	}
	// 去除单引号
	raw := strings.TrimSpace(string(out))
	raw = strings.Trim(raw, "'")
	var portsMap map[string]interface{}
	if err := json.Unmarshal([]byte(raw), &portsMap); err != nil {
		return nil, fmt.Errorf("解析端口 JSON 失败: %v", err)
	}
	var entries []PortEntry
	for containerPortProto, bindings := range portsMap {
		// containerPortProto 格式如 "8080/tcp"
		parts := strings.Split(containerPortProto, "/")
		if len(parts) != 2 {
			continue
		}
		portInt, _ := strconv.Atoi(parts[0])
		protocol := parts[1]
		// bindings 可能是 nil 或数组
		if bindings == nil {
			// 未映射
			entries = append(entries, PortEntry{Port: portInt, Protocol: protocol})
			continue
		}
		bindArr, ok := bindings.([]interface{})
		if !ok || len(bindArr) == 0 {
			entries = append(entries, PortEntry{Port: portInt, Protocol: protocol})
			continue
		}
		for _, b := range bindArr {
			binding := b.(map[string]interface{})
			hostIP := binding["HostIp"].(string)
			hostPortStr := binding["HostPort"].(string)
			hostPort, _ := strconv.Atoi(hostPortStr)
			entries = append(entries, PortEntry{
				Port:     portInt,
				Protocol: protocol,
				HostPort: hostPort,
				HostIP:   hostIP,
			})
		}
	}
	return entries, nil
}

// GetNginxPorts 解析 nginx.conf 提取 listen 端口（简化版，支持 include 一级）
func GetNginxPorts(confPath string) ([]PortEntry, error) {
	content, err := os.ReadFile(confPath)
	if err != nil {
		return nil, err
	}
	// 简单正则匹配 listen 指令
	// 支持 listen 80; listen 443 ssl; listen 127.0.0.1:8080;
	re := regexp.MustCompile(`listen\s+([^;]+);`)
	matches := re.FindAllStringSubmatch(string(content), -1)
	entries := []PortEntry{}
	for _, match := range matches {
		if len(match) < 2 {
			continue
		}
		listenStr := strings.TrimSpace(match[1])
		// 处理 host:port 格式
		var host, portPart string
		if strings.Contains(listenStr, ":") {
			parts := strings.SplitN(listenStr, ":", 2)
			host = parts[0]
			portPart = parts[1]
		} else {
			host = ""
			portPart = listenStr
		}
		// 去除可能的 ssl 等参数
		portField := strings.Fields(portPart)[0]
		port, err := strconv.Atoi(portField)
		if err != nil {
			continue
		}
		entry := PortEntry{Port: port, Protocol: "tcp"}
		if host != "" {
			entry.HostIP = host
		}
		entries = append(entries, entry)
	}
	// 简单支持 include（只处理一层，若包含其他文件，递归读取暂不实现）
	// 这里省略 include 解析，可后续扩展
	return entries, nil
}
