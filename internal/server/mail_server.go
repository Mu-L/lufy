package server

import (
	"context"
	"fmt"
	"reflect"

	"lufy/internal/database"
	"lufy/internal/logger"
	"lufy/pkg/proto"
)

// MailServer 邮件服务器
type MailServer struct {
	*BaseServer
	mailRepo *database.MailRepository
}

// NewMailServer 创建邮件服务器
func NewMailServer(configFile, nodeID string) *MailServer {
	baseServer, err := NewBaseServer(configFile, "mail", nodeID)
	if err != nil {
		logger.Fatal(fmt.Sprintf("Failed to create base server: %v", err))
	}

	mailServer := &MailServer{
		BaseServer: baseServer,
		mailRepo:   database.NewMailRepository(baseServer.mongoManager),
	}

	// 注册通用服务
	if err := RegisterCommonServices(baseServer); err != nil {
		logger.Fatal(fmt.Sprintf("Failed to register common services: %v", err))
	}

	// 注册邮件服务
	mailService := NewMailService(mailServer)
	if err := baseServer.rpcServer.RegisterService(mailService); err != nil {
		logger.Fatal(fmt.Sprintf("Failed to register mail service: %v", err))
	}

	return mailServer
}

// MailService 邮件RPC服务
type MailService struct {
	server *MailServer
}

// NewMailService 创建邮件服务
func NewMailService(server *MailServer) *MailService {
	return &MailService{
		server: server,
	}
}

// GetName 获取服务名称
func (ms *MailService) GetName() string {
	return "MailService"
}

// RegisterMethods 注册方法
func (ms *MailService) RegisterMethods() map[string]reflect.Value {
	methods := make(map[string]reflect.Value)

	methods["GetMailList"] = reflect.ValueOf(ms.GetMailList)
	methods["ReadMail"] = reflect.ValueOf(ms.ReadMail)
	methods["ClaimRewards"] = reflect.ValueOf(ms.ClaimRewards)
	methods["DeleteMail"] = reflect.ValueOf(ms.DeleteMail)
	methods["SendMail"] = reflect.ValueOf(ms.SendMail)

	return methods
}

// GetMailList 获取邮件列表
func (ms *MailService) GetMailList(ctx context.Context, req *proto.BaseRequest) (*proto.BaseResponse, error) {
	// TODO: 实现获取邮件列表逻辑
	return &proto.BaseResponse{
		Header: req.Header,
		Code:   0,
		Msg:    "success",
	}, nil
}

// ReadMail 读取邮件
func (ms *MailService) ReadMail(ctx context.Context, req *proto.BaseRequest) (*proto.BaseResponse, error) {
	// TODO: 实现读取邮件逻辑
	return &proto.BaseResponse{
		Header: req.Header,
		Code:   0,
		Msg:    "mail read",
	}, nil
}

// ClaimRewards 领取奖励
func (ms *MailService) ClaimRewards(ctx context.Context, req *proto.BaseRequest) (*proto.BaseResponse, error) {
	// TODO: 实现领取奖励逻辑
	return &proto.BaseResponse{
		Header: req.Header,
		Code:   0,
		Msg:    "rewards claimed",
	}, nil
}

// DeleteMail 删除邮件
func (ms *MailService) DeleteMail(ctx context.Context, req *proto.BaseRequest) (*proto.BaseResponse, error) {
	// TODO: 实现删除邮件逻辑
	return &proto.BaseResponse{
		Header: req.Header,
		Code:   0,
		Msg:    "mail deleted",
	}, nil
}

// SendMail 发送邮件
func (ms *MailService) SendMail(ctx context.Context, req *proto.BaseRequest) (*proto.BaseResponse, error) {
	// TODO: 实现发送邮件逻辑
	return &proto.BaseResponse{
		Header: req.Header,
		Code:   0,
		Msg:    "mail sent",
	}, nil
}
