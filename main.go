package main

import (
	"context"
	"database/sql"
	"encoding/json"
	"fmt"
	"io"
	"log"
	"net/http"
	"os"
	"path/filepath"
	"strconv"
	"strings"
	"tatai/internal/auth"
	"tatai/internal/config"
	"tatai/internal/database"
	"tatai/internal/guard"
	"tatai/internal/manager"
	"tatai/internal/model"
	"tatai/internal/monitor"
	"tatai/internal/notify"
	"time"

	"github.com/go-chi/chi/v5"
	"github.com/go-chi/chi/v5/middleware"
	"github.com/gorilla/websocket"
	"golang.org/x/crypto/bcrypt"
)

// AppCreateRequest 应用创建请求结构
type AppCreateRequest struct {
	AppName string `json:"appName"`
	Type    string `json:"type"`
	Remark  string `json:"remark"`
	Ports   string `json:"ports"`

	Docker    string `json:"docker"`
	Cmd       string `json:"cmd"`
	NginxConf string `json:"nginxConf"`
	NginxExec string `json:"nginxExec"`
	JdkKey    string `json:"jdkKey"`
	JarPath   string `json:"jarPath"`
	IsDaemon  bool   `json:"isDaemon"`
	Memory    string `json:"memory"`
	StartCmd  string `json:"startCmd"`
	StopCmd   string `json:"stopCmd"`
	CheckCmd  string `json:"checkCmd"`
}

// WebSocket 升级器配置
var upgrader = websocket.Upgrader{
	CheckOrigin: func(r *http.Request) bool {
		return true
	},
}

// 全局变量
var (
	db         *sql.DB
	jwtManager *auth.JWTManager
	cfg        *config.Config
)

func main() {
	var err error
	cfg, err = config.LoadConfig("config.yaml")
	if err != nil {
		log.Printf("加载配置失败: %v，使用默认配置", err)
		cfg = &config.Config{}
	}

	if _, err := os.Stat("data"); os.IsNotExist(err) {
		_ = os.MkdirAll("data", 0755)
	}

	db, err = database.InitDB("./data/tatai.db")
	if err != nil {
		log.Fatalf("初始化数据库失败: %v", err)
	}
	defer db.Close()

	// 初始化JWT管理器
	jwtManager = auth.NewJWTManager(cfg.JWT.Secret, cfg.JWT.ExpireHours)
	log.Println("[Auth] JWT管理器已初始化")

	// 初始化通知模块
	serverIdentifier := cfg.GetServerName()
	if err := notify.Init(db, serverIdentifier); err != nil {
		fmt.Printf("初始化通知模块失败: %v\n", err)
	}

	// 启动监控模块
	monitorCfg := monitor.LoadFromAppConfig(cfg)
	mon := monitor.NewMonitor(monitorCfg)
	mon.Start()
	defer mon.Stop()

	// 初始化 JDKPool
	manager.InitJDKPool(cfg.JDK.Paths)

	// 设置数据库获取回调
	manager.Mgr.SetGetAppByIDFunc(func(appID int) (*model.App, error) {
		var app model.App
		var jdkKey, command, jarPath, memory sql.NullString
		var appType sql.NullString

		err := db.QueryRow("SELECT id, name, command, type, jdk_key, jar_path, memory FROM apps WHERE id = ?", appID).
			Scan(&app.ID, &app.Name, &command, &appType, &jdkKey, &jarPath, &memory)
		if err != nil {
			return nil, err
		}
		app.Command = command.String
		app.Type = appType.String
		app.JDKKey = jdkKey.String
		app.JarPath = jarPath.String
		app.Memory = memory.String

		return &app, nil
	})

	// 初始化管理器
	manager.InitManager(db)
	if err := manager.Mgr.RestoreAppsState(); err != nil {
		log.Printf("恢复应用状态失败: %v", err)
	}
	defer manager.Mgr.Guard.Stop()

	// 创建路由
	r := chi.NewRouter()

	// 使用 chi 官方中间件
	r.Use(middleware.Logger)
	r.Use(middleware.Recoverer)

	// WebSocket 路由（官方中间件支持 Hijack，可正常注册）
	r.Get("/api/logs/stream", handleWebSocketLogs)

	// ==================== 认证相关API（无需认证） ====================
	r.Post("/api/auth/login", handleLogin)
	r.Post("/api/auth/logout", handleLogout)

	// ==================== 需要认证的API路由组 ====================
	r.Group(func(r chi.Router) {
		r.Use(authMiddleware) // 使用认证中间件

		r.Get("/api/auth/userinfo", handleUserInfo)
		r.Get("/api/apps", handleGetApps)
		r.Post("/api/apps", handleCreateApp)
		r.Post("/api/apps/reorder", handleAppsReorder)
		r.Post("/api/apps/scan", handleAppsScan)
		r.Put("/api/apps/{id}", handleUpdateApp)
		r.Delete("/api/apps/{id}", handleDeleteApp)
		r.Post("/api/apps/{id}/start", handleStartApp)
		r.Post("/api/apps/{id}/stop", handleStopApp)
		r.Post("/api/apps/{id}/refresh-ports", handleRefreshPorts)
		r.Get("/api/apps/{id}/logs", handleGetLogs)

		r.Get("/api/jdk/list", handleJDKList)
		r.Get("/api/sys/stats", handleSysStats)
		r.Get("/api/sys/top-processes", handleTopProcesses)
		r.Get("/api/sys/disk-suggestions", handleDiskSuggestions)
		r.Get("/api/sys/webhook", handleGetWebhook)
		r.Post("/api/sys/webhook", handleSaveWebhook)
		r.Post("/api/test/webhook", handleTestWebhook)
		r.Post("/api/sys/clear-file", handleClearFile)

		r.Post("/api/guard/{id}/enable", handleGuardEnable)
		r.Post("/api/guard/{id}/disable", handleGuardDisable)
		r.Get("/api/guard/{id}/status", handleGuardStatus)
		r.Get("/api/guard/list", handleGuardList)
		r.Get("/api/apps/summary", handleGetSummary)

		r.Get("/api/users", handleGetUsers)                               // 获取用户列表
		r.Post("/api/users", handleCreateUser)                            // 创建用户
		r.Put("/api/users/{id}/status", handleUpdateUserStatus)           // 修改用户状态
		r.Put("/api/users/password", handleUpdateSelfPassword)            // 当前用户修改自己的密码
		r.Put("/api/admin/users/{id}/password", handleAdminResetPassword) // 管理员重置他人密码
	})

	// 静态文件托管（必须放在最后）
	workDir, _ := os.Getwd()
	distPath := filepath.Join(workDir, "dist")
	r.Handle("/*", http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if strings.HasPrefix(r.URL.Path, "/api") {
			http.NotFound(w, r)
			return
		}
		target := filepath.Join(distPath, r.URL.Path)
		if info, err := os.Stat(target); err != nil || info.IsDir() {
			http.ServeFile(w, r, filepath.Join(distPath, "index.html"))
			return
		}
		http.FileServer(http.Dir(distPath)).ServeHTTP(w, r)
	}))

	addr := fmt.Sprintf("%s:%d", cfg.Server.Address, cfg.Server.Port)
	log.Printf("Tatai Backend 运行中: http://%s", addr)
	log.Fatal(http.ListenAndServe(addr, r))
}

// ==================== 认证中间件 ====================
func authMiddleware(next http.Handler) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		authHeader := r.Header.Get("Authorization")
		if authHeader == "" {
			http.Error(w, `{"error":"未提供认证令牌"}`, http.StatusUnauthorized)
			return
		}

		parts := strings.SplitN(authHeader, " ", 2)
		if len(parts) != 2 || strings.ToLower(parts[0]) != "bearer" {
			http.Error(w, `{"error":"无效的认证格式"}`, http.StatusUnauthorized)
			return
		}

		tokenString := parts[1]
		claims, err := jwtManager.Verify(tokenString)
		if err != nil {
			if err == auth.ErrExpiredToken {
				http.Error(w, `{"error":"认证令牌已过期"}`, http.StatusUnauthorized)
			} else {
				http.Error(w, `{"error":"无效的认证令牌"}`, http.StatusUnauthorized)
			}
			return
		}

		// 将用户信息存入请求上下文
		ctx := context.WithValue(r.Context(), "user", claims)
		next.ServeHTTP(w, r.WithContext(ctx))
	})
}

func getUserFromContext(r *http.Request) *auth.Claims {
	if claims, ok := r.Context().Value("user").(*auth.Claims); ok {
		return claims
	}
	return nil
}

// ==================== 认证处理函数 ====================

func handleLogin(w http.ResponseWriter, r *http.Request) {
	var req model.LoginRequest
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		http.Error(w, `{"error":"参数错误"}`, http.StatusBadRequest)
		return
	}

	if req.Username == "" || req.Password == "" {
		http.Error(w, `{"error":"用户名和密码不能为空"}`, http.StatusBadRequest)
		return
	}

	user, err := database.GetUserByUsername(db, req.Username)
	if err != nil {
		log.Printf("[Auth] 查询用户失败: %v", err)
		http.Error(w, `{"error":"系统错误"}`, http.StatusInternalServerError)
		return
	}

	if user == nil {
		http.Error(w, `{"error":"用户名或密码错误"}`, http.StatusUnauthorized)
		return
	}

	if user.Status != 1 {
		http.Error(w, `{"error":"账号已被禁用"}`, http.StatusForbidden)
		return
	}

	if !database.VerifyPassword(user.Password, req.Password) {
		http.Error(w, `{"error":"用户名或密码错误"}`, http.StatusUnauthorized)
		return
	}

	token, err := jwtManager.Generate(user.ID, user.Username, user.Role)
	if err != nil {
		log.Printf("[Auth] 生成令牌失败: %v", err)
		http.Error(w, `{"error":"登录失败"}`, http.StatusInternalServerError)
		return
	}

	_ = database.UpdateLastLogin(db, user.ID)

	response := model.LoginResponse{
		Token:    token,
		Username: user.Username,
		Role:     user.Role,
	}

	w.Header().Set("Content-Type", "application/json")
	json.NewEncoder(w).Encode(response)
}

func handleLogout(w http.ResponseWriter, r *http.Request) {
	w.Header().Set("Content-Type", "application/json")
	json.NewEncoder(w).Encode(map[string]interface{}{
		"message": "退出成功",
	})
}

func handleUserInfo(w http.ResponseWriter, r *http.Request) {
	claims := getUserFromContext(r)
	if claims == nil {
		http.Error(w, `{"error":"未认证"}`, http.StatusUnauthorized)
		return
	}

	user, err := database.GetUserByID(db, claims.UserID)
	if err != nil || user == nil {
		http.Error(w, `{"error":"用户不存在"}`, http.StatusUnauthorized)
		return
	}

	response := model.UserInfoResponse{
		ID:       user.ID,
		Username: user.Username,
		Role:     user.Role,
	}

	w.Header().Set("Content-Type", "application/json")
	json.NewEncoder(w).Encode(response)
}

// ==================== 以下函数需要根据您的原有代码实现 ====================

func handleGetApps(w http.ResponseWriter, r *http.Request) {
	appType := r.URL.Query().Get("type")
	statusFilter := r.URL.Query().Get("status")
	keyword := r.URL.Query().Get("query")

	apps, err := database.GetAllApps(db, appType, keyword)
	if err != nil {
		http.Error(w, err.Error(), 500)
		return
	}

	// 构建响应，增加实际端口和匹配状态
	type AppWithPorts struct {
		model.App
		ActualPorts      []int  `json:"actual_ports,omitempty"`
		PortsMatchStatus string `json:"ports_match_status,omitempty"`
	}

	result := make([]AppWithPorts, 0)
	for _, app := range apps {
		// 运行状态
		if manager.Mgr.IsRunning(app.ID) {
			app.Status = "running"
		} else {
			app.Status = "stopped"
		}

		// 状态筛选
		if statusFilter != "" && statusFilter != "null" && app.Status != statusFilter {
			continue
		}

		resp := AppWithPorts{App: app}

		// 仅对 jar / other 且运行中的应用进行端口校验
		if (app.Type == "jar" || app.Type == "other") && app.Status == "running" {
			if pid, ok := manager.Mgr.GetPID(app.ID); ok {
				actual, err := manager.GetProcessPorts(pid)
				if err == nil {
					resp.ActualPorts = actual
					// 解析期望端口
					var expected []int
					if app.Ports != "" && app.Ports != "[]" {
						json.Unmarshal([]byte(app.Ports), &expected)
					}
					// 比较
					log.Printf("期望端口: %v", expected)
					log.Printf("实际端口: %v", actual)
					if matchPorts(expected, actual) {
						resp.PortsMatchStatus = "matched"
					} else {
						resp.PortsMatchStatus = "mismatch"
					}
				} else {
					resp.PortsMatchStatus = "unknown"
				}
			} else {
				resp.PortsMatchStatus = "unknown"
			}
		}

		result = append(result, resp)
	}

	w.Header().Set("Content-Type", "application/json")
	json.NewEncoder(w).Encode(result)
}

func handleCreateApp(w http.ResponseWriter, r *http.Request) {
	var req AppCreateRequest
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		http.Error(w, "无效的请求参数: "+err.Error(), 400)
		return
	}

	if req.Type == "" {
		http.Error(w, "应用类型(type)不能为空", 400)
		return
	}
	// TODO Docker、Nginx、Other 类型暂不开放，返回开发中提示
	if req.Type == "docker" || req.Type == "nginx" || req.Type == "other" {
		w.Header().Set("Content-Type", "application/json")
		json.NewEncoder(w).Encode(map[string]interface{}{
			"error":   "feature_not_ready",
			"message": "In the process of feature development, please stay tuned",
		})
		return
	}

	app := model.App{
		Type:   req.Type,
		Remark: req.Remark,
		Name:   generateAppName(req),
	}

	// 根据类型处理特定字段和 ports
	portsJSON := "[]"
	if req.Type == "jar" || req.Type == "other" {
		if req.Ports != "" {
			portsJSON = req.Ports
		}
	}
	app.Ports = portsJSON

	switch req.Type {
	case "docker":
		if req.AppName == "" {
			http.Error(w, "Docker 容器名称不能为空", 400)
			return
		}
		if req.Cmd == "" {
			http.Error(w, "Docker 运行命令不能为空", 400)
			return
		}
		app.DockerName = req.AppName
		app.Command = req.Cmd
		app.Name = req.AppName

	case "nginx":
		if req.NginxConf == "" {
			http.Error(w, "Nginx 配置文件路径不能为空", 400)
			return
		}
		app.NginxPath = req.NginxConf
		app.Command = req.NginxExec
		if app.Command == "" {
			app.Command = "nginx"
		}
		app.Name = extractNameFromPath(req.NginxConf, "nginx")

	case "jar":
		if req.JdkKey == "" {
			http.Error(w, "JDK 未指定", 400)
			return
		}
		if req.JarPath == "" {
			http.Error(w, "JAR 文件路径不能为空", 400)
			return
		}
		jdkPath := getJDKPathByKey(req.JdkKey)
		if jdkPath == "" {
			http.Error(w, "未找到对应的 JDK 配置", 400)
			return
		}
		app.JDKKey = req.JdkKey
		app.JarPath = jdkPath
		app.IsDaemon = req.IsDaemon
		if req.Memory == "" {
			app.Memory = "512m"
		} else {
			app.Memory = req.Memory
		}
		app.Name = req.AppName

	case "other":
		if req.StartCmd == "" {
			http.Error(w, "启动命令(startCmd)不能为空", 400)
			return
		}
		app.Command = req.StartCmd
		app.StopCmd = req.StopCmd
		app.CheckCmd = req.CheckCmd
		if app.Name == "" {
			app.Name = "custom-app"
		}

	default:
		http.Error(w, "不支持的应用类型: "+req.Type, 400)
		return
	}

	id, err := database.AddApp(db, app)
	if err != nil {
		log.Printf("[API] 保存应用失败: %v", err)
		http.Error(w, "保存应用失败: "+err.Error(), 500)
		return
	}
	app.ID = int(id)

	log.Printf("[API] 应用注册成功: ID=%d, Name=%s, Type=%s", id, app.Name, app.Type)

	// 异步启动
	go func() {
		time.Sleep(500 * time.Millisecond)
		if err := manager.Mgr.StartApp(&app); err != nil {
			log.Printf("[API] 自动启动应用失败 (ID=%d): %v", id, err)
		}
	}()

	response := map[string]interface{}{
		"id":      id,
		"message": "应用创建成功",
		"name":    app.Name,
	}
	w.Header().Set("Content-Type", "application/json")
	json.NewEncoder(w).Encode(response)
}

func handleAppsReorder(w http.ResponseWriter, r *http.Request) {
	// 应用排序逻辑
	var request struct {
		IDs []int `json:"ids"`
	}

	if err := json.NewDecoder(r.Body).Decode(&request); err != nil {
		http.Error(w, "参数错误", 400)
		return
	}

	if err := database.UpdateAppsOrder(db, request.IDs); err != nil {
		http.Error(w, err.Error(), 500)
		return
	}

	w.Header().Set("Content-Type", "application/json")
	json.NewEncoder(w).Encode(map[string]interface{}{"message": "success"})
}

func handleAppsScan(w http.ResponseWriter, r *http.Request) {
	var req struct {
		Type    string `json:"type"`
		Keyword string `json:"keyword"` // 容器名或进程关键字
	}
	json.NewDecoder(r.Body).Decode(&req)

	// TODO: 实现 manager.ScanProcess(type, keyword)
	// 逻辑：通过 pgrep 或 docker inspect 检查进程是否存在

	w.Header().Set("Content-Type", "application/json")
	json.NewEncoder(w).Encode(map[string]interface{}{
		"exists": false,
		"msg":    "TODO: 实现进程扫描逻辑",
	})
}

func handleUpdateApp(w http.ResponseWriter, r *http.Request) {
	id, err := strconv.Atoi(chi.URLParam(r, "id"))
	if err != nil {
		http.Error(w, "无效的应用ID", 400)
		return
	}

	var req AppCreateRequest
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		http.Error(w, "无效的请求参数: "+err.Error(), 400)
		return
	}

	if req.Type == "" {
		http.Error(w, "应用类型(type)不能为空", 400)
		return
	}

	// 构建 model.App 对象（复用部分创建逻辑）
	app := model.App{
		ID:     id,
		Type:   req.Type,
		Remark: req.Remark,
		Name:   req.AppName,
		Ports:  "[]",
	}

	if req.Type == "jar" || req.Type == "other" {
		if req.Ports != "" {
			app.Ports = req.Ports
		}
	}

	switch req.Type {
	case "docker":
		if req.AppName == "" {
			http.Error(w, "Docker 容器名称不能为空", 400)
			return
		}
		app.DockerName = req.AppName
		app.Command = req.Cmd

	case "nginx":
		if req.NginxConf == "" {
			http.Error(w, "Nginx 配置文件路径不能为空", 400)
			return
		}
		app.NginxPath = req.NginxConf
		app.Command = req.NginxExec
		if app.Command == "" {
			app.Command = "nginx"
		}

	case "jar":
		if req.JdkKey == "" || req.JarPath == "" {
			http.Error(w, "JDK 未指定", 400)
			return
		}
		jdkPath := getJDKPathByKey(req.JdkKey)
		if jdkPath == "" {
			http.Error(w, "未找到对应的 JDK 配置", 400)
			return
		}
		app.JDKKey = req.JdkKey
		app.JarPath = jdkPath
		app.IsDaemon = req.IsDaemon
		if req.Memory == "" {
			app.Memory = "512m"
		} else {
			app.Memory = req.Memory
		}

	case "other":
		if req.StartCmd == "" {
			http.Error(w, "启动命令(startCmd)不能为空", 400)
			return
		}
		app.Command = req.StartCmd
		app.StopCmd = req.StopCmd
		app.CheckCmd = req.CheckCmd
		if app.Name == "" {
			app.Name = "custom-app"
		}
	}

	// 调用数据库更新
	if err := database.UpdateApp(db, app); err != nil {
		log.Printf("[API] 更新应用失败: %v", err)
		http.Error(w, "更新失败: "+err.Error(), 500)
		return
	}

	w.Header().Set("Content-Type", "application/json")
	json.NewEncoder(w).Encode(map[string]interface{}{
		"message": "应用更新成功",
		"id":      id,
	})
}

func handleDeleteApp(w http.ResponseWriter, r *http.Request) {
	idStr := chi.URLParam(r, "id")
	id, err := strconv.Atoi(idStr)
	if err != nil {
		http.Error(w, "Invalid application ID", 400)
		return
	}

	// 检查应用是否正在运行
	if manager.Mgr.IsRunning(id) {
		http.Error(w, "Please stop the application first", 400)
		return
	}

	// 如果开启了守护，先关闭守护
	if manager.Mgr.IsGuarded(id) {
		_ = manager.Mgr.DisableGuard(id)
	}

	// 从数据库中删除
	if err := database.DeleteApp(db, id); err != nil {
		http.Error(w, "Deletion failed: "+err.Error(), 500)
		return
	}

	w.Header().Set("Content-Type", "application/json")
	json.NewEncoder(w).Encode(map[string]interface{}{
		"message": "Application deleted successfully",
		"id":      id,
	})
}

func handleStartApp(w http.ResponseWriter, r *http.Request) {
	id, err := strconv.Atoi(chi.URLParam(r, "id"))
	if err != nil {
		http.Error(w, "Invalid application ID", 400)
		return
	}

	// 从数据库获取应用完整信息
	var app model.App
	var jdkKey, command, jarPath, memory sql.NullString
	var appType, dockerName, nginxPath, remark sql.NullString
	var autoRestartInt int

	err = db.QueryRow(`
        SELECT id, name, command, auto_restart, type, jdk_key, jar_path, memory,
               docker_name, nginx_path, remark
        FROM apps WHERE id = ?
    `, id).Scan(&app.ID, &app.Name, &command, &autoRestartInt, &appType, &jdkKey, &jarPath, &memory,
		&dockerName, &nginxPath, &remark)
	if err != nil {
		http.Error(w, "应用不存在", 404)
		return
	}
	app.Command = command.String
	app.AutoRestart = autoRestartInt == 1
	app.Type = appType.String
	app.JDKKey = jdkKey.String
	app.JarPath = jarPath.String
	app.Memory = memory.String
	app.DockerName = dockerName.String
	app.NginxPath = nginxPath.String
	app.Remark = remark.String

	// 调用 manager 启动
	if err := manager.Mgr.StartApp(&app); err != nil {
		http.Error(w, "启动失败: "+err.Error(), 500)
		return
	}
	log.Println("启动应用成功")
	w.Header().Set("Content-Type", "application/json")
	json.NewEncoder(w).Encode(map[string]interface{}{"message": "started"})
}

func handleStopApp(w http.ResponseWriter, r *http.Request) {
	id, _ := strconv.Atoi(chi.URLParam(r, "id"))
	if err := manager.Mgr.StopApp(id); err != nil {
		http.Error(w, err.Error(), 500)
		return
	}

	log.Println("停止应用成功")
	w.Write([]byte(`{"message":"stopped"}`))
}

func handleRefreshPorts(w http.ResponseWriter, r *http.Request) {
	id, err := strconv.Atoi(chi.URLParam(r, "id"))
	if err != nil {
		http.Error(w, "无效的应用ID", 400)
		return
	}

	// 查询应用信息
	var app model.App
	var jdkKey, command, jarPath, memory, portsSQL sql.NullString
	var appType, dockerName, nginxPath, remark sql.NullString
	var autoRestartInt int

	err = db.QueryRow(`
		SELECT id, name, command, auto_restart, type, jdk_key, jar_path, memory,
			   docker_name, nginx_path, remark, ports
		FROM apps WHERE id = ?
	`, id).Scan(&app.ID, &app.Name, &command, &autoRestartInt, &appType, &jdkKey, &jarPath, &memory,
		&dockerName, &nginxPath, &remark, &portsSQL)
	if err != nil {
		http.Error(w, "应用不存在", 404)
		return
	}
	app.Command = command.String
	app.AutoRestart = autoRestartInt == 1
	app.Type = appType.String
	app.JDKKey = jdkKey.String
	app.JarPath = jarPath.String
	app.Memory = memory.String
	app.DockerName = dockerName.String
	app.NginxPath = nginxPath.String
	app.Remark = remark.String
	app.Ports = portsSQL.String
	if app.Ports == "" {
		app.Ports = "[]"
	}

	switch app.Type {
	case "docker":
		ports, err := manager.GetDockerPorts(app.DockerName)
		if err != nil {
			http.Error(w, "获取 Docker 端口失败: "+err.Error(), 500)
			return
		}
		portsJSON, _ := json.Marshal(ports)
		_, err = db.Exec("UPDATE apps SET ports = ? WHERE id = ?", string(portsJSON), id)
		if err != nil {
			http.Error(w, "更新端口失败: "+err.Error(), 500)
			return
		}
		json.NewEncoder(w).Encode(map[string]interface{}{
			"message": "端口已刷新",
			"ports":   ports,
		})

	case "nginx":
		ports, err := manager.GetNginxPorts(app.NginxPath)
		if err != nil {
			http.Error(w, "解析 Nginx 配置失败: "+err.Error(), 500)
			return
		}
		portsJSON, _ := json.Marshal(ports)
		_, err = db.Exec("UPDATE apps SET ports = ? WHERE id = ?", string(portsJSON), id)
		if err != nil {
			http.Error(w, "更新端口失败: "+err.Error(), 500)
			return
		}
		json.NewEncoder(w).Encode(map[string]interface{}{
			"message": "端口已刷新",
			"ports":   ports,
		})

	case "jar", "other":
		pid, ok := manager.Mgr.GetPID(id)
		if !ok {
			http.Error(w, "应用未运行", 400)
			return
		}
		actual, err := manager.GetProcessPorts(pid)
		if err != nil {
			http.Error(w, "获取实际端口失败: "+err.Error(), 500)
			return
		}
		var expected []int
		if app.Ports != "" && app.Ports != "[]" {
			json.Unmarshal([]byte(app.Ports), &expected)
		}
		status := "matched"
		log.Printf("期望端口: %v", expected)
		log.Printf("实际端口: %v", actual)
		if !matchPorts(expected, actual) {
			status = "mismatch"
		}
		json.NewEncoder(w).Encode(map[string]interface{}{
			"actual_ports":   actual,
			"expected_ports": expected,
			"match_status":   status,
		})

	default:
		http.Error(w, "不支持的应用类型", 400)
	}
}

func handleGetLogs(w http.ResponseWriter, r *http.Request) {
	// 1.0 逻辑：通过 offset 轮询文件
	// 2.0 需求：AppCenter 需要更低延迟。
	// TODO: 适配 Docker 日志拉取 (docker logs) 或 Nginx 错误日志

	// 暂时保持原有的 ReadLogByOffset 逻辑
}

func handleJDKList(w http.ResponseWriter, r *http.Request) {
	jdkList := make([]map[string]string, 0)
	for key, path := range manager.JDKPool {
		jdkList = append(jdkList, map[string]string{
			"key":  key,
			"path": path,
		})
	}
	w.Header().Set("Content-Type", "application/json")
	json.NewEncoder(w).Encode(jdkList)
}

func handleSysStats(w http.ResponseWriter, r *http.Request) {
	stats, err := manager.GetSystemStats()
	if err != nil {
		http.Error(w, err.Error(), 500)
		return
	}
	w.Header().Set("Content-Type", "application/json")
	json.NewEncoder(w).Encode(stats)
}

func handleTopProcesses(w http.ResponseWriter, r *http.Request) {
	processType := r.URL.Query().Get("type")
	if processType != "cpu" && processType != "mem" {
		http.Error(w, "参数 type 必须是 cpu 或 mem", 400)
		return
	}

	processes, err := manager.GetTopProcesses(processType)
	if err != nil {
		http.Error(w, "获取进程列表失败: "+err.Error(), 500)
		return
	}

	w.Header().Set("Content-Type", "application/json")
	json.NewEncoder(w).Encode(processes)
}

func handleDiskSuggestions(w http.ResponseWriter, r *http.Request) {
	suggestions, err := manager.GetDiskSuggestions()
	if err != nil {
		http.Error(w, "获取磁盘建议失败: "+err.Error(), 500)
		return
	}

	if suggestions == nil {
		suggestions = []manager.DiskSuggestion{}
	}

	w.Header().Set("Content-Type", "application/json")
	json.NewEncoder(w).Encode(suggestions)
}

func handleGetWebhook(w http.ResponseWriter, r *http.Request) {
	cfg, err := notify.GetWebhookConfig(db)
	if err != nil {
		w.Header().Set("Content-Type", "application/json")
		json.NewEncoder(w).Encode(map[string]interface{}{
			"url":    "",
			"events": []string{},
		})
		return
	}
	w.Header().Set("Content-Type", "application/json")
	json.NewEncoder(w).Encode(cfg)
}

func handleSaveWebhook(w http.ResponseWriter, r *http.Request) {
	var req struct {
		URL    string   `json:"url"`
		Events []string `json:"events"`
	}

	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		http.Error(w, "参数错误: "+err.Error(), 400)
		return
	}

	if req.URL == "" {
		http.Error(w, "Webhook地址不能为空", 400)
		return
	}

	if err := notify.SaveWebhookConfig(db, req.URL, req.Events); err != nil {
		http.Error(w, "保存失败: "+err.Error(), 500)
		return
	}

	w.Header().Set("Content-Type", "application/json")
	json.NewEncoder(w).Encode(map[string]interface{}{
		"message": "保存成功",
	})
}

func handleTestWebhook(w http.ResponseWriter, r *http.Request) {
	var req struct {
		Title   string `json:"title"`
		Content string `json:"content"`
	}
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		http.Error(w, "参数错误", 400)
		return
	}
	notify.SendAlert(notify.AlertMessage{
		EventType: notify.EventAppCrash,
		Title:     req.Title,
		Content:   req.Content,
		Level:     notify.Error,
	})
	w.Header().Set("Content-Type", "application/json")
	json.NewEncoder(w).Encode(map[string]interface{}{"message": "测试通知已发送（异步）"})
}

func handleClearFile(w http.ResponseWriter, r *http.Request) {
	var req struct {
		Path string `json:"path"`
	}
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		http.Error(w, "参数错误", 400)
		return
	}

	if req.Path == "" {
		http.Error(w, "文件路径不能为空", 400)
		return
	}

	// 清空文件内容（不是删除文件）
	file, err := os.OpenFile(req.Path, os.O_TRUNC|os.O_WRONLY, 0644)
	if err != nil {
		http.Error(w, "无法打开文件: "+err.Error(), 500)
		return
	}
	defer file.Close()

	if err := file.Truncate(0); err != nil {
		http.Error(w, "清空文件失败: "+err.Error(), 500)
		return
	}

	w.Header().Set("Content-Type", "application/json")
	json.NewEncoder(w).Encode(map[string]interface{}{
		"message": "文件已清空",
		"path":    req.Path,
	})
}

func handleGuardEnable(w http.ResponseWriter, r *http.Request) {
	id, err := strconv.Atoi(chi.URLParam(r, "id"))
	if err != nil {
		http.Error(w, "无效的应用ID", 400)
		return
	}

	// 获取应用信息
	var app model.App
	var jdkKey, command, jarPath, memory sql.NullString
	var appType sql.NullString

	err = db.QueryRow("SELECT id, name, command, type, jdk_key, jar_path, memory FROM apps WHERE id = ?", id).
		Scan(&app.ID, &app.Name, &command, &appType, &jdkKey, &jarPath, &memory)
	if err != nil {
		http.Error(w, "获取应用失败: "+err.Error(), 404)
		return
	}
	app.Command = command.String
	app.Type = appType.String
	app.JDKKey = jdkKey.String
	app.JarPath = jarPath.String
	app.Memory = memory.String

	// 解析策略参数（可选）
	var req struct {
		MaxRetries     int `json:"max_retries"`
		WindowSeconds  int `json:"window_seconds"`
		BackoffBaseSec int `json:"backoff_base_sec"`
		BackoffMaxSec  int `json:"backoff_max_sec"`
	}

	policy := guard.DefaultPolicy()
	if r.ContentLength > 0 {
		if err := json.NewDecoder(r.Body).Decode(&req); err == nil {
			if req.MaxRetries > 0 {
				policy.MaxRetries = req.MaxRetries
			}
			if req.WindowSeconds > 0 {
				policy.WindowDuration = time.Duration(req.WindowSeconds) * time.Second
			}
			if req.BackoffBaseSec > 0 {
				policy.BackoffBase = time.Duration(req.BackoffBaseSec) * time.Second
			}
			if req.BackoffMaxSec > 0 {
				policy.BackoffMax = time.Duration(req.BackoffMaxSec) * time.Second
			}
			policy.Validate()
		}
	}

	if err := manager.Mgr.EnableGuard(&app, policy); err != nil {
		http.Error(w, err.Error(), 500)
		return
	}

	w.Header().Set("Content-Type", "application/json")
	json.NewEncoder(w).Encode(map[string]interface{}{
		"message": "守护已开启",
		"app_id":  id,
	})
}

func handleGuardDisable(w http.ResponseWriter, r *http.Request) {
	id, err := strconv.Atoi(chi.URLParam(r, "id"))
	if err != nil {
		http.Error(w, "无效的应用ID", 400)
		return
	}

	if err := manager.Mgr.DisableGuard(id); err != nil {
		http.Error(w, err.Error(), 500)
		return
	}

	w.Header().Set("Content-Type", "application/json")
	json.NewEncoder(w).Encode(map[string]interface{}{
		"message": "守护已关闭",
		"app_id":  id,
	})
}

func handleGuardStatus(w http.ResponseWriter, r *http.Request) {
	id, err := strconv.Atoi(chi.URLParam(r, "id"))
	if err != nil {
		http.Error(w, "无效的应用ID", 400)
		return
	}

	isGuarded := manager.Mgr.IsGuarded(id)
	guardInfo := manager.Mgr.GetGuardInfo(id)

	response := map[string]interface{}{
		"app_id":     id,
		"is_guarded": isGuarded,
	}

	if guardInfo != nil {
		// 获取重启历史
		history := guardInfo.GetRestartHistory(10)
		response["restart_history"] = history
		response["policy"] = guardInfo.Policy
	}

	w.Header().Set("Content-Type", "application/json")
	json.NewEncoder(w).Encode(response)
}

func handleGuardList(w http.ResponseWriter, r *http.Request) {
	guardedApps := manager.Mgr.Guard.GetAllGuardedApps()

	result := make([]map[string]interface{}, 0)
	for _, ga := range guardedApps {
		result = append(result, map[string]interface{}{
			"app_id":        ga.App.ID,
			"app_name":      ga.App.Name,
			"policy":        ga.Policy,
			"restart_count": len(ga.GetRestartHistory(100)),
		})
	}

	w.Header().Set("Content-Type", "application/json")
	json.NewEncoder(w).Encode(result)
}

func handleWebSocketLogs(w http.ResponseWriter, r *http.Request) {

	appIdStr := r.URL.Query().Get("appId")
	if appIdStr == "" {
		http.Error(w, "缺少 appId 参数", 400)
		return
	}

	// ==================== 新增：Token 验证 ====================
	tokenStr := r.URL.Query().Get("token")
	if tokenStr == "" {
		// 也支持从 Header 获取（WebSocket 握手时）
		tokenStr = r.Header.Get("Authorization")
		if tokenStr != "" {
			// 去掉 "Bearer " 前缀
			parts := strings.SplitN(tokenStr, " ", 2)
			if len(parts) == 2 && strings.ToLower(parts[0]) == "bearer" {
				tokenStr = parts[1]
			}
		}
	}

	if tokenStr == "" {
		http.Error(w, "未提供认证令牌", http.StatusUnauthorized)
		return
	}

	// 验证 Token
	claims, err := jwtManager.Verify(tokenStr)
	if err != nil {
		log.Printf("[WebSocket] Token验证失败: %v", err)
		http.Error(w, "无效的认证令牌", http.StatusUnauthorized)
		return
	}
	log.Printf("[WebSocket] 用户 %s 请求应用 %s 的日志流", claims.Username, appIdStr)
	// =====================================================

	if appIdStr == "" {
		http.Error(w, "缺少 appId 参数", 400)
		return
	}

	appId, err := strconv.Atoi(appIdStr)
	if err != nil {
		http.Error(w, "无效的 appId 参数", 400)
		return
	}

	// 升级为 WebSocket 连接
	conn, err := upgrader.Upgrade(w, r, nil)
	if err != nil {
		log.Printf("WebSocket 升级失败: %v", err)
		return
	}
	defer conn.Close()

	// 从数据库获取应用完整信息
	var app model.App
	var jdkKey, command, jarPath, memory sql.NullString
	var appType, dockerName, nginxPath, remark sql.NullString
	var autoRestartInt int

	err = db.QueryRow(`
			SELECT id, name, command, auto_restart, type, jdk_key, jar_path, memory,
				   docker_name, nginx_path, remark
			FROM apps WHERE id = ?
		`, appId).Scan(&app.ID, &app.Name, &command, &autoRestartInt, &appType, &jdkKey, &jarPath, &memory,
		&dockerName, &nginxPath, &remark)
	if err != nil {
		conn.WriteMessage(websocket.TextMessage, []byte(`{"error":"应用不存在"}`))
		return
	}
	app.Command = command.String
	app.AutoRestart = autoRestartInt == 1
	app.Type = appType.String
	app.JDKKey = jdkKey.String
	app.JarPath = jarPath.String
	app.Memory = memory.String
	app.DockerName = dockerName.String
	app.NginxPath = nginxPath.String
	app.Remark = remark.String

	// 获取日志文件路径
	logFilePath := manager.Mgr.GetLogFilePath(&app)
	log.Printf("[WebSocket] 日志文件路径: %s", logFilePath)

	// 创建日志流（使用 appId 作为唯一标识）
	streamId := fmt.Sprintf("app_%d", appId)
	stream := manager.GetLogStreamManager().GetOrCreateStream(streamId, appId, logFilePath)

	// 注册客户端
	stream.AddClient(conn)
	defer stream.RemoveClient(conn)

	// 发送历史日志（最近50行）
	history, err := getRecentLogs(logFilePath, 50)
	if err != nil {
		log.Printf("[WebSocket] 读取历史日志失败: %v", err)
		conn.WriteJSON(manager.LogMessage{
			Time:    time.Now().Format("15:04:05"),
			Content: fmt.Sprintf("日志文件不存在或无法读取: %v", err),
			Tag:     "SYSTEM",
			Type:    "warning",
		})
	} else {
		log.Printf("[WebSocket] 读取到 %d 行历史日志", len(history))
		for _, line := range history {
			conn.WriteJSON(manager.LogMessage{
				Time:    time.Now().Format("15:04:05"),
				Content: line,
				Tag:     "HISTORY",
				Type:    "info",
			})
		}
	}

	// 保持连接，直到客户端断开或流关闭
	for {
		select {
		case <-stream.Cancel:
			return
		default:
			conn.SetReadDeadline(time.Now().Add(60 * time.Second))
			if _, _, err := conn.ReadMessage(); err != nil {
				return
			}
		}
	}
}

func handleGetSummary(w http.ResponseWriter, r *http.Request) {
	// 获取所有应用列表
	apps, err := database.GetAllApps(db, "", "")
	if err != nil {
		log.Printf("[API] 获取应用列表失败: %v", err)
		http.Error(w, "获取统计数据失败", 500)
		return
	}

	// 统计各项数据
	var total, running, daemon int

	for _, app := range apps {
		total++

		// 统计运行中进程数（通过 manager 判断实际运行状态）
		if manager.Mgr.IsRunning(app.ID) {
			running++
		}

		// 统计守护进程数（根据 is_daemon 字段，数据库中没有该字段时可用 auto_restart）
		if app.IsDaemon {
			daemon++
		}
	}

	// 异常进程数：当前阶段预留，给 mock 值 0
	abnormal := 0

	summary := map[string]interface{}{
		"total":    total,
		"running":  running,
		"daemon":   daemon,
		"abnormal": abnormal,
	}

	w.Header().Set("Content-Type", "application/json")
	json.NewEncoder(w).Encode(summary)
}

// 辅助函数
func matchPorts(expected, actual []int) bool {
	if len(expected) != len(actual) {
		return false
	}
	m := make(map[int]bool)
	for _, p := range expected {
		m[p] = true
	}
	for _, p := range actual {
		if !m[p] {
			return false
		}
	}
	return true
}

func generateAppName(req AppCreateRequest) string {
	switch req.Type {
	case "docker":
		if req.AppName != "" {
			return req.AppName
		}
		return "docker-app"
	case "nginx":
		if req.NginxConf != "" {
			return filepath.Base(strings.TrimSuffix(req.NginxConf, filepath.Ext(req.NginxConf)))
		}
		return "nginx-server"
	case "jar":
		if req.JarPath != "" {
			return filepath.Base(strings.TrimSuffix(req.JarPath, filepath.Ext(req.JarPath)))
		}
		return "java-application"
	case "other":
		return "custom-application"
	default:
		return "unknown-app"
	}
}

func extractNameFromPath(path, defaultName string) string {
	if path == "" {
		return defaultName
	}
	base := filepath.Base(path)
	if dotIndex := strings.LastIndex(base, "."); dotIndex > 0 {
		base = base[:dotIndex]
	}
	if base == "" {
		return defaultName
	}
	return base
}

func getJDKPathByKey(key string) string {
	if path, ok := manager.JDKPool[key]; ok {
		return path
	}
	return ""
}

// getRecentLogs 获取日志文件最后 N 行
func getRecentLogs(filePath string, lines int) ([]string, error) {
	file, err := os.Open(filePath)
	if err != nil {
		return nil, err
	}
	defer file.Close()

	stat, err := file.Stat()
	if err != nil {
		return nil, err
	}
	fileSize := stat.Size()

	bufferSize := int64(8192)
	if fileSize < bufferSize {
		bufferSize = fileSize
	}

	offset := fileSize - bufferSize
	if offset < 0 {
		offset = 0
	}

	data := make([]byte, bufferSize)
	_, err = file.ReadAt(data, offset)
	if err != nil && err != io.EOF {
		return nil, err
	}

	// 调用 manager 包导出的函数（需要先修改 log_reader.go 导出）
	content := manager.DecodeLogData(data)
	logLines := strings.Split(content, "\n")

	result := make([]string, 0)
	for i := len(logLines) - 1; i >= 0 && len(result) < lines; i-- {
		if strings.TrimSpace(logLines[i]) != "" {
			result = append([]string{logLines[i]}, result...)
		}
	}
	return result, nil
}

// 获取用户列表（仅 admin）
func handleGetUsers(w http.ResponseWriter, r *http.Request) {
	claims := getUserFromContext(r)
	if claims == nil || claims.Username != "admin" {
		http.Error(w, `{"error":"权限不足"}`, http.StatusForbidden)
		return
	}
	users, err := database.GetAllUsers(db)
	if err != nil {
		http.Error(w, `{"error":"获取用户列表失败"}`, http.StatusInternalServerError)
		return
	}
	w.Header().Set("Content-Type", "application/json")
	json.NewEncoder(w).Encode(users)
}

// 创建用户（仅 admin）
func handleCreateUser(w http.ResponseWriter, r *http.Request) {
	claims := getUserFromContext(r)
	if claims == nil || claims.Username != "admin" {
		http.Error(w, `{"error":"权限不足"}`, http.StatusForbidden)
		return
	}
	var req model.CreateUserRequest
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		http.Error(w, `{"error":"参数错误"}`, http.StatusBadRequest)
		return
	}
	if req.Username == "" || req.Password == "" {
		http.Error(w, `{"error":"用户名和密码不能为空"}`, http.StatusBadRequest)
		return
	}
	// 检查用户名是否已存在
	exist, _ := database.GetUserByUsername(db, req.Username)
	if exist != nil {
		http.Error(w, `{"error":"用户名已存在"}`, http.StatusConflict)
		return
	}
	hashed, err := bcrypt.GenerateFromPassword([]byte(req.Password), bcrypt.DefaultCost)
	if err != nil {
		http.Error(w, `{"error":"密码加密失败"}`, http.StatusInternalServerError)
		return
	}
	// 角色默认为 "operator"
	_, err = database.CreateUser(db, req.Username, string(hashed), "operator", 1)
	if err != nil {
		http.Error(w, `{"error":"创建用户失败"}`, http.StatusInternalServerError)
		return
	}
	w.Header().Set("Content-Type", "application/json")
	json.NewEncoder(w).Encode(map[string]interface{}{"message": "用户创建成功"})
}

// 修改用户状态（仅 admin，禁止修改 admin 自己）
func handleUpdateUserStatus(w http.ResponseWriter, r *http.Request) {
	claims := getUserFromContext(r)
	if claims == nil || claims.Username != "admin" {
		http.Error(w, `{"error":"权限不足"}`, http.StatusForbidden)
		return
	}
	idStr := chi.URLParam(r, "id")
	id, err := strconv.Atoi(idStr)
	if err != nil {
		http.Error(w, `{"error":"无效的用户ID"}`, http.StatusBadRequest)
		return
	}
	// 禁止修改 admin 用户
	isAdmin, err := database.IsAdmin(db, id)
	if err != nil {
		http.Error(w, `{"error":"查询用户失败"}`, http.StatusInternalServerError)
		return
	}
	if isAdmin {
		http.Error(w, `{"error":"不能修改 admin 用户的状态"}`, http.StatusForbidden)
		return
	}
	var req model.UpdateUserStatusRequest
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		http.Error(w, `{"error":"参数错误"}`, http.StatusBadRequest)
		return
	}
	if req.Status != 0 && req.Status != 1 {
		http.Error(w, `{"error":"状态值必须为0或1"}`, http.StatusBadRequest)
		return
	}
	if err := database.UpdateUserStatus(db, id, req.Status); err != nil {
		http.Error(w, `{"error":"更新状态失败"}`, http.StatusInternalServerError)
		return
	}
	w.Header().Set("Content-Type", "application/json")
	json.NewEncoder(w).Encode(map[string]interface{}{"message": "状态更新成功"})
}

// 当前用户修改自己的密码
func handleUpdateSelfPassword(w http.ResponseWriter, r *http.Request) {
	claims := getUserFromContext(r)
	if claims == nil {
		http.Error(w, `{"error":"未认证"}`, http.StatusUnauthorized)
		return
	}
	var req model.UpdatePasswordRequest
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		http.Error(w, `{"error":"参数错误"}`, http.StatusBadRequest)
		return
	}
	if req.NewPassword == "" {
		http.Error(w, `{"error":"新密码不能为空"}`, http.StatusBadRequest)
		return
	}
	hashed, err := bcrypt.GenerateFromPassword([]byte(req.NewPassword), bcrypt.DefaultCost)
	if err != nil {
		http.Error(w, `{"error":"密码加密失败"}`, http.StatusInternalServerError)
		return
	}
	if err := database.UpdatePasswordSelf(db, claims.UserID, string(hashed)); err != nil {
		http.Error(w, `{"error":"修改密码失败"}`, http.StatusInternalServerError)
		return
	}
	w.Header().Set("Content-Type", "application/json")
	json.NewEncoder(w).Encode(map[string]interface{}{"message": "密码修改成功"})
}

// 管理员重置他人密码
func handleAdminResetPassword(w http.ResponseWriter, r *http.Request) {
	claims := getUserFromContext(r)
	if claims == nil || claims.Username != "admin" {
		http.Error(w, `{"error":"权限不足"}`, http.StatusForbidden)
		return
	}
	idStr := chi.URLParam(r, "id")
	id, err := strconv.Atoi(idStr)
	if err != nil {
		http.Error(w, `{"error":"无效的用户ID"}`, http.StatusBadRequest)
		return
	}
	var req model.UpdatePasswordRequest
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		http.Error(w, `{"error":"参数错误"}`, http.StatusBadRequest)
		return
	}
	if req.NewPassword == "" {
		http.Error(w, `{"error":"新密码不能为空"}`, http.StatusBadRequest)
		return
	}
	hashed, err := bcrypt.GenerateFromPassword([]byte(req.NewPassword), bcrypt.DefaultCost)
	if err != nil {
		http.Error(w, `{"error":"密码加密失败"}`, http.StatusInternalServerError)
		return
	}
	if err := database.ResetUserPassword(db, id, string(hashed)); err != nil {
		http.Error(w, `{"error":"重置密码失败"}`, http.StatusInternalServerError)
		return
	}
	w.Header().Set("Content-Type", "application/json")
	json.NewEncoder(w).Encode(map[string]interface{}{"message": "密码重置成功"})
}
