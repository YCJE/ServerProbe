# 服务器探针系统 - 技术规格文档

> **版本**: v1.0
> **日期**: 2026-06-21
> **状态**: 待审核

---

## 1. 项目概述

### 1.1 项目背景

2026 年 5 月，主流服务器探针 Nezha（哪吒探针）集中爆发 9 个安全漏洞，其中 2 个为严重级别的跨租户 RCE，导致大量部署 Nezha 的服务器被入侵。根因在于多租户授权缺失、默认明文通信、控制通道攻击面过大、Webhook SSRF 以及 Agent 默认 root 运行。

本项目旨在开发一款以安全为核心的服务器探针监控系统，从架构上彻底消除 RCE 攻击面，区别于 Nezha 和 Komari。

### 1.2 产品定位

| 维度 | 定位 |
|------|------|
| 核心卖点 | 安全第一的纯只读服务器监控探针 |
| 差异化 | 从架构上彻底消除 RCE 攻击面（无控制通道） |
| 目标用户 | 个人开发者、小团队（第一版聚焦个人自用几台到十几台，架构预留扩展空间） |
| 参考产品 | Nezha（功能参考）、Komari（轻量参考） |

### 1.3 核心决策汇总

| 决策项 | 选择 | 理由 |
|--------|------|------|
| 安全边界 | 纯只读监控 | 从架构上杜绝 RCE 类漏洞，成为核心差异化卖点 |
| 使用规模 | 先个人自用，预留扩展 | 第一版聚焦安全只读，不背多租户复杂度包袱 |
| 网络探测 | 预置目标 + 自定义目标 | 开箱即用 + 灵活性，配置同步是只读数据不违反安全原则 |
| Agent 权限 | 非 root，自动选择探测方式 | 始终非 root，优先 setcap ICMP，降级 TCP Ping |
| 技术栈 | 全 Go + 前端内嵌 | 单二进制部署，Go 交叉编译多平台，前端 embed 极简部署 |
| 通信协议 | WebSocket + 强制 wss | 强制 TLS 是底线，复用 HTTPS 证书生态，JSON 易调试 |
| 数据存储 | SQLite + 内存环形缓冲 | 零配置，内存缓冲吸收高频写入，聚合后落盘 |
| 告警通知 | 基础告警 + SSRF 防护 + 去重 | 对症防范 Nezha SSRF 漏洞，避免告警风暴 |
| 前端面板 | 标准面板 + 主题/自定义/分享 | 覆盖核心场景 + 个性化体验 |
| 部署支持 | 全平台多架构 | Go 交叉编译成本低，覆盖 VPS/NAS/树莓派/PC |
| 架构方案 | 经典分层架构 | 清晰分层适合按层开发，扩展性恰到好处 |

---

## 2. 核心安全原则

以下 8 条原则是不可妥协的设计红线，所有后续设计必须符合。

### 2.1 安全原则清单

| 编号 | 原则 | 说明 |
|------|------|------|
| S1 | 纯只读架构 | Server 到 Agent 方向不存在任何控制通道。Agent 只采集和上报，Server 不下发任何指令 |
| S2 | 强制 TLS | 所有通信（Agent↔Server、浏览器↔Server）强制加密，不允许关闭。Agent 拒绝连接未启用 TLS 的 Server |
| S3 | 非 root 运行 | Agent 始终以非特权用户运行，通过 setcap 获取最小能力（仅 ICMP），不支持则降级 TCP Ping |
| S4 | 无远程执行 | 代码中不存在任何"执行命令""建立终端会话""操作文件"的功能 |
| S5 | 单管理员 + 强认证 | 第一版单管理员，强密码 + 登录限速 + 可选 TOTP。不引入多租户，避免 BOLA/IDOR 类漏洞 |
| S6 | SSRF 防护 | 所有对外发起 HTTP 请求的功能做内网地址过滤、禁止重定向到内网、限制响应体大小 |
| S7 | 最小权限采集 | Agent 只采集不需要 root 的数据。需要 root 的数据不采集或标注"不可用" |
| S8 | 配置文件权限控制 | Agent 配置文件（含 Token）权限 600，仅属主可读写 |

### 2.2 数据流方向

```
Agent ────采集数据 + 探测结果──────────▶ Server  (主动推送, WS)
Agent ◀───探测目标配置(只读JSON)────────  Server  (Agent 主动拉取, HTTPS GET)
浏览器 ────查看请求─────────────────────▶ Server  (REST API)
浏览器 ◀───实时数据推送────────────────  Server  (WebSocket)
```

Server 永远不会主动向 Agent 发起连接。WebSocket 连接由 Agent 发起，协议中只定义了 Agent→Server 的数据帧和 Server→Agent 的配置响应帧，没有"命令"帧。即使 Server 被攻破，攻击者也无法通过 Server 向 Agent 下发任何东西。

---

## 3. 系统架构

### 3.1 系统全景

系统由三大组件构成：

- **Server（服务端）**：单二进制，内嵌 React 前端。提供 REST API、WebSocket 接入、数据存储与聚合、告警引擎、通知发送
- **Agent（客户端）**：单二进制，非 root 运行。负责系统指标采集、网络探测、数据上报、配置拉取
- **浏览器（前端面板）**：React SPA，内嵌于 Server 二进制。通过 REST API 和 WebSocket 与 Server 交互

### 3.2 技术栈

| 层级 | 技术 | 理由 |
|------|------|------|
| 后端语言 | Go 1.22+ | 单二进制、跨平台、并发强 |
| Web 框架 | Gin | 成熟、性能好、中间件生态丰富 |
| 通信协议 | WebSocket + JSON（强制 wss） | 强制 TLS、复用 HTTPS 证书、JSON 易调试 |
| ORM | GORM | 支持 SQLite/MySQL/PostgreSQL，AutoMigrate 方便 |
| 数据库 | SQLite（内嵌） | 零配置、单文件、适合个人十几台规模 |
| 实时数据 | 内存环形缓冲 | 吸收高频写入，前端读内存快 |
| 前端框架 | React 18 + TypeScript | 生态成熟、组件化适合主题/布局/分享 |
| 前端构建 | Vite | 快速构建，产物供 Go embed |
| UI 库 | shadcn/ui | 基于 Radix UI + Tailwind，可定制性强 |
| 图表库 | ECharts | 性能好、支持实时数据流、图表类型丰富 |
| 状态管理 | Zustand | 轻量、适合中等复杂度 |
| ICMP 库 | pro-bing（prometheus-community） | 活跃维护、统计信息丰富 |

### 3.3 Server 目录结构

```
server/
├── cmd/
│   └── server/
│       └── main.go              # 入口, 初始化配置/数据库/路由, 启动服务
├── internal/
│   ├── api/                     # API层 (HTTP路由处理)
│   │   ├── router.go            # 路由注册
│   │   ├── middleware.go        # 中间件(认证/限速/请求日志)
│   │   ├── handler_auth.go      # 登录/登出/TOTP
│   │   ├── handler_server.go    # 服务器列表/详情
│   │   ├── handler_agent.go     # Agent WebSocket接入/注册/配置下发
│   │   ├── handler_alert.go     # 告警规则CRUD
│   │   ├── handler_notify.go    # 通知渠道CRUD
│   │   ├── handler_config.go    # 探测目标/系统设置
│   │   └── handler_public.go    # 公开分享页
│   ├── service/                 # 业务层
│   │   ├── monitor.go           # 实时数据管理(Agent连接池+环形缓冲)
│   │   ├── alert.go             # 告警引擎(阈值检测+状态机)
│   │   ├── notify.go            # 通知发送(SSRF防护+去重+静默期)
│   │   ├── agent_registry.go    # Agent注册/Token管理/注册码
│   │   └── config_sync.go       # 探测目标配置同步
│   ├── repository/              # 数据层
│   │   ├── sqlite.go            # SQLite连接管理
│   │   ├── repo_agent.go        # Agent元数据CRUD
│   │   ├── repo_alert.go        # 告警规则CRUD
│   │   ├── repo_notify.go       # 通知渠道CRUD
│   │   ├── repo_record.go       # 历史聚合数据CRUD
│   │   └── ringbuffer.go        # 内存环形缓冲(实时数据)
│   ├── model/                   # 数据模型(共享)
│   │   ├── agent.go             # Agent/ServerInfo
│   │   ├── metric.go            # 监控指标结构
│   │   ├── alert.go             # 告警规则模型
│   │   └── notify.go            # 通知渠道模型
│   └── pkg/                     # 工具包
│       ├── auth.go              # JWT/密码哈希/TOTP
│       ├── ssrf.go              # SSRF防护(内网过滤/重定向检测)
│       ├── tls.go               # 强制TLS/证书管理
│       └── embed.go             # 前端静态资源embed
├── frontend/                    # React前端(独立开发, 构建后embed)
│   ├── package.json
│   ├── src/
│   └── dist/                    # 构建产物, 被 embed.go 引用
├── web/                         # 前端构建产物(embed源)
├── go.mod
└── go.sum
```

### 3.4 Agent 目录结构

```
agent/
├── cmd/
│   └── agent/
│       └── main.go              # 入口, 加载配置, 启动各模块
├── internal/
│   ├── collector/               # 采集器组
│   │   ├── cpu.go               # CPU使用率/核心数/型号/负载
│   │   ├── memory.go            # 内存/Swap使用率
│   │   ├── disk.go              # 磁盘分区使用率/IO
│   │   ├── network.go           # 网卡流量/TCP UDP连接数
│   │   ├── system.go            # 运行时间/系统信息/Agent版本
│   │   ├── gpu.go               # GPU使用率(如可用, 非root)
│   │   └── ping.go              # 三网延迟/丢包(ICMP或TCP)
│   ├── reporter/                # 上报器
│   │   ├── ws.go                # WebSocket客户端, 维持长连接
│   │   ├── heartbeat.go         # 心跳维持(每30秒)
│   │   └── upload.go            # 数据打包上报(每3秒)
│   ├── config/                  # 配置拉取器
│   │   └── sync.go              # 定时HTTPS GET拉取探测目标配置
│   └── register/                # 注册器
│       └── register.go          # 首次启动用注册码换取持久Token
├── go.mod
└── go.sum
```

---

## 4. Agent 详细设计

### 4.1 采集项清单与权限要求

| 采集项 | 数据来源 | 是否需要root | 备注 |
|--------|---------|-------------|------|
| CPU使用率 | `/proc/stat` | 否 | 读取即可 |
| CPU型号/核心数 | `/proc/cpuinfo` | 否 | 读取即可 |
| 负载(1/5/15分) | `/proc/loadavg` | 否 | 读取即可 |
| 内存/Swap | `/proc/meminfo` | 否 | 读取即可 |
| 磁盘使用率 | 系统调用(statfs) | 否 | 读取即可 |
| 磁盘IO | `/proc/diskstats` | 否 | 读取即可 |
| 网卡流量 | `/proc/net/dev` | 否 | 读取即可 |
| TCP/UDP连接数 | `/proc/net/tcp`, `/proc/net/udp` | 否 | 读取即可 |
| 进程数 | `/proc` 遍历 | 否 | 只统计数量 |
| 运行时间 | `/proc/uptime` | 否 | 读取即可 |
| 系统信息 | `uname` 系统调用 | 否 | 读取即可 |
| GPU使用率 | nvidia-smi/系统API | 否 | 如可用 |
| ICMP Ping | 原始套接字 | 需`CAP_NET_RAW` | setcap赋予,不支持则降级TCP Ping |
| TCP Ping | TCP连接 | 否 | 降级方案 |
| 温度 | `/sys/class/thermal` | 否 | 如可用 |

### 4.2 一键安装上线流程

```
管理员在面板上                    被控服务器上
┌──────────────┐                ┌──────────────┐
│ 1. 点击"添加  │                │              │
│    服务器"    │                │              │
│ 2. 生成一次性 │                │              │
│    注册码     │                │              │
│ 3. 显示一键   │ ──复制命令──▶  │ 4. 粘贴执行   │
│    安装命令   │                │ 5. 脚本检测架构│
│              │                │ 6. 下载Agent  │
│              │                │ 7. 创建probe用户│
│              │                │ 8. setcap     │
│              │                │ 9. 安装服务    │
│              │                │10. 启动Agent  │
│              │                │11. 注册码换Token│
│ 12. 收到注册  │ ◀──WebSocket──│12. 连接Server │
│    服务器上线 │                │    注册并上报  │
└──────────────┘                └──────────────┘
```

一键命令示例：
```bash
curl -fsSL https://your-server.com/install.sh | bash -s -- --server https://your-server.com --code ABC123XY
```

### 4.3 注册码安全机制

| 属性 | 设计 |
|------|------|
| 有效期 | 生成后 15 分钟内有效，超时自动失效 |
| 使用次数 | 一次性，注册成功后立即失效 |
| 绑定信息 | 注册成功后绑定 Agent 的主机指纹（hostname + CPU ID 哈希），防止 Token 被盗用到其他机器 |
| 数量限制 | 每个管理员最多 5 个未使用的注册码同时存在 |
| 传输安全 | 注册码通过 wss 传输，不裸露在明文 HTTP 中 |

### 4.4 Agent 配置文件

```yaml
# /etc/probe-agent/config.yml (权限: 600, 属主: probe)
server: "https://your-server.com"    # Server地址
token: "persistent-token-xxx"        # 注册后获得,首次安装时为空
# register_code: "ABC123XY"          # 首次安装时有,注册后删除
report_interval: 3                   # 上报间隔(秒)
config_sync_interval: 3600           # 配置拉取间隔(秒)
ping_method: "auto"                  # auto/icmp/tcp, auto=优先ICMP降级TCP
```

### 4.5 Agent 上报数据格式

```json
{
  "type": "report",
  "token": "persistent-token-xxx",
  "timestamp": 1718900000,
  "hostname": "web-server-01",
  "data": {
    "cpu": {
      "usage": 45.2,
      "cores": 4,
      "model": "Intel Xeon E5-2680",
      "load_1": 0.52,
      "load_5": 0.48,
      "load_15": 0.50
    },
    "memory": {
      "total": 8589934592,
      "used": 4294967296,
      "swap_total": 4294967296,
      "swap_used": 0
    },
    "disk": [
      {"device": "/", "total": 53687091200, "used": 26843545600},
      {"device": "/data", "total": 107374182400, "used": 53687091200}
    ],
    "network": {
      "rx_speed": 1048576,
      "tx_speed": 524288,
      "tcp_connections": 128,
      "udp_connections": 16
    },
    "uptime": 86400,
    "process_count": 156
  }
}
```

### 4.6 Ping 探测数据格式

```json
{
  "type": "ping_result",
  "token": "persistent-token-xxx",
  "data": [
    {
      "target": "114.114.114.114",
      "name": "电信",
      "method": "icmp",
      "avg_latency": 12.5,
      "min_latency": 10.2,
      "max_latency": 15.8,
      "jitter": 1.8,
      "loss": 0.0,
      "packets_sent": 10,
      "packets_recv": 10
    }
  ]
}
```

### 4.7 Ping 探测方案

**ICMP Ping 参数**：

| 参数 | 设计值 | 设计理由 |
|------|--------|---------|
| 采样数 | 10 个包 | 10 包的标准误是 5 包的 1/√2 ≈ 71%，统计更稳定 |
| 发包间隔 | 0.5 秒 | 0.5 秒能在 5 秒内完成 10 包 |
| 总超时 | 15 秒 | 10 包×0.5 秒=5 秒，15 秒超时留足余量 |
| DNS 解析 | 预解析排除 | 排除 DNS 时间，只测网络延迟 |
| 特权模式 | 优先 privileged，降级 unprivileged | Linux 3.0+ 支持 unprivileged ICMP |

**延迟和丢包率计算**：

| 指标 | 计算方式 |
|------|---------|
| 平均延迟 | 10 个包的 AvgRtt |
| 最小延迟 | MinRtt |
| 最大延迟 | MaxRtt |
| 抖动(Jitter) | StdDevRtt |
| 丢包率 | `(PacketsSent - PacketsRecv) / PacketsSent × 100%` |

**TCP Ping 参数**（ICMP 不可用时的降级方案）：

| 参数 | 设计值 |
|------|--------|
| 采样数 | 5 次取平均 |
| 超时 | 5 秒/次 |
| 间隔 | 0.5 秒 |
| 丢包率 | 5 次中失败比例 |

**HTTP Ping 参数**：

| 参数 | 设计值 |
|------|--------|
| 采样数 | 3 次取平均 |
| 超时 | 10 秒/次 |
| DNS 解析 | 排除（自定义 DialContext） |
| 状态码判定 | 2xx-3xx 成功 |

**探测调度策略**：Ping 探测独立于常规监控数据上报，默认每 60 秒执行一轮完整探测。探测间隔可在服务端配置。

### 4.8 配置拉取

Agent 每小时通过 HTTPS GET 拉取探测目标配置，这是纯只读的数据同步：

```
Agent ────GET /api/v1/agent/config?token=xxx────────▶ Server
Agent ◀───{"ping_targets": [{"target":"114.114.114.114","name":"电信"}]}─── Server
```

Agent 本地缓存这份配置，按配置执行探测。如果拉取失败，使用上次的缓存配置。

### 4.9 Agent 权限降级流程

```
安装脚本
    │
    ▼
创建probe用户 (useradd -r -s /usr/sbin/nologin probe)
    │
    ▼
下载Agent二进制
    │
    ▼
尝试 setcap cap_net_raw+ep ./agent
    │
    ├─ 成功 ──▶ ICMP Ping (privileged模式), 标记 ping_method=icmp
    │
    └─ 失败 ──▶ 尝试 unprivileged ICMP (Linux 3.0+)
                    │
                    ├─ 成功 ──▶ ICMP Ping (unprivileged模式), 标记 ping_method=icmp_unprivileged
                    │
                    └─ 失败 ──▶ TCP Ping, 标记 ping_method=tcp
    │
    ▼
安装systemd service (User=probe, 无root)
    │
    ▼
启动Agent
```

systemd service 文件：
```ini
[Unit]
Description=Server Probe Agent
After=network.target

[Service]
Type=simple
User=probe
Group=probe
ExecStart=/usr/local/bin/probe-agent --config /etc/probe-agent/config.yml
Restart=always
RestartSec=5
NoNewPrivileges=true
ProtectSystem=strict
ProtectHome=true
PrivateTmp=true
ReadWritePaths=/etc/probe-agent

[Install]
WantedBy=multi-user.target
```

---

## 5. Server 详细设计

### 5.1 REST API 端点

| 方法 | 路径 | 认证 | 功能 | 安全要点 |
|------|------|------|------|---------|
| POST | `/api/v1/auth/login` | 无 | 登录 | 限速5次/分钟, 密码哈希验证 |
| POST | `/api/v1/auth/logout` | JWT | 登出 | 清除Cookie |
| POST | `/api/v1/auth/totp/verify` | JWT | TOTP验证 | 登录第二步 |
| GET | `/api/v1/servers` | JWT | 服务器列表 | - |
| GET | `/api/v1/servers/:id` | JWT | 单台详情(含历史) | - |
| GET | `/api/v1/servers/:id/history` | JWT | 历史趋势数据 | 按时间范围查询 |
| POST | `/api/v1/agent/register` | 注册码 | Agent注册 | 注册码一次性, 15分钟有效 |
| GET | `/api/v1/agent/config` | Agent Token | 拉取探测目标配置 | 只读JSON, 无控制指令 |
| WS | `/api/v1/agent/report` | Agent Token | Agent WebSocket接入 | 仅接受Agent→Server数据帧 |
| GET | `/api/v1/alerts` | JWT | 告警规则列表 | - |
| POST | `/api/v1/alerts` | JWT | 创建告警规则 | 仅管理员 |
| PUT | `/api/v1/alerts/:id` | JWT | 修改告警规则 | 仅管理员 |
| DELETE | `/api/v1/alerts/:id` | JWT | 删除告警规则 | 仅管理员 |
| GET | `/api/v1/notify/channels` | JWT | 通知渠道列表 | - |
| POST | `/api/v1/notify/channels` | JWT | 添加通知渠道 | SSRF校验URL |
| PUT | `/api/v1/notify/channels/:id` | JWT | 修改通知渠道 | SSRF校验URL |
| DELETE | `/api/v1/notify/channels/:id` | JWT | 删除通知渠道 | - |
| GET | `/api/v1/config/ping-targets` | JWT | 探测目标列表 | - |
| POST | `/api/v1/config/ping-targets` | JWT | 添加探测目标 | - |
| PUT | `/api/v1/config/ping-targets/:id` | JWT | 修改探测目标 | - |
| DELETE | `/api/v1/config/ping-targets/:id` | JWT | 删除探测目标 | - |
| GET | `/api/v1/config/settings` | JWT | 系统设置 | - |
| PUT | `/api/v1/config/settings` | JWT | 修改系统设置 | 仅管理员 |
| GET | `/api/v1/agents/tokens` | JWT | Agent Token列表 | - |
| POST | `/api/v1/agents/register-codes` | JWT | 生成注册码 | 最多5个未使用 |
| DELETE | `/api/v1/agents/register-codes/:id` | JWT | 删除注册码 | - |
| GET | `/api/v1/public/:share_id` | 无 | 公开分享页 | 只读, 无敏感信息 |
| GET | `/ws/dashboard` | JWT | 面板实时数据推送 | WebSocket |

### 5.2 WebSocket 消息协议

**Agent → Server 帧**（仅这些类型，无其他）：

```json
{"type": "register", "code": "ABC123XY", "hostname": "...", "os": "..."}
{"type": "report", "token": "xxx", "timestamp": 1718900000, "data": {...}}
{"type": "ping_result", "token": "xxx", "data": [...]}
{"type": "heartbeat", "token": "xxx", "timestamp": 1718900030}
```

**Server → Agent 帧**（仅配置响应，无控制指令）：

```json
{"type": "register_ok", "token": "persistent-token-xxx"}
{"type": "register_fail", "reason": "invalid code"}
{"type": "config_update", "ping_targets": [...], "ping_interval": 60}
{"type": "heartbeat_ack"}
```

Server → Agent 方向只有 4 种帧，全部是数据响应或确认，不存在任何"执行命令""建立会话""操作文件"的帧类型。这是 S1（纯只读架构）和 S4（无远程执行）的协议级保障。

### 5.3 实时数据管理

- **Agent 连接池**：`map[agentID]*Conn`，维护在线状态和心跳超时检测
- **环形缓冲**：每 Agent 一个实例，CPU/内存/磁盘/网络最近 3600 点，Ping 最近 60 点
- **聚合策略**：每 5 分钟将当前实时数据聚合为一个点（平均值/最大值），写入 SQLite
- **历史数据保留**：SQLite 中的聚合数据保留 90 天，超期自动清理
- **离线检测**：Agent 心跳超时（默认 90 秒无心跳）标记为离线，触发离线告警

### 5.4 告警引擎

告警状态机：`OK → PENDING(超阈值但未达duration) → FIRING(达到duration) → RESOLVED(恢复正常)`

- 通知时机：进入 FIRING 时发送告警通知，进入 RESOLVED 时发送恢复通知
- 静默期：同一告警 FIRING 状态下，不重复发送通知，默认静默 60 分钟
- `duration`：持续 N 秒超过阈值才触发，防抖动（默认 300 秒 = 5 分钟）

告警规则示例：
```json
{
  "id": 1,
  "name": "CPU高负载",
  "metric": "cpu_usage",
  "operator": ">",
  "threshold": 80,
  "duration": 300,
  "enabled": true,
  "notify_channel_id": 1
}
```

### 5.5 通知发送（SSRF 防护重点）

SSRF 防护要点：

| 防护点 | 措施 |
|--------|------|
| 内网地址过滤 | 检查 10/8、172.16/12、192.168/16、127/8、169.254/16、::1、fc00::/7 |
| DNS 重绑定 | 自定义 Dialer 强制使用预解析 IP |
| 重定向攻击 | CheckRedirect 中再次 SSRF 检查 |
| 响应体反射 | 最多读 1KB，不反射给用户 |
| 超时限制 | 10 秒 |
| TLS 验证 | 强制验证 |

通知渠道：Webhook（POST JSON）、Telegram（Bot API）、邮件（SMTP）

### 5.6 SQLite 表结构

```sql
-- Agent元数据
CREATE TABLE agents (
    id INTEGER PRIMARY KEY AUTOINCREMENT,
    token TEXT UNIQUE NOT NULL,
    hostname TEXT NOT NULL,
    os TEXT,
    arch TEXT,
    agent_version TEXT,
    host_fingerprint TEXT,
    last_seen INTEGER,
    online INTEGER DEFAULT 0,
    created_at INTEGER NOT NULL,
    UNIQUE(host_fingerprint)
);

-- 注册码
CREATE TABLE register_codes (
    code TEXT PRIMARY KEY,
    created_at INTEGER NOT NULL,
    expires_at INTEGER NOT NULL,
    used INTEGER DEFAULT 0,
    used_by_agent_id INTEGER
);

-- 告警规则
CREATE TABLE alert_rules (
    id INTEGER PRIMARY KEY AUTOINCREMENT,
    name TEXT NOT NULL,
    metric TEXT NOT NULL,
    operator TEXT NOT NULL,
    threshold REAL NOT NULL,
    duration INTEGER NOT NULL,
    enabled INTEGER DEFAULT 1,
    notify_channel_id INTEGER,
    created_at INTEGER NOT NULL
);

-- 通知渠道
CREATE TABLE notify_channels (
    id INTEGER PRIMARY KEY AUTOINCREMENT,
    name TEXT NOT NULL,
    type TEXT NOT NULL,
    config TEXT NOT NULL,
    created_at INTEGER NOT NULL
);

-- 探测目标
CREATE TABLE ping_targets (
    id INTEGER PRIMARY KEY AUTOINCREMENT,
    target TEXT NOT NULL,
    name TEXT NOT NULL,
    enabled INTEGER DEFAULT 1,
    created_at INTEGER NOT NULL
);

-- 历史聚合数据(每5分钟一个点)
CREATE TABLE metric_records (
    id INTEGER PRIMARY KEY AUTOINCREMENT,
    agent_id INTEGER NOT NULL,
    timestamp INTEGER NOT NULL,
    cpu_usage REAL,
    mem_usage REAL,
    disk_usage TEXT,
    net_rx INTEGER,
    net_tx INTEGER,
    ping_data TEXT,
    FOREIGN KEY (agent_id) REFERENCES agents(id)
);
CREATE INDEX idx_metric_records_agent_time ON metric_records(agent_id, timestamp);

-- 管理员账户
CREATE TABLE admin (
    id INTEGER PRIMARY KEY,
    username TEXT UNIQUE NOT NULL,
    password_hash TEXT NOT NULL,
    totp_secret TEXT,
    totp_enabled INTEGER DEFAULT 0,
    created_at INTEGER NOT NULL
);

-- 公开分享页配置
CREATE TABLE share_pages (
    id INTEGER PRIMARY KEY AUTOINCREMENT,
    share_id TEXT UNIQUE NOT NULL,
    title TEXT,
    agent_ids TEXT,
    created_at INTEGER NOT NULL
);
```

---

## 6. 前端面板设计

### 6.1 技术选型

| 维度 | 选择 | 理由 |
|------|------|------|
| 框架 | React 18 + TypeScript | 生态成熟，组件化模型适合主题/布局/分享功能 |
| 构建 | Vite | 快速构建，产物供 Go embed |
| UI 库 | shadcn/ui | 基于 Radix UI + Tailwind，可定制性强 |
| 图表 | ECharts | 性能好，支持实时数据流，图表类型丰富 |
| 状态管理 | Zustand | 轻量，适合中等复杂度应用 |
| 路由 | React Router | 标准选择 |
| 样式 | Tailwind CSS | 与 shadcn/ui 配套，主题切换方便 |

### 6.2 页面结构

```
┌─────────────────────────────────────────────────────────┐
│  顶栏: Logo | 搜索 | 主题切换 | 用户菜单                  │
├──────────┬──────────────────────────────────────────────┤
│          │                                              │
│  侧边栏   │              主内容区                         │
│          │                                              │
│  ├ 仪表盘 │  (根据路由切换不同页面)                        │
│  ├ 服务器 │                                              │
│  ├ 告警   │                                              │
│  ├ 通知   │                                              │
│  ├ 设置   │                                              │
│  └ 分享   │                                              │
│          │                                              │
└──────────┴──────────────────────────────────────────────┘
```

### 6.3 仪表盘（Dashboard）

卡片展示内容：
- 在线状态、运行时间
- CPU/内存/磁盘使用率（进度条）
- 上下行速度
- 三网（电信/联通/移动）平均延迟和丢包率
- 丢包率 > 0 时橙色标注，> 20% 红色标注
- 离线服务器延迟/丢包显示 `---`
- 点击卡片跳转到服务器详情页

### 6.4 服务器详情（Server Detail）

核心图表：
- CPU 占用率实时折线图
- 内存占用率实时折线图
- 延迟与丢包率融合图（上半部分延迟折线图三网三条线，下半部分丢包率柱状图，共享 X 轴）
- 磁盘使用率、网络流量、网络连接信息卡片
- 系统信息卡片（OS、内核、CPU 型号、Agent 版本、探测方式、上报间隔）

时间范围切换数据源：

| 时间范围 | 数据源 | 数据粒度 | 刷新方式 |
|---------|--------|---------|---------|
| 1 小时 | 环形缓冲 | 3 秒/点 | WebSocket 实时 |
| 6 小时 | 环形缓冲 + SQLite 聚合 | 3 秒/点 + 5 分钟/点 | WebSocket 实时 |
| 12 小时 | SQLite 聚合 | 5 分钟/点 | 定时轮询(5分钟) |
| 1 天 | SQLite 聚合 | 5 分钟/点 | 定时轮询(5分钟) |
| 2 天 | SQLite 聚合 | 5 分钟/点 | 定时轮询(5分钟) |

### 6.5 其他页面

- **告警管理**：告警规则列表（CRUD）+ 告警历史记录
- **通知渠道**：Webhook/Telegram/邮件渠道管理，支持测试发送
- **设置**：账户安全（密码/TOTP）、探测目标管理、Agent管理（注册码/Token/一键命令）、系统设置、外观（主题/布局）
- **公开分享页**：无需登录访问，仅展示管理员选择公开的 Agent，不包含敏感信息

### 6.6 主题系统

支持主题：浅色（Light）、深色（Dark）、跟随系统（System）、自定义（可选，通过 CSS 变量覆盖）

主题切换流程：用户选择主题 → localStorage 存储 → CSS 变量切换 → shadcn/ui 组件自动适配

### 6.7 实时数据 WebSocket 流

```
浏览器 ──▶ WebSocket连接 /ws/dashboard (JWT认证)
                    │
                    ▼
            Server推送实时数据(每3秒):
            {
              "type": "dashboard_update",
              "servers": [
                {"id": 1, "online": true, "cpu": 45.2, "mem": 60.0, ...},
                {"id": 2, "online": true, "cpu": 12.5, "mem": 30.0, ...}
              ]
            }
                    │
                    ▼
            前端Zustand store更新 ──▶ React组件重渲染 ──▶ ECharts图表更新
```

---

## 7. 安全设计

### 7.1 威胁模型

| 编号 | 威胁场景 | 我们的防御 |
|------|---------|-----------|
| T1 | 攻击者通过 Server 向 Agent 下发命令 | S1+S4：协议中无控制帧，代码中无执行功能 |
| T2 | 攻击者截获 Agent-Server 通信 | S2：强制 TLS，Agent 拒绝明文连接 |
| T3 | 攻击者利用多租户授权缺陷越权 | S5：第一版单管理员，无多租户攻击面 |
| T4 | 攻击者通过 Webhook 通知发起 SSRF | S6：SSRF 防护层 |
| T5 | 攻击者暴力破解管理员密码 | 登录限速 + 可选 TOTP |
| T6 | 攻击者窃取 Agent Token 后在其他机器使用 | 主机指纹绑定 |
| T7 | 攻击者伪造 Agent 上报数据 | 数据合理性校验 + 上报频率限制 |
| T8 | 攻击者通过 CSRF 触发状态变更 | JWT 存储在 HttpOnly Cookie + SameSite=Strict |
| T9 | 攻击者利用注册码重复注册 | 注册码一次性 + 15 分钟有效期 |
| T10 | Agent 以 root 运行导致 RCE 危害放大 | S3：非 root + 最小 capabilities |
| T11 | 攻击者通过公开分享页泄露敏感信息 | 分享页白名单字段，仅展示非敏感信息 |
| T12 | 攻击者利用 WebSocket 端点未授权访问 | WS 端点强制 JWT 认证 |

### 7.2 S1 + S4：纯只读架构（根除 RCE）

- **协议层**：WebSocket 消息协议中，Server→Agent 方向只有 4 种帧，全部是数据响应，不存在"命令"帧
- **代码层**：Agent 代码中不引入任何命令执行相关功能（不调用 `os/exec`、不建立 PTY/Shell 会话、不提供文件系统操作接口、不包含 Cron 任务执行器）
- **架构层**：Server 到 Agent 方向不存在主动连接。所有通信由 Agent 发起

### 7.3 S5：单管理员 + 强认证

JWT 安全配置：

| 配置项 | 值 | 理由 |
|--------|-----|------|
| 存储 | HttpOnly Cookie | 防止 JS 读取（防 XSS 窃取） |
| SameSite | Strict | 防 CSRF |
| Secure | true | 仅 HTTPS 传输 |
| 有效期 | 12 小时 | 平衡安全性和体验 |
| 签名算法 | HS256 | 对称签名，单服务端够用 |
| 密钥 | 随机生成 32 字节 | 首次启动生成，存配置文件 |

密码策略：
- 首次启动强制设置密码（无默认密码）
- 最小长度 12 位
- 必须包含大小写字母 + 数字
- bcrypt cost = 12

### 7.4 S6：SSRF 防护实现

```go
// 伪代码: SSRF防护检查流程
func safeHTTPSend(url string) error {
    // 1. 解析URL
    parsed := net.ParseURL(url)
    if parsed.Scheme != "https" && parsed.Scheme != "http" {
        return errors.New("only http/https allowed")
    }

    // 2. 解析所有IP地址
    ips, _ := net.LookupIP(parsed.Hostname())
    for _, ip := range ips {
        if isPrivateIP(ip) {
            return errors.New("private IP blocked")
        }
    }

    // 3. 自定义Dialer, 强制连接到解析的IP(防DNS重绑定)
    dialer := &net.Dialer{Timeout: 10 * time.Second}
    transport := &http.Transport{
        DialContext: func(ctx, network, addr) (net.Conn, error) {
            host, port, _ := net.SplitHostPort(addr)
            if host == parsed.Hostname() {
                addr = net.JoinHostPort(ips[0].String(), port)
            }
            return dialer.DialContext(ctx, network, addr)
        },
    }

    // 4. 禁止重定向到内网
    client := &http.Client{
        Timeout: 10 * time.Second,
        Transport: transport,
        CheckRedirect: func(req *http.Request, via []*http.Request) error {
            return checkSSRF(req.URL.String())
        },
    }
    resp, _ := client.Get(url)
    defer resp.Body.Close()

    // 5. 限制响应体读取(最多1KB)
    body, _ := io.ReadAll(io.LimitReader(resp.Body, 1024))
    return nil
}
```

### 7.5 主机指纹绑定

```go
func generateHostFingerprint() string {
    hostname, _ := os.Hostname()
    cpuInfo := readCPUInfo()
    macAddr := getPrimaryMAC()
    raw := hostname + cpuInfo + macAddr
    h := sha256.Sum256([]byte(raw))
    return hex.EncodeToString(h[:])
}
```

- 注册时上报主机指纹，Server 绑定 Token 与指纹
- 后续上报时 Server 校验指纹，不匹配则拒绝并记录告警
- 指纹变更（如更换网卡）需管理员在面板上重置

### 7.6 数据合理性校验

| 校验项 | 规则 | 异常处理 |
|--------|------|---------|
| CPU 使用率 | 0-100% | 超范围丢弃，记录告警 |
| 内存使用率 | 0-100%，used ≤ total | 超范围丢弃 |
| 磁盘使用率 | 0-100% | 超范围丢弃 |
| 延迟 | 0-60000ms | 超范围丢弃 |
| 丢包率 | 0-100% | 超范围丢弃 |
| 上报频率 | 每 3 秒 ±1 秒 | 过快拒绝（防刷），过慢标记离线 |
| 数据大小 | 单次上报 ≤ 10KB | 超大拒绝 |

---

## 8. 部署与运维

### 8.1 Server 部署方式

Docker 部署（推荐）：
```yaml
version: '3.8'
services:
  probe-server:
    image: probe-server:latest
    container_name: probe-server
    restart: unless-stopped
    ports:
      - "443:443"
    volumes:
      - ./data:/app/data
      - ./certs:/app/certs
    environment:
      - PROBE_ADMIN_USER=admin
      - PROBE_DATA_RETENTION=90
    read_only: true
    tmpfs:
      - /tmp
    security_opt:
      - no-new-privileges:true
    cap_drop:
      - ALL
```

### 8.2 Server 配置文件

```yaml
listen: ":443"
data_dir: "/var/lib/probe-server"

tls:
  cert: "/etc/probe-server/cert.pem"
  key: "/etc/probe-server/key.pem"

auth:
  jwt_secret: "auto-generated-on-first-start"
  jwt_expiry: "12h"
  login_rate_limit: 5

monitor:
  report_interval: 3
  heartbeat_timeout: 90
  ring_buffer_size: 3600
  aggregation_interval: 300

storage:
  data_retention_days: 90

ping:
  default_interval: 60
  icmp_count: 10
  icmp_timeout: 15
  icmp_interval: 500
  tcp_count: 5
  tcp_timeout: 5

alert:
  default_silence_period: 3600

notify:
  webhook_timeout: 10
  webhook_max_response: 1024

log:
  level: "info"
  file: "/var/log/probe-server/server.log"
  max_size: 100
  max_backups: 5
  max_age: 30
```

### 8.3 Agent 一键安装脚本

```bash
#!/bin/bash
# install-agent.sh

# 1. 解析参数
SERVER_URL=""
REGISTER_CODE=""
while [[ $# -gt 0 ]]; do
    case $1 in
        --server) SERVER_URL="$2"; shift 2;;
        --code) REGISTER_CODE="$2"; shift 2;;
        *) echo "Unknown: $1"; exit 1;;
    esac
done

# 2. 检测系统
OS=$(uname -s | tr '[:upper:]' '[:lower:]')
ARCH=$(uname -m)
case $ARCH in
    x86_64)  ARCH="amd64";;
    aarch64) ARCH="arm64";;
    armv7l)  ARCH="armv7";;
    *) echo "Unsupported: $ARCH"; exit 1;;
esac

# 3. 下载二进制(校验SHA256)
AGENT_URL="${SERVER_URL}/download/agent/${OS}/${ARCH}"
curl -fsSL -o /tmp/probe-agent "${AGENT_URL}"
curl -fsSL -o /tmp/probe-agent.sha256 "${AGENT_URL}.sha256"
echo "$(cat /tmp/probe-agent.sha256)  /tmp/probe-agent" | sha256sum -c -

# 4. 安装二进制
chmod +x /tmp/probe-agent
mv /tmp/probe-agent /usr/local/bin/probe-agent

# 5. 创建用户
if ! id probe &>/dev/null; then
    useradd -r -s /usr/sbin/nologin probe
fi

# 6. 创建配置目录
mkdir -p /etc/probe-agent
cat > /etc/probe-agent/config.yml << EOF
server: "${SERVER_URL}"
register_code: "${REGISTER_CODE}"
report_interval: 3
config_sync_interval: 3600
ping_method: "auto"
EOF
chmod 600 /etc/probe-agent/config.yml
chown probe:probe /etc/probe-agent/config.yml

# 7. 尝试setcap (ICMP Ping)
if setcap cap_net_raw+ep /usr/local/bin/probe-agent 2>/dev/null; then
    echo "ICMP Ping enabled (CAP_NET_RAW)"
else
    echo "setcap failed, will try unprivileged ICMP or fallback to TCP Ping"
fi

# 8. 安装systemd service
cat > /etc/systemd/system/probe-agent.service << 'EOF'
[Unit]
Description=Server Probe Agent
After=network.target

[Service]
Type=simple
User=probe
Group=probe
ExecStart=/usr/local/bin/probe-agent --config /etc/probe-agent/config.yml
Restart=always
RestartSec=5
NoNewPrivileges=true
ProtectSystem=strict
ProtectHome=true
PrivateTmp=true
ReadWritePaths=/etc/probe-agent

[Install]
WantedBy=multi-user.target
EOF

# 9. 启动服务
systemctl daemon-reload
systemctl enable probe-agent
systemctl start probe-agent

echo "Agent installed and started."
echo "Check status: systemctl status probe-agent"
```

### 8.4 版本发布流程

1. 更新版本号（VERSION 文件）
2. 构建前端（cd frontend && npm run build → web/）
3. 交叉编译：Server（linux-amd64, linux-arm64）、Agent（linux-amd64, linux-arm64, linux-armv7, windows-amd64, darwin-amd64, darwin-arm64）
4. 生成 SHA256 校验文件
5. 构建 Docker 镜像（linux/amd64, linux/arm64）
6. 发布 GitHub Release

### 8.5 升级方案

- **Server 升级**：停止服务 → 替换二进制 → 启动服务（SQLite 自动迁移）
- **Agent 升级**：手动升级（第一版不做自动升级，因为自动升级违反纯只读架构）
- Server 向下兼容旧版 Agent 的上报格式

### 8.6 日志设计

日志级别：DEBUG / INFO / WARN / ERROR

日志格式（JSON）：
```json
{"time":"2026-06-21T10:00:00Z","level":"INFO","module":"monitor","msg":"agent connected","agent_id":1}
{"time":"2026-06-21T10:05:00Z","level":"WARN","module":"monitor","msg":"agent heartbeat timeout","agent_id":3}
{"time":"2026-06-21T10:10:00Z","level":"ERROR","module":"notify","msg":"webhook send failed","error":"timeout"}
```

安全相关日志（独立记录）：登录失败/限速、主机指纹不匹配、SSRF 阻断、注册码使用

### 8.7 监控自身健康

Server 面板"系统状态"页面（仅管理员）：

| 指标 | 说明 |
|------|------|
| Server 运行时间 | 当前进程运行时长 |
| 内存占用 | Server 自身内存使用 |
| SQLite 大小 | 数据库文件大小 |
| 在线 Agent 数 | 当前连接的 Agent 数量 |
| WebSocket 连接数 | 浏览器面板连接数 |
| 上报 QPS | 每秒接收的 Agent 上报数 |
| 磁盘剩余空间 | 数据目录所在分区剩余空间 |

---

## 9. 开发计划与里程碑

### 9.1 里程碑规划

基于 TDD（测试驱动开发）原则，每个里程碑都遵循"先写测试 → 实现 → 测试通过"的循环。

| 里程碑 | 工期 | 累计 | 优先级 | 目标 |
|--------|------|------|--------|------|
| M1 基础架构 + 核心采集 | 2 周 | 2 周 | P0 | Agent 能采集系统指标，Server 能存储数据 |
| M2 通信 + 注册上线 | 2 周 | 4 周 | P0 | Agent 能注册到 Server 并维持 WebSocket 连接 |
| M3 实时监控 + 前端面板 | 3 周 | 7 周 | P0 | 浏览器面板能实时展示服务器状态 |
| M4 网络探测 + 历史数据 | 2 周 | 9 周 | P0 | Agent 执行三网 Ping 探测，历史数据落盘和查询 |
| M5 告警 + 通知 + 安全加固 | 2 周 | 11 周 | P1 | 告警引擎、通知渠道、安全审计全部就绪 |
| M6 部署 + 发布 + 文档 | 1 周 | 12 周 | P1 | 可发布的生产版本 |

M1-M4 是最小可用版本（MVP），M5-M6 是完整版本。总计约 12 周（3 个月）。

### 9.2 技术风险与缓解

| 风险 | 影响 | 缓解措施 |
|------|------|---------|
| pro-bing 库在特定平台行为不一致 | Ping 数据不准 | M4 阶段在多平台测试，记录差异 |
| 环形缓冲并发写入竞争 | 数据丢失或 panic | M1 阶段充分测试并发安全，用 sync.Mutex 或 channel |
| SQLite 高频写入性能 | Server 卡顿 | 环形缓冲吸收高频写入，SQLite 只存聚合数据 |
| 前端 embed 后调试困难 | 开发效率低 | 开发模式用独立前端 dev server，构建时才 embed |
| WebSocket 断线重连风暴 | Server 连接数飙升 | 指数退避重连（1s→2s→4s→...→60s 上限） |
