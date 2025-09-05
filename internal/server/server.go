package server

import (
	"context"
	"fmt"
	"os"
	"os/signal"
	"sync"
	"syscall"
	"time"

	"github.com/spf13/viper"

	"lufy/internal/actor"
	"lufy/internal/database"
	"lufy/internal/discovery"
	"lufy/internal/logger"
	"lufy/internal/mq"
	"lufy/internal/network"
	"lufy/internal/rpc"
)

// ServerConfig 服务器配置
type ServerConfig struct {
	Server struct {
		Name    string `yaml:"name"`
		Version string `yaml:"version"`
		Debug   bool   `yaml:"debug"`
	} `yaml:"server"`

	Network struct {
		TCPPort        int `yaml:"tcp_port"`
		RPCPort        int `yaml:"rpc_port"`
		HTTPPort       int `yaml:"http_port"`
		MaxConnections int `yaml:"max_connections"`
		ReadTimeout    int `yaml:"read_timeout"`
		WriteTimeout   int `yaml:"write_timeout"`
	} `yaml:"network"`

	Database struct {
		Redis   database.RedisConfig `yaml:"redis"`
		MongoDB database.MongoConfig `yaml:"mongodb"`
	} `yaml:"database"`

	NSQ mq.NSQConfig `yaml:"nsq"`

	ETCD struct {
		Endpoints   []string `yaml:"endpoints"`
		DialTimeout int      `yaml:"dial_timeout"`
	} `yaml:"etcd"`

	Log logger.LogConfig `yaml:"log"`

	Nodes map[string]struct {
		Count int   `yaml:"count"`
		Ports []int `yaml:"ports"`
	} `yaml:"nodes"`

	ObjectPool struct {
		MessagePoolSize    int `yaml:"message_pool_size"`
		ConnectionPoolSize int `yaml:"connection_pool_size"`
		ActorPoolSize      int `yaml:"actor_pool_size"`
	} `yaml:"object_pool"`

	RPC struct {
		PoolSize    int `yaml:"pool_size"`
		MaxIdle     int `yaml:"max_idle"`
		IdleTimeout int `yaml:"idle_timeout"`
	} `yaml:"rpc"`
}

// Server 服务器接口
type Server interface {
	Start() error
	Stop() error
	GetNodeID() string
	GetNodeType() string
	GetStatus() string
}

// BaseServer 基础服务器实现
type BaseServer struct {
	config   *ServerConfig
	nodeType string
	nodeID   string
	status   string

	// 组件
	actorSystem   *actor.ActorSystem
	tcpServer     *network.TCPServer
	rpcServer     *rpc.RPCServer
	rpcClient     *rpc.RPCClient
	redisManager  *database.RedisManager
	mongoManager  *database.MongoManager
	nsqManager    *mq.NSQManager
	messageBroker *mq.MessageBroker
	discovery     *discovery.ServiceDiscovery
	registry      *discovery.ETCDRegistry

	// 上下文
	ctx    context.Context
	cancel context.CancelFunc
	wg     sync.WaitGroup
	mutex  sync.RWMutex
}

// NewBaseServer 创建基础服务器
func NewBaseServer(configFile, nodeType, nodeID string) (*BaseServer, error) {
	// 加载配置
	config, err := loadConfig(configFile)
	if err != nil {
		return nil, fmt.Errorf("failed to load config: %v", err)
	}

	// 初始化日志
	logger.InitGlobalLogger(&config.Log)

	ctx, cancel := context.WithCancel(context.Background())

	server := &BaseServer{
		config:   config,
		nodeType: nodeType,
		nodeID:   nodeID,
		status:   "initializing",
		ctx:      ctx,
		cancel:   cancel,
	}

	// 初始化组件
	if err := server.initComponents(); err != nil {
		cancel()
		return nil, fmt.Errorf("failed to init components: %v", err)
	}

	logger.Info(fmt.Sprintf("Server %s/%s initialized", nodeType, nodeID))
	return server, nil
}

// loadConfig 加载配置文件
func loadConfig(configFile string) (*ServerConfig, error) {
	viper.SetConfigFile(configFile)
	viper.SetConfigType("yaml")

	if err := viper.ReadInConfig(); err != nil {
		return nil, err
	}

	var config ServerConfig
	if err := viper.Unmarshal(&config); err != nil {
		return nil, err
	}

	return &config, nil
}

// initComponents 初始化组件
func (bs *BaseServer) initComponents() error {
	// 初始化Actor系统
	bs.actorSystem = actor.NewActorSystem(fmt.Sprintf("%s-%s", bs.nodeType, bs.nodeID))

	// 初始化Redis
	redisManager, err := database.NewRedisManager(&bs.config.Database.Redis)
	if err != nil {
		return fmt.Errorf("failed to init redis: %v", err)
	}
	bs.redisManager = redisManager

	// 初始化MongoDB
	mongoManager, err := database.NewMongoManager(&bs.config.Database.MongoDB)
	if err != nil {
		return fmt.Errorf("failed to init mongodb: %v", err)
	}
	bs.mongoManager = mongoManager

	// 初始化NSQ
	nsqManager, err := mq.NewNSQManager(&bs.config.NSQ)
	if err != nil {
		return fmt.Errorf("failed to init nsq: %v", err)
	}
	bs.nsqManager = nsqManager
	bs.messageBroker = mq.NewMessageBroker(nsqManager, bs.nodeID)

	// 初始化ETCD服务注册
	registry, err := discovery.NewETCDRegistry(
		bs.config.ETCD.Endpoints,
		time.Duration(bs.config.ETCD.DialTimeout)*time.Second,
	)
	if err != nil {
		return fmt.Errorf("failed to init etcd registry: %v", err)
	}
	bs.registry = registry

	// 初始化服务发现
	bs.discovery = discovery.NewServiceDiscovery(
		registry,
		bs.nodeType,
		discovery.NewWeightedLoadBalancer(),
	)

	// 初始化RPC服务器
	rpcServer := rpc.NewRPCServer("0.0.0.0", bs.config.Network.RPCPort)
	bs.rpcServer = rpcServer

	return nil
}

// Start 启动服务器
func (bs *BaseServer) Start() error {
	bs.mutex.Lock()
	defer bs.mutex.Unlock()

	if bs.status != "initializing" {
		return fmt.Errorf("server already started")
	}

	logger.Info(fmt.Sprintf("Starting server %s/%s", bs.nodeType, bs.nodeID))

	// 启动RPC服务器
	if err := bs.rpcServer.Start(); err != nil {
		return fmt.Errorf("failed to start rpc server: %v", err)
	}

	// 注册服务
	serviceInfo := &discovery.ServiceInfo{
		NodeID:     bs.nodeID,
		NodeType:   bs.nodeType,
		Address:    "0.0.0.0",
		Port:       bs.config.Network.RPCPort,
		Load:       0,
		Status:     "online",
		Metadata:   map[string]string{},
		UpdateTime: time.Now().Unix(),
	}

	if err := bs.registry.Register(serviceInfo); err != nil {
		return fmt.Errorf("failed to register service: %v", err)
	}

	// 启动负载更新
	bs.wg.Add(1)
	go bs.loadUpdateLoop()

	// 监听系统信号
	bs.wg.Add(1)
	go bs.signalHandler()

	bs.status = "running"
	logger.Info(fmt.Sprintf("Server %s/%s started", bs.nodeType, bs.nodeID))

	return nil
}

// Stop 停止服务器
func (bs *BaseServer) Stop() error {
	bs.mutex.Lock()
	defer bs.mutex.Unlock()

	if bs.status != "running" {
		return nil
	}

	logger.Info(fmt.Sprintf("Stopping server %s/%s", bs.nodeType, bs.nodeID))

	bs.status = "stopping"
	bs.cancel()

	// 停止组件
	if bs.tcpServer != nil {
		bs.tcpServer.Stop()
	}

	if bs.rpcServer != nil {
		bs.rpcServer.Stop()
	}

	if bs.actorSystem != nil {
		bs.actorSystem.Shutdown()
	}

	if bs.nsqManager != nil {
		bs.nsqManager.Close()
	}

	if bs.registry != nil {
		bs.registry.Unregister(bs.nodeID)
		bs.registry.Close()
	}

	if bs.redisManager != nil {
		bs.redisManager.Close()
	}

	if bs.mongoManager != nil {
		bs.mongoManager.Close()
	}

	// 等待所有goroutine结束
	bs.wg.Wait()

	bs.status = "stopped"
	logger.Info(fmt.Sprintf("Server %s/%s stopped", bs.nodeType, bs.nodeID))

	return nil
}

// GetNodeID 获取节点ID
func (bs *BaseServer) GetNodeID() string {
	return bs.nodeID
}

// GetNodeType 获取节点类型
func (bs *BaseServer) GetNodeType() string {
	return bs.nodeType
}

// GetStatus 获取状态
func (bs *BaseServer) GetStatus() string {
	bs.mutex.RLock()
	defer bs.mutex.RUnlock()

	return bs.status
}

// loadUpdateLoop 负载更新循环
func (bs *BaseServer) loadUpdateLoop() {
	defer bs.wg.Done()

	ticker := time.NewTicker(30 * time.Second)
	defer ticker.Stop()

	for {
		select {
		case <-ticker.C:
			// 计算当前负载
			load := bs.calculateLoad()

			// 更新服务注册信息
			if err := bs.registry.UpdateLoad(bs.nodeID, load); err != nil {
				logger.Error(fmt.Sprintf("Failed to update load: %v", err))
			}

		case <-bs.ctx.Done():
			return
		}
	}
}

// calculateLoad 计算当前负载
func (bs *BaseServer) calculateLoad() int {
	// 基础负载计算：连接数 + Actor数量
	load := 0

	if bs.tcpServer != nil {
		load += bs.tcpServer.GetConnectionCount()
	}

	if bs.actorSystem != nil {
		load += bs.actorSystem.GetActorCount()
	}

	// 如果有RPC服务器，加上连接数
	if bs.rpcServer != nil {
		load += int(bs.rpcServer.GetConnectionCount())
	}

	return load
}

// signalHandler 信号处理
func (bs *BaseServer) signalHandler() {
	defer bs.wg.Done()

	sigChan := make(chan os.Signal, 1)
	signal.Notify(sigChan, syscall.SIGINT, syscall.SIGTERM)

	select {
	case sig := <-sigChan:
		logger.Info(fmt.Sprintf("Received signal %v, shutting down...", sig))
		bs.Stop()

	case <-bs.ctx.Done():
		return
	}
}

// GetActorSystem 获取Actor系统
func (bs *BaseServer) GetActorSystem() *actor.ActorSystem {
	return bs.actorSystem
}

// GetRedisManager 获取Redis管理器
func (bs *BaseServer) GetRedisManager() *database.RedisManager {
	return bs.redisManager
}

// GetMongoManager 获取MongoDB管理器
func (bs *BaseServer) GetMongoManager() *database.MongoManager {
	return bs.mongoManager
}

// GetMessageBroker 获取消息代理
func (bs *BaseServer) GetMessageBroker() *mq.MessageBroker {
	return bs.messageBroker
}

// GetDiscovery 获取服务发现
func (bs *BaseServer) GetDiscovery() *discovery.ServiceDiscovery {
	return bs.discovery
}

// NewServer 创建新服务器
func NewServer(configFile, nodeType, nodeID string) Server {
	switch nodeType {
	case "gateway":
		return NewGatewayServer(configFile, nodeID)
	case "login":
		return NewLoginServer(configFile, nodeID)
	case "lobby":
		return NewLobbyServer(configFile, nodeID)
	case "game":
		return NewGameServer(configFile, nodeID)
	case "enhanced_game":
		return NewEnhancedGameServer(configFile, nodeID)
	case "friend":
		return NewFriendServer(configFile, nodeID)
	case "chat":
		return NewChatServer(configFile, nodeID)
	case "mail":
		return NewMailServer(configFile, nodeID)
	case "gm":
		return NewGMServer(configFile, nodeID)
	case "center":
		return NewCenterServer(configFile, nodeID)
	default:
		logger.Fatal(fmt.Sprintf("Unknown node type: %s", nodeType))
		return nil
	}
}

// RegisterCommonServices 注册通用服务
func RegisterCommonServices(server *BaseServer) error {
	// 注册系统服务
	systemService := NewSystemService(server)
	if err := server.rpcServer.RegisterService(systemService); err != nil {
		return fmt.Errorf("failed to register system service: %v", err)
	}

	// 订阅系统消息
	systemHandler := mq.NewSystemMessageHandler(server.nodeID)
	systemHandler.RegisterHandler(mq.SYS_CMD_RELOAD_CONFIG, systemService.HandleReloadConfig)
	systemHandler.RegisterHandler(mq.SYS_CMD_UPDATE_LOAD, systemService.HandleUpdateLoad)
	systemHandler.RegisterHandler(mq.SYS_CMD_SHUTDOWN, systemService.HandleShutdown)
	systemHandler.RegisterHandler(mq.SYS_CMD_HOT_UPDATE, systemService.HandleHotUpdate)

	if err := server.messageBroker.SubscribeSystemMessages(systemHandler); err != nil {
		return fmt.Errorf("failed to subscribe system messages: %v", err)
	}

	return nil
}
