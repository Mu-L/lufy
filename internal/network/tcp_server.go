package network

import (
	"context"
	"fmt"
	"net"
	"sync"
	"sync/atomic"
	"time"

	"github.com/phuhao00/lufy/internal/logger"
	"github.com/phuhao00/lufy/internal/pool"
)

// Connection TCP连接
type Connection struct {
	ID           uint64
	Conn         net.Conn
	UserID       uint64
	SessionID    string
	LastActivity time.Time
	closed       int32
	writeMutex   sync.Mutex
	readBuffer   []byte
	writeBuffer  []byte
}

// NewConnection 创建新连接
func NewConnection(id uint64, conn net.Conn) *Connection {
	return &Connection{
		ID:           id,
		Conn:         conn,
		LastActivity: time.Now(),
		readBuffer:   make([]byte, 4096),
		writeBuffer:  make([]byte, 4096),
	}
}

// Write 写入数据
func (c *Connection) Write(data []byte) error {
	if atomic.LoadInt32(&c.closed) == 1 {
		return fmt.Errorf("connection closed")
	}

	c.writeMutex.Lock()
	defer c.writeMutex.Unlock()

	c.LastActivity = time.Now()
	_, err := c.Conn.Write(data)
	return err
}

// Read 读取数据
func (c *Connection) Read(buf []byte) (int, error) {
	if atomic.LoadInt32(&c.closed) == 1 {
		return 0, fmt.Errorf("connection closed")
	}

	c.LastActivity = time.Now()
	return c.Conn.Read(buf)
}

// Close 关闭连接
func (c *Connection) Close() error {
	if !atomic.CompareAndSwapInt32(&c.closed, 0, 1) {
		return nil
	}

	return c.Conn.Close()
}

// IsClosed 检查连接是否已关闭
func (c *Connection) IsClosed() bool {
	return atomic.LoadInt32(&c.closed) == 1
}

// Reset 重置连接状态
func (c *Connection) Reset() {
	c.UserID = 0
	c.SessionID = ""
	c.LastActivity = time.Time{}
	atomic.StoreInt32(&c.closed, 0)
}

// MessageHandler 消息处理器接口
type MessageHandler interface {
	HandleMessage(conn *Connection, data []byte) error
}

// TCPServer TCP服务器
type TCPServer struct {
	address      string
	port         int
	listener     net.Listener
	connections  sync.Map
	connCounter  uint64
	running      bool
	ctx          context.Context
	cancel       context.CancelFunc
	wg           sync.WaitGroup
	handler      MessageHandler
	maxConns     int
	readTimeout  time.Duration
	writeTimeout time.Duration
	connPool     *pool.ConnectionPool
}

// NewTCPServer 创建TCP服务器
func NewTCPServer(address string, port int, handler MessageHandler, maxConns int) *TCPServer {
	ctx, cancel := context.WithCancel(context.Background())

	return &TCPServer{
		address:      address,
		port:         port,
		handler:      handler,
		maxConns:     maxConns,
		readTimeout:  30 * time.Second,
		writeTimeout: 30 * time.Second,
		ctx:          ctx,
		cancel:       cancel,
		connPool:     pool.NewConnectionPool(maxConns, func() interface{} {
			return &Connection{}
		}),
	}
}

// Start 启动TCP服务器
func (s *TCPServer) Start() error {
	listener, err := net.Listen("tcp", fmt.Sprintf("%s:%d", s.address, s.port))
	if err != nil {
		return fmt.Errorf("failed to listen on %s:%d: %v", s.address, s.port, err)
	}

	s.listener = listener
	s.running = true

	logger.Info(fmt.Sprintf("TCP server listening on %s:%d", s.address, s.port))

	s.wg.Add(2)
	go s.acceptLoop()
	go s.heartbeatLoop()

	return nil
}

// Stop 停止TCP服务器
func (s *TCPServer) Stop() error {
	if !s.running {
		return nil
	}

	s.running = false
	s.cancel()

	if s.listener != nil {
		s.listener.Close()
	}

	// 关闭所有连接
	s.connections.Range(func(key, value interface{}) bool {
		if conn, ok := value.(*Connection); ok {
			conn.Close()
		}
		return true
	})

	s.wg.Wait()
	logger.Info("TCP server stopped")

	return nil
}

// acceptLoop 接受连接循环
func (s *TCPServer) acceptLoop() {
	defer s.wg.Done()

	for s.running {
		conn, err := s.listener.Accept()
		if err != nil {
			if s.running {
				logger.Error(fmt.Sprintf("Accept error: %v", err))
			}
			continue
		}

		// 检查连接数限制
		if s.GetConnectionCount() >= s.maxConns {
			logger.Warn("Max connections reached, closing new connection")
			conn.Close()
			continue
		}

		// 创建新连接
		connID := atomic.AddUint64(&s.connCounter, 1)
		connection := NewConnection(connID, conn)

		s.connections.Store(connID, connection)
		logger.Debug(fmt.Sprintf("New connection %d from %s", connID, conn.RemoteAddr()))

		// 启动连接处理goroutine
		s.wg.Add(1)
		go s.handleConnection(connection)
	}
}

// handleConnection 处理连接
func (s *TCPServer) handleConnection(conn *Connection) {
	defer s.wg.Done()
	defer func() {
		conn.Close()
		s.connections.Delete(conn.ID)
		s.connPool.Put(conn)
		logger.Debug(fmt.Sprintf("Connection %d closed", conn.ID))
	}()

	// 设置超时
	conn.Conn.SetReadDeadline(time.Now().Add(s.readTimeout))
	conn.Conn.SetWriteDeadline(time.Now().Add(s.writeTimeout))

	for !conn.IsClosed() && s.running {
		// 读取消息长度 (4字节)
		lengthBuf := make([]byte, 4)
		if _, err := conn.Read(lengthBuf); err != nil {
			if !conn.IsClosed() {
				logger.Debug(fmt.Sprintf("Read length error for connection %d: %v", conn.ID, err))
			}
			break
		}

		// 解析消息长度
		msgLen := uint32(lengthBuf[0])<<24 | uint32(lengthBuf[1])<<16 | uint32(lengthBuf[2])<<8 | uint32(lengthBuf[3])

		// 检查消息长度合法性
		if msgLen == 0 || msgLen > 1024*1024 { // 最大1MB
			logger.Warn(fmt.Sprintf("Invalid message length %d for connection %d", msgLen, conn.ID))
			break
		}

		// 读取消息内容
		msgBuf := make([]byte, msgLen)
		if _, err := conn.Read(msgBuf); err != nil {
			logger.Debug(fmt.Sprintf("Read message error for connection %d: %v", conn.ID, err))
			break
		}

		// 处理消息
		if err := s.handler.HandleMessage(conn, msgBuf); err != nil {
			logger.Error(fmt.Sprintf("Handle message error for connection %d: %v", conn.ID, err))
		}

		// 更新超时时间
		conn.Conn.SetReadDeadline(time.Now().Add(s.readTimeout))
		conn.Conn.SetWriteDeadline(time.Now().Add(s.writeTimeout))
	}
}

// heartbeatLoop 心跳检测循环
func (s *TCPServer) heartbeatLoop() {
	defer s.wg.Done()

	ticker := time.NewTicker(30 * time.Second)
	defer ticker.Stop()

	for {
		select {
		case <-ticker.C:
			now := time.Now()
			var expiredConns []uint64

			s.connections.Range(func(key, value interface{}) bool {
				if conn, ok := value.(*Connection); ok {
					if now.Sub(conn.LastActivity) > 60*time.Second {
						expiredConns = append(expiredConns, conn.ID)
					}
				}
				return true
			})

			// 关闭过期连接
			for _, connID := range expiredConns {
				if value, ok := s.connections.Load(connID); ok {
					if conn, ok := value.(*Connection); ok {
						logger.Debug(fmt.Sprintf("Closing expired connection %d", connID))
						conn.Close()
					}
				}
			}

		case <-s.ctx.Done():
			return
		}
	}
}

// GetConnection 获取连接
func (s *TCPServer) GetConnection(connID uint64) (*Connection, bool) {
	value, ok := s.connections.Load(connID)
	if !ok {
		return nil, false
	}
	return value.(*Connection), true
}

// GetConnectionByUserID 根据用户ID获取连接
func (s *TCPServer) GetConnectionByUserID(userID uint64) (*Connection, bool) {
	var result *Connection
	s.connections.Range(func(key, value interface{}) bool {
		if conn, ok := value.(*Connection); ok && conn.UserID == userID {
			result = conn
			return false // 停止迭代
		}
		return true
	})
	return result, result != nil
}

// GetConnectionCount 获取连接数
func (s *TCPServer) GetConnectionCount() int {
	count := 0
	s.connections.Range(func(key, value interface{}) bool {
		count++
		return true
	})
	return count
}

// Broadcast 广播消息
func (s *TCPServer) Broadcast(data []byte) {
	s.connections.Range(func(key, value interface{}) bool {
		if conn, ok := value.(*Connection); ok && !conn.IsClosed() {
			conn.Write(data)
		}
		return true
	})
}

// SendToUser 发送消息给特定用户
func (s *TCPServer) SendToUser(userID uint64, data []byte) error {
	conn, ok := s.GetConnectionByUserID(userID)
	if !ok {
		return fmt.Errorf("user %d not connected", userID)
	}
	return conn.Write(data)
}
