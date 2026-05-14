package model

type App struct {
	ID          int    `json:"id"`
	Name        string `json:"name"`
	Status      string `json:"status"`       // 运行状态: running, stopped
	AutoRestart bool   `json:"auto_restart"` // 自动重启开关
	SortOrder   int    `json:"sort_order"`
	Remark      string `json:"remark"` // 2.0 新增：应用备注

	// --- 多模态核心字段 ---
	Type string `json:"type"` // "docker", "jar", "nginx", "other"

	// 1. JAR 配置
	JDKKey   string `json:"jdk_key"`   // 对应 JDKPool 的 Key
	JarPath  string `json:"jar_path"`  // JAR包绝对路径
	Memory   string `json:"memory"`    // 内存配置，如 "512m"
	IsDaemon bool   `json:"is_daemon"` // 是否开启守护（自动重启）

	// 2. Docker 配置
	DockerName string `json:"docker_name"` // 容器名或 ID

	// 3. Nginx 配置
	NginxPath string `json:"nginx_path"` // Nginx 配置文件或执行路径

	// 4. 通用/自定义配置 (取代原有的单一 Command)
	Command  string            `json:"command"`   // 启动命令
	StopCmd  string            `json:"stop_cmd"`  // 停止命令 (2.0 针对 non-java 进程)
	CheckCmd string            `json:"check_cmd"` // 自定义健康检查命令
	Env      map[string]string `json:"env"`       // 环境变量

	Ports string `json:"ports"` // 存储 JSON 数组，默认 "[]"

	PID int `json:"pid,omitempty"` // 存储进程 ID（仅 jar/other 类型有效）
}
