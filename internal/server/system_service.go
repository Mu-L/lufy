package server

import (
	"context"
	"fmt"
	"reflect"
	"runtime"
	"time"

	"github.com/phuhao00/lufy/internal/logger"
	"github.com/phuhao00/lufy/internal/mq"
	"github.com/phuhao00/lufy/internal/pool"
	"github.com/phuhao00/lufy/pkg/proto"
)

// SystemService 系统服务
type SystemService struct {
	server *BaseServer
}

// NewSystemService 创建系统服务
func NewSystemService(server *BaseServer) *SystemService {
	return &SystemService{
		server: server,
	}
}

// GetName 获取服务名称
func (ss *SystemService) GetName() string {
	return "SystemService"
}

// RegisterMethods 注册方法
func (ss *SystemService) RegisterMethods() map[string]reflect.Value {
	methods := make(map[string]reflect.Value)

	methods["GetServerInfo"] = reflect.ValueOf(ss.GetServerInfo)
	methods["GetServerStats"] = reflect.ValueOf(ss.GetServerStats)
	methods["ReloadConfig"] = reflect.ValueOf(ss.ReloadConfig)
	methods["UpdateLoad"] = reflect.ValueOf(ss.UpdateLoad)
	methods["Shutdown"] = reflect.ValueOf(ss.Shutdown)
	methods["GetActorStats"] = reflect.ValueOf(ss.GetActorStats)
	methods["GetPoolStats"] = reflect.ValueOf(ss.GetPoolStats)

	return methods
}

// GetServerInfo 获取服务器信息
func (ss *SystemService) GetServerInfo(ctx context.Context, req *proto.BaseRequest) (*proto.BaseResponse, error) {
	info := &proto.NodeInfo{
		NodeId:     ss.server.nodeID,
		NodeType:   ss.server.nodeType,
		Address:    "0.0.0.0",
		Port:       int32(ss.server.config.Network.RPCPort),
		Online:     ss.server.status == "running",
		Load:       int32(ss.server.calculateLoad()),
		UpdateTime: uint32(time.Now().Unix()),
	}

	data, err := proto.Marshal(info)
	if err != nil {
		return nil, fmt.Errorf("failed to marshal server info: %v", err)
	}

	return &proto.BaseResponse{
		Header: req.Header,
		Code:   0,
		Msg:    "success",
		Data:   data,
	}, nil
}

// ServerStats 服务器统计信息
type ServerStats struct {
	NodeID         string              `json:"node_id"`
	NodeType       string              `json:"node_type"`
	Status         string              `json:"status"`
	Uptime         int64               `json:"uptime"`
	Load           int                 `json:"load"`
	Memory         MemoryStats         `json:"memory"`
	Goroutines     int                 `json:"goroutines"`
	Connections    int                 `json:"connections"`
	ActorCount     int                 `json:"actor_count"`
	RPCConnections int64               `json:"rpc_connections"`
	PoolStats      map[string]PoolStat `json:"pool_stats"`
}

// MemoryStats 内存统计
type MemoryStats struct {
	Alloc      uint64 `json:"alloc"`
	TotalAlloc uint64 `json:"total_alloc"`
	Sys        uint64 `json:"sys"`
	NumGC      uint32 `json:"num_gc"`
}

// PoolStat 对象池统计
type PoolStat struct {
	Size      int   `json:"size"`
	Available int   `json:"available"`
	Created   int64 `json:"created"`
	Gotten    int64 `json:"gotten"`
	Put       int64 `json:"put"`
}

// GetServerStats 获取服务器统计信息
func (ss *SystemService) GetServerStats(ctx context.Context, req *proto.BaseRequest) (*proto.BaseResponse, error) {
	var memStats runtime.MemStats
	runtime.ReadMemStats(&memStats)

	// 获取对象池统计
	poolStats := make(map[string]PoolStat)

	stats := &ServerStats{
		NodeID:     ss.server.nodeID,
		NodeType:   ss.server.nodeType,
		Status:     ss.server.status,
		Load:       ss.server.calculateLoad(),
		Goroutines: runtime.NumGoroutine(),
		Memory: MemoryStats{
			Alloc:      memStats.Alloc,
			TotalAlloc: memStats.TotalAlloc,
			Sys:        memStats.Sys,
			NumGC:      memStats.NumGC,
		},
		PoolStats: poolStats,
	}

	if ss.server.tcpServer != nil {
		stats.Connections = ss.server.tcpServer.GetConnectionCount()
	}

	if ss.server.actorSystem != nil {
		stats.ActorCount = ss.server.actorSystem.GetActorCount()
	}

	if ss.server.rpcServer != nil {
		stats.RPCConnections = ss.server.rpcServer.GetConnectionCount()
	}

	data, err := proto.Marshal(&proto.BaseResponse{
		Header: req.Header,
		Code:   0,
		Msg:    "success",
	})
	if err != nil {
		return nil, fmt.Errorf("failed to marshal server stats: %v", err)
	}

	return &proto.BaseResponse{
		Header: req.Header,
		Code:   0,
		Msg:    "success",
		Data:   data,
	}, nil
}

// ReloadConfig 重新加载配置
func (ss *SystemService) ReloadConfig(ctx context.Context, req *proto.BaseRequest) (*proto.BaseResponse, error) {
	logger.Info(fmt.Sprintf("Reloading config for %s", ss.server.nodeID))

	// 这里可以实现配置重新加载逻辑
	// 目前只是记录日志

	return &proto.BaseResponse{
		Header: req.Header,
		Code:   0,
		Msg:    "config reloaded",
	}, nil
}

// UpdateLoad 更新负载
func (ss *SystemService) UpdateLoad(ctx context.Context, req *proto.BaseRequest) (*proto.BaseResponse, error) {
	load := ss.server.calculateLoad()

	// 更新服务注册中的负载信息
	if err := ss.server.registry.UpdateLoad(ss.server.nodeID, load); err != nil {
		logger.Error(fmt.Sprintf("Failed to update load: %v", err))
		return &proto.BaseResponse{
			Header: req.Header,
			Code:   -1,
			Msg:    err.Error(),
		}, nil
	}

	return &proto.BaseResponse{
		Header: req.Header,
		Code:   0,
		Msg:    fmt.Sprintf("load updated: %d", load),
	}, nil
}

// Shutdown 关闭服务器
func (ss *SystemService) Shutdown(ctx context.Context, req *proto.BaseRequest) (*proto.BaseResponse, error) {
	logger.Info(fmt.Sprintf("Shutdown requested for %s", ss.server.nodeID))

	// 异步关闭服务器
	go func() {
		time.Sleep(1 * time.Second) // 给响应时间
		ss.server.Stop()
	}()

	return &proto.BaseResponse{
		Header: req.Header,
		Code:   0,
		Msg:    "shutdown initiated",
	}, nil
}

// GetActorStats 获取Actor统计信息
func (ss *SystemService) GetActorStats(ctx context.Context, req *proto.BaseRequest) (*proto.BaseResponse, error) {
	if ss.server.actorSystem == nil {
		return &proto.BaseResponse{
			Header: req.Header,
			Code:   -1,
			Msg:    "actor system not initialized",
		}, nil
	}

	// TODO: 获取Actor系统统计信息
	// actorStats := map[string]interface{}{
	//	"total_actors":   actorSystem.GetActorCount(),
	//	"active_actors":  actorSystem.GetActiveActorCount(),
	//	"message_queue":  actorSystem.GetMessageQueueSize(),
	//	"processed_msgs": actorSystem.GetProcessedMessageCount(),
	// }

	data, err := proto.Marshal(&proto.BaseResponse{
		Header: req.Header,
		Code:   0,
		Msg:    "success",
	})
	if err != nil {
		return nil, fmt.Errorf("failed to marshal actor stats: %v", err)
	}

	return &proto.BaseResponse{
		Header: req.Header,
		Code:   0,
		Msg:    "success",
		Data:   data,
	}, nil
}

// GetPoolStats 获取对象池统计信息
func (ss *SystemService) GetPoolStats(ctx context.Context, req *proto.BaseRequest) (*proto.BaseResponse, error) {
	// 获取全局对象池统计
	pools := pool.GetGlobalPools()
	stats := pools.GetStats()

	data, err := proto.Marshal(&proto.BaseResponse{
		Header: req.Header,
		Code:   0,
		Msg:    "success",
	})
	if err != nil {
		return nil, fmt.Errorf("failed to marshal pool stats: %v", err)
	}

	logger.Debug(fmt.Sprintf("Pool stats: %+v", stats))

	return &proto.BaseResponse{
		Header: req.Header,
		Code:   0,
		Msg:    "success",
		Data:   data,
	}, nil
}

// 系统消息处理器

// HandleReloadConfig 处理重新加载配置消息
func (ss *SystemService) HandleReloadConfig(msg *mq.SystemMessage) error {
	logger.Info(fmt.Sprintf("Received reload config command for %s", ss.server.nodeID))

	// 这里可以实现具体的配置重新加载逻辑
	// 比如重新读取配置文件，更新相关组件等

	return nil
}

// HandleUpdateLoad 处理更新负载消息
func (ss *SystemService) HandleUpdateLoad(msg *mq.SystemMessage) error {
	load := ss.server.calculateLoad()

	if err := ss.server.registry.UpdateLoad(ss.server.nodeID, load); err != nil {
		logger.Error(fmt.Sprintf("Failed to update load: %v", err))
		return err
	}

	logger.Debug(fmt.Sprintf("Load updated for %s: %d", ss.server.nodeID, load))
	return nil
}

// HandleShutdown 处理关闭消息
func (ss *SystemService) HandleShutdown(msg *mq.SystemMessage) error {
	logger.Info(fmt.Sprintf("Received shutdown command for %s", ss.server.nodeID))

	// 异步关闭服务器
	go func() {
		time.Sleep(1 * time.Second)
		ss.server.Stop()
	}()

	return nil
}

// HandleHotUpdate 处理热更新消息
func (ss *SystemService) HandleHotUpdate(msg *mq.SystemMessage) error {
	logger.Info(fmt.Sprintf("Received hot update command for %s", ss.server.nodeID))

	// 这里可以实现热更新逻辑
	// 比如重新加载某些模块，更新游戏逻辑等

	// 从消息参数中获取更新内容
	if updateType, exists := msg.Args["type"]; exists {
		switch updateType {
		case "config":
			return ss.handleConfigHotUpdate(msg)
		case "logic":
			return ss.handleLogicHotUpdate(msg)
		case "data":
			return ss.handleDataHotUpdate(msg)
		default:
			logger.Warn(fmt.Sprintf("Unknown hot update type: %v", updateType))
		}
	}

	return nil
}

// handleConfigHotUpdate 处理配置热更新
func (ss *SystemService) handleConfigHotUpdate(msg *mq.SystemMessage) error {
	logger.Info("Performing config hot update")

	// 重新加载配置文件
	// 更新相关组件配置

	return nil
}

// handleLogicHotUpdate 处理逻辑热更新
func (ss *SystemService) handleLogicHotUpdate(msg *mq.SystemMessage) error {
	logger.Info("Performing logic hot update")

	// 重新加载游戏逻辑模块
	// 更新Actor行为

	return nil
}

// handleDataHotUpdate 处理数据热更新
func (ss *SystemService) handleDataHotUpdate(msg *mq.SystemMessage) error {
	logger.Info("Performing data hot update")

	// 重新加载游戏数据
	// 更新缓存

	return nil
}
