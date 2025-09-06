package database

import (
	"context"
	"fmt"
	"strconv"
	"strings"
	"time"

	"go.mongodb.org/mongo-driver/bson"
	"go.mongodb.org/mongo-driver/bson/primitive"
	"go.mongodb.org/mongo-driver/mongo"
	"go.mongodb.org/mongo-driver/mongo/options"

	"lufy/internal/logger"
)

// MongoConfig MongoDB配置
type MongoConfig struct {
	// 单机模式
	URI      string `yaml:"uri"`
	Database string `yaml:"database"`

	// 副本集模式
	ReplicaSet     bool     `yaml:"replica_set"`
	ReplicaSetName string   `yaml:"replica_set_name"`
	Hosts          []string `yaml:"hosts"`
	AuthSource     string   `yaml:"auth_source"`
	Username       string   `yaml:"username"`
	Password       string   `yaml:"password"`

	// 分片模式
	ShardedCluster bool     `yaml:"sharded_cluster"`
	MongosHosts    []string `yaml:"mongos_hosts"`

	// 连接配置
	ConnectTimeout  time.Duration `yaml:"connect_timeout"`
	MaxPoolSize     uint64        `yaml:"max_pool_size"`
	MinPoolSize     uint64        `yaml:"min_pool_size"`
	MaxConnIdleTime time.Duration `yaml:"max_conn_idle_time"`

	// 读写配置
	ReadPreference string `yaml:"read_preference"` // primary, primaryPreferred, secondary, etc.
	WriteConcern   string `yaml:"write_concern"`   // majority, 1, 2, etc.
	ReadConcern    string `yaml:"read_concern"`    // local, available, majority, etc.

	// SSL/TLS配置
	TLSEnabled  bool   `yaml:"tls_enabled"`
	TLSCertFile string `yaml:"tls_cert_file"`
	TLSKeyFile  string `yaml:"tls_key_file"`
	TLSCAFile   string `yaml:"tls_ca_file"`
}

// MongoManager MongoDB管理器
type MongoManager struct {
	client   *mongo.Client
	database *mongo.Database
	config   *MongoConfig
	ctx      context.Context
	mode     string // "single", "replica_set", "sharded"
}

// NewMongoManager 创建MongoDB管理器
func NewMongoManager(config *MongoConfig) (*MongoManager, error) {
	ctx := context.Background()

	manager := &MongoManager{
		config: config,
		ctx:    ctx,
	}

	var clientOptions *options.ClientOptions
	var err error

	// 根据配置选择MongoDB模式
	if config.ShardedCluster {
		manager.mode = "sharded"
		clientOptions, err = manager.buildShardedClusterOptions()
	} else if config.ReplicaSet {
		manager.mode = "replica_set"
		clientOptions, err = manager.buildReplicaSetOptions()
	} else {
		manager.mode = "single"
		clientOptions, err = manager.buildSingleOptions()
	}

	if err != nil {
		return nil, fmt.Errorf("failed to build client options: %v", err)
	}

	// 连接MongoDB
	client, err := mongo.Connect(ctx, clientOptions)
	if err != nil {
		return nil, fmt.Errorf("failed to connect to mongodb: %v", err)
	}

	// 测试连接
	if err := client.Ping(ctx, nil); err != nil {
		client.Disconnect(ctx)
		return nil, fmt.Errorf("failed to ping mongodb: %v", err)
	}

	manager.client = client
	manager.database = client.Database(config.Database)

	logger.Infof("MongoDB connected in %s mode", manager.mode)
	return manager, nil
}

// buildSingleOptions 构建单机模式选项
func (mm *MongoManager) buildSingleOptions() (*options.ClientOptions, error) {
	opts := options.Client().
		ApplyURI(mm.config.URI).
		SetConnectTimeout(mm.config.ConnectTimeout).
		SetMaxPoolSize(mm.config.MaxPoolSize).
		SetMinPoolSize(mm.config.MinPoolSize).
		SetMaxConnIdleTime(mm.config.MaxConnIdleTime)

	// 添加认证信息
	if mm.config.Username != "" && mm.config.Password != "" {
		credential := options.Credential{
			Username:   mm.config.Username,
			Password:   mm.config.Password,
			AuthSource: mm.config.AuthSource,
		}
		opts.SetAuth(credential)
	}

	return opts, nil
}

// buildReplicaSetOptions 构建副本集模式选项
func (mm *MongoManager) buildReplicaSetOptions() (*options.ClientOptions, error) {
	if len(mm.config.Hosts) == 0 {
		return nil, fmt.Errorf("replica set hosts not configured")
	}

	if mm.config.ReplicaSetName == "" {
		return nil, fmt.Errorf("replica set name not configured")
	}

	// 构建连接URI
	uri := "mongodb://"
	if mm.config.Username != "" && mm.config.Password != "" {
		uri += fmt.Sprintf("%s:%s@", mm.config.Username, mm.config.Password)
	}
	uri += strings.Join(mm.config.Hosts, ",")
	uri += fmt.Sprintf("/%s?replicaSet=%s", mm.config.Database, mm.config.ReplicaSetName)

	if mm.config.AuthSource != "" {
		uri += fmt.Sprintf("&authSource=%s", mm.config.AuthSource)
	}

	opts := options.Client().
		ApplyURI(uri).
		SetConnectTimeout(mm.config.ConnectTimeout).
		SetMaxPoolSize(mm.config.MaxPoolSize).
		SetMinPoolSize(mm.config.MinPoolSize).
		SetMaxConnIdleTime(mm.config.MaxConnIdleTime).
		SetReplicaSet(mm.config.ReplicaSetName)

	// 设置读偏好
	if mm.config.ReadPreference != "" {
		readPref, err := parseReadPreference(mm.config.ReadPreference)
		if err != nil {
			return nil, fmt.Errorf("invalid read preference: %v", err)
		}
		opts.SetReadPreference(readPref)
	}

	// 设置写关注
	if mm.config.WriteConcern != "" {
		writeConcern, err := parseWriteConcern(mm.config.WriteConcern)
		if err != nil {
			return nil, fmt.Errorf("invalid write concern: %v", err)
		}
		opts.SetWriteConcern(writeConcern)
	}

	return opts, nil
}

// buildShardedClusterOptions 构建分片集群模式选项
func (mm *MongoManager) buildShardedClusterOptions() (*options.ClientOptions, error) {
	if len(mm.config.MongosHosts) == 0 {
		return nil, fmt.Errorf("mongos hosts not configured")
	}

	// 构建连接URI
	uri := "mongodb://"
	if mm.config.Username != "" && mm.config.Password != "" {
		uri += fmt.Sprintf("%s:%s@", mm.config.Username, mm.config.Password)
	}
	uri += strings.Join(mm.config.MongosHosts, ",")
	uri += fmt.Sprintf("/%s", mm.config.Database)

	if mm.config.AuthSource != "" {
		uri += fmt.Sprintf("?authSource=%s", mm.config.AuthSource)
	}

	opts := options.Client().
		ApplyURI(uri).
		SetConnectTimeout(mm.config.ConnectTimeout).
		SetMaxPoolSize(mm.config.MaxPoolSize).
		SetMinPoolSize(mm.config.MinPoolSize).
		SetMaxConnIdleTime(mm.config.MaxConnIdleTime)

	return opts, nil
}

// parseReadPreference 解析读偏好
func parseReadPreference(pref string) (*options.ReadPreference, error) {
	switch pref {
	case "primary":
		return options.Primary(), nil
	case "primaryPreferred":
		return options.PrimaryPreferred(), nil
	case "secondary":
		return options.Secondary(), nil
	case "secondaryPreferred":
		return options.SecondaryPreferred(), nil
	case "nearest":
		return options.Nearest(), nil
	default:
		return nil, fmt.Errorf("unknown read preference: %s", pref)
	}
}

// parseWriteConcern 解析写关注
func parseWriteConcern(concern string) (*options.WriteConcern, error) {
	switch concern {
	case "majority":
		return options.WriteConcern().SetW("majority"), nil
	case "1":
		return options.WriteConcern().SetW(1), nil
	case "2":
		return options.WriteConcern().SetW(2), nil
	case "3":
		return options.WriteConcern().SetW(3), nil
	default:
		// 尝试解析为数字
		if w := parseIntOrDefault(concern, -1); w > 0 {
			return options.WriteConcern().SetW(w), nil
		}
		return nil, fmt.Errorf("unknown write concern: %s", concern)
	}
}

// parseIntOrDefault 解析整数或返回默认值
func parseIntOrDefault(s string, defaultValue int) int {
	if val, err := strconv.Atoi(s); err == nil {
		return val
	}
	return defaultValue
}

// GetDatabase 获取数据库
func (mm *MongoManager) GetDatabase() *mongo.Database {
	return mm.database
}

// GetCollection 获取集合
func (mm *MongoManager) GetCollection(name string) *mongo.Collection {
	return mm.database.Collection(name)
}

// Close 关闭MongoDB连接
func (mm *MongoManager) Close() error {
	return mm.client.Disconnect(mm.ctx)
}

// UserRepository 用户数据仓库
type UserRepository struct {
	collection *mongo.Collection
}

// User 用户模型
type User struct {
	ID          primitive.ObjectID `bson:"_id,omitempty" json:"id"`
	UserID      uint64             `bson:"user_id" json:"user_id"`
	Username    string             `bson:"username" json:"username"`
	Password    string             `bson:"password" json:"password"`
	Nickname    string             `bson:"nickname" json:"nickname"`
	Email       string             `bson:"email,omitempty" json:"email"`
	Phone       string             `bson:"phone,omitempty" json:"phone"`
	Level       int32              `bson:"level" json:"level"`
	Experience  int64              `bson:"experience" json:"experience"`
	Gold        int64              `bson:"gold" json:"gold"`
	Diamond     int64              `bson:"diamond" json:"diamond"`
	Avatar      string             `bson:"avatar,omitempty" json:"avatar"`
	Status      int32              `bson:"status" json:"status"` // 0-正常 1-封禁
	LastLoginIP string             `bson:"last_login_ip" json:"last_login_ip"`
	LastLoginAt time.Time          `bson:"last_login_at" json:"last_login_at"`
	CreatedAt   time.Time          `bson:"created_at" json:"created_at"`
	UpdatedAt   time.Time          `bson:"updated_at" json:"updated_at"`
}

// NewUserRepository 创建用户仓库
func NewUserRepository(mm *MongoManager) *UserRepository {
	collection := mm.GetCollection("users")

	// 创建索引
	indexes := []mongo.IndexModel{
		{
			Keys:    bson.D{{Key: "user_id", Value: 1}},
			Options: options.Index().SetUnique(true),
		},
		{
			Keys:    bson.D{{Key: "username", Value: 1}},
			Options: options.Index().SetUnique(true),
		},
		{
			Keys: bson.D{{Key: "email", Value: 1}},
		},
	}

	collection.Indexes().CreateMany(context.Background(), indexes)

	return &UserRepository{
		collection: collection,
	}
}

// Create 创建用户
func (ur *UserRepository) Create(user *User) error {
	user.CreatedAt = time.Now()
	user.UpdatedAt = time.Now()

	result, err := ur.collection.InsertOne(context.Background(), user)
	if err != nil {
		return fmt.Errorf("failed to create user: %v", err)
	}

	user.ID = result.InsertedID.(primitive.ObjectID)
	return nil
}

// GetByUserID 根据用户ID获取用户
func (ur *UserRepository) GetByUserID(userID uint64) (*User, error) {
	var user User
	err := ur.collection.FindOne(context.Background(), bson.M{"user_id": userID}).Decode(&user)
	if err != nil {
		if err == mongo.ErrNoDocuments {
			return nil, fmt.Errorf("user not found")
		}
		return nil, fmt.Errorf("failed to get user: %v", err)
	}
	return &user, nil
}

// GetByUsername 根据用户名获取用户
func (ur *UserRepository) GetByUsername(username string) (*User, error) {
	var user User
	err := ur.collection.FindOne(context.Background(), bson.M{"username": username}).Decode(&user)
	if err != nil {
		if err == mongo.ErrNoDocuments {
			return nil, fmt.Errorf("user not found")
		}
		return nil, fmt.Errorf("failed to get user: %v", err)
	}
	return &user, nil
}

// Update 更新用户
func (ur *UserRepository) Update(user *User) error {
	user.UpdatedAt = time.Now()

	filter := bson.M{"user_id": user.UserID}
	update := bson.M{"$set": user}

	_, err := ur.collection.UpdateOne(context.Background(), filter, update)
	if err != nil {
		return fmt.Errorf("failed to update user: %v", err)
	}
	return nil
}

// UpdateFields 更新指定字段
func (ur *UserRepository) UpdateFields(userID uint64, fields bson.M) error {
	fields["updated_at"] = time.Now()

	filter := bson.M{"user_id": userID}
	update := bson.M{"$set": fields}

	_, err := ur.collection.UpdateOne(context.Background(), filter, update)
	if err != nil {
		return fmt.Errorf("failed to update user fields: %v", err)
	}
	return nil
}

// Delete 删除用户
func (ur *UserRepository) Delete(userID uint64) error {
	filter := bson.M{"user_id": userID}
	_, err := ur.collection.DeleteOne(context.Background(), filter)
	if err != nil {
		return fmt.Errorf("failed to delete user: %v", err)
	}
	return nil
}

// List 获取用户列表
func (ur *UserRepository) List(offset, limit int64) ([]*User, error) {
	options := options.Find().
		SetSkip(offset).
		SetLimit(limit).
		SetSort(bson.D{{Key: "created_at", Value: -1}})

	cursor, err := ur.collection.Find(context.Background(), bson.M{}, options)
	if err != nil {
		return nil, fmt.Errorf("failed to list users: %v", err)
	}
	defer cursor.Close(context.Background())

	var users []*User
	if err := cursor.All(context.Background(), &users); err != nil {
		return nil, fmt.Errorf("failed to decode users: %v", err)
	}

	return users, nil
}

// FriendRepository 好友关系仓库
type FriendRepository struct {
	collection *mongo.Collection
}

// Friend 好友关系模型
type Friend struct {
	ID        primitive.ObjectID `bson:"_id,omitempty" json:"id"`
	UserID    uint64             `bson:"user_id" json:"user_id"`
	FriendID  uint64             `bson:"friend_id" json:"friend_id"`
	Status    int32              `bson:"status" json:"status"` // 0-待确认 1-已确认 2-已拒绝
	Message   string             `bson:"message,omitempty" json:"message"`
	CreatedAt time.Time          `bson:"created_at" json:"created_at"`
	UpdatedAt time.Time          `bson:"updated_at" json:"updated_at"`
}

// NewFriendRepository 创建好友仓库
func NewFriendRepository(mm *MongoManager) *FriendRepository {
	collection := mm.GetCollection("friends")

	// 创建索引
	indexes := []mongo.IndexModel{
		{
			Keys: bson.D{{Key: "user_id", Value: 1}, {Key: "friend_id", Value: 1}},
		},
		{
			Keys: bson.D{{Key: "user_id", Value: 1}},
		},
		{
			Keys: bson.D{{Key: "friend_id", Value: 1}},
		},
	}

	collection.Indexes().CreateMany(context.Background(), indexes)

	return &FriendRepository{
		collection: collection,
	}
}

// AddFriend 添加好友请求
func (fr *FriendRepository) AddFriend(userID, friendID uint64, message string) error {
	friend := &Friend{
		UserID:    userID,
		FriendID:  friendID,
		Status:    0, // 待确认
		Message:   message,
		CreatedAt: time.Now(),
		UpdatedAt: time.Now(),
	}

	_, err := fr.collection.InsertOne(context.Background(), friend)
	if err != nil {
		return fmt.Errorf("failed to add friend: %v", err)
	}
	return nil
}

// AcceptFriend 接受好友请求
func (fr *FriendRepository) AcceptFriend(userID, friendID uint64) error {
	// 更新请求状态
	filter := bson.M{"user_id": friendID, "friend_id": userID, "status": 0}
	update := bson.M{"$set": bson.M{"status": 1, "updated_at": time.Now()}}

	_, err := fr.collection.UpdateOne(context.Background(), filter, update)
	if err != nil {
		return fmt.Errorf("failed to accept friend request: %v", err)
	}

	// 添加反向关系
	friend := &Friend{
		UserID:    userID,
		FriendID:  friendID,
		Status:    1, // 已确认
		CreatedAt: time.Now(),
		UpdatedAt: time.Now(),
	}

	_, err = fr.collection.InsertOne(context.Background(), friend)
	if err != nil {
		return fmt.Errorf("failed to add reverse friend relation: %v", err)
	}

	return nil
}

// GetFriends 获取好友列表
func (fr *FriendRepository) GetFriends(userID uint64) ([]*Friend, error) {
	filter := bson.M{"user_id": userID, "status": 1}
	cursor, err := fr.collection.Find(context.Background(), filter)
	if err != nil {
		return nil, fmt.Errorf("failed to get friends: %v", err)
	}
	defer cursor.Close(context.Background())

	var friends []*Friend
	if err := cursor.All(context.Background(), &friends); err != nil {
		return nil, fmt.Errorf("failed to decode friends: %v", err)
	}

	return friends, nil
}

// MailRepository 邮件仓库
type MailRepository struct {
	collection *mongo.Collection
}

// Mail 邮件模型
type Mail struct {
	ID         primitive.ObjectID `bson:"_id,omitempty" json:"id"`
	MailID     uint64             `bson:"mail_id" json:"mail_id"`
	ToUserID   uint64             `bson:"to_user_id" json:"to_user_id"`
	FromUserID uint64             `bson:"from_user_id,omitempty" json:"from_user_id"`
	Title      string             `bson:"title" json:"title"`
	Content    string             `bson:"content" json:"content"`
	Rewards    []MailReward       `bson:"rewards,omitempty" json:"rewards"`
	IsRead     bool               `bson:"is_read" json:"is_read"`
	IsClaimed  bool               `bson:"is_claimed" json:"is_claimed"`
	ExpireAt   time.Time          `bson:"expire_at" json:"expire_at"`
	CreatedAt  time.Time          `bson:"created_at" json:"created_at"`
	UpdatedAt  time.Time          `bson:"updated_at" json:"updated_at"`
}

// MailReward 邮件奖励
type MailReward struct {
	Type   int32  `bson:"type" json:"type"`
	ItemID int32  `bson:"item_id" json:"item_id"`
	Count  int64  `bson:"count" json:"count"`
	Name   string `bson:"name,omitempty" json:"name"`
}

// NewMailRepository 创建邮件仓库
func NewMailRepository(mm *MongoManager) *MailRepository {
	collection := mm.GetCollection("mails")

	// 创建索引
	indexes := []mongo.IndexModel{
		{
			Keys: bson.D{{Key: "mail_id", Value: 1}},
		},
		{
			Keys: bson.D{{Key: "to_user_id", Value: 1}},
		},
		{
			Keys: bson.D{{Key: "expire_at", Value: 1}},
		},
	}

	collection.Indexes().CreateMany(context.Background(), indexes)

	return &MailRepository{
		collection: collection,
	}
}

// SendMail 发送邮件
func (mr *MailRepository) SendMail(mail *Mail) error {
	mail.CreatedAt = time.Now()
	mail.UpdatedAt = time.Now()

	result, err := mr.collection.InsertOne(context.Background(), mail)
	if err != nil {
		return fmt.Errorf("failed to send mail: %v", err)
	}

	mail.ID = result.InsertedID.(primitive.ObjectID)
	return nil
}

// GetUserMails 获取用户邮件列表
func (mr *MailRepository) GetUserMails(userID uint64, limit int64) ([]*Mail, error) {
	filter := bson.M{
		"to_user_id": userID,
		"expire_at":  bson.M{"$gt": time.Now()},
	}

	options := options.Find().
		SetLimit(limit).
		SetSort(bson.D{{Key: "created_at", Value: -1}})

	cursor, err := mr.collection.Find(context.Background(), filter, options)
	if err != nil {
		return nil, fmt.Errorf("failed to get user mails: %v", err)
	}
	defer cursor.Close(context.Background())

	var mails []*Mail
	if err := cursor.All(context.Background(), &mails); err != nil {
		return nil, fmt.Errorf("failed to decode mails: %v", err)
	}

	return mails, nil
}

// MarkAsRead 标记邮件为已读
func (mr *MailRepository) MarkAsRead(mailID uint64) error {
	filter := bson.M{"mail_id": mailID}
	update := bson.M{"$set": bson.M{"is_read": true, "updated_at": time.Now()}}

	_, err := mr.collection.UpdateOne(context.Background(), filter, update)
	if err != nil {
		return fmt.Errorf("failed to mark mail as read: %v", err)
	}
	return nil
}

// ClaimRewards 领取邮件奖励
func (mr *MailRepository) ClaimRewards(mailID uint64) error {
	filter := bson.M{"mail_id": mailID}
	update := bson.M{"$set": bson.M{"is_claimed": true, "updated_at": time.Now()}}

	_, err := mr.collection.UpdateOne(context.Background(), filter, update)
	if err != nil {
		return fmt.Errorf("failed to claim rewards: %v", err)
	}
	return nil
}

// GameRecordRepository 游戏记录仓库
type GameRecordRepository struct {
	collection *mongo.Collection
}

// GameRecord 游戏记录模型
type GameRecord struct {
	ID        primitive.ObjectID `bson:"_id,omitempty" json:"id"`
	GameID    uint64             `bson:"game_id" json:"game_id"`
	RoomID    uint64             `bson:"room_id" json:"room_id"`
	GameType  int32              `bson:"game_type" json:"game_type"`
	Players   []GamePlayer       `bson:"players" json:"players"`
	Winner    uint64             `bson:"winner,omitempty" json:"winner"`
	Duration  int32              `bson:"duration" json:"duration"` // 游戏时长（秒）
	Status    int32              `bson:"status" json:"status"`     // 0-进行中 1-已结束 2-异常结束
	GameData  bson.M             `bson:"game_data,omitempty" json:"game_data"`
	CreatedAt time.Time          `bson:"created_at" json:"created_at"`
	UpdatedAt time.Time          `bson:"updated_at" json:"updated_at"`
}

// GamePlayer 游戏玩家信息
type GamePlayer struct {
	UserID   uint64 `bson:"user_id" json:"user_id"`
	Nickname string `bson:"nickname" json:"nickname"`
	Level    int32  `bson:"level" json:"level"`
	Score    int64  `bson:"score" json:"score"`
	Rank     int32  `bson:"rank" json:"rank"`
}

// NewGameRecordRepository 创建游戏记录仓库
func NewGameRecordRepository(mm *MongoManager) *GameRecordRepository {
	collection := mm.GetCollection("game_records")

	// 创建索引
	indexes := []mongo.IndexModel{
		{
			Keys: bson.D{{Key: "game_id", Value: 1}},
		},
		{
			Keys: bson.D{{Key: "room_id", Value: 1}},
		},
		{
			Keys: bson.D{{Key: "players.user_id", Value: 1}},
		},
		{
			Keys: bson.D{{Key: "created_at", Value: -1}},
		},
	}

	collection.Indexes().CreateMany(context.Background(), indexes)

	return &GameRecordRepository{
		collection: collection,
	}
}

// CreateRecord 创建游戏记录
func (grr *GameRecordRepository) CreateRecord(record *GameRecord) error {
	record.CreatedAt = time.Now()
	record.UpdatedAt = time.Now()

	result, err := grr.collection.InsertOne(context.Background(), record)
	if err != nil {
		return fmt.Errorf("failed to create game record: %v", err)
	}

	record.ID = result.InsertedID.(primitive.ObjectID)
	return nil
}

// UpdateRecord 更新游戏记录
func (grr *GameRecordRepository) UpdateRecord(record *GameRecord) error {
	record.UpdatedAt = time.Now()

	filter := bson.M{"game_id": record.GameID}
	update := bson.M{"$set": record}

	_, err := grr.collection.UpdateOne(context.Background(), filter, update)
	if err != nil {
		return fmt.Errorf("failed to update game record: %v", err)
	}
	return nil
}

// GetUserGameRecords 获取用户游戏记录
func (grr *GameRecordRepository) GetUserGameRecords(userID uint64, limit int64) ([]*GameRecord, error) {
	filter := bson.M{"players.user_id": userID}
	options := options.Find().
		SetLimit(limit).
		SetSort(bson.D{{Key: "created_at", Value: -1}})

	cursor, err := grr.collection.Find(context.Background(), filter, options)
	if err != nil {
		return nil, fmt.Errorf("failed to get user game records: %v", err)
	}
	defer cursor.Close(context.Background())

	var records []*GameRecord
	if err := cursor.All(context.Background(), &records); err != nil {
		return nil, fmt.Errorf("failed to decode game records: %v", err)
	}

	return records, nil
}
