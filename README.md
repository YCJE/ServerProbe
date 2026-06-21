# Server Probe - 服务器探针监控系统

> 安全优先、只读架构的服务器监控探针系统

[![Go Version](https://img.shields.io/badge/Go-1.23+-00ADD8)](https://go.dev/)
[![License](https://img.shields.io/badge/License-MIT-blue)](LICENSE)

## 特性

- **只读架构**: Agent 仅采集系统指标,不接收任何控制指令
- **强制 TLS**: 全程加密通信,拒绝明文连接
- **非 root 运行**: Agent 以 `probe` 用户运行,最小权限
- **无远程执行**: Agent 不包含任何命令执行/终端/文件操作能力
- **SSRF 防护**: Webhook 通知内置 SSRF 防护层
- **单管理员**: 无多租户攻击面,JWT + HttpOnly Cookie
- **实时监控**: CPU/内存/磁盘/网络,3 秒粒度
- **网络探测**: ICMP/TCP/HTTP Ping,自动降级
- **告警通知**: Webhook/Telegram/Email,状态机去重
- **单二进制部署**: 前端内嵌,一个文件即可运行

## 架构

```
┌──────────────┐     WSS (TLS)     ┌──────────────┐     HTTPS      ┌────────────┐
│   Agent      │ ◄──────────────► │   Server     │ ◄────────────► │  Browser   │
│  (Collector) │   上报 + 心跳      │ (Backend +   │   JWT Cookie   │  (Panel)   │
│              │                   │   Frontend)  │                │            │
└──────────────┘                   └──────────────┘                └────────────┘
```

- **Agent**: 部署在被监控服务器,采集系统指标并上报
- **Server**: 接收数据,提供 Web 面板和 API,内嵌 React 前端
- **Browser**: 管理员通过浏览器访问监控面板

## 快速开始

### 安装 Server

```bash
# 一键安装
curl -fsSL https://raw.githubusercontent.com/YCJE/ServerProbe/main/scripts/install-server.sh | bash -s -- --port 8443

# 或手动安装
# 1. 下载二进制
wget https://github.com/YCJE/ServerProbe/releases/latest/download/probe-server-linux-amd64
chmod +x probe-server-linux-amd64
mv probe-server-linux-amd64 /usr/local/bin/probe-server

# 2. 创建配置
mkdir -p /etc/probe-server /var/lib/probe-server
cat > /etc/probe-server/config.yml << 'EOF'
listen: ":8443"
data_dir: "/var/lib/probe-server"
jwt_secret: "your-random-secret"
tls:
  auto: true
EOF

# 3. 启动
probe-server --config /etc/probe-server/config.yml
```

首次访问 `https://your-server-ip:8443` 设置管理员账号。

### 安装 Agent

1. 在 Server 面板中生成注册码
2. 在被监控服务器上执行:

```bash
curl -fsSL https://raw.githubusercontent.com/YCJE/ServerProbe/main/scripts/install-agent.sh | bash -s -- --server https://your-server.com:8443 --code ABC123XY
```

### Docker 部署

```bash
docker compose up -d
```

参见 `docker-compose.yml`。

## 配置

### Server 配置 (`/etc/probe-server/config.yml`)

```yaml
listen: ":8443"              # 监听地址
data_dir: "/var/lib/probe-server"  # 数据目录
jwt_secret: "random-secret"  # JWT 签名密钥
tls:
  auto: true                 # 自动生成自签证书
  cert_file: ""              # 或指定证书路径
  key_file: ""               # 或指定私钥路径
aggregation:
  interval: 300              # 聚合间隔 (秒)
  retention_days: 90         # 数据保留天数
ring_buffer:
  size: 3600                 # 环形缓冲大小 (点数)
```

### Agent 配置 (`/etc/probe-agent/config.yml`)

```yaml
server: "https://your-server.com:8443"  # Server 地址 (必须 https://)
register_code: "ABC123XY"               # 注册码 (首次注册用)
report_interval: 3                      # 上报间隔 (秒)
config_sync_interval: 3600              # 配置同步间隔 (秒)
ping_method: "auto"                     # auto / icmp / tcp / http
```

## 使用

### 面板功能

- **仪表盘**: 所有服务器概览,CPU/内存/磁盘/网络/延迟/丢包率
- **服务器详情**: 实时折线图,时间范围切换 (1H/6H/24H/7D/30D)
- **告警管理**: 创建告警规则,查看告警历史
- **通知渠道**: 配置 Webhook/Telegram/Email 通知
- **Ping 探测**: 配置探测目标,查看三网延迟
- **主题切换**: 浅色/深色/跟随系统

### 命令行

```bash
# Server
systemctl start probe-server    # 启动
systemctl stop probe-server     # 停止
systemctl restart probe-server  # 重启
systemctl status probe-server   # 状态
journalctl -u probe-server -f   # 日志

# Agent
systemctl start probe-agent     # 启动
systemctl stop probe-agent      # 停止
systemctl status probe-agent    # 状态
journalctl -u probe-agent -f    # 日志
```

### 升级 Agent

```bash
curl -fsSL https://raw.githubusercontent.com/YCJE/ServerProbe/main/scripts/upgrade-agent.sh | bash
```

## 构建

### 环境要求

- Go 1.23+
- Node.js 20+
- npm

### 从源码构建

```bash
# 构建前端
cd server/frontend
npm install
npm run build  # 输出到 server/web/

# 构建 Server
cd ../..
cd server
go build -o ../bin/probe-server ./cmd/server

# 构建 Agent
cd ../agent
go build -o ../bin/probe-agent ./cmd/agent
```

### 交叉编译

```bash
# Linux amd64
CGO_ENABLED=0 GOOS=linux GOARCH=amd64 go build -o probe-server ./cmd/server

# Linux arm64
CGO_ENABLED=0 GOOS=linux GOARCH=arm64 go build -o probe-server ./cmd/server
```

## 项目结构

```
server_status/
├── agent/                  # Agent 模块
│   ├── cmd/agent/          # 入口
│   └── internal/
│       ├── collector/      # 采集器 (CPU/内存/磁盘/网络/系统/Ping)
│       ├── config/         # 配置同步
│       └── reporter/       # WebSocket 上报
├── server/                 # Server 模块
│   ├── cmd/server/         # 入口
│   ├── frontend/           # React 前端源码
│   ├── web/                # 前端构建产物 (go:embed)
│   ├── web.go              # embed 包
│   └── internal/
│       ├── api/            # HTTP API + WebSocket
│       ├── model/          # GORM 模型
│       ├── pkg/            # 工具包 (auth/ssrf/tls)
│       ├── repository/     # 数据层 (SQLite + RingBuffer)
│       └── service/        # 业务层
├── shared/                 # 共享模型
│   └── model/
├── scripts/                # 安装/升级脚本
├── Dockerfile              # Docker 构建
├── docker-compose.yml      # Docker Compose
├── Makefile                # 构建命令
└── go.work                 # Go workspace
```

## 安全

请查阅 [SECURITY.md](SECURITY.md) 了解安全设计原则和漏洞报告流程。

## FAQ

### Q: 浏览器提示证书不安全?

Server 默认使用自签名 TLS 证书。生产环境建议替换为正式证书:

```yaml
tls:
  auto: false
  cert_file: "/path/to/cert.pem"
  key_file: "/path/to/key.pem"
```

或使用 Let's Encrypt + 反向代理 (Nginx/Caddy)。

### Q: Agent 无法连接 Server?

1. 确认 Server 地址以 `https://` 开头
2. 检查防火墙是否放行端口
3. 查看日志: `journalctl -u probe-agent -f`

### Q: ICMP Ping 不工作?

Agent 默认尝试 ICMP Ping,需要 `CAP_NET_RAW` 权限。安装脚本会自动设置 `setcap`。如果不可用,Agent 会自动降级到 TCP Ping。

### Q: 如何修改上报间隔?

编辑 Agent 配置文件 `/etc/probe-agent/config.yml`,修改 `report_interval` 后重启服务。注意:Server 端会校验上报频率,过快会被拒绝。

### Q: 数据存储在哪里?

- 实时数据: 内存环形缓冲 (最近 1 小时)
- 历史数据: SQLite (`/var/lib/probe-server/data.db`)
- 默认保留 90 天,可在配置中修改

### Q: 支持哪些操作系统?

- Agent: Linux (amd64/arm64/armv7)
- Server: Linux (amd64/arm64/armv7)
- 浏览器: Chrome/Firefox/Safari/Edge 最新版

### Q: 如何备份数据?

```bash
# 停止 Server
systemctl stop probe-server

# 备份 SQLite
cp /var/lib/probe-server/data.db /backup/data-$(date +%Y%m%d).db

# 重启
systemctl start probe-server
```

## License

MIT
