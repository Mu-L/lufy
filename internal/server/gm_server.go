package server

import (
	"context"
	"fmt"
	"reflect"

	"lufy/internal/logger"
	"lufy/pkg/proto"
)

// GMServer GM服务器
type GMServer struct {
	*BaseServer
}

// NewGMServer 创建GM服务器
func NewGMServer(configFile, nodeID string) *GMServer {
	baseServer, err := NewBaseServer(configFile, "gm", nodeID)
	if err != nil {
		logger.Fatal(fmt.Sprintf("Failed to create base server: %v", err))
	}

	gmServer := &GMServer{
		BaseServer: baseServer,
	}

	// 注册通用服务
	if err := RegisterCommonServices(baseServer); err != nil {
		logger.Fatal(fmt.Sprintf("Failed to register common services: %v", err))
	}

	// 注册GM服务
	gmService := NewGMService(gmServer)
	if err := baseServer.rpcServer.RegisterService(gmService); err != nil {
		logger.Fatal(fmt.Sprintf("Failed to register gm service: %v", err))
	}

	return gmServer
}

// GMService GM RPC服务
type GMService struct {
	server *GMServer
}

// NewGMService 创建GM服务
func NewGMService(server *GMServer) *GMService {
	return &GMService{
		server: server,
	}
}

// GetName 获取服务名称
func (gs *GMService) GetName() string {
	return "GMService"
}

// RegisterMethods 注册方法
func (gs *GMService) RegisterMethods() map[string]reflect.Value {
	methods := make(map[string]reflect.Value)

	methods["ExecuteCommand"] = reflect.ValueOf(gs.ExecuteCommand)
	methods["KickUser"] = reflect.ValueOf(gs.KickUser)
	methods["BanUser"] = reflect.ValueOf(gs.BanUser)
	methods["UnbanUser"] = reflect.ValueOf(gs.UnbanUser)
	methods["SendNotice"] = reflect.ValueOf(gs.SendNotice)
	methods["ReloadConfig"] = reflect.ValueOf(gs.ReloadConfig)

	return methods
}

// ExecuteCommand 执行GM命令
func (gs *GMService) ExecuteCommand(ctx context.Context, req *proto.BaseRequest) (*proto.BaseResponse, error) {
	// TODO: 实现GM命令执行逻辑
	return &proto.BaseResponse{
		Header: req.Header,
		Code:   0,
		Msg:    "command executed",
	}, nil
}

// KickUser 踢出用户
func (gs *GMService) KickUser(ctx context.Context, req *proto.BaseRequest) (*proto.BaseResponse, error) {
	// TODO: 实现踢出用户逻辑
	return &proto.BaseResponse{
		Header: req.Header,
		Code:   0,
		Msg:    "user kicked",
	}, nil
}

// BanUser 封禁用户
func (gs *GMService) BanUser(ctx context.Context, req *proto.BaseRequest) (*proto.BaseResponse, error) {
	// TODO: 实现封禁用户逻辑
	return &proto.BaseResponse{
		Header: req.Header,
		Code:   0,
		Msg:    "user banned",
	}, nil
}

// UnbanUser 解封用户
func (gs *GMService) UnbanUser(ctx context.Context, req *proto.BaseRequest) (*proto.BaseResponse, error) {
	// TODO: 实现解封用户逻辑
	return &proto.BaseResponse{
		Header: req.Header,
		Code:   0,
		Msg:    "user unbanned",
	}, nil
}

// SendNotice 发送公告
func (gs *GMService) SendNotice(ctx context.Context, req *proto.BaseRequest) (*proto.BaseResponse, error) {
	// TODO: 实现发送公告逻辑
	return &proto.BaseResponse{
		Header: req.Header,
		Code:   0,
		Msg:    "notice sent",
	}, nil
}

// ReloadConfig 重新加载配置
func (gs *GMService) ReloadConfig(ctx context.Context, req *proto.BaseRequest) (*proto.BaseResponse, error) {
	// 广播配置重载命令
	gs.server.messageBroker.BroadcastSystemMessage("reload_config", nil)

	return &proto.BaseResponse{
		Header: req.Header,
		Code:   0,
		Msg:    "config reload requested",
	}, nil
}
