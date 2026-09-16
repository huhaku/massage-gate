# 构建阶段
FROM golang:1.26-alpine AS builder

WORKDIR /app

# 安装依赖
RUN apk add --no-cache git nodejs npm

# 复制 go.mod 和 go.sum
COPY go.mod go.sum ./
RUN go mod download

# 复制源码
COPY . .

# 构建前端
WORKDIR /app/web-src
RUN npm install && npm run build

# 构建后端
WORKDIR /app
RUN cp -r web-src/dist internal/web/ && \
    CGO_ENABLED=0 GOOS=linux go build -o massage-gate ./cmd/server

# 运行阶段
FROM alpine:3.19

WORKDIR /app

# 安装 ca-certificates (HTTPS 请求需要)
RUN apk --no-cache add ca-certificates tzdata

# 从构建阶段复制二进制文件
COPY --from=builder /app/massage-gate .

# 创建数据目录
RUN mkdir -p /app/data

# 暴露端口
EXPOSE 8080

# 设置时区 (可通过环境变量覆盖)
ENV TZ=Asia/Shanghai

# 启动服务
ENTRYPOINT ["./massage-gate"]
CMD ["-data", "/app/data"]