package server

import (
	"context"
	"encoding/json"
	"fmt"
	"net/http"
	_ "net/http/pprof"
	"reflect"
	"time"

	"lufy/internal/gameplay"
	"lufy/internal/hotreload"
	"lufy/internal/i18n"
	"lufy/internal/logger"
	"lufy/internal/monitoring"
	"lufy/internal/security"
	"lufy/pkg/proto"
)

// EnhancedGameServer 增强版游戏服务器
type EnhancedGameServer struct {
	*BaseServer
	gameplay    *gameplay.GameplayManager
	security    *security.SecurityManager
	monitoring  *monitoring.MonitoringManager
	i18n        *i18n.I18nManager
	hotReload   *hotreload.HotReloadManager
	pprofServer *http.Server
}

// NewEnhancedGameServer 创建增强版游戏服务器
func NewEnhancedGameServer(configFile, nodeID string) *EnhancedGameServer {
	baseServer, err := NewBaseServer(configFile, "game", nodeID)
	if err != nil {
		logger.Fatal(fmt.Sprintf("Failed to create base server: %v", err))
	}

	enhancedServer := &EnhancedGameServer{
		BaseServer: baseServer,
	}

	// 初始化扩展组件
	if err := enhancedServer.initEnhancedComponents(); err != nil {
		logger.Fatal(fmt.Sprintf("Failed to init enhanced components: %v", err))
	}

	// 注册通用服务
	if err := RegisterCommonServices(baseServer); err != nil {
		logger.Fatal(fmt.Sprintf("Failed to register common services: %v", err))
	}

	// 注册增强游戏服务
	enhancedGameService := NewEnhancedGameService(enhancedServer)
	if err := baseServer.rpcServer.RegisterService(enhancedGameService); err != nil {
		logger.Fatal(fmt.Sprintf("Failed to register enhanced game service: %v", err))
	}

	return enhancedServer
}

// initEnhancedComponents 初始化增强组件
func (egs *EnhancedGameServer) initEnhancedComponents() error {
	var err error

	// 初始化安全管理器
	egs.security, err = security.NewSecurityManager()
	if err != nil {
		return fmt.Errorf("failed to init security manager: %v", err)
	}

	// 初始化监控管理器
	monitoringPort := egs.config.Network.HTTPPort
	egs.monitoring, err = monitoring.NewMonitoringManager(egs.nodeID, egs.nodeType, monitoringPort)
	if err != nil {
		return fmt.Errorf("failed to init monitoring manager: %v", err)
	}

	// 初始化国际化管理器
	egs.i18n = i18n.NewI18nManager("en")
	if err := egs.i18n.LoadLanguage("zh-CN"); err != nil {
		logger.Warn(fmt.Sprintf("Failed to load Chinese language: %v", err))
	}
	if err := egs.i18n.LoadLanguage("ja"); err != nil {
		logger.Warn(fmt.Sprintf("Failed to load Japanese language: %v", err))
	}

	// 初始化玩法管理器
	egs.gameplay = gameplay.NewGameplayManager()

	// 注册默认游戏模块
	cardGameModule := gameplay.NewCardGameModule()
	if err := egs.gameplay.RegisterModule(cardGameModule); err != nil {
		logger.Warn(fmt.Sprintf("Failed to register card game module: %v", err))
	}

	// 初始化热更新管理器
	egs.hotReload, err = hotreload.NewHotReloadManager()
	if err != nil {
		return fmt.Errorf("failed to init hot reload manager: %v", err)
	}

	// 注册配置文件热更新
	configParser := &hotreload.YAMLConfigParser{}
	if err := egs.hotReload.RegisterConfig("config/config.yaml", configParser); err != nil {
		logger.Warn(fmt.Sprintf("Failed to register config hot reload: %v", err))
	}

	// 启动pprof服务器
	egs.startPprofServer()

	logger.Info("Enhanced components initialized")
	return nil
}

// startPprofServer 启动pprof服务器
func (egs *EnhancedGameServer) startPprofServer() {
	pprofPort := egs.config.Network.HTTPPort + 1000

	egs.pprofServer = &http.Server{
		Addr: fmt.Sprintf(":%d", pprofPort),
	}

	go func() {
		logger.Info(fmt.Sprintf("pprof server listening on :%d", pprofPort))
		if err := egs.pprofServer.ListenAndServe(); err != nil && err != http.ErrServerClosed {
			logger.Error(fmt.Sprintf("pprof server error: %v", err))
		}
	}()
}

// Start 启动增强版游戏服务器
func (egs *EnhancedGameServer) Start() error {
	// 启动基础服务器
	if err := egs.BaseServer.Start(); err != nil {
		return err
	}

	// 启动监控服务
	if err := egs.monitoring.Start(); err != nil {
		logger.Error(fmt.Sprintf("Failed to start monitoring: %v", err))
	}

	logger.Info(fmt.Sprintf("Enhanced game server %s started", egs.nodeID))
	return nil
}

// Stop 停止增强版游戏服务器
func (egs *EnhancedGameServer) Stop() error {
	// 停止监控服务
	if egs.monitoring != nil {
		egs.monitoring.Stop()
	}

	// 停止pprof服务器
	if egs.pprofServer != nil {
		ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
		egs.pprofServer.Shutdown(ctx)
		cancel()
	}

	// 停止热更新管理器
	if egs.hotReload != nil {
		egs.hotReload.Close()
	}

	// 停止基础服务器
	return egs.BaseServer.Stop()
}

// EnhancedGameService 增强游戏RPC服务
type EnhancedGameService struct {
	server *EnhancedGameServer
}

// NewEnhancedGameService 创建增强游戏服务
func NewEnhancedGameService(server *EnhancedGameServer) *EnhancedGameService {
	return &EnhancedGameService{
		server: server,
	}
}

// GetName 获取服务名称
func (egs *EnhancedGameService) GetName() string {
	return "EnhancedGameService"
}

// RegisterMethods 注册方法
func (egs *EnhancedGameService) RegisterMethods() map[string]reflect.Value {
	methods := make(map[string]reflect.Value)

	// 基础游戏方法
	methods["CreateRoom"] = reflect.ValueOf(egs.CreateRoom)
	methods["JoinRoom"] = reflect.ValueOf(egs.JoinRoom)
	methods["LeaveRoom"] = reflect.ValueOf(egs.LeaveRoom)
	methods["GameAction"] = reflect.ValueOf(egs.GameAction)
	methods["GetRoomState"] = reflect.ValueOf(egs.GetRoomState)

	// 安全相关方法
	methods["ValidateToken"] = reflect.ValueOf(egs.ValidateToken)
	methods["CheckSecurity"] = reflect.ValueOf(egs.CheckSecurity)

	// 监控相关方法
	methods["GetMetrics"] = reflect.ValueOf(egs.GetMetrics)
	methods["GetAlerts"] = reflect.ValueOf(egs.GetAlerts)

	// 热更新方法
	methods["HotReload"] = reflect.ValueOf(egs.HotReload)

	return methods
}

// CreateRoom 创建游戏房间
func (egs *EnhancedGameService) CreateRoom(ctx context.Context, req *proto.BaseRequest) (*proto.BaseResponse, error) {
	// 安全验证
	session, err := egs.validateRequest(req)
	if err != nil {
		return egs.createErrorResponse(req, -1, "security_validation_failed", nil)
	}

	// 限流检查
	if !egs.server.security.CheckIPSecurity(session.IP) {
		return egs.createErrorResponse(req, -2, "rate_limit_exceeded", nil)
	}

	// 创建房间配置
	config := &gameplay.RoomConfig{
		MaxPlayers: 2,
		MinPlayers: 2,
		AutoStart:  true,
		TimeLimit:  30 * time.Minute,
	}

	// 创建房间
	room, err := egs.server.gameplay.CreateRoom("card_game", config)
	if err != nil {
		return egs.createErrorResponse(req, -3, "room_creation_failed", nil)
	}

	// 记录监控指标
	egs.server.monitoring.RecordMessage("create_room")

	// 返回本地化响应
	return egs.createSuccessResponse(req, "success.room_created", map[string]interface{}{
		"room_id": room.ID,
	})
}

// JoinRoom 加入房间
func (egs *EnhancedGameService) JoinRoom(ctx context.Context, req *proto.BaseRequest) (*proto.BaseResponse, error) {
	session, err := egs.validateRequest(req)
	if err != nil {
		return egs.createErrorResponse(req, -1, "security_validation_failed", nil)
	}

	// TODO: 从请求中解析房间ID
	roomID := uint64(1) // 示例

	// 创建玩家对象
	player := &gameplay.Player{
		UserID:   session.UserID,
		Nickname: "Player", // 应该从用户信息中获取
		Level:    1,
		Status:   gameplay.PlayerStatusWaiting,
	}

	// 加入房间
	if err := egs.server.gameplay.JoinRoom(roomID, player); err != nil {
		return egs.createErrorResponse(req, -2, "join_room_failed", nil)
	}

	egs.server.monitoring.RecordMessage("join_room")

	return egs.createSuccessResponse(req, "success.room_joined", map[string]interface{}{
		"room_id": roomID,
	})
}

// LeaveRoom 离开房间
func (egs *EnhancedGameService) LeaveRoom(ctx context.Context, req *proto.BaseRequest) (*proto.BaseResponse, error) {
	session, err := egs.validateRequest(req)
	if err != nil {
		return egs.createErrorResponse(req, -1, "security_validation_failed", nil)
	}

	// TODO: 从请求中解析房间ID
	roomID := uint64(1) // 示例

	if err := egs.server.gameplay.LeaveRoom(roomID, session.UserID); err != nil {
		return egs.createErrorResponse(req, -2, "leave_room_failed", nil)
	}

	egs.server.monitoring.RecordMessage("leave_room")

	return egs.createSuccessResponse(req, "success.room_left", nil)
}

// GameAction 处理游戏操作
func (egs *EnhancedGameService) GameAction(ctx context.Context, req *proto.BaseRequest) (*proto.BaseResponse, error) {
	start := time.Now()
	defer func() {
		duration := time.Since(start)
		egs.server.monitoring.RecordRequestDuration("POST", "/game_action", duration)
	}()

	session, err := egs.validateRequest(req)
	if err != nil {
		return egs.createErrorResponse(req, -1, "security_validation_failed", nil)
	}

	// 反作弊检查
	egs.server.security.antiCheat.RecordAction(session.UserID, "game_action", req.Data, 0.1)
	if isCheat, patterns := egs.server.security.antiCheat.CheckCheat(session.UserID); isCheat {
		logger.Warn(fmt.Sprintf("Cheat detected for user %d: %v", session.UserID, patterns))
		return egs.createErrorResponse(req, -2, "cheat_detected", nil)
	}

	// TODO: 从请求中解析游戏操作
	action := &gameplay.GameAction{
		Type:      "play_card",
		PlayerID:  session.UserID,
		Timestamp: time.Now(),
	}

	roomID := uint64(1) // 示例
	result, err := egs.server.gameplay.ProcessAction(roomID, action)
	if err != nil {
		egs.server.monitoring.RecordError("game_action_failed")
		return egs.createErrorResponse(req, -3, "action_failed", nil)
	}

	egs.server.monitoring.RecordMessage("game_action")

	return egs.createSuccessResponse(req, "success.action_processed", map[string]interface{}{
		"result": result,
	})
}

// GetRoomState 获取房间状态
func (egs *EnhancedGameService) GetRoomState(ctx context.Context, req *proto.BaseRequest) (*proto.BaseResponse, error) {
	session, err := egs.validateRequest(req)
	if err != nil {
		return egs.createErrorResponse(req, -1, "security_validation_failed", nil)
	}

	roomID := uint64(1) // 示例
	room, exists := egs.server.gameplay.GetRoom(roomID)
	if !exists {
		return egs.createErrorResponse(req, -2, "room_not_found", nil)
	}

	// 检查玩家权限
	if _, exists := room.GetPlayer(session.UserID); !exists {
		return egs.createErrorResponse(req, -3, "permission_denied", nil)
	}

	return egs.createSuccessResponse(req, "success", map[string]interface{}{
		"room_state": room,
	})
}

// ValidateToken 验证令牌
func (egs *EnhancedGameService) ValidateToken(ctx context.Context, req *proto.BaseRequest) (*proto.BaseResponse, error) {
	tokenString := req.Header.SessionId
	if tokenString == "" {
		return egs.createErrorResponse(req, -1, "error.missing_token", nil)
	}

	session, err := egs.server.security.auth.ValidateSession(tokenString)
	if err != nil {
		return egs.createErrorResponse(req, -2, "error.invalid_token", nil)
	}

	return egs.createSuccessResponse(req, "success.token_valid", map[string]interface{}{
		"user_id": session.UserID,
	})
}

// CheckSecurity 安全检查
func (egs *EnhancedGameService) CheckSecurity(ctx context.Context, req *proto.BaseRequest) (*proto.BaseResponse, error) {
	metrics := egs.server.security.GetSecurityMetrics()

	return egs.createSuccessResponse(req, "success", metrics)
}

// GetMetrics 获取监控指标
func (egs *EnhancedGameService) GetMetrics(ctx context.Context, req *proto.BaseRequest) (*proto.BaseResponse, error) {
	// 验证管理员权限
	session, err := egs.validateRequest(req)
	if err != nil {
		return egs.createErrorResponse(req, -1, "security_validation_failed", nil)
	}

	if !egs.hasPermission(session, "admin") {
		return egs.createErrorResponse(req, -2, "permission_denied", nil)
	}

	// 获取指标数据
	// 这里应该从监控系统获取指标
	metrics := map[string]interface{}{
		"node_id":   egs.nodeID,
		"node_type": egs.nodeType,
		"timestamp": time.Now().Unix(),
	}

	return egs.createSuccessResponse(req, "success", metrics)
}

// GetAlerts 获取告警信息
func (egs *EnhancedGameService) GetAlerts(ctx context.Context, req *proto.BaseRequest) (*proto.BaseResponse, error) {
	session, err := egs.validateRequest(req)
	if err != nil {
		return egs.createErrorResponse(req, -1, "security_validation_failed", nil)
	}

	if !egs.hasPermission(session, "admin") {
		return egs.createErrorResponse(req, -2, "permission_denied", nil)
	}

	// TODO: 从监控系统获取告警信息
	alerts := []interface{}{}

	return egs.createSuccessResponse(req, "success", map[string]interface{}{
		"alerts": alerts,
	})
}

// HotReload 热更新
func (egs *EnhancedGameService) HotReload(ctx context.Context, req *proto.BaseRequest) (*proto.BaseResponse, error) {
	session, err := egs.validateRequest(req)
	if err != nil {
		return egs.createErrorResponse(req, -1, "security_validation_failed", nil)
	}

	if !egs.hasPermission(session, "admin") {
		return egs.createErrorResponse(req, -2, "permission_denied", nil)
	}

	// TODO: 从请求中解析热更新类型和模块
	updateType := "config"      // 示例
	moduleName := "game_config" // 示例

	logger.Info(fmt.Sprintf("Hot reload requested: %s/%s by user %d",
		updateType, moduleName, session.UserID))

	return egs.createSuccessResponse(req, "success.hot_reload", nil)
}

// validateRequest 验证请求
func (egs *EnhancedGameService) validateRequest(req *proto.BaseRequest) (*security.Session, error) {
	// 验证会话
	sessionToken := req.Header.SessionId
	if sessionToken == "" {
		return nil, fmt.Errorf("missing session token")
	}

	session, err := egs.server.security.auth.ValidateSession(sessionToken)
	if err != nil {
		return nil, fmt.Errorf("invalid session: %v", err)
	}

	return session, nil
}

// hasPermission 检查权限
func (egs *EnhancedGameService) hasPermission(session *security.Session, permission string) bool {
	for _, perm := range session.Permissions {
		if perm == permission || perm == "admin" {
			return true
		}
	}
	return false
}

// createSuccessResponse 创建成功响应
func (egs *EnhancedGameService) createSuccessResponse(req *proto.BaseRequest, messageID string, data interface{}) (*proto.BaseResponse, error) {
	// 获取客户端语言
	langCode := egs.detectLanguage(req)

	// 本地化消息
	message := egs.server.i18n.Translate(langCode, messageID, nil)

	response := &proto.BaseResponse{
		Header: req.Header,
		Code:   0,
		Msg:    message,
	}

	if data != nil {
		responseData, err := json.Marshal(data)
		if err != nil {
			return nil, fmt.Errorf("failed to marshal response data: %v", err)
		}
		response.Data = responseData
	}

	return response, nil
}

// createErrorResponse 创建错误响应
func (egs *EnhancedGameService) createErrorResponse(req *proto.BaseRequest, code int32, messageID string, data interface{}) (*proto.BaseResponse, error) {
	// 获取客户端语言
	langCode := egs.detectLanguage(req)

	// 本地化错误消息
	message := egs.server.i18n.Translate(langCode, messageID, nil)

	response := &proto.BaseResponse{
		Header: req.Header,
		Code:   code,
		Msg:    message,
	}

	if data != nil {
		responseData, err := json.Marshal(data)
		if err != nil {
			return response, nil // 忽略数据序列化错误
		}
		response.Data = responseData
	}

	// 记录错误指标
	egs.server.monitoring.RecordError(messageID)

	return response, nil
}

// detectLanguage 检测客户端语言
func (egs *EnhancedGameService) detectLanguage(req *proto.BaseRequest) string {
	// 可以从请求头或用户设置中获取语言偏好
	// 这里简化实现，返回默认语言
	return "en"
}

// SecurityMiddleware 安全中间件
type SecurityMiddleware struct {
	security *security.SecurityManager
}

// NewSecurityMiddleware 创建安全中间件
func NewSecurityMiddleware(security *security.SecurityManager) *SecurityMiddleware {
	return &SecurityMiddleware{
		security: security,
	}
}

// ValidateRequest 验证请求
func (sm *SecurityMiddleware) ValidateRequest(req *proto.BaseRequest, clientIP string) error {
	// IP安全检查
	if err := sm.security.CheckIPSecurity(clientIP); err != nil {
		return fmt.Errorf("IP security check failed: %v", err)
	}

	// 限流检查
	rateLimitKey := fmt.Sprintf("user_%d", req.Header.UserId)
	if !sm.security.rateLimit.CheckLimit(rateLimitKey, 100, time.Minute) {
		return fmt.Errorf("rate limit exceeded")
	}

	// 输入验证
	if err := sm.security.ValidateInput(req); err != nil {
		return fmt.Errorf("input validation failed: %v", err)
	}

	return nil
}
