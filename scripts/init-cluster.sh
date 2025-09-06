#!/bin/bash

# Lufy 集群初始化脚本
set -e

echo "🚀 初始化 Lufy 游戏服务器集群..."

# 等待基础设施服务启动
echo "等待基础设施服务启动..."
sleep 30

# 1. 初始化Redis集群
echo "📍 初始化Redis集群..."
redis-cli --cluster create \
  172.20.1.1:6379 172.20.1.2:6379 172.20.1.3:6379 \
  172.20.1.4:6379 172.20.1.5:6379 172.20.1.6:6379 \
  --cluster-replicas 1 --cluster-yes

if [ $? -eq 0 ]; then
    echo "✅ Redis集群初始化成功"
else
    echo "❌ Redis集群初始化失败"
    exit 1
fi

# 2. 验证MongoDB副本集
echo "📍 验证MongoDB副本集..."
mongosh --host 172.20.2.1:27017 --eval "
try {
    var status = rs.status();
    if (status.ok) {
        print('✅ MongoDB副本集状态正常');
        print('主节点:', status.members.find(m => m.stateStr === 'PRIMARY').name);
        print('从节点数:', status.members.filter(m => m.stateStr === 'SECONDARY').length);
    } else {
        print('❌ MongoDB副本集状态异常');
        exit(1);
    }
} catch (e) {
    print('❌ MongoDB副本集检查失败:', e);
    exit(1);
}
"

# 3. 验证ETCD集群
echo "📍 验证ETCD集群..."
etcdctl --endpoints=172.20.3.1:2379,172.20.3.2:2379,172.20.3.3:2379 endpoint health

if [ $? -eq 0 ]; then
    echo "✅ ETCD集群健康检查通过"
    
    # 设置集群配置键
    etcdctl --endpoints=172.20.3.1:2379,172.20.3.2:2379,172.20.3.3:2379 put /lufy/cluster/status "initialized"
    etcdctl --endpoints=172.20.3.1:2379,172.20.3.2:2379,172.20.3.3:2379 put /lufy/cluster/version "1.0.0"
    etcdctl --endpoints=172.20.3.1:2379,172.20.3.2:2379,172.20.3.3:2379 put /lufy/cluster/init_time "$(date -u +%Y-%m-%dT%H:%M:%SZ)"
    
else
    echo "❌ ETCD集群健康检查失败"
    exit 1
fi

# 4. 验证NSQ集群
echo "📍 验证NSQ集群..."
nsq_healthy=0

# 检查NSQLookupd
for addr in "172.20.4.1:4161" "172.20.4.2:4161"; do
    if curl -s "http://$addr/ping" >/dev/null; then
        echo "✅ NSQLookupd $addr 正常"
        nsq_healthy=$((nsq_healthy + 1))
    else
        echo "❌ NSQLookupd $addr 异常"
    fi
done

# 检查NSQD
for addr in "172.20.4.11:4151" "172.20.4.12:4151" "172.20.4.13:4151"; do
    if curl -s "http://$addr/ping" >/dev/null; then
        echo "✅ NSQD $addr 正常"
        nsq_healthy=$((nsq_healthy + 1))
    else
        echo "❌ NSQD $addr 异常"
    fi
done

if [ $nsq_healthy -ge 3 ]; then
    echo "✅ NSQ集群基本可用 ($nsq_healthy/5 节点正常)"
else
    echo "⚠️ NSQ集群部分异常 ($nsq_healthy/5 节点正常)"
fi

# 5. 初始化应用数据
echo "📍 初始化应用数据..."

# 创建Redis中的基础数据结构
redis-cli -c -h 172.20.1.1 -p 6379 << 'EOF'
# 创建全局配置
HSET lufy:config server_version "1.0.0"
HSET lufy:config cluster_mode "true"
HSET lufy:config max_users "100000"

# 创建游戏配置
HSET lufy:game:config max_rooms "10000"
HSET lufy:game:config room_timeout "1800"
HSET lufy:game:config turn_timeout "75"

# 初始化计数器
SET lufy:counters:user_id 10000
SET lufy:counters:room_id 1000
SET lufy:counters:game_id 1

echo "Redis基础数据初始化完成"
EOF

# MongoDB中创建基础数据
mongosh --host 172.20.2.1:27017 lufy_game << 'EOF'
// 创建系统配置文档
db.system_config.insertOne({
    _id: "cluster_config",
    version: "1.0.0",
    cluster_mode: true,
    init_time: new Date(),
    settings: {
        max_users_per_node: 10000,
        max_rooms_per_node: 1000,
        session_timeout: 7200,
        data_retention_days: 90
    }
});

// 创建管理员账户
db.users.insertOne({
    user_id: 1,
    username: "admin",
    password: "$2a$10$abcdefghijklmnopqrstuvwxyz", // 应该用真实哈希
    nickname: "系统管理员",
    level: 100,
    experience: 999999,
    gold: 999999,
    diamond: 999999,
    status: 0,
    permissions: ["admin", "gm", "super"],
    created_at: new Date(),
    updated_at: new Date()
});

print("MongoDB基础数据初始化完成");
EOF

# 6. 创建NSQ主题
echo "📍 创建NSQ主题..."
nsq_topics=("game_events" "chat_messages" "system_messages" "user_events" "admin_commands")

for topic in "${nsq_topics[@]}"; do
    # 在每个NSQD节点上创建主题
    for addr in "172.20.4.11:4151" "172.20.4.12:4151" "172.20.4.13:4151"; do
        if curl -s "http://$addr/topic/create?topic=$topic" >/dev/null; then
            echo "✅ 主题 $topic 在 $addr 创建成功"
        fi
    done
done

# 7. 等待服务完全启动
echo "📍 等待服务完全启动..."
sleep 15

# 8. 最终验证
echo "📍 执行最终验证..."

# 验证Redis集群
redis_cluster_ok=false
if redis-cli -c -h 172.20.1.1 -p 6379 cluster info | grep -q "cluster_state:ok"; then
    echo "✅ Redis集群验证通过"
    redis_cluster_ok=true
else
    echo "❌ Redis集群验证失败"
fi

# 验证MongoDB副本集
mongodb_rs_ok=false
if mongosh --host 172.20.2.1:27017 --eval "rs.status().ok" --quiet | grep -q "1"; then
    echo "✅ MongoDB副本集验证通过"
    mongodb_rs_ok=true
else
    echo "❌ MongoDB副本集验证失败"
fi

# 验证ETCD集群
etcd_cluster_ok=false
if etcdctl --endpoints=172.20.3.1:2379,172.20.3.2:2379,172.20.3.3:2379 get /lufy/cluster/status | grep -q "initialized"; then
    echo "✅ ETCD集群验证通过"
    etcd_cluster_ok=true
else
    echo "❌ ETCD集群验证失败"
fi

# 生成初始化报告
echo ""
echo "🏁 集群初始化报告"
echo "========================================"
echo "时间: $(date '+%Y-%m-%d %H:%M:%S')"
echo ""

if [ "$redis_cluster_ok" = true ] && [ "$mongodb_rs_ok" = true ] && [ "$etcd_cluster_ok" = true ]; then
    echo "✅ 集群初始化完全成功！"
    
    # 设置初始化完成标记
    etcdctl --endpoints=172.20.3.1:2379,172.20.3.2:2379,172.20.3.3:2379 put /lufy/cluster/init_status "completed"
    
    echo ""
    echo "🎮 集群已就绪，可以开始使用："
    echo "  - 游戏客户端可连接到: localhost:8001, localhost:8002"
    echo "  - 管理面板: http://localhost:7001"
    echo "  - 监控面板: http://localhost:9090"
    echo "  - 数据库管理: http://localhost:8081 (Redis), http://localhost:8082 (MongoDB)"
    
    exit 0
else
    echo "⚠️ 集群初始化部分失败："
    [ "$redis_cluster_ok" = false ] && echo "  - Redis集群初始化失败"
    [ "$mongodb_rs_ok" = false ] && echo "  - MongoDB副本集初始化失败"
    [ "$etcd_cluster_ok" = false ] && echo "  - ETCD集群初始化失败"
    
    echo ""
    echo "🔧 请检查日志并重试初始化"
    
    exit 1
fi
