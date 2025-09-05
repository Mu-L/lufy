package server

import (
	"context"
	"fmt"
	"reflect"

	"lufy/internal/logger"
	"lufy/pkg/proto"
)

// GameServer 游戏服务器
type GameServer struct {
	*BaseServer
}

// NewGameServer 创建游戏服务器
func NewGameServer(configFile, nodeID string) *GameServer {
	baseServer, err := NewBaseServer(configFile, "game", nodeID)
	if err != nil {
		logger.Fatal(fmt.Sprintf("Failed to create base server: %v", err))
	}

	gameServer := &GameServer{
		BaseServer: baseServer,
	}

	// 注册通用服务
	if err := RegisterCommonServices(baseServer); err != nil {
		logger.Fatal(fmt.Sprintf("Failed to register common services: %v", err))
	}

	// 注册游戏服务
	gameService := NewGameService(gameServer)
	if err := baseServer.rpcServer.RegisterService(gameService); err != nil {
		logger.Fatal(fmt.Sprintf("Failed to register game service: %v", err))
	}

	return gameServer
}

// GameService 游戏RPC服务
type GameService struct {
	server *GameServer
}

// NewGameService 创建游戏服务
func NewGameService(server *GameServer) *GameService {
	return &GameService{
		server: server,
	}
}

// GetName 获取服务名称
func (gs *GameService) GetName() string {
	return "GameService"
}

// RegisterMethods 注册方法
func (gs *GameService) RegisterMethods() map[string]reflect.Value {
	methods := make(map[string]reflect.Value)

	methods["StartGame"] = reflect.ValueOf(gs.StartGame)
	methods["EndGame"] = reflect.ValueOf(gs.EndGame)
	methods["PlayerAction"] = reflect.ValueOf(gs.PlayerAction)
	methods["GetGameState"] = reflect.ValueOf(gs.GetGameState)

	return methods
}

// StartGame 开始游戏
func (gs *GameService) StartGame(ctx context.Context, req *proto.BaseRequest) (*proto.BaseResponse, error) {
	// TODO: 实现开始游戏逻辑
	return &proto.BaseResponse{
		Header: req.Header,
		Code:   0,
		Msg:    "game started",
	}, nil
}

// EndGame 结束游戏
func (gs *GameService) EndGame(ctx context.Context, req *proto.BaseRequest) (*proto.BaseResponse, error) {
	// TODO: 实现结束游戏逻辑
	return &proto.BaseResponse{
		Header: req.Header,
		Code:   0,
		Msg:    "game ended",
	}, nil
}

// PlayerAction 玩家操作
func (gs *GameService) PlayerAction(ctx context.Context, req *proto.BaseRequest) (*proto.BaseResponse, error) {
	// TODO: 实现玩家操作逻辑
	return &proto.BaseResponse{
		Header: req.Header,
		Code:   0,
		Msg:    "action processed",
	}, nil
}

// GetGameState 获取游戏状态
func (gs *GameService) GetGameState(ctx context.Context, req *proto.BaseRequest) (*proto.BaseResponse, error) {
	// TODO: 实现获取游戏状态逻辑
	return &proto.BaseResponse{
		Header: req.Header,
		Code:   0,
		Msg:    "success",
	}, nil
}
