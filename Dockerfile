# 多阶段构建

# 阶段1: 构建前端
FROM node:22-alpine AS frontend-builder
WORKDIR /app/frontend
COPY server/frontend/package*.json ./
RUN npm ci
COPY server/frontend/ ./
RUN npm run build

# 阶段2: 构建后端
FROM golang:1.22-alpine AS backend-builder
WORKDIR /app
COPY go.work go.work.sum ./
COPY shared/ ./shared/
COPY server/go.mod server/go.sum ./server/
COPY agent/go.mod agent/go.sum ./agent/
RUN go mod download
COPY shared/ ./shared/
COPY server/ ./server/
COPY agent/ ./agent/
# 复制前端构建产物
COPY --from=frontend-builder /app/web ./server/web
RUN CGO_ENABLED=0 GOOS=linux GOARCH=amd64 go build -o /bin/probe-server ./server/cmd/server
RUN CGO_ENABLED=0 GOOS=linux GOARCH=amd64 go build -o /bin/probe-agent ./agent/cmd/agent

# 阶段3: 运行时
FROM alpine:3.20
RUN apk --no-cache add ca-certificates tzdata && \
    adduser -D -h /app probe

WORKDIR /app
COPY --from=backend-builder /bin/probe-server /usr/local/bin/probe-server
COPY --from=backend-builder /bin/probe-agent /usr/local/bin/probe-agent

# 创建数据目录
RUN mkdir -p /app/data /app/certs && chown -R probe:probe /app

USER probe
EXPOSE 443

VOLUME ["/app/data", "/app/certs"]

ENTRYPOINT ["probe-server"]
CMD ["--data-dir", "/app/data", "--cert-dir", "/app/certs", "--listen", ":443"]
