package server

import (
	"context"
	"fmt"
	"reflect"

	"lufy/internal/database"
	"lufy/internal/logger"
	"lufy/pkg/proto"
)

// FriendServer 好友服务器
type FriendServer struct {
	*BaseServer
	friendRepo *database.FriendRepository
}

// NewFriendServer 创建好友服务器
func NewFriendServer(configFile, nodeID string) *FriendServer {
	baseServer, err := NewBaseServer(configFile, "friend", nodeID)
	if err != nil {
		logger.Fatal(fmt.Sprintf("Failed to create base server: %v", err))
	}

	friendServer := &FriendServer{
		BaseServer: baseServer,
		friendRepo: database.NewFriendRepository(baseServer.mongoManager),
	}

	// 注册通用服务
	if err := RegisterCommonServices(baseServer); err != nil {
		logger.Fatal(fmt.Sprintf("Failed to register common services: %v", err))
	}

	// 注册好友服务
	friendService := NewFriendService(friendServer)
	if err := baseServer.rpcServer.RegisterService(friendService); err != nil {
		logger.Fatal(fmt.Sprintf("Failed to register friend service: %v", err))
	}

	return friendServer
}

// FriendService 好友RPC服务
type FriendService struct {
	server *FriendServer
}

// NewFriendService 创建好友服务
func NewFriendService(server *FriendServer) *FriendService {
	return &FriendService{
		server: server,
	}
}

// GetName 获取服务名称
func (fs *FriendService) GetName() string {
	return "FriendService"
}

// RegisterMethods 注册方法
func (fs *FriendService) RegisterMethods() map[string]reflect.Value {
	methods := make(map[string]reflect.Value)

	methods["AddFriend"] = reflect.ValueOf(fs.AddFriend)
	methods["AcceptFriend"] = reflect.ValueOf(fs.AcceptFriend)
	methods["GetFriendList"] = reflect.ValueOf(fs.GetFriendList)
	methods["DeleteFriend"] = reflect.ValueOf(fs.DeleteFriend)

	return methods
}

// AddFriend 添加好友
func (fs *FriendService) AddFriend(ctx context.Context, req *proto.BaseRequest) (*proto.BaseResponse, error) {
	// TODO: 实现添加好友逻辑
	return &proto.BaseResponse{
		Header: req.Header,
		Code:   0,
		Msg:    "friend request sent",
	}, nil
}

// AcceptFriend 接受好友请求
func (fs *FriendService) AcceptFriend(ctx context.Context, req *proto.BaseRequest) (*proto.BaseResponse, error) {
	// TODO: 实现接受好友逻辑
	return &proto.BaseResponse{
		Header: req.Header,
		Code:   0,
		Msg:    "friend request accepted",
	}, nil
}

// GetFriendList 获取好友列表
func (fs *FriendService) GetFriendList(ctx context.Context, req *proto.BaseRequest) (*proto.BaseResponse, error) {
	// TODO: 实现获取好友列表逻辑
	return &proto.BaseResponse{
		Header: req.Header,
		Code:   0,
		Msg:    "success",
	}, nil
}

// DeleteFriend 删除好友
func (fs *FriendService) DeleteFriend(ctx context.Context, req *proto.BaseRequest) (*proto.BaseResponse, error) {
	// TODO: 实现删除好友逻辑
	return &proto.BaseResponse{
		Header: req.Header,
		Code:   0,
		Msg:    "friend deleted",
	}, nil
}
