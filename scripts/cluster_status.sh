#!/bin/bash

# Lufy 集群状态检查脚本
set -e

PROJECT_ROOT=$(cd "$(dirname "$0")/.." && pwd)

# 颜色定义
RED='\033[0;31m'
GREEN='\033[0;32m'
YELLOW='\033[0;33m'
BLUE='\033[0;34m'
PURPLE='\033[0;35m'
CYAN='\033[0;36m'
NC='\033[0m'

# 打印状态
print_status() {
    local status=$1
    local message=$2
    local details=$3
    
    case "$status" in
        "HEALTHY")
            echo -e "✅ ${GREEN}${message}${NC}"
            ;;
        "WARNING")
            echo -e "⚠️  ${YELLOW}${message}${NC}"
            ;;
        "ERROR")
            echo -e "❌ ${RED}${message}${NC}"
            ;;
        "INFO")
            echo -e "ℹ️  ${BLUE}${message}${NC}"
            ;;
        "HEADER")
            echo -e "${PURPLE}${message}${NC}"
            ;;
    esac
    
    if [ -n "$details" ]; then
        echo -e "   ${CYAN}${details}${NC}"
    fi
}

# 检查Docker容器状态
check_docker_containers() {
    print_status "HEADER" "📦 Docker 容器状态"
    echo ""
    
    # 获取所有lufy相关容器
    local containers=$(docker ps -a --filter "name=lufy-" --format "table {{.Names}}\t{{.Status}}\t{{.Ports}}" | tail -n +2)
    
    if [ -z "$containers" ]; then
        print_status "WARNING" "未发现Lufy集群容器"
        return 1
    fi
    
    local total_containers=0
    local running_containers=0
    
    while IFS=$'\t' read -r name status ports; do
        total_containers=$((total_containers + 1))
        
        if [[ $status == *"Up"* ]]; then
            print_status "HEALTHY" "$name" "$status"
            running_containers=$((running_containers + 1))
        else
            print_status "ERROR" "$name" "$status"
        fi
    done <<< "$containers"
    
    echo ""
    local availability=$((running_containers * 100 / total_containers))
    print_status "INFO" "容器可用率: ${availability}% (${running_containers}/${total_containers})"
    
    return $((total_containers - running_containers))
}

# 检查Redis集群状态
check_redis_cluster() {
    print_status "HEADER" "🔴 Redis 集群状态"
    echo ""
    
    # 检查Redis集群基本状态
    if docker exec lufy-redis-cluster-1 redis-cli cluster info >/dev/null 2>&1; then
        local cluster_state=$(docker exec lufy-redis-cluster-1 redis-cli cluster info | grep cluster_state | cut -d: -f2)
        local cluster_slots=$(docker exec lufy-redis-cluster-1 redis-cli cluster info | grep cluster_slots_assigned | cut -d: -f2)
        
        if [ "$cluster_state" = "ok" ]; then
            print_status "HEALTHY" "Redis集群状态正常" "已分配槽位: $cluster_slots/16384"
        else
            print_status "ERROR" "Redis集群状态异常" "状态: $cluster_state"
        fi
        
        # 检查集群节点
        local nodes_info=$(docker exec lufy-redis-cluster-1 redis-cli cluster nodes)
        local master_count=$(echo "$nodes_info" | grep -c "master")
        local slave_count=$(echo "$nodes_info" | grep -c "slave")
        
        print_status "INFO" "集群节点信息" "主节点: $master_count, 从节点: $slave_count"
        
        # 检查每个节点的内存使用
        for i in {1..6}; do
            local container_name="lufy-redis-cluster-$i"
            if docker ps --filter "name=$container_name" --filter "status=running" | grep -q "$container_name"; then
                local memory_usage=$(docker exec "$container_name" redis-cli info memory | grep used_memory_human | cut -d: -f2 | tr -d '\r')
                print_status "INFO" "节点$i 内存使用: $memory_usage"
            fi
        done
        
    else
        print_status "ERROR" "无法连接到Redis集群"
        return 1
    fi
    
    echo ""
}

# 检查MongoDB副本集状态  
check_mongodb_replica() {
    print_status "HEADER" "🍃 MongoDB 副本集状态"
    echo ""
    
    if docker exec lufy-mongodb-rs-1 mongosh --eval "rs.status().ok" --quiet >/dev/null 2>&1; then
        # 获取副本集状态
        local rs_status=$(docker exec lufy-mongodb-rs-1 mongosh --eval "rs.status()" --quiet 2>/dev/null)
        local primary_node=$(echo "$rs_status" | grep -A1 '"stateStr" : "PRIMARY"' | grep '"name"' | cut -d'"' -f4)
        local secondary_count=$(echo "$rs_status" | grep -c '"stateStr" : "SECONDARY"')
        
        print_status "HEALTHY" "MongoDB副本集状态正常" "主节点: $primary_node"
        print_status "INFO" "从节点数量: $secondary_count"
        
        # 检查副本延迟
        for i in {1..3}; do
            local container_name="lufy-mongodb-rs-$i"
            if docker ps --filter "name=$container_name" --filter "status=running" | grep -q "$container_name"; then
                local port=$((27016 + i))
                local lag=$(docker exec "$container_name" mongosh --eval "rs.printSlaveReplicationInfo()" --quiet 2>/dev/null | grep "syncedTo:" || echo "N/A")
                print_status "INFO" "节点$i 同步状态" "端口: $port"
            fi
        done
        
    else
        print_status "ERROR" "MongoDB副本集状态异常"
        return 1
    fi
    
    echo ""
}

# 检查ETCD集群状态
check_etcd_cluster() {
    print_status "HEADER" "⚡ ETCD 集群状态"
    echo ""
    
    local endpoints="http://172.20.3.1:2379,http://172.20.3.2:2379,http://172.20.3.3:2379"
    
    if docker exec lufy-etcd-1 etcdctl endpoint health --endpoints="$endpoints" >/dev/null 2>&1; then
        # 检查集群健康状态
        local health_result=$(docker exec lufy-etcd-1 etcdctl endpoint health --endpoints="$endpoints" 2>/dev/null)
        local healthy_count=$(echo "$health_result" | grep -c "is healthy")
        
        print_status "HEALTHY" "ETCD集群状态正常" "健康节点: $healthy_count/3"
        
        # 检查集群成员
        local member_list=$(docker exec lufy-etcd-1 etcdctl member list --endpoints="$endpoints" 2>/dev/null)
        echo "$member_list" | while read line; do
            if [ -n "$line" ]; then
                local member_id=$(echo "$line" | cut -d',' -f1)
                local member_name=$(echo "$line" | cut -d',' -f2)
                print_status "INFO" "成员: $member_name" "ID: $member_id"
            fi
        done
        
        # 检查存储的服务信息
        local service_count=$(docker exec lufy-etcd-1 etcdctl get /lufy/services/ --prefix --endpoints="$endpoints" 2>/dev/null | wc -l)
        print_status "INFO" "注册的服务数量: $((service_count / 2))"
        
    else
        print_status "ERROR" "ETCD集群连接失败"
        return 1
    fi
    
    echo ""
}

# 检查NSQ集群状态
check_nsq_cluster() {
    print_status "HEADER" "📢 NSQ 集群状态"
    echo ""
    
    # 检查NSQLookupd
    local lookup_healthy=0
    for port in 4161 4163; do
        if curl -s "http://localhost:$port/ping" >/dev/null 2>&1; then
            print_status "HEALTHY" "NSQLookupd:$port 运行正常"
            lookup_healthy=$((lookup_healthy + 1))
        else
            print_status "ERROR" "NSQLookupd:$port 不可用"
        fi
    done
    
    # 检查NSQD
    local nsqd_healthy=0
    for port in 4150 4152 4154; do
        if curl -s "http://localhost:$port/ping" >/dev/null 2>&1; then
            print_status "HEALTHY" "NSQD:$port 运行正常"
            nsqd_healthy=$((nsqd_healthy + 1))
            
            # 获取统计信息
            local stats=$(curl -s "http://localhost:$port/stats" | jq -r '.topics | length' 2>/dev/null || echo "0")
            print_status "INFO" "  主题数量: $stats"
        else
            print_status "ERROR" "NSQD:$port 不可用"
        fi
    done
    
    print_status "INFO" "NSQ集群健康状态" "NSQLookupd: $lookup_healthy/2, NSQD: $nsqd_healthy/3"
    echo ""
}

# 检查Lufy应用服务
check_lufy_services() {
    print_status "HEADER" "🎮 Lufy 应用服务状态"
    echo ""
    
    local services=(
        "lufy-center-cluster:7010"
        "lufy-gateway-cluster-1:7001"
        "lufy-gateway-cluster-2:7002"
    )
    
    local healthy_services=0
    local total_services=${#services[@]}
    
    for service_info in "${services[@]}"; do
        local service_name=$(echo "$service_info" | cut -d: -f1)
        local port=$(echo "$service_info" | cut -d: -f2)
        
        if curl -s "http://localhost:$port/health" >/dev/null 2>&1; then
            local health_data=$(curl -s "http://localhost:$port/health" 2>/dev/null)
            local node_id=$(echo "$health_data" | jq -r .node_id 2>/dev/null || echo "unknown")
            local status=$(echo "$health_data" | jq -r .status 2>/dev/null || echo "unknown")
            
            if [ "$status" = "healthy" ]; then
                print_status "HEALTHY" "$service_name" "节点ID: $node_id"
                healthy_services=$((healthy_services + 1))
                
                # 获取性能指标
                if curl -s "http://localhost:$port/api/metrics" >/dev/null 2>&1; then
                    local metrics=$(curl -s "http://localhost:$port/api/metrics" 2>/dev/null)
                    local cpu=$(echo "$metrics" | jq -r '.system.cpu_percent[0]' 2>/dev/null || echo "N/A")
                    local memory=$(echo "$metrics" | jq -r '.system.memory_percent' 2>/dev/null || echo "N/A")
                    print_status "INFO" "  性能指标" "CPU: ${cpu}%, 内存: ${memory}%"
                fi
            else
                print_status "WARNING" "$service_name" "状态: $status"
            fi
        else
            print_status "ERROR" "$service_name" "健康检查失败"
        fi
    done
    
    local service_availability=$((healthy_services * 100 / total_services))
    print_status "INFO" "服务可用率: ${service_availability}% (${healthy_services}/${total_services})"
    
    echo ""
}

# 检查集群网络连通性
check_cluster_network() {
    print_status "HEADER" "🌐 集群网络连通性"
    echo ""
    
    # 检查容器间网络
    local network_name="lufy_lufy-cluster"
    if docker network ls | grep -q "$network_name"; then
        print_status "HEALTHY" "集群网络存在" "网络: $network_name"
        
        # 检查网络中的容器
        local network_containers=$(docker network inspect "$network_name" | jq -r '.[0].Containers | keys | length' 2>/dev/null || echo "0")
        print_status "INFO" "网络中的容器数量: $network_containers"
        
    else
        print_status "ERROR" "集群网络不存在"
        return 1
    fi
    
    # 检查端口可达性
    local critical_ports=(
        "8001:Gateway-1"
        "8002:Gateway-2" 
        "9010:Center RPC"
        "7001:Gateway-1 Monitor"
        "9090:Prometheus"
    )
    
    for port_info in "${critical_ports[@]}"; do
        local port=$(echo "$port_info" | cut -d: -f1)
        local service=$(echo "$port_info" | cut -d: -f2)
        
        if timeout 2 bash -c "</dev/tcp/localhost/$port" 2>/dev/null; then
            print_status "HEALTHY" "$service 端口可达" "端口: $port"
        else
            print_status "ERROR" "$service 端口不可达" "端口: $port"
        fi
    done
    
    echo ""
}

# 检查集群性能指标
check_cluster_performance() {
    print_status "HEADER" "📊 集群性能指标"
    echo ""
    
    # 系统资源使用
    local total_memory_mb=0
    local total_cpu_percent=0
    local container_count=0
    
    # 检查每个Lufy容器的资源使用
    for container in $(docker ps --filter "name=lufy-" --format "{{.Names}}"); do
        if docker stats --no-stream --format "{{.Container}}\t{{.CPUPerc}}\t{{.MemUsage}}" "$container" >/dev/null 2>&1; then
            local stats=$(docker stats --no-stream --format "{{.CPUPerc}}\t{{.MemUsage}}" "$container" 2>/dev/null)
            local cpu=$(echo "$stats" | cut -f1 | sed 's/%//')
            local memory=$(echo "$stats" | cut -f2 | cut -d'/' -f1 | sed 's/MiB//')
            
            print_status "INFO" "$container" "CPU: ${cpu}%, 内存: ${memory}MB"
            
            # 累计统计
            if [[ $cpu =~ ^[0-9.]+$ ]] && [[ $memory =~ ^[0-9.]+$ ]]; then
                total_cpu_percent=$(echo "$total_cpu_percent + $cpu" | bc 2>/dev/null || echo "$total_cpu_percent")
                total_memory_mb=$(echo "$total_memory_mb + $memory" | bc 2>/dev/null || echo "$total_memory_mb")
                container_count=$((container_count + 1))
            fi
        fi
    done
    
    if [ $container_count -gt 0 ]; then
        print_status "INFO" "集群资源汇总" "总CPU: ${total_cpu_percent}%, 总内存: ${total_memory_mb}MB"
    fi
    
    echo ""
}

# 检查集群业务指标
check_business_metrics() {
    print_status "HEADER" "🎯 业务指标状态"
    echo ""
    
    # 尝试从网关获取业务指标
    if curl -s http://localhost:7001/api/metrics >/dev/null 2>&1; then
        local metrics=$(curl -s http://localhost:7001/api/metrics 2>/dev/null)
        
        # 解析关键业务指标
        local connections=$(echo "$metrics" | jq -r '.connections // 0' 2>/dev/null)
        local actors=$(echo "$metrics" | jq -r '.actor_count // 0' 2>/dev/null)
        
        print_status "INFO" "当前连接数: $connections"
        print_status "INFO" "Actor数量: $actors"
        
        # 检查错误率
        if curl -s http://localhost:9090/api/v1/query >/dev/null 2>&1; then
            print_status "HEALTHY" "Prometheus可访问" "可以查询详细指标"
        fi
        
    else
        print_status "WARNING" "无法获取业务指标" "网关服务可能未启动"
    fi
    
    echo ""
}

# 检查数据库集群连接
check_database_connections() {
    print_status "HEADER" "🗄️ 数据库集群连接"
    echo ""
    
    # 测试Redis集群连接
    if docker exec lufy-redis-cluster-1 redis-cli ping >/dev/null 2>&1; then
        local redis_info=$(docker exec lufy-redis-cluster-1 redis-cli info server | grep redis_version | cut -d: -f2 | tr -d '\r')
        print_status "HEALTHY" "Redis集群可连接" "版本: $redis_info"
        
        # 测试集群操作
        if docker exec lufy-redis-cluster-1 redis-cli set test_key "cluster_test" >/dev/null 2>&1; then
            docker exec lufy-redis-cluster-1 redis-cli del test_key >/dev/null 2>&1
            print_status "HEALTHY" "Redis集群读写正常"
        else
            print_status "WARNING" "Redis集群读写测试失败"
        fi
    else
        print_status "ERROR" "Redis集群连接失败"
    fi
    
    # 测试MongoDB副本集连接
    if docker exec lufy-mongodb-rs-1 mongosh --eval "db.runCommand('ping').ok" --quiet >/dev/null 2>&1; then
        local mongo_version=$(docker exec lufy-mongodb-rs-1 mongosh --eval "db.version()" --quiet 2>/dev/null || echo "unknown")
        print_status "HEALTHY" "MongoDB副本集可连接" "版本: $mongo_version"
        
        # 测试副本集读写
        local write_test=$(docker exec lufy-mongodb-rs-1 mongosh lufy_game --eval "db.test.insertOne({test: 'cluster_test', timestamp: new Date()})" --quiet 2>/dev/null)
        if echo "$write_test" | grep -q "acknowledged.*true"; then
            docker exec lufy-mongodb-rs-1 mongosh lufy_game --eval "db.test.deleteOne({test: 'cluster_test'})" --quiet >/dev/null 2>&1
            print_status "HEALTHY" "MongoDB副本集读写正常"
        else
            print_status "WARNING" "MongoDB副本集写入测试失败"
        fi
    else
        print_status "ERROR" "MongoDB副本集连接失败"
    fi
    
    echo ""
}

# 生成集群状态报告
generate_cluster_report() {
    local timestamp=$(date '+%Y-%m-%d %H:%M:%S')
    local report_file="$PROJECT_ROOT/reports/cluster_status_$(date +%Y%m%d_%H%M%S).json"
    
    mkdir -p "$PROJECT_ROOT/reports"
    
    print_status "INFO" "生成集群状态报告..."
    
    # 收集所有状态信息
    local report_data='{"timestamp":"'$timestamp'","cluster_status":{}}'
    
    # 添加容器状态
    local container_data=$(docker ps -a --filter "name=lufy-" --format '{"name":"{{.Names}}","status":"{{.Status}}","image":"{{.Image}}"}' | jq -s .)
    report_data=$(echo "$report_data" | jq ".cluster_status.containers = $container_data")
    
    # 添加Redis集群状态
    if docker exec lufy-redis-cluster-1 redis-cli cluster info >/dev/null 2>&1; then
        local redis_info=$(docker exec lufy-redis-cluster-1 redis-cli cluster info 2>/dev/null | awk -F: 'BEGIN{print "{"} {printf "\"%s\":\"%s\",", $1, $2} END{print "}"}' | sed 's/,}/}/')
        report_data=$(echo "$report_data" | jq ".cluster_status.redis = $redis_info")
    fi
    
    # 保存报告
    echo "$report_data" | jq . > "$report_file"
    print_status "INFO" "报告已保存: $report_file"
}

# 主函数
main() {
    echo "🔍 Lufy 集群状态检查"
    echo "========================================"
    echo "检查时间: $(date '+%Y-%m-%d %H:%M:%S')"
    echo ""
    
    local failed_checks=0
    
    # 执行各项检查
    check_docker_containers || failed_checks=$((failed_checks + 1))
    check_redis_cluster || failed_checks=$((failed_checks + 1))
    check_mongodb_replica || failed_checks=$((failed_checks + 1))
    check_etcd_cluster || failed_checks=$((failed_checks + 1))
    check_nsq_cluster || failed_checks=$((failed_checks + 1))
    check_lufy_services || failed_checks=$((failed_checks + 1))
    check_cluster_network || failed_checks=$((failed_checks + 1))
    check_cluster_performance || failed_checks=$((failed_checks + 1))
    check_business_metrics || failed_checks=$((failed_checks + 1))
    check_database_connections || failed_checks=$((failed_checks + 1))
    
    # 生成状态总结
    echo ""
    print_status "HEADER" "📋 集群状态总结"
    echo ""
    
    if [ $failed_checks -eq 0 ]; then
        print_status "HEALTHY" "🎉 集群状态完全正常！"
        echo ""
        echo "🚀 集群运行状态:"
        echo "  - 所有组件运行正常"
        echo "  - 数据库集群健康"
        echo "  - 应用服务可用"
        echo "  - 网络连通正常"
    elif [ $failed_checks -le 2 ]; then
        print_status "WARNING" "⚠️ 集群基本正常，有$failed_checks 个组件需要关注"
    else
        print_status "ERROR" "❌ 集群状态异常，有$failed_checks 个组件失败"
        echo ""
        echo "🔧 建议的修复步骤:"
        echo "  1. 检查Docker容器状态: docker-compose -f docker-compose.cluster.yml ps"
        echo "  2. 查看容器日志: docker-compose -f docker-compose.cluster.yml logs [service]"
        echo "  3. 重启异常服务: docker-compose -f docker-compose.cluster.yml restart [service]"
        echo "  4. 完全重启集群: ./scripts/restart_cluster.sh"
    fi
    
    echo ""
    echo "🛠️ 有用的命令:"
    echo "  ./scripts/cluster_scale.sh     # 集群扩缩容"
    echo "  ./scripts/cluster_backup.sh    # 集群数据备份"
    echo "  go run tools/performance_analyzer.go collect  # 性能分析"
    echo "  docker-compose -f docker-compose.cluster.yml logs -f  # 实时日志"
    
    return $failed_checks
}

# 实时监控模式
watch_mode() {
    while true; do
        clear
        main
        echo ""
        echo "每30秒刷新一次，按Ctrl+C退出..."
        sleep 30
    done
}

# 执行检查
case "${1:-status}" in
    "status")
        main
        exit $?
        ;;
    "watch")
        watch_mode
        ;;
    "report")
        main
        generate_cluster_report
        ;;
    "help")
        echo "Lufy 集群状态检查脚本"
        echo ""
        echo "用法: $0 [模式]"
        echo ""
        echo "模式:"
        echo "  status   检查集群状态（默认）"
        echo "  watch    实时监控模式"
        echo "  report   生成详细报告"
        echo "  help     显示帮助信息"
        ;;
    *)
        echo "未知模式: $1"
        echo "使用 '$0 help' 查看帮助信息"
        exit 1
        ;;
esac
