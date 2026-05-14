package manager

import (
	"bufio"
	"fmt"
	"io"
	"log"
	"os"
	"strings"
	"sync"
	"time"

	"github.com/gorilla/websocket"
)

// LogStreamManager 日志流管理器
type LogStreamManager struct {
	streams map[string]*LogStream
	mu      sync.RWMutex
}

// LogStream 单个日志流
type LogStream struct {
	ID      string
	AppID   int
	LogFile string
	Cancel  chan struct{}
	Closed  bool
	mu      sync.Mutex
	clients map[*websocket.Conn]bool
}

// LogMessage 日志消息格式
type LogMessage struct {
	Time    string `json:"time"`
	Content string `json:"content"`
	Tag     string `json:"tag"`
	Type    string `json:"type"` // info, success, warning, error
}

var logStreamMgr = &LogStreamManager{
	streams: make(map[string]*LogStream),
}

// GetLogStreamManager 获取日志流管理器实例
func GetLogStreamManager() *LogStreamManager {
	return logStreamMgr
}

// CreateLogStream 创建日志流
func (m *LogStreamManager) CreateLogStream(appID int, logFilePath string) string {
	m.mu.Lock()
	defer m.mu.Unlock()

	streamID := fmt.Sprintf("app_%d_%d", appID, time.Now().UnixNano())
	stream := &LogStream{
		ID:      streamID,
		AppID:   appID,
		LogFile: logFilePath,
		Cancel:  make(chan struct{}),
		clients: make(map[*websocket.Conn]bool),
	}

	m.streams[streamID] = stream

	// 启动日志监听协程
	go stream.watchLogFile()

	return streamID
}

// GetStream 获取日志流
func (m *LogStreamManager) GetStream(streamID string) *LogStream {
	m.mu.RLock()
	defer m.mu.RUnlock()
	return m.streams[streamID]
}

// CloseStream 关闭日志流
func (m *LogStreamManager) CloseStream(streamID string) {
	m.mu.Lock()
	defer m.mu.Unlock()

	if stream, exists := m.streams[streamID]; exists {
		stream.Close()
		delete(m.streams, streamID)
	}
}

// AddClient 添加 WebSocket 客户端
func (s *LogStream) AddClient(conn *websocket.Conn) {
	s.mu.Lock()
	defer s.mu.Unlock()
	s.clients[conn] = true
}

// RemoveClient 移除 WebSocket 客户端
func (s *LogStream) RemoveClient(conn *websocket.Conn) {
	s.mu.Lock()
	delete(s.clients, conn)
	count := len(s.clients)
	s.mu.Unlock()
	log.Println("开始移除 WebSocket 客户端")
	if count == 0 {

		log.Println("没有客户端了，关闭整个流")
		// 没有客户端了，关闭整个流（会关闭 Cancel 通道，让 watchLogFile 退出）
		s.Close()
		// 并从管理器中删除
		GetLogStreamManager().CloseStream(s.ID)
	}
}

// broadcast 广播消息给所有客户端
func (s *LogStream) broadcast(msg LogMessage) {
	s.mu.Lock()
	clients := make([]*websocket.Conn, 0, len(s.clients))
	for conn := range s.clients {
		clients = append(clients, conn)
	}
	s.mu.Unlock()

	for _, conn := range clients {
		// 使用互斥锁保护每个连接的写入
		connMutex := sync.Mutex{} // 需要在 LogStream 中为每个连接维护锁
		connMutex.Lock()
		err := conn.WriteJSON(msg)
		connMutex.Unlock()
		if err != nil {
			conn.Close()
			s.RemoveClient(conn)
		}
	}
}

// Close 关闭日志流
func (s *LogStream) Close() {
	s.mu.Lock()
	defer s.mu.Unlock()
	if s.Closed {
		return
	}
	s.Closed = true
	close(s.Cancel)

	// 关闭所有客户端连接
	for conn := range s.clients {
		conn.Close()
	}
	s.clients = make(map[*websocket.Conn]bool)
}

// watchLogFile 监听日志文件变化
func (s *LogStream) watchLogFile() {
	file, err := os.Open(s.LogFile)
	if err != nil {
		s.broadcast(LogMessage{
			Time:    time.Now().Format("15:04:05"),
			Content: fmt.Sprintf("无法打开日志文件: %v", err),
			Tag:     "ERROR",
			Type:    "error",
		})
		return
	}
	defer file.Close()

	// 移动到文件末尾
	file.Seek(0, io.SeekEnd)

	reader := bufio.NewReader(file)

	for {
		select {
		case <-s.Cancel:
			return
		default:
			// 读取新行
			line, err := reader.ReadString('\n')
			if err != nil {
				if err == io.EOF {
					// 没有新内容，等待一会儿
					time.Sleep(500 * time.Millisecond)
					continue
				}
				// 其他错误，可能是文件被重新创建，尝试重新打开
				time.Sleep(1 * time.Second)
				newFile, err := os.Open(s.LogFile)
				if err == nil {
					file.Close()
					file = newFile
					file.Seek(0, io.SeekEnd)
					reader = bufio.NewReader(file)
				}
				continue
			}

			// 发送日志消息
			s.broadcast(LogMessage{
				Time:    time.Now().Format("15:04:05"),
				Content: strings.TrimSpace(line),
				Tag:     "LOG",
				Type:    "info",
			})
		}
	}
}

// GetOrCreateStream 获取或创建日志流
func (m *LogStreamManager) GetOrCreateStream(streamID string, appID int, logFilePath string) *LogStream {
	m.mu.Lock()
	defer m.mu.Unlock()

	if stream, exists := m.streams[streamID]; exists {
		return stream
	}

	stream := &LogStream{
		ID:      streamID,
		AppID:   appID,
		LogFile: logFilePath,
		Cancel:  make(chan struct{}),
		clients: make(map[*websocket.Conn]bool),
	}

	m.streams[streamID] = stream
	go stream.watchLogFile()

	return stream
}
