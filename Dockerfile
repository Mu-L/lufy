# 多阶段构建 Dockerfile
FROM golang:1.21-alpine AS builder

# 安装必要工具
RUN apk add --no-cache git ca-certificates

# 设置工作目录
WORKDIR /app

# 复制 go mod 文件
COPY go.mod go.sum ./

# 下载依赖
RUN go mod download

# 复制源代码
COPY . .

# 构建应用
RUN CGO_ENABLED=0 GOOS=linux go build -a -installsuffix cgo -o lufy cmd/main.go

# 运行阶段
FROM alpine:latest

# 安装 ca-certificates（用于HTTPS请求）
RUN apk --no-cache add ca-certificates tzdata

# 设置时区
ENV TZ=Asia/Shanghai

# 创建非root用户
RUN addgroup -g 1001 -S lufy && \
    adduser -u 1001 -S lufy -G lufy

# 设置工作目录
WORKDIR /app

# 从构建阶段复制二进制文件
COPY --from=builder /app/lufy .

# 复制配置文件
COPY --from=builder /app/config ./config
COPY --from=builder /app/scripts ./scripts

# 设置权限
RUN chown -R lufy:lufy /app && \
    chmod +x lufy && \
    chmod +x scripts/*.sh

# 创建日志目录
RUN mkdir -p logs && chown lufy:lufy logs

# 切换到非root用户
USER lufy

# 暴露端口
EXPOSE 8001 9001 7001

# 健康检查
HEALTHCHECK --interval=30s --timeout=10s --start-period=60s --retries=3 \
    CMD ./lufy -config=config/config.yaml -node=health -id=check || exit 1

# 启动命令
ENTRYPOINT ["./lufy"]
CMD ["-config=config/config.yaml", "-node=gateway", "-id=gateway1"]
