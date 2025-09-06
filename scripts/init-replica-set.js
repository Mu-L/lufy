// MongoDB 副本集初始化脚本

// 等待服务器启动
sleep(5000);

print('开始初始化MongoDB副本集...');

try {
    // 检查副本集状态
    var status = rs.status();
    print('副本集已经初始化');
} catch (e) {
    print('初始化副本集配置...');
    
    // 初始化副本集
    var config = {
        _id: "rs0",
        version: 1,
        members: [
            {
                _id: 0,
                host: "172.20.2.1:27017",
                priority: 3,
                votes: 1
            },
            {
                _id: 1,
                host: "172.20.2.2:27017", 
                priority: 2,
                votes: 1
            },
            {
                _id: 2,
                host: "172.20.2.3:27017",
                priority: 1,
                votes: 1
            }
        ]
    };
    
    var result = rs.initiate(config);
    print('副本集初始化结果:', JSON.stringify(result));
    
    if (result.ok) {
        print('副本集初始化成功');
        
        // 等待选举完成
        sleep(10000);
        
        // 创建管理员用户
        print('创建管理员用户...');
        var adminResult = db.getSiblingDB('admin').createUser({
            user: 'admin',
            pwd: 'password123',
            roles: [
                { role: 'root', db: 'admin' },
                { role: 'clusterAdmin', db: 'admin' }
            ]
        });
        print('管理员用户创建结果:', JSON.stringify(adminResult));
        
        // 创建应用用户
        print('创建应用用户...');
        var appResult = db.getSiblingDB('lufy_game').createUser({
            user: 'lufy_user',
            pwd: 'lufy_password123',
            roles: [
                { role: 'readWrite', db: 'lufy_game' }
            ]
        });
        print('应用用户创建结果:', JSON.stringify(appResult));
        
        // 创建基础集合和索引
        print('创建基础集合...');
        var gameDB = db.getSiblingDB('lufy_game');
        
        // 用户集合
        gameDB.users.createIndex({ "user_id": 1 }, { unique: true });
        gameDB.users.createIndex({ "username": 1 }, { unique: true });
        gameDB.users.createIndex({ "email": 1 });
        print('用户集合索引创建完成');
        
        // 好友集合
        gameDB.friends.createIndex({ "user_id": 1, "friend_id": 1 });
        gameDB.friends.createIndex({ "user_id": 1 });
        gameDB.friends.createIndex({ "friend_id": 1 });
        print('好友集合索引创建完成');
        
        // 邮件集合
        gameDB.mails.createIndex({ "mail_id": 1 });
        gameDB.mails.createIndex({ "to_user_id": 1 });
        gameDB.mails.createIndex({ "expire_at": 1 });
        print('邮件集合索引创建完成');
        
        // 游戏记录集合
        gameDB.game_records.createIndex({ "game_id": 1 });
        gameDB.game_records.createIndex({ "room_id": 1 });
        gameDB.game_records.createIndex({ "players.user_id": 1 });
        gameDB.game_records.createIndex({ "created_at": -1 });
        print('游戏记录集合索引创建完成');
        
        // 设置副本集读偏好
        print('配置读偏好...');
        rs.secondaryOk();
        
        print('MongoDB副本集初始化完成！');
        
    } else {
        print('副本集初始化失败:', JSON.stringify(result));
    }
}

// 显示副本集状态
print('\n当前副本集状态:');
try {
    var status = rs.status();
    print('副本集名称:', status.set);
    print('成员数量:', status.members.length);
    
    status.members.forEach(function(member) {
        print('节点', member._id, ':', member.name, '-', member.stateStr);
    });
} catch (e) {
    print('无法获取副本集状态:', e);
}

print('副本集初始化脚本执行完成');
