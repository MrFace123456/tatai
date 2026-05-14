package guard

import (
	"time"
)

// Policy 守护策略配置
type Policy struct {
	// Enabled 是否启用守护
	Enabled bool `json:"enabled"`

	// MaxRetries 最大重启次数（滑动窗口内）
	MaxRetries int `json:"max_retries"`

	// WindowDuration 滑动窗口时长
	WindowDuration time.Duration `json:"window_duration"`

	// BackoffBase 退避基础时间（首次重启等待时间）
	BackoffBase time.Duration `json:"backoff_base"`

	// BackoffMax 最大退避时间
	BackoffMax time.Duration `json:"backoff_max"`

	// CheckInterval 健康检查间隔
	CheckInterval time.Duration `json:"check_interval"`
}

// DefaultPolicy 默认守护策略
func DefaultPolicy() *Policy {
	return &Policy{
		Enabled:        true,
		MaxRetries:     3,
		WindowDuration: 60 * time.Second,
		BackoffBase:    5 * time.Second,
		BackoffMax:     60 * time.Second,
		CheckInterval:  10 * time.Second,
	}
}

// Validate 校验并修正策略参数
func (p *Policy) Validate() {
	if p.MaxRetries <= 0 {
		p.MaxRetries = 3
	}
	if p.WindowDuration <= 0 {
		p.WindowDuration = 60 * time.Second
	}
	if p.BackoffBase <= 0 {
		p.BackoffBase = 5 * time.Second
	}
	if p.BackoffMax <= 0 {
		p.BackoffMax = 60 * time.Second
	}
	if p.BackoffBase > p.BackoffMax {
		p.BackoffBase = p.BackoffMax
	}
	if p.CheckInterval <= 0 {
		p.CheckInterval = 10 * time.Second
	}
}

// GetBackoffDuration 根据重启次数计算退避等待时间
func (p *Policy) GetBackoffDuration(restartCount int) time.Duration {
	if restartCount <= 0 {
		return 0
	}

	backoff := p.BackoffBase
	for i := 1; i < restartCount; i++ {
		backoff *= 2
		if backoff > p.BackoffMax {
			return p.BackoffMax
		}
	}
	return backoff
}
