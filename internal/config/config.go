package config

import (
	"os"
	"time"

	"gopkg.in/yaml.v3"
)

type Config struct {
	Server struct {
		Port    int    `yaml:"port"`
		Address string `yaml:"address"`
		Name    string `yaml:"name"` // 服务器标识
	} `yaml:"server"`
	JDK struct {
		Paths map[string]string `yaml:"paths"`
	} `yaml:"jdk"`
	Monitor MonitorConfig `yaml:"monitor"` // 监控配置
	JWT     JWTConfig     `yaml:"jwt"`     // JWT认证配置
}

// MonitorConfig 监控模块配置
type MonitorConfig struct {
	Enabled         bool    `yaml:"enabled"`          // 是否启用
	IntervalSeconds int     `yaml:"interval_seconds"` // 检测间隔（秒）
	DiskThreshold   float64 `yaml:"disk_threshold"`   // 磁盘告警阈值（%）
	MemoryThreshold float64 `yaml:"memory_threshold"` // 内存告警阈值（%）
}

// JWTConfig JWT认证配置
type JWTConfig struct {
	Secret      string `yaml:"secret"`       // JWT签名密钥
	ExpireHours int    `yaml:"expire_hours"` // 过期时间（小时）
}

// GetInterval 获取检测间隔（转换为 time.Duration）
func (m *MonitorConfig) GetInterval() time.Duration {
	if m.IntervalSeconds <= 0 {
		return 5 * time.Minute // 默认5分钟
	}
	return time.Duration(m.IntervalSeconds) * time.Second
}

// IsEnabled 是否启用
func (m *MonitorConfig) IsEnabled() bool {
	// 默认启用
	return m.Enabled
}

// LoadConfig 加载配置文件
func LoadConfig(path string) (*Config, error) {
	file, err := os.ReadFile(path)
	if err != nil {
		return nil, err
	}
	var cfg Config
	if err := yaml.Unmarshal(file, &cfg); err != nil {
		return nil, err
	}

	// 设置默认值（如果配置文件中没有）
	cfg.setDefaults()

	return &cfg, nil
}

// setDefaults 设置默认值
func (c *Config) setDefaults() {
	// 监控模块默认值
	if c.Monitor.IntervalSeconds == 0 {
		c.Monitor.IntervalSeconds = 300 // 5分钟
	}
	if c.Monitor.DiskThreshold == 0 {
		c.Monitor.DiskThreshold = 85.0
	}
	if c.Monitor.MemoryThreshold == 0 {
		c.Monitor.MemoryThreshold = 90.0
	}

	// JWT默认配置
	if c.JWT.Secret == "" {
		c.JWT.Secret = "tatai-default-secret-key-change-in-production"
	}
	if c.JWT.ExpireHours == 0 {
		c.JWT.ExpireHours = 24 // 默认24小时
	}
}

// GetServerName 获取服务器标识，如果未配置则返回主机名
func (c *Config) GetServerName() string {
	if c.Server.Name != "" {
		return c.Server.Name
	}
	hostname, err := os.Hostname()
	if err != nil {
		return "unknown-host"
	}
	return hostname
}
