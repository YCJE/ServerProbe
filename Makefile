.PHONY: all server agent frontend test clean

VERSION := $(shell cat VERSION)
GO := go

all: server agent

# 构建 Server
server: frontend
	cd server && $(GO) build -o ../bin/probe-server ./cmd/server

# 构建 Agent
agent:
	cd agent && $(GO) build -o ../bin/probe-agent ./cmd/agent

# 构建前端
frontend:
	cd server/frontend && npm run build

# 运行所有测试
test:
	cd shared && $(GO) test ./...
	cd server && $(GO) test ./...
	cd agent && $(GO) test ./...

# 运行测试并生成覆盖率
test-coverage:
	cd shared && $(GO) test -coverprofile=coverage.out ./...
	cd server && $(GO) test -coverprofile=coverage.out ./...
	cd agent && $(GO) test -coverprofile=coverage.out ./...

# 清理构建产物
clean:
	rm -rf bin/ server/web/ coverage.out

# 交叉编译 Agent
agent-linux-amd64:
	cd agent && GOOS=linux GOARCH=amd64 $(GO) build -o ../bin/probe-agent-linux-amd64 ./cmd/agent

agent-linux-arm64:
	cd agent && GOOS=linux GOARCH=arm64 $(GO) build -o ../bin/probe-agent-linux-arm64 ./cmd/agent

agent-windows-amd64:
	cd agent && GOOS=windows GOARCH=amd64 $(GO) build -o ../bin/probe-agent-windows-amd64.exe ./cmd/agent

# 交叉编译 Server
server-linux-amd64: frontend
	cd server && GOOS=linux GOARCH=amd64 $(GO) build -o ../bin/probe-server-linux-amd64 ./cmd/server

server-linux-arm64: frontend
	cd server && GOOS=linux GOARCH=arm64 $(GO) build -o ../bin/probe-server-linux-arm64 ./cmd/server
