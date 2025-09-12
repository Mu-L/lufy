package server

import (
	"context"
	"crypto/md5"
	"fmt"
	"reflect"
	"time"

	"github.com/phuhao00/lufy/internal/actor"
	"github.com/phuhao00/lufy/internal/database"
	"github.com/phuhao00/lufy/internal/logger"
	"github.com/phuhao00/lufy/pkg/proto"
)

// LoginServer 登录服务器
type LoginServer struct {
	*BaseServer
	userRepo  *database.UserRepository
	userCache *database.UserCache
}

// NewLoginServer 创建登录服务器
func NewLoginServer(configFile, nodeID string) *LoginServer {
	baseServer, err := NewBaseServer(configFile, "login", nodeID)
	if err != nil {
		logger.Fatal(fmt.Sprintf("Failed to create base server: %v", err))
	}

	loginServer := &LoginServer{
		BaseServer: baseServer,
		userRepo:   database.NewUserRepository(baseServer.mongoManager),
		userCache:  database.NewUserCache(baseServer.redisManager),
	}

	// 注册通用服务
	if err := RegisterCommonServices(baseServer); err != nil {
		logger.Fatal(fmt.Sprintf("Failed to register common services: %v", err))
	}

	// 注册登录服务
	loginService := NewLoginService(loginServer)
	if err := baseServer.rpcServer.RegisterService(loginService); err != nil {
		logger.Fatal(fmt.Sprintf("Failed to register login service: %v", err))
	}

	// 创建登录Actor
	loginActor := NewLoginActor(loginServer)
	if err := baseServer.actorSystem.SpawnActor(loginActor); err != nil {
		logger.Fatal(fmt.Sprintf("Failed to spawn login actor: %v", err))
	}

	return loginServer
}

// LoginService 登录RPC服务
type LoginService struct {
	server *LoginServer
}

// NewLoginService 创建登录服务
func NewLoginService(server *LoginServer) *LoginService {
	return &LoginService{
		server: server,
	}
}

// GetName 获取服务名称
func (ls *LoginService) GetName() string {
	return "LoginService"
}

// RegisterMethods 注册方法
func (ls *LoginService) RegisterMethods() map[string]reflect.Value {
	methods := make(map[string]reflect.Value)

	methods["Login"] = reflect.ValueOf(ls.Login)
	methods["Register"] = reflect.ValueOf(ls.Register)
	methods["Logout"] = reflect.ValueOf(ls.Logout)
	methods["ValidateToken"] = reflect.ValueOf(ls.ValidateToken)
	methods["RefreshToken"] = reflect.ValueOf(ls.RefreshToken)

	return methods
}

// Login 用户登录
func (ls *LoginService) Login(ctx context.Context, req *proto.LoginRequest) (*proto.LoginResponse, error) {
	logger.Info(fmt.Sprintf("User login attempt: %s", req.Username))

	// 验证用户名和密码
	user, err := ls.server.userRepo.GetByUsername(req.Username)
	if err != nil {
		logger.Warn(fmt.Sprintf("User not found: %s", req.Username))
		return nil, fmt.Errorf("invalid username or password")
	}

	// 验证密码
	if !ls.verifyPassword(req.Password, user.Password) {
		logger.Warn(fmt.Sprintf("Password verification failed for user: %s", req.Username))
		return nil, fmt.Errorf("invalid username or password")
	}

	// 检查用户状态
	if user.Status != 0 {
		logger.Warn(fmt.Sprintf("User is banned: %s", req.Username))
		return nil, fmt.Errorf("user is banned")
	}

	// 生成登录令牌
	token := ls.generateToken(user.UserID)

	// 更新用户登录信息
	err = ls.server.userRepo.UpdateFields(user.UserID, map[string]interface{}{
		"last_login_at": time.Now(),
		"last_login_ip": "0.0.0.0", // 实际应该从请求中获取
	})
	if err != nil {
		logger.Error(fmt.Sprintf("Failed to update user login info: %v", err))
	}

	// 缓存用户信息
	ls.server.userCache.SetUserInfo(user.UserID, user)

	// 设置用户会话
	sessionCache := database.NewSessionCache(ls.server.redisManager)
	sessionCache.SetSession(token, user.UserID)

	logger.Info(fmt.Sprintf("User login successful: %s (ID: %d)", req.Username, user.UserID))

	return &proto.LoginResponse{
		UserId:   user.UserID,
		Token:    token,
		Nickname: user.Nickname,
		Level:    user.Level,
		Exp:      user.Experience,
		Gold:     user.Gold,
		Diamond:  user.Diamond,
	}, nil
}

// Register 用户注册
func (ls *LoginService) Register(ctx context.Context, req *proto.LoginRequest) (*proto.LoginResponse, error) {
	logger.Info(fmt.Sprintf("User registration attempt: %s", req.Username))

	// 检查用户名是否已存在
	existingUser, _ := ls.server.userRepo.GetByUsername(req.Username)
	if existingUser != nil {
		return nil, fmt.Errorf("username already exists")
	}

	// 生成用户ID
	userID := uint64(time.Now().UnixNano())

	// 创建新用户
	newUser := &database.User{
		UserID:      userID,
		Username:    req.Username,
		Password:    ls.hashPassword(req.Password),
		Nickname:    req.Username, // 默认昵称为用户名
		Level:       1,
		Experience:  0,
		Gold:        1000, // 初始金币
		Diamond:     100,  // 初始钻石
		Status:      0,    // 正常状态
		LastLoginAt: time.Now(),
		CreatedAt:   time.Now(),
		UpdatedAt:   time.Now(),
	}

	// 保存到数据库
	if err := ls.server.userRepo.Create(newUser); err != nil {
		logger.Error(fmt.Sprintf("Failed to create user: %v", err))
		return nil, fmt.Errorf("failed to create user")
	}

	// 生成登录令牌
	token := ls.generateToken(userID)

	// 缓存用户信息
	ls.server.userCache.SetUserInfo(userID, newUser)

	// 设置用户会话
	sessionCache := database.NewSessionCache(ls.server.redisManager)
	sessionCache.SetSession(token, userID)

	logger.Info(fmt.Sprintf("User registration successful: %s (ID: %d)", req.Username, userID))

	return &proto.LoginResponse{
		UserId:   userID,
		Token:    token,
		Nickname: newUser.Nickname,
		Level:    newUser.Level,
		Exp:      newUser.Experience,
		Gold:     newUser.Gold,
		Diamond:  newUser.Diamond,
	}, nil
}

// Logout 用户登出
func (ls *LoginService) Logout(ctx context.Context, req *proto.BaseRequest) (*proto.BaseResponse, error) {
	userID := req.Header.UserId

	if userID == 0 {
		return &proto.BaseResponse{
			Header: req.Header,
			Code:   -1,
			Msg:    "invalid user id",
		}, nil
	}

	// 清理会话
	sessionID := req.Header.SessionId
	if sessionID != "" {
		sessionCache := database.NewSessionCache(ls.server.redisManager)
		sessionCache.DeleteSession(sessionID)
	}

	// 设置用户离线
	ls.server.userCache.SetUserOffline(userID)

	logger.Info(fmt.Sprintf("User logout: %d", userID))

	return &proto.BaseResponse{
		Header: req.Header,
		Code:   0,
		Msg:    "logout success",
	}, nil
}

// ValidateToken 验证令牌
func (ls *LoginService) ValidateToken(ctx context.Context, req *proto.BaseRequest) (*proto.BaseResponse, error) {
	sessionID := req.Header.SessionId
	if sessionID == "" {
		return &proto.BaseResponse{
			Header: req.Header,
			Code:   -1,
			Msg:    "missing session id",
		}, nil
	}

	// 验证会话
	sessionCache := database.NewSessionCache(ls.server.redisManager)
	userID, err := sessionCache.GetSession(sessionID)
	if err != nil {
		return &proto.BaseResponse{
			Header: req.Header,
			Code:   -2,
			Msg:    "invalid session",
		}, nil
	}

	// 刷新会话
	sessionCache.RefreshSession(sessionID)

	return &proto.BaseResponse{
		Header: req.Header,
		Code:   0,
		Msg:    "token valid",
		Data:   []byte(fmt.Sprintf(`{"user_id":%d}`, userID)),
	}, nil
}

// RefreshToken 刷新令牌
func (ls *LoginService) RefreshToken(ctx context.Context, req *proto.BaseRequest) (*proto.BaseResponse, error) {
	userID := req.Header.UserId

	if userID == 0 {
		return &proto.BaseResponse{
			Header: req.Header,
			Code:   -1,
			Msg:    "invalid user id",
		}, nil
	}

	// 生成新令牌
	newToken := ls.generateToken(userID)

	// 删除旧会话
	oldSessionID := req.Header.SessionId
	if oldSessionID != "" {
		sessionCache := database.NewSessionCache(ls.server.redisManager)
		sessionCache.DeleteSession(oldSessionID)
	}

	// 创建新会话
	sessionCache := database.NewSessionCache(ls.server.redisManager)
	sessionCache.SetSession(newToken, userID)

	return &proto.BaseResponse{
		Header: req.Header,
		Code:   0,
		Msg:    "token refreshed",
		Data:   []byte(fmt.Sprintf(`{"token":"%s"}`, newToken)),
	}, nil
}

// hashPassword 哈希密码
func (ls *LoginService) hashPassword(password string) string {
	hash := md5.Sum([]byte(password + "lufy_game_salt"))
	return fmt.Sprintf("%x", hash)
}

// verifyPassword 验证密码
func (ls *LoginService) verifyPassword(plainPassword, hashedPassword string) bool {
	return ls.hashPassword(plainPassword) == hashedPassword
}

// generateToken 生成令牌
func (ls *LoginService) generateToken(userID uint64) string {
	data := fmt.Sprintf("%d_%d_%s", userID, time.Now().Unix(), "lufy_token_salt")
	hash := md5.Sum([]byte(data))
	return fmt.Sprintf("%x", hash)
}

// LoginActor 登录Actor
type LoginActor struct {
	*actor.BaseActor
	server *LoginServer
}

// NewLoginActor 创建登录Actor
func NewLoginActor(server *LoginServer) *LoginActor {
	baseActor := actor.NewBaseActor("login_actor", "login", 1000)

	return &LoginActor{
		BaseActor: baseActor,
		server:    server,
	}
}

// OnReceive 处理消息
func (la *LoginActor) OnReceive(ctx context.Context, msg actor.Message) error {
	switch msg.GetType() {
	case actor.MSG_TYPE_USER_LOGIN:
		return la.handleUserLogin(msg)
	case actor.MSG_TYPE_USER_LOGOUT:
		return la.handleUserLogout(msg)
	default:
		logger.Debug(fmt.Sprintf("Unknown message type: %s", msg.GetType()))
	}

	return nil
}

// OnStart 启动时处理
func (la *LoginActor) OnStart(ctx context.Context) error {
	logger.Info("Login actor started")
	return nil
}

// OnStop 停止时处理
func (la *LoginActor) OnStop(ctx context.Context) error {
	logger.Info("Login actor stopped")
	return nil
}

// handleUserLogin 处理用户登录
func (la *LoginActor) handleUserLogin(msg actor.Message) error {
	logger.Debug("Handling user login in login actor")
	// 可以在这里处理登录相关的异步逻辑
	// 比如记录登录日志、更新统计信息等
	return nil
}

// handleUserLogout 处理用户登出
func (la *LoginActor) handleUserLogout(msg actor.Message) error {
	logger.Debug("Handling user logout in login actor")
	// 处理登出相关的异步逻辑
	return nil
}
