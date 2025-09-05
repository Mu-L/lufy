package server

import (
	"context"
	"fmt"
	"reflect"
	"time"

	"lufy/internal/discovery"
	"lufy/internal/logger"
	"lufy/pkg/proto"
)

// CenterServer 中心服务器
type CenterServer struct {
	*BaseServer
}

// NewCenterServer 创建中心服务器
func NewCenterServer(configFile, nodeID string) *CenterServer {
	baseServer, err := NewBaseServer(configFile, "center", nodeID)
	if err != nil {
		logger.Fatal(fmt.Sprintf("Failed to create base server: %v", err))
	}

	centerServer := &CenterServer{
		BaseServer: baseServer,
	}

	// 注册通用服务
	if err := RegisterCommonServices(baseServer); err != nil {
		logger.Fatal(fmt.Sprintf("Failed to register common services: %v", err))
	}

	// 注册中心服务
	centerService := NewCenterService(centerServer)
	if err := baseServer.rpcServer.RegisterService(centerService); err != nil {
		logger.Fatal(fmt.Sprintf("Failed to register center service: %v", err))
	}

	// 启动管理任务
	go centerServer.managementLoop()

	return centerServer
}

// managementLoop 管理循环
func (cs *CenterServer) managementLoop() {
	ticker := time.NewTicker(60 * time.Second)
	defer ticker.Stop()

	for {
		select {
		case <-ticker.C:
			// 执行定期管理任务
			cs.performHealthChecks()
			cs.collectStatistics()

		case <-cs.ctx.Done():
			return
		}
	}
}

// performHealthChecks 执行健康检查
func (cs *CenterServer) performHealthChecks() {
	// 获取所有注册的服务
	serviceTypes := []string{"gateway", "login", "lobby", "game", "friend", "chat", "mail", "gm"}

	for _, serviceType := range serviceTypes {
		services, err := cs.registry.GetServices(serviceType)
		if err != nil {
			logger.Error(fmt.Sprintf("Failed to get services for %s: %v", serviceType, err))
			continue
		}

		logger.Debug(fmt.Sprintf("Health check for %s: %d services online", serviceType, len(services)))
	}
}

// collectStatistics 收集统计信息
func (cs *CenterServer) collectStatistics() {
	// TODO: 实现统计信息收集
	logger.Debug("Collecting server statistics")
}

// CenterService 中心RPC服务
type CenterService struct {
	server *CenterServer
}

// NewCenterService 创建中心服务
func NewCenterService(server *CenterServer) *CenterService {
	return &CenterService{
		server: server,
	}
}

// GetName 获取服务名称
func (cs *CenterService) GetName() string {
	return "CenterService"
}

// RegisterMethods 注册方法
func (cs *CenterService) RegisterMethods() map[string]reflect.Value {
	methods := make(map[string]reflect.Value)

	methods["GetServiceList"] = reflect.ValueOf(cs.GetServiceList)
	methods["GetClusterStatus"] = reflect.ValueOf(cs.GetClusterStatus)
	methods["BroadcastMessage"] = reflect.ValueOf(cs.BroadcastMessage)
	methods["ShutdownService"] = reflect.ValueOf(cs.ShutdownService)
	methods["RestartService"] = reflect.ValueOf(cs.RestartService)

	return methods
}

// GetServiceList 获取服务列表
func (cs *CenterService) GetServiceList(ctx context.Context, req *proto.BaseRequest) (*proto.BaseResponse, error) {
	serviceTypes := []string{"gateway", "login", "lobby", "game", "friend", "chat", "mail", "gm", "center"}
	allServices := make([]*discovery.ServiceInfo, 0)

	for _, serviceType := range serviceTypes {
		services, err := cs.server.registry.GetServices(serviceType)
		if err != nil {
			logger.Error(fmt.Sprintf("Failed to get services for %s: %v", serviceType, err))
			continue
		}
		allServices = append(allServices, services...)
	}

	// TODO: 将服务信息序列化为响应数据

	return &proto.BaseResponse{
		Header: req.Header,
		Code:   0,
		Msg:    fmt.Sprintf("found %d services", len(allServices)),
	}, nil
}

// GetClusterStatus 获取集群状态
func (cs *CenterService) GetClusterStatus(ctx context.Context, req *proto.BaseRequest) (*proto.BaseResponse, error) {
	// TODO: 实现集群状态统计
	return &proto.BaseResponse{
		Header: req.Header,
		Code:   0,
		Msg:    "cluster status retrieved",
	}, nil
}

// BroadcastMessage 广播消息
func (cs *CenterService) BroadcastMessage(ctx context.Context, req *proto.BaseRequest) (*proto.BaseResponse, error) {
	// TODO: 从请求中解析广播内容
	// 广播系统消息给所有节点
	cs.server.messageBroker.BroadcastSystemMessage("broadcast_notice", map[string]interface{}{
		"message": "test broadcast from center",
	})

	return &proto.BaseResponse{
		Header: req.Header,
		Code:   0,
		Msg:    "message broadcasted",
	}, nil
}

// ShutdownService 关闭服务
func (cs *CenterService) ShutdownService(ctx context.Context, req *proto.BaseRequest) (*proto.BaseResponse, error) {
	// TODO: 从请求中解析目标节点ID
	targetNodeID := "target_node" // 从请求中获取

	cs.server.messageBroker.SendToNode(targetNodeID, "shutdown", nil)

	return &proto.BaseResponse{
		Header: req.Header,
		Code:   0,
		Msg:    "shutdown command sent",
	}, nil
}

// RestartService 重启服务
func (cs *CenterService) RestartService(ctx context.Context, req *proto.BaseRequest) (*proto.BaseResponse, error) {
	// TODO: 实现重启服务逻辑
	return &proto.BaseResponse{
		Header: req.Header,
		Code:   0,
		Msg:    "restart command sent",
	}, nil
}
