package manager

import (
	"database/sql"
	"fmt"
	"log"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"sync"
	"tatai/internal/database"
	"tatai/internal/guard"
	"tatai/internal/model"
	"tatai/internal/notify"
)

// JDKPool JDK路径映射表
var JDKPool map[string]string

// GlobalManager 全局管理器
type GlobalManager struct {
	ActiveProcs map[int]*exec.Cmd
	mu          sync.Mutex
	Guard       *guard.ProcessGuard // 进程守护器
	db          *sql.DB             // 数据库连接
	// 数据库回调函数（由 main.go 注入）
	getAppByIDFunc func(appID int) (*model.App, error)
}

func (m *GlobalManager) GetPID(appID int) (int, bool) {
	m.mu.Lock()
	defer m.mu.Unlock()
	cmd, exists := m.ActiveProcs[appID]
	if !exists || cmd.Process == nil {
		return 0, false
	}
	return cmd.Process.Pid, true
}

// Mgr 全局管理器实例
var Mgr = &GlobalManager{
	ActiveProcs: make(map[int]*exec.Cmd),
}

// InitJDKPool 初始化 JDK 路径映射（在 main.go 中调用）
func InitJDKPool(jdkPaths map[string]string) {
	JDKPool = jdkPaths
	log.Printf("[Manager] JDK路径已初始化，共 %d 个配置", len(JDKPool))
}

// InitManager 初始化管理器（在 main.go 中调用）
func InitManager(db *sql.DB) {
	Mgr.db = db
	Mgr.Guard = guard.NewProcessGuard(Mgr)
	Mgr.Guard.Start()
	log.Println("[Manager] 管理器已初始化，守护器已启动")
}

// SetGetAppByIDFunc 设置获取应用信息的回调函数
func (m *GlobalManager) SetGetAppByIDFunc(fn func(appID int) (*model.App, error)) {
	m.getAppByIDFunc = fn
}

// GetAppByID 获取应用信息（实现 guard.AppRunner 接口）
func (m *GlobalManager) GetAppByID(appID int) (*model.App, error) {
	if m.getAppByIDFunc == nil {
		return nil, fmt.Errorf("获取应用信息回调函数未设置")
	}
	return m.getAppByIDFunc(appID)
}

// IsRunning 检查进程是否在运行
func (m *GlobalManager) IsRunning(appID int) bool {
	m.mu.Lock()
	defer m.mu.Unlock()
	_, exists := m.ActiveProcs[appID]
	return exists
}

// StartApp 启动应用
func (m *GlobalManager) StartApp(app *model.App) error {
	m.mu.Lock()
	defer m.mu.Unlock()

	if _, exists := m.ActiveProcs[app.ID]; exists {
		return fmt.Errorf("应用(ID:%d)已在运行中", app.ID)
	}

	// 规范化类型：前端可能传 "jar"，后端内部使用 "jar_template"
	appType := app.Type
	if appType == "jar" {
		appType = "jar_template"
	}

	var cmd *exec.Cmd
	var logDir string

	switch appType {
	case "docker":
		logDir = filepath.Join("logs", "docker")
		// 检查容器是否存在
		checkCmd := exec.Command("docker", "ps", "-a", "--filter", "name="+app.DockerName, "--format", "{{.Names}}")
		out, _ := checkCmd.Output()
		existingName := strings.TrimSpace(string(out))
		if existingName == app.DockerName {
			// 容器存在，启动它
			cmd = exec.Command("docker", "start", app.DockerName)
		} else {
			// 容器不存在，使用创建命令（需从 app.Command 获取 docker run 完整命令）
			if app.Command == "" {
				return fmt.Errorf("Docker 创建命令为空，无法自动创建容器")
			}
			cmd = GetCommand(app.Command)
		}

	case "nginx":
		logDir = filepath.Join("logs", "nginx")
		if app.NginxPath == "" {
			return fmt.Errorf("Nginx 配置文件路径为空")
		}
		cmd = exec.Command("nginx", "-c", app.NginxPath)

	case "jar_template":
		javaBin, ok := JDKPool[app.JDKKey]
		if !ok {
			javaBin = "java"
		}
		jarDir := filepath.Dir(app.JarPath)
		logDir = filepath.Join(jarDir, "logs")
		cmd = NewCommand(javaBin,
			"-Xmx"+app.Memory,
			"-Dfile.encoding=UTF-8",
			"-jar",
			app.JarPath)

	default:
		// other 或 自定义命令
		if app.Command == "" {
			return fmt.Errorf("自定义命令不能为空")
		}
		workDir, _ := os.Getwd()
		logDir = filepath.Join(workDir, "logs")
		cmd = GetCommand(app.Command)
	}

	// --- 以下所有逻辑完全照搬原件，严禁删改 ---

	// 创建日志目录
	if err := os.MkdirAll(logDir, 0755); err != nil {
		return fmt.Errorf("创建日志目录失败: %v", err)
	}

	// 创建日志文件（按应用ID分文件）
	logFile := filepath.Join(logDir, fmt.Sprintf("app_%d.log", app.ID))
	f, err := os.OpenFile(logFile, os.O_CREATE|os.O_WRONLY|os.O_APPEND, 0644)
	if err != nil {
		return fmt.Errorf("打开日志文件失败: %v", err)
	}

	// 重定向标准输出和标准错误到日志文件
	cmd.Stdout = f
	cmd.Stderr = f

	if err := cmd.Start(); err != nil {
		f.Close()
		return err
	}

	m.ActiveProcs[app.ID] = cmd

	// 存储 PID 到数据库
	if m.db != nil && (app.Type == "jar" || app.Type == "jar_template" || app.Type == "other") {
		if err := database.UpdateAppPID(m.db, app.ID, cmd.Process.Pid); err != nil {
			log.Printf("[Manager] 存储 PID 失败 (ID=%d): %v", app.ID, err)
		}
	}

	// 【核心保留】发送启动成功通知 (严格按原件名)
	notify.SendAppStart(app.ID, app.Name, cmd.Process.Pid)

	// 异步等待进程退出
	go func() {
		err := cmd.Wait()
		f.Close()

		m.mu.Lock()
		delete(m.ActiveProcs, app.ID)
		m.mu.Unlock()

		if err != nil {
			log.Printf("[Manager] 应用(ID:%d)异常退出: %v", app.ID, err)
			// 【核心保留】发送崩溃通知
			notify.SendAppCrash(app.ID, app.Name)
		} else {
			log.Printf("[Manager] 应用(ID:%d)正常退出", app.ID)
			// 【核心保留】发送停止通知
			notify.SendAppStop(app.ID, app.Name)
		}
	}()
	// 在 cmd.Start() 成功后，添加守护注册
	if app.IsDaemon && m.Guard != nil {
		if err := m.Guard.EnableGuard(app, nil); err != nil {
			log.Printf("[Manager] 开启守护失败 (ID=%d): %v", app.ID, err)
		} else {
			log.Printf("[Manager] 守护已开启 (ID=%d)", app.ID)
		}
	}
	log.Printf("[Manager] 应用(ID:%d, %s)已启动，PID: %d，日志文件: %s",
		app.ID, app.Name, cmd.Process.Pid, logFile)
	return nil
}

// StopApp 停止应用
func (m *GlobalManager) StopApp(appID int) error {
	m.mu.Lock()
	defer m.mu.Unlock()

	// 首先检查进程中是否有该应用（普通进程类型）
	if cmd, exists := m.ActiveProcs[appID]; exists {
		// 使用跨平台的终止函数
		pid := cmd.Process.Pid
		if err := KillProcess(pid); err != nil {
			return fmt.Errorf("停止进程失败: %v", err)
		}
		delete(m.ActiveProcs, appID)
		log.Printf("[Manager] 应用(ID:%d)已停止（kill）", appID)
		// 清除 PID
		if m.db != nil {
			if err := database.ClearAppPID(m.db, appID); err != nil {
				log.Printf("[Manager] 清除 PID 失败 (ID=%d): %v", appID, err)
			}
		}
		return nil
	}

	// 如果不在 ActiveProcs 中，可能是 Docker 或 Nginx 类型，需要根据数据库配置停止
	// 获取应用信息
	if m.getAppByIDFunc == nil {
		return fmt.Errorf("无法获取应用信息，回调未设置")
	}
	app, err := m.getAppByIDFunc(appID)
	if err != nil {
		return fmt.Errorf("获取应用信息失败: %v", err)
	}

	// 根据类型执行停止命令
	switch app.Type {
	case "docker":
		if app.DockerName == "" {
			return fmt.Errorf("Docker 容器名称为空")
		}
		stopCmd := exec.Command("docker", "stop", app.DockerName)
		if output, err := stopCmd.CombinedOutput(); err != nil {
			return fmt.Errorf("docker stop 失败: %v, 输出: %s", err, output)
		}
		log.Printf("[Manager] Docker 容器 %s 已停止", app.DockerName)

	case "nginx":
		// 通过 nginx -s stop 停止，需要知道配置文件路径
		if app.NginxPath == "" {
			// 尝试使用 pkill
			pkillCmd := exec.Command("pkill", "-f", "nginx -c "+app.NginxPath)
			if err := pkillCmd.Run(); err != nil {
				return fmt.Errorf("停止 Nginx 失败: %v", err)
			}
		} else {
			stopCmd := exec.Command("nginx", "-s", "stop", "-c", app.NginxPath)
			if err := stopCmd.Run(); err != nil {
				// 降级使用 pkill
				pkillCmd := exec.Command("pkill", "-f", "nginx -c "+app.NginxPath)
				if err := pkillCmd.Run(); err != nil {
					return fmt.Errorf("停止 Nginx 失败: %v", err)
				}
			}
		}
		log.Printf("[Manager] Nginx 应用 %s 已停止", app.Name)

	case "jar", "jar_template", "other":
		// 对于这些类型，如果进程不在 ActiveProcs 中，可能已经退出，返回错误
		return fmt.Errorf("应用(ID:%d)未在运行（无活动进程）", appID)

	default:
		// 未知类型，尝试 kill 进程（如果有PID文件或其他方式），简单返回错误
		return fmt.Errorf("不支持停止类型: %s", app.Type)
	}

	// 注意：对于 Docker 和 Nginx，停止后不需要从 ActiveProcs 删除（因为本来就不在里面）
	return nil
}

// RestoreAppsState 系统启动时恢复所有正在运行的应用状态
func (m *GlobalManager) RestoreAppsState() error {
	if m.db == nil {
		return fmt.Errorf("数据库连接未设置")
	}

	apps, err := database.GetAppsWithValidPID(m.db)
	if err != nil {
		return err
	}

	for _, app := range apps {
		if app.PID <= 0 {
			continue
		}

		// 使用跨平台的进程存活检测
		if !IsProcessAlive(app.PID) {
			log.Printf("[Restore] 应用 %d 进程不存在 (PID=%d)，清除记录", app.ID, app.PID)
			_ = database.ClearAppPID(m.db, app.ID)
			continue
		}

		// 进程存在，重建 exec.Cmd 对象
		cmd := &exec.Cmd{
			Process: &os.Process{Pid: app.PID},
		}
		m.mu.Lock()
		m.ActiveProcs[app.ID] = cmd
		m.mu.Unlock()

		log.Printf("[Restore] 恢复应用 %d (%s) 运行状态，PID=%d", app.ID, app.Name, app.PID)

		// 恢复守护（如果开启了 auto_restart）
		fullApp, _ := m.GetAppByID(app.ID)
		if fullApp != nil && fullApp.AutoRestart && m.Guard != nil {
			if err := m.Guard.EnableGuard(fullApp, nil); err != nil {
				log.Printf("[Restore] 恢复守护失败 (ID=%d): %v", app.ID, err)
			} else {
				log.Printf("[Restore] 恢复守护成功 (ID=%d)", app.ID)
			}
		}
	}
	return nil
}

// GetLogFilePath 获取应用日志文件路径
func (m *GlobalManager) GetLogFilePath(app *model.App) string {
	if app.Type == "jar" && app.JarPath != "" {
		jarDir := filepath.Dir(app.JarPath)
		return filepath.Join(jarDir, "logs", fmt.Sprintf("app_%d.log", app.ID))
	}
	workDir, _ := os.Getwd()
	return filepath.Join(workDir, "logs", fmt.Sprintf("app_%d.log", app.ID))
}

// ========== 守护相关方法 ==========

// EnableGuard 为应用开启守护
func (m *GlobalManager) EnableGuard(app *model.App, policy *guard.Policy) error {
	if m.Guard == nil {
		return fmt.Errorf("守护器未初始化")
	}
	return m.Guard.EnableGuard(app, policy)
}

// DisableGuard 关闭应用守护
func (m *GlobalManager) DisableGuard(appID int) error {
	if m.Guard == nil {
		return fmt.Errorf("守护器未初始化")
	}
	return m.Guard.DisableGuard(appID)
}

// IsGuarded 检查应用是否被守护
func (m *GlobalManager) IsGuarded(appID int) bool {
	if m.Guard == nil {
		return false
	}
	return m.Guard.IsGuarded(appID)
}

// GetGuardInfo 获取守护信息
func (m *GlobalManager) GetGuardInfo(appID int) *guard.GuardedApp {
	if m.Guard == nil {
		return nil
	}
	return m.Guard.GetGuardInfo(appID)
}

// CreateLogStream 为应用创建日志流
func (m *GlobalManager) CreateLogStream(appID int, logFilePath string) string {
	return GetLogStreamManager().CreateLogStream(appID, logFilePath)
}

// CloseLogStream 关闭日志流
func (m *GlobalManager) CloseLogStream(streamID string) {
	GetLogStreamManager().CloseStream(streamID)
}
