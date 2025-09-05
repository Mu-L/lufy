package rpc

import (
	"context"
	"encoding/json"
	"fmt"
	"net"
	"reflect"
	"sync"
	"sync/atomic"
	"time"

	"google.golang.org/protobuf/proto"

	"lufy/internal/logger"
	"lufy/pkg/proto"
)

// RPCService RPC服务接口
type RPCService interface {
	GetName() string
	RegisterMethods() map[string]reflect.Value
}

// RPCRequest RPC请求
type RPCRequest struct {
	ID       uint64            `json:"id"`
	Service  string            `json:"service"`
	Method   string            `json:"method"`
	Args     []byte            `json:"args"`
	Timeout  int64             `json:"timeout"`
	Callback chan *RPCResponse `json:"-"`
}

// RPCResponse RPC响应
type RPCResponse struct {
	ID    uint64 `json:"id"`
	Error string `json:"error,omitempty"`
	Data  []byte `json:"data,omitempty"`
}

// RPCServer RPC服务器
type RPCServer struct {
	address   string
	port      int
	listener  net.Listener
	services  map[string]RPCService
	methods   map[string]reflect.Value
	running   bool
	ctx       context.Context
	cancel    context.CancelFunc
	wg        sync.WaitGroup
	mutex     sync.RWMutex
	connCount int64
}

// NewRPCServer 创建RPC服务器
func NewRPCServer(address string, port int) *RPCServer {
	ctx, cancel := context.WithCancel(context.Background())
	return &RPCServer{
		address:  address,
		port:     port,
		services: make(map[string]RPCService),
		methods:  make(map[string]reflect.Value),
		ctx:      ctx,
		cancel:   cancel,
	}
}

// RegisterService 注册服务
func (s *RPCServer) RegisterService(service RPCService) error {
	s.mutex.Lock()
	defer s.mutex.Unlock()

	name := service.GetName()
	if _, exists := s.services[name]; exists {
		return fmt.Errorf("service %s already registered", name)
	}

	s.services[name] = service

	// 注册方法
	methods := service.RegisterMethods()
	for methodName, method := range methods {
		fullName := fmt.Sprintf("%s.%s", name, methodName)
		s.methods[fullName] = method
	}

	logger.Info(fmt.Sprintf("RPC service %s registered with %d methods", name, len(methods)))
	return nil
}

// Start 启动RPC服务器
func (s *RPCServer) Start() error {
	listener, err := net.Listen("tcp", fmt.Sprintf("%s:%d", s.address, s.port))
	if err != nil {
		return fmt.Errorf("failed to listen on %s:%d: %v", s.address, s.port, err)
	}

	s.listener = listener
	s.running = true

	logger.Info(fmt.Sprintf("RPC server listening on %s:%d", s.address, s.port))

	s.wg.Add(1)
	go s.acceptLoop()

	return nil
}

// Stop 停止RPC服务器
func (s *RPCServer) Stop() error {
	if !s.running {
		return nil
	}

	s.running = false
	s.cancel()

	if s.listener != nil {
		s.listener.Close()
	}

	s.wg.Wait()
	logger.Info("RPC server stopped")

	return nil
}

// acceptLoop 接受连接循环
func (s *RPCServer) acceptLoop() {
	defer s.wg.Done()

	for s.running {
		conn, err := s.listener.Accept()
		if err != nil {
			if s.running {
				logger.Error(fmt.Sprintf("Accept error: %v", err))
			}
			continue
		}

		atomic.AddInt64(&s.connCount, 1)
		s.wg.Add(1)
		go s.handleConnection(conn)
	}
}

// handleConnection 处理连接
func (s *RPCServer) handleConnection(conn net.Conn) {
	defer s.wg.Done()
	defer func() {
		conn.Close()
		atomic.AddInt64(&s.connCount, -1)
	}()

	logger.Debug(fmt.Sprintf("New RPC connection from %s", conn.RemoteAddr()))

	for s.running {
		// 读取请求长度
		lengthBuf := make([]byte, 4)
		if _, err := conn.Read(lengthBuf); err != nil {
			break
		}

		// 解析消息长度
		msgLen := uint32(lengthBuf[0])<<24 | uint32(lengthBuf[1])<<16 | uint32(lengthBuf[2])<<8 | uint32(lengthBuf[3])

		// 检查消息长度
		if msgLen == 0 || msgLen > 1024*1024 {
			logger.Warn(fmt.Sprintf("Invalid RPC message length: %d", msgLen))
			break
		}

		// 读取请求数据
		requestBuf := make([]byte, msgLen)
		if _, err := conn.Read(requestBuf); err != nil {
			break
		}

		// 处理请求
		response := s.handleRequest(requestBuf)

		// 发送响应
		responseData, _ := json.Marshal(response)
		responseLen := make([]byte, 4)
		responseLen[0] = byte(len(responseData) >> 24)
		responseLen[1] = byte(len(responseData) >> 16)
		responseLen[2] = byte(len(responseData) >> 8)
		responseLen[3] = byte(len(responseData))

		conn.Write(responseLen)
		conn.Write(responseData)
	}
}

// handleRequest 处理RPC请求
func (s *RPCServer) handleRequest(data []byte) *RPCResponse {
	var request RPCRequest
	if err := json.Unmarshal(data, &request); err != nil {
		return &RPCResponse{
			ID:    0,
			Error: fmt.Sprintf("unmarshal request error: %v", err),
		}
	}

	// 查找方法
	methodKey := fmt.Sprintf("%s.%s", request.Service, request.Method)
	s.mutex.RLock()
	method, exists := s.methods[methodKey]
	s.mutex.RUnlock()

	if !exists {
		return &RPCResponse{
			ID:    request.ID,
			Error: fmt.Sprintf("method %s not found", methodKey),
		}
	}

	// 调用方法
	start := time.Now()
	result, err := s.callMethod(method, request.Args)
	duration := time.Since(start)

	logger.Debug(fmt.Sprintf("RPC call %s took %v", methodKey, duration))

	response := &RPCResponse{ID: request.ID}
	if err != nil {
		response.Error = err.Error()
	} else {
		response.Data = result
	}

	return response
}

// callMethod 调用方法
func (s *RPCServer) callMethod(method reflect.Value, args []byte) ([]byte, error) {
	methodType := method.Type()
	if methodType.NumIn() != 2 {
		return nil, fmt.Errorf("method must have exactly 2 parameters")
	}

	// 创建参数
	argsType := methodType.In(1)
	argsValue := reflect.New(argsType.Elem())

	// 反序列化参数
	if len(args) > 0 {
		if err := proto.Unmarshal(args, argsValue.Interface().(proto.Message)); err != nil {
			return nil, fmt.Errorf("unmarshal args error: %v", err)
		}
	}

	// 调用方法
	results := method.Call([]reflect.Value{
		reflect.ValueOf(context.Background()),
		argsValue,
	})

	if len(results) != 2 {
		return nil, fmt.Errorf("method must return exactly 2 values")
	}

	// 检查错误
	if !results[1].IsNil() {
		return nil, results[1].Interface().(error)
	}

	// 序列化结果
	if results[0].IsNil() {
		return nil, nil
	}

	return proto.Marshal(results[0].Interface().(proto.Message))
}

// GetConnectionCount 获取连接数
func (s *RPCServer) GetConnectionCount() int64 {
	return atomic.LoadInt64(&s.connCount)
}

// RPCClient RPC客户端
type RPCClient struct {
	address   string
	port      int
	conn      net.Conn
	mutex     sync.Mutex
	requestID uint64
	callbacks map[uint64]chan *RPCResponse
	running   bool
	ctx       context.Context
	cancel    context.CancelFunc
	wg        sync.WaitGroup
	pool      *RPCConnectionPool
}

// NewRPCClient 创建RPC客户端
func NewRPCClient(address string, port int) *RPCClient {
	ctx, cancel := context.WithCancel(context.Background())
	return &RPCClient{
		address:   address,
		port:      port,
		callbacks: make(map[uint64]chan *RPCResponse),
		ctx:       ctx,
		cancel:    cancel,
	}
}

// Connect 连接到RPC服务器
func (c *RPCClient) Connect() error {
	conn, err := net.Dial("tcp", fmt.Sprintf("%s:%d", c.address, c.port))
	if err != nil {
		return fmt.Errorf("failed to connect to %s:%d: %v", c.address, c.port, err)
	}

	c.conn = conn
	c.running = true

	// 启动响应处理goroutine
	c.wg.Add(1)
	go c.responseLoop()

	logger.Debug(fmt.Sprintf("Connected to RPC server %s:%d", c.address, c.port))
	return nil
}

// Disconnect 断开连接
func (c *RPCClient) Disconnect() error {
	if !c.running {
		return nil
	}

	c.running = false
	c.cancel()

	if c.conn != nil {
		c.conn.Close()
	}

	// 清理回调
	c.mutex.Lock()
	for _, callback := range c.callbacks {
		close(callback)
	}
	c.callbacks = make(map[uint64]chan *RPCResponse)
	c.mutex.Unlock()

	c.wg.Wait()
	logger.Debug("Disconnected from RPC server")

	return nil
}

// Call 同步调用RPC方法
func (c *RPCClient) Call(service, method string, args proto.Message, timeout time.Duration) ([]byte, error) {
	if !c.running {
		return nil, fmt.Errorf("client not connected")
	}

	// 序列化参数
	var argsData []byte
	var err error
	if args != nil {
		argsData, err = proto.Marshal(args)
		if err != nil {
			return nil, fmt.Errorf("marshal args error: %v", err)
		}
	}

	// 创建请求
	requestID := atomic.AddUint64(&c.requestID, 1)
	request := &RPCRequest{
		ID:      requestID,
		Service: service,
		Method:  method,
		Args:    argsData,
		Timeout: int64(timeout / time.Millisecond),
	}

	// 创建回调通道
	callback := make(chan *RPCResponse, 1)
	c.mutex.Lock()
	c.callbacks[requestID] = callback
	c.mutex.Unlock()

	// 发送请求
	requestData, _ := json.Marshal(request)
	requestLen := make([]byte, 4)
	requestLen[0] = byte(len(requestData) >> 24)
	requestLen[1] = byte(len(requestData) >> 16)
	requestLen[2] = byte(len(requestData) >> 8)
	requestLen[3] = byte(len(requestData))

	c.mutex.Lock()
	_, err = c.conn.Write(requestLen)
	if err == nil {
		_, err = c.conn.Write(requestData)
	}
	c.mutex.Unlock()

	if err != nil {
		c.mutex.Lock()
		delete(c.callbacks, requestID)
		c.mutex.Unlock()
		close(callback)
		return nil, fmt.Errorf("send request error: %v", err)
	}

	// 等待响应
	select {
	case response := <-callback:
		c.mutex.Lock()
		delete(c.callbacks, requestID)
		c.mutex.Unlock()

		if response.Error != "" {
			return nil, fmt.Errorf("rpc error: %s", response.Error)
		}
		return response.Data, nil

	case <-time.After(timeout):
		c.mutex.Lock()
		delete(c.callbacks, requestID)
		c.mutex.Unlock()
		close(callback)
		return nil, fmt.Errorf("rpc call timeout")
	}
}

// responseLoop 响应处理循环
func (c *RPCClient) responseLoop() {
	defer c.wg.Done()

	for c.running {
		// 读取响应长度
		lengthBuf := make([]byte, 4)
		if _, err := c.conn.Read(lengthBuf); err != nil {
			if c.running {
				logger.Error(fmt.Sprintf("Read response length error: %v", err))
			}
			break
		}

		// 解析消息长度
		msgLen := uint32(lengthBuf[0])<<24 | uint32(lengthBuf[1])<<16 | uint32(lengthBuf[2])<<8 | uint32(lengthBuf[3])

		// 读取响应数据
		responseBuf := make([]byte, msgLen)
		if _, err := c.conn.Read(responseBuf); err != nil {
			logger.Error(fmt.Sprintf("Read response data error: %v", err))
			break
		}

		// 解析响应
		var response RPCResponse
		if err := json.Unmarshal(responseBuf, &response); err != nil {
			logger.Error(fmt.Sprintf("Unmarshal response error: %v", err))
			continue
		}

		// 处理响应
		c.mutex.Lock()
		if callback, exists := c.callbacks[response.ID]; exists {
			select {
			case callback <- &response:
			default:
				// 回调通道已满或已关闭
			}
		}
		c.mutex.Unlock()
	}
}

// RPCConnectionPool RPC连接池
type RPCConnectionPool struct {
	address string
	port    int
	maxSize int
	pool    chan *RPCClient
	created int64
	mutex   sync.Mutex
	ctx     context.Context
	cancel  context.CancelFunc
}

// NewRPCConnectionPool 创建RPC连接池
func NewRPCConnectionPool(address string, port int, maxSize int) *RPCConnectionPool {
	ctx, cancel := context.WithCancel(context.Background())
	return &RPCConnectionPool{
		address: address,
		port:    port,
		maxSize: maxSize,
		pool:    make(chan *RPCClient, maxSize),
		ctx:     ctx,
		cancel:  cancel,
	}
}

// Get 获取连接
func (p *RPCConnectionPool) Get() (*RPCClient, error) {
	select {
	case client := <-p.pool:
		return client, nil
	default:
		if atomic.LoadInt64(&p.created) < int64(p.maxSize) {
			client := NewRPCClient(p.address, p.port)
			if err := client.Connect(); err != nil {
				return nil, err
			}
			client.pool = p
			atomic.AddInt64(&p.created, 1)
			return client, nil
		}

		// 等待连接可用
		select {
		case client := <-p.pool:
			return client, nil
		case <-time.After(5 * time.Second):
			return nil, fmt.Errorf("connection pool timeout")
		}
	}
}

// Put 归还连接
func (p *RPCConnectionPool) Put(client *RPCClient) {
	if client == nil {
		return
	}

	select {
	case p.pool <- client:
	default:
		// 池已满，关闭连接
		client.Disconnect()
		atomic.AddInt64(&p.created, -1)
	}
}

// Close 关闭连接池
func (p *RPCConnectionPool) Close() {
	p.cancel()

	// 关闭所有连接
	close(p.pool)
	for client := range p.pool {
		client.Disconnect()
	}
}

// Size 获取池大小
func (p *RPCConnectionPool) Size() int {
	return len(p.pool)
}

// Created 获取已创建的连接数
func (p *RPCConnectionPool) Created() int64 {
	return atomic.LoadInt64(&p.created)
}
