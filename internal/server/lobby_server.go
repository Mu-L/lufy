package server

import (
	"context"
	"fmt"
	"reflect"

	"lufy/internal/logger"
	"lufy/pkg/proto"
)

// LobbyServer 游戏大厅服务器
type LobbyServer struct {
	*BaseServer
}

// NewLobbyServer 创建游戏大厅服务器
func NewLobbyServer(configFile, nodeID string) *LobbyServer {
	baseServer, err := NewBaseServer(configFile, "lobby", nodeID)
	if err != nil {
		logger.Fatal(fmt.Sprintf("Failed to create base server: %v", err))
	}

	lobbyServer := &LobbyServer{
		BaseServer: baseServer,
	}

	// 注册通用服务
	if err := RegisterCommonServices(baseServer); err != nil {
		logger.Fatal(fmt.Sprintf("Failed to register common services: %v", err))
	}

	// 注册大厅服务
	lobbyService := NewLobbyService(lobbyServer)
	if err := baseServer.rpcServer.RegisterService(lobbyService); err != nil {
		logger.Fatal(fmt.Sprintf("Failed to register lobby service: %v", err))
	}

	return lobbyServer
}

// LobbyService 大厅RPC服务
type LobbyService struct {
	server *LobbyServer
}

// NewLobbyService 创建大厅服务
func NewLobbyService(server *LobbyServer) *LobbyService {
	return &LobbyService{
		server: server,
	}
}

// GetName 获取服务名称
func (ls *LobbyService) GetName() string {
	return "LobbyService"
}

// RegisterMethods 注册方法
func (ls *LobbyService) RegisterMethods() map[string]reflect.Value {
	methods := make(map[string]reflect.Value)

	methods["GetRoomList"] = reflect.ValueOf(ls.GetRoomList)
	methods["CreateRoom"] = reflect.ValueOf(ls.CreateRoom)
	methods["JoinRoom"] = reflect.ValueOf(ls.JoinRoom)
	methods["LeaveRoom"] = reflect.ValueOf(ls.LeaveRoom)

	return methods
}

// GetRoomList 获取房间列表
func (ls *LobbyService) GetRoomList(ctx context.Context, req *proto.BaseRequest) (*proto.BaseResponse, error) {
	// TODO: 实现获取房间列表逻辑
	return &proto.BaseResponse{
		Header: req.Header,
		Code:   0,
		Msg:    "success",
	}, nil
}

// CreateRoom 创建房间
func (ls *LobbyService) CreateRoom(ctx context.Context, req *proto.BaseRequest) (*proto.BaseResponse, error) {
	// TODO: 实现创建房间逻辑
	return &proto.BaseResponse{
		Header: req.Header,
		Code:   0,
		Msg:    "room created",
	}, nil
}

// JoinRoom 加入房间
func (ls *LobbyService) JoinRoom(ctx context.Context, req *proto.BaseRequest) (*proto.BaseResponse, error) {
	// TODO: 实现加入房间逻辑
	return &proto.BaseResponse{
		Header: req.Header,
		Code:   0,
		Msg:    "joined room",
	}, nil
}

// LeaveRoom 离开房间
func (ls *LobbyService) LeaveRoom(ctx context.Context, req *proto.BaseRequest) (*proto.BaseResponse, error) {
	// TODO: 实现离开房间逻辑
	return &proto.BaseResponse{
		Header: req.Header,
		Code:   0,
		Msg:    "left room",
	}, nil
}
