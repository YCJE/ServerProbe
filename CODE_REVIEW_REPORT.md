# 服务器探针监控系统 - 全量代码审查报告

审查日期: 2026-06-22
审查范围: server/internal、agent/internal、server/frontend/src、scripts、pkg

---

## 第一部分：设计初衷审查 (S1-S8 安全原则)

### S1 纯只读架构 — 通过

- `server/internal/api/handler_agent.go`: Server→Agent 方向的消息类型仅有 `register_ok`、`register_fail`、`config_update`、`heartbeat_ack`。
- `shared/model/message.go` 定义的服务器消息类型仅 4 种，无任何命令执行类消息。
- Agent 侧 `agent/internal/reporter/ws.go` 的 `handleServerMessage` 仅处理 `config_update` 和 `heartbeat_ack`。
- **结论**: 不存在任何控制通道，符合纯只读架构。

### S2 强制 TLS — 通过

- `agent/internal/reporter/ws.go` 第 66-68 行: 拒绝非 https URL。
- `agent/internal/reporter/ws.go` 第 79-81 行: 验证 URL scheme 必须为 wss。
- `agent/internal/config/sync.go` 第 74-77 行: 拒绝非 https URL。
- 两处均设置 `MinVersion: tls.VersionTLS12`。
- `insecure_tls` 选项仅跳过证书验证，仍使用 wss/https 加密通道，不违背 TLS 强制原则。
- **结论**: 所有通信强制加密。

### S3 非 root 运行 — 通过

- `scripts/install-agent.sh` 第 155-158 行: 创建 probe 用户。
- `scripts/install-agent.sh` 第 191-211 行: systemd 服务配置 `User=probe`、`Group=probe`。
- **结论**: Agent 以 probe 用户运行。

### S4 无远程执行 — 通过

- 全局搜索 `exec.Command`、`os/exec` 无任何匹配结果。
- Agent 仅通过读取 `/proc` 文件系统和 syscall 采集数据，ICMP ping 使用 `golang.org/x/net/icmp`。
- **结论**: 代码中不存在任何命令执行、终端会话或文件操作功能。

### S5 单管理员 — 通过

- 无多租户相关代码，Admin 模型为单一管理员。
- `server/internal/api/router.go`: 管理 API 均通过 `AuthRequired()` 中间件保护。
- **结论**: 单管理员模型，管理 API 正确保护。

### S6 SSRF 防护 — 基本通过，存在缺陷

- `server/internal/pkg/ssrf.go`: 实现了内网地址过滤，包含自定义 `DialContext` 防止 DNS 重绑定。
- `server/internal/service/notify.go`: Webhook 通知使用 `SSRFProtector`。
- **缺陷**: `isPrivateIP` 未覆盖 `0.0.0.0/8`（未指定地址）、`100.64.0.0/10`（运营商级 NAT）、IPv6 映射 IPv4 地址。
- **缺陷**: 邮件通知使用 `smtp.SendMail` 直接连接，未经过 SSRF 防护。
- **结论**: Webhook SSRF 防护到位，邮件通知存在绕过风险。

### S7 最小权限采集 — 通过

- Agent 读取 `/proc/stat`、`/proc/meminfo`、`/proc/cpuinfo`、`/proc/loadavg`、`/proc/net/dev`、`/proc/net/tcp`、`/proc/net/udp`、`/proc/uptime`、`/proc/mounts` 等文件，均为非 root 可读。
- 磁盘信息使用 `syscall.Statfs`，非 root 可调用。
- ICMP ping 通过 `setcap cap_net_raw` 赋权，无需 root。
- **结论**: 采集均为最小权限操作。

### S8 配置文件权限控制 — 通过

- `scripts/install-agent.sh` 第 174 行: `chmod 600`。
- `scripts/install-agent.sh` 第 175 行: `chown probe:probe`。
- `agent/cmd/agent/main.go` 第 209 行: `os.WriteFile(path, data, 0600)`。
- **结论**: 配置文件权限为 600。

---

## 第二部分：全量代码审查

### 严重问题 (Critical)

---

#### C1. WebSocket 并发写入竞争 — 可导致连接损坏

**文件**: `server/internal/api/handler_agent.go` 第 73-80 行、第 98-130 行
**文件**: `server/internal/api/handler_dashboard_ws.go` 第 73-79 行、第 129-130 行

**问题描述**:

gorilla/websocket 文档明确指出: "Connections support one concurrent reader and one concurrent writer."（连接支持一个并发读者和一个并发写者）。

在 `handler_agent.go` 中:
- 第 74-79 行的 ping 协程直接调用 `conn.WriteMessage(websocket.PingMessage, nil)`
- 主循环中 `handleRegister`、`handleReport`、`handleHeartbeat` 等方法直接调用 `conn.WriteJSON()`
- 两处写入无任何互斥锁保护

在 `handler_dashboard_ws.go` 中:
- 第 73-79 行的 ping 协程直接调用 `conn.WriteMessage()`
- 第 130 行 `pushDashboardData` 也直接调用 `conn.WriteMessage()`
- 第 129 行注释写着"加锁写入，避免与 ping 协程竞争"，但实际**没有任何锁**

并发写入会导致 WebSocket 帧损坏，引发连接异常断开或数据错乱。

**修复建议**:

为每个 WebSocket 连接添加写锁:

```go
type connWriter struct {
    conn *websocket.Conn
    mu   sync.Mutex
}

func (w *connWriter) WriteMessage(msgType int, data []byte) error {
    w.mu.Lock()
    defer w.mu.Unlock()
    return w.conn.WriteMessage(msgType, data)
}

func (w *connWriter) WriteJSON(v interface{}) error {
    w.mu.Lock()
    defer w.mu.Unlock()
    return w.conn.WriteJSON(v)
}
```

所有写入操作（包括 ping 协程）都通过加锁的 wrapper 进行。

---

#### C2. WebSocket CheckOrigin 允许所有来源 — 跨站 WebSocket 劫持风险

**文件**: `server/internal/api/handler_agent.go` 第 37-39 行
**文件**: `server/internal/api/handler_dashboard_ws.go` 第 29-31 行

**问题描述**:

两个 WebSocket Upgrader 均配置 `CheckOrigin: func(r *http.Request) bool { return true }`，允许任意来源的跨站 WebSocket 连接。

对于 Dashboard WebSocket（需要 JWT 认证），虽然 token 通过 URL query 传递而非 cookie，降低了 CSWSH 风险，但仍不符合安全最佳实践。对于公开仪表盘 WebSocket，无认证无限制，攻击者可任意连接。

**修复建议**:

```go
upgrader: websocket.Upgrader{
    CheckOrigin: func(r *http.Request) bool {
        // 仅允许同源请求
        origin := r.Header.Get("Origin")
        if origin == "" {
            return true // 非浏览器客户端
        }
        u, err := url.Parse(origin)
        if err != nil {
            return false
        }
        return u.Host == r.Host
    },
},
```

---

### 中等问题 (Medium)

---

#### M1. Token 在 URL Query 参数中暴露

**文件**: `agent/internal/config/sync.go` 第 79 行
**文件**: `server/internal/api/handler_dashboard_ws.go` 第 40 行
**文件**: `server/frontend/src/lib/websocket.ts` 第 42-49 行

**问题描述**:

1. Agent 配置拉取: `url := s.serverURL + "/api/v1/agent/config?token=" + s.token` — Token 出现在 URL 中。
2. Dashboard WebSocket: JWT token 通过 `?token=JWT_TOKEN` 传递。
3. 前端 WebSocket 连接同样将 token 拼入 URL。

URL query 参数会出现在服务器访问日志、反向代理日志、浏览器历史记录中，增加 Token 泄露风险。

**修复建议**:

- Agent 配置拉取: 改用 HTTP Header 传递 Token (`Authorization: Bearer <token>`)。
- Dashboard WebSocket: WebSocket 握手阶段无法使用自定义 Header，可在握手后通过首条消息发送 Token 进行认证（先升级连接，再验证，验证失败则关闭）。

---

#### M2. WritePingData 逻辑错误 — Ping 数据写入为新数据点而非更新

**文件**: `server/internal/service/monitor.go` 第 147-165 行

**问题描述**:

```go
func (m *MonitorService) WritePingData(agentID int64, results []sharedmodel.PingResult) {
    m.mu.RLock()
    rb, ok := m.ringBuffers[agentID]
    m.mu.RUnlock()
    if !ok {
        return
    }
    points := rb.Latest(1)  // 获取最新一个点的副本
    if len(points) == 0 {
        return
    }
    points[0].PingData = results  // 修改副本
    rb.Write(points[0])           // 写入为新数据点（而非更新原数据）
}
```

`rb.Latest(1)` 返回的是数据副本，`rb.Write(points[0])` 将修改后的数据作为**新数据点**写入环形缓冲区，而非更新原始数据点。这导致:
- 环形缓冲区中出现重复时间戳的数据点
- Ping 数据与对应的指标数据时间戳不匹配
- 数据消费端可能读取到不一致的数据

**修复建议**:

在 RingBuffer 中实现 `UpdateLatest` 方法，直接更新最新数据点而非追加新点；或在写入指标数据时预留 PingData 字段，由 Ping 协程填充。

---

#### M3. HandleDeleteAgent 不清理 MonitorService 资源

**文件**: `server/internal/api/handler_agent_api.go` 第 117-135 行

**问题描述**:

删除 Agent 时仅调用 `h.agentRepo.Delete(id)` 删除数据库记录，未调用:
- `monitor.UnregisterConnection(id)` — 残留的 WebSocket 连接不会被关闭
- 清理 `monitor.ringBuffers[id]` — 环形缓冲区内存不会被释放
- 清理 `validator.lastReportTime[id]` — 校验器中的记录不会被清理

这导致资源泄漏，且被删除 Agent 的活跃连接可能继续接收数据。

**修复建议**:

```go
func (h *AgentAPIHandler) HandleDeleteAgent(c *gin.Context) {
    // ...
    if err := h.agentRepo.Delete(id); err != nil {
        c.JSON(http.StatusInternalServerError, gin.H{"error": "删除失败"})
        return
    }
    // 清理 MonitorService 资源
    h.monitor.UnregisterConnection(id)
    h.monitor.RemoveRingBuffer(id)  // 需新增此方法
    h.validator.RemoveAgent(id)     // 需新增此方法
    c.JSON(http.StatusOK, gin.H{"message": "删除成功"})
}
```

---

#### M4. 登录限速器 Slice 别名 Bug

**文件**: `server/internal/api/middleware.go` 第 79-94 行

**问题描述**:

```go
valid := attempts[:0]  // 复用底层数组
for _, t := range attempts {
    if time.Since(t) < window {
        valid = append(valid, t)  // 可能覆盖 attempts 中尚未读取的元素
    }
}
```

`valid := attempts[:0]` 与 `attempts` 共享同一底层数组。`append(valid, t)` 会写入底层数组的前部，可能覆盖 `attempts` 中尚未遍历到的元素，导致部分尝试记录被意外删除，限速器行为不正确。

**修复建议**:

```go
valid := make([]time.Time, 0, len(attempts))
for _, t := range attempts {
    if time.Since(t) < window {
        valid = append(valid, t)
    }
}
```

---

#### M5. Setup 端点 TOCTOU 竞争条件

**文件**: `server/internal/api/handler_auth.go` 第 92-130 行

**问题描述**:

`HandleSetup` 先调用 `h.adminRepo.Count()` 检查是否已有管理员，再调用 `h.adminRepo.Create()` 创建管理员。两步操作之间无事务保护，存在 Time-of-Check to Time-of-Use (TOCTOU) 竞争条件。

攻击者可并发发送多个 setup 请求，在 Count() 返回 0 后、Create() 执行前，多个请求均可通过检查，导致创建多个管理员账户或覆盖已有管理员。

**修复建议**:

在数据库层面添加唯一约束（如固定 admin ID = 1），或使用事务 + 行锁:
```go
err := h.db.Transaction(func(tx *gorm.DB) error {
    var count int64
    tx.Model(&model.Admin{}).Count(&count)
    if count > 0 {
        return errors.New("admin already exists")
    }
    return tx.Create(&admin).Error
})
```

---

#### M6. insecure_tls 默认为 true

**文件**: `scripts/install-agent.sh` 第 172 行

**问题描述**:

安装脚本生成的 Agent 配置默认 `insecure_tls: true`，这意味着 Agent 默认接受任意 TLS 证书，容易遭受中间人攻击。虽然方便了自签名证书的使用，但作为默认值过于宽松。

**修复建议**:

- 默认设为 `false`，在安装脚本中提示用户如使用自签名证书需手动开启。
- 或在 `insecure_tls: true` 时输出醒目警告。

---

#### M7. 前端 Token 存储在 localStorage — XSS 可窃取

**文件**: `server/frontend/src/lib/api.ts` 第 19-31 行

**问题描述**:

JWT token 存储在 `localStorage` 中。虽然服务器同时设置了 HttpOnly cookie，但前端从 localStorage 读取 token 放入 `Authorization` Header 发送请求。如果页面存在 XSS 漏洞，攻击者可直接从 localStorage 读取 token。

**修复建议**:

- 优先依赖 HttpOnly cookie 进行认证，前端不存储 token。
- 如必须使用 Header 认证，考虑使用内存变量（页面刷新后重新获取）替代 localStorage。
- 添加严格的 Content-Security-Policy 头降低 XSS 风险。

---

#### M8. 公开 WebSocket 无连接限制 — DoS 风险

**文件**: `server/internal/api/handler_dashboard_ws.go` 第 140-198 行

**问题描述**:

公开仪表盘 WebSocket (`/ws/public/dashboard`) 无认证、无速率限制、无连接数限制。攻击者可打开大量 WebSocket 连接，耗尽服务器资源（每个连接占用一个 goroutine 和一个 ticker）。

**修复建议**:

- 添加全局 WebSocket 连接计数器，限制最大并发连接数。
- 对同一 IP 的连接数进行限制。
- 添加连接空闲超时自动断开。

---

#### M9. WebSocket 消息大小无限制

**文件**: `server/internal/api/handler_agent.go` — 无 `SetReadLimit()` 调用
**文件**: `server/internal/api/handler_dashboard_ws.go` — 无 `SetReadLimit()` 调用

**问题描述**:

两个 WebSocket 端点均未调用 `conn.SetReadLimit()` 限制消息大小。默认限制为 0（无限制）。恶意客户端可发送超大消息消耗服务器内存。

**修复建议**:

```go
conn.SetReadLimit(65536) // 限制为 64KB
```

---

#### M10. Agent pingTargets 并发数据竞争

**文件**: `agent/cmd/agent/main.go` 第 52、68-71、95、226 行

**问题描述**:

`pingTargets` 变量在两个 goroutine 间共享:
- 配置更新回调（`wsClient.Run()` goroutine 内）执行 `pingTargets = config.PingTargets`（写）
- Ping 探测 goroutine 执行 `len(*targets)` 和 `pinger.PingTargets(*targets)`（读）

无任何互斥锁保护，构成数据竞争。Go 的竞态检测器 (`-race`) 会报错。slice header 的并发读写可能导致读取到部分更新的值。

**修复建议**:

使用 `sync.RWMutex` 保护 `pingTargets`，或使用 `atomic.Value` 存储:
```go
var pingTargets atomic.Value
pingTargets.Store([]sharedmodel.PingTarget{})

// 写入
pingTargets.Store(config.PingTargets)

// 读取
targets := pingTargets.Load().([]sharedmodel.PingTarget)
```

---

#### M11. HandleCheckSetup 泄露内部错误信息

**文件**: `server/internal/api/handler_auth.go` 第 181-196 行

**问题描述**:

```go
c.JSON(http.StatusInternalServerError, gin.H{
    "error":  "查询失败",
    "detail": err.Error(),  // 泄露数据库内部错误
})
```

将数据库错误详情返回给客户端，可能泄露数据库类型、表结构等信息。

**修复建议**:

仅返回通用错误信息，将详细错误记录到服务端日志:
```go
log.Printf("查询管理员数量失败: %v", err)
c.JSON(http.StatusInternalServerError, gin.H{"error": "内部错误"})
```

---

#### M12. 公开 API 暴露 PingData — 可能泄露网络拓扑

**文件**: `server/internal/api/handler_server.go` 第 279-322 行

**问题描述**:

公开仪表盘 API (`HandlePublicDashboard`) 返回的数据包含 `PingData`，其中包含 ping 目标的 IP 地址或域名。如果 ping 目标配置为内网地址，将向公开访问者暴露内网网络拓扑。

**修复建议**:

- 在公开 API 中过滤掉 PingData 或仅返回 ping 成功/失败的布尔值，不返回目标地址。
- 或在配置中增加 `public_visible` 标记，控制 ping 数据是否对公开可见。

---

### 低等问题 (Low)

---

#### L1. 心跳检查器 goroutine 无法停止

**文件**: `server/internal/service/monitor.go` 第 211-218 行

**问题描述**:

`StartHeartbeatChecker` 启动的 goroutine 使用 `for range ticker.C` 循环，无停止通道。ticker 也未在 `Stop()` 中停止。服务器关闭时 goroutine 无法优雅退出。

**修复建议**:

添加 `stopCh` 和 context 支持:
```go
func (m *MonitorService) StartHeartbeatChecker(ctx context.Context) {
    ticker := time.NewTicker(30 * time.Second)
    go func() {
        defer ticker.Stop()
        for {
            select {
            case <-ctx.Done():
                return
            case <-ticker.C:
                m.checkHeartbeats()
            }
        }
    }()
}
```

---

#### L2. 数据校验器内存泄漏

**文件**: `server/internal/service/validator.go` 第 12-15 行

**问题描述**:

`lastReportTime map[int64]time.Time` 在 Agent 被删除后不会清理对应记录。长期运行且频繁增删 Agent 时，map 会持续增长。

**修复建议**:

添加 `RemoveAgent(agentID int64)` 方法，在删除 Agent 时调用。

---

#### L3. MonitorService ringBuffers 内存泄漏

**文件**: `server/internal/service/monitor.go` 第 30-45 行

**问题描述**:

`ringBuffers map[int64]*RingBuffer` 在 Agent 被删除后不会清理。每个 RingBuffer 预分配 2880 个数据点，约占用数百 KB 内存。大量增删 Agent 后会累积内存泄漏。

**修复建议**:

添加 `RemoveRingBuffer(agentID int64)` 方法，在删除 Agent 时调用。

---

#### L4. Agent 重连无指数退避

**文件**: `agent/internal/reporter/ws.go` 第 298-303 行

**问题描述**:

`getReconnectInterval` 始终返回 5 秒，注释提到"实际实现应记录重连次数"但未实现。当服务器不可用时，大量 Agent 以固定 5 秒间隔重连，可能对服务器造成"惊群"压力。

**修复建议**:

实现指数退避 + 抖动:
```go
func (c *WSClient) getReconnectInterval() time.Duration {
    c.reconnectCount++
    backoff := time.Duration(1<<uint(c.reconnectCount)) * time.Second
    if backoff > 5*time.Minute {
        backoff = 5 * time.Minute
    }
    jitter := time.Duration(rand.Intn(1000)) * time.Millisecond
    return backoff + jitter
}
```

---

#### L5. 缺少 Content-Security-Policy 安全头

**文件**: `server/internal/api/middleware.go` 第 134-141 行

**问题描述**:

`SecurityHeaders` 中间件设置了 X-Content-Type-Options、X-Frame-Options、Referrer-Policy，但缺少 `Content-Security-Policy` 头。CSP 是防御 XSS 的重要防线。

**修复建议**:

添加 CSP 头（需根据前端实际使用的内联样式/脚本调整）:
```go
c.Header("Content-Security-Policy", 
    "default-src 'self'; script-src 'self'; style-src 'self' 'unsafe-inline'; connect-src 'self' wss: ws:; img-src 'self' data:;")
```

---

#### L6. Agent 注册消息无速率限制

**文件**: `server/internal/api/handler_agent.go` 第 98-100 行

**问题描述**:

WebSocket 注册消息处理无速率限制。虽然注册码为 8 位（36^8 ≈ 2.8 万亿种组合），暴力破解不现实，但无限制的注册尝试会对数据库造成查询压力。

**修复建议**:

对同一 WebSocket 连接的注册失败次数进行限制，超过阈值后断开连接。

---

#### L7. 心跳消息不验证 Token

**文件**: `server/internal/api/handler_agent.go` 第 224-236 行

**问题描述**:

`handleHeartbeat` 仅检查 Agent 是否已注册 (`registered && agentID != 0`)，不验证消息中的 Token。攻击者若知道 agentID，可在未持有 Token 的情况下发送心跳消息维持在线状态。

**修复建议**:

在心跳处理中验证 Token:
```go
func (h *AgentHandler) handleHeartbeat(conn *websocket.Conn, msg *sharedmodel.WSMessage, agentID int64, registered *bool) {
    if !*registered || agentID == 0 {
        return
    }
    agent, err := h.agentRepo.FindByID(agentID)
    if err != nil || agent.Token != msg.Token {
        return
    }
    // ... 更新 LastSeen
}
```

---

#### L8. SQLite 单连接性能限制

**文件**: `server/internal/repository/sqlite.go` 第 50-51 行

**问题描述**:

`SetMaxOpenConns(1)` 和 `SetMaxIdleConns(1)` 将数据库连接限制为 1 个。虽然已启用 WAL 模式（支持并发读），但单连接设置使所有数据库操作串行化，可能成为性能瓶颈。

**修复建议**:

使用读写分离连接池，或适当增加连接数:
```go
db.SetMaxOpenConns(4)  // WAL 模式支持并发读
db.SetMaxIdleConns(2)
```

---

#### L9. 配置拉取 URL Token 未编码

**文件**: `agent/internal/config/sync.go` 第 79 行

**问题描述**:

```go
url := s.serverURL + "/api/v1/agent/config?token=" + s.token
```

Token 未经过 URL 编码。虽然当前 Token 为 hex 编码（不含特殊字符），但这是不良实践。

**修复建议**:

```go
url := s.serverURL + "/api/v1/agent/config?token=" + url.QueryEscape(s.token)
```

---

#### L10. JWT 密钥文件权限未验证

**文件**: `server/cmd/server/main.go` 第 174-198 行

**问题描述**:

生成 JWT 密钥时使用 `0600` 权限写入（正确），但读取已有密钥文件时不验证文件权限是否为 0600。如果文件权限被意外修改为 0644，其他系统用户可读取密钥。

**修复建议**:

读取密钥文件后检查权限:
```go
info, _ := os.Stat(secretPath)
if info.Mode() != 0600 {
    os.Chmod(secretPath, 0600)
    log.Printf("警告: JWT 密钥文件权限已修正为 600")
}
```

---

#### L11. 公开 API 缺少分页

**文件**: `server/internal/api/handler_server.go` 第 221-277 行、第 279-322 行

**问题描述**:

`HandlePublicServers` 和 `HandlePublicDashboard` 返回所有 Agent 数据，无分页。Agent 数量较多时可能影响性能。

**修复建议**:

添加分页参数 (`?page=1&size=20`)，或限制返回数量。

---

#### L12. AgentConn.Send 与直接 conn.WriteJSON 混用

**文件**: `server/internal/service/monitor.go` 第 88-96 行 vs `server/internal/api/handler_agent.go` 多处

**问题描述**:

`AgentConn` 结构体有 `mu sync.Mutex` 保护的 `Send()` 方法，但 `handler_agent.go` 中的 `handleRegister`、`handleReport` 等方法直接调用 `conn.WriteJSON()`，绕过了 `AgentConn.Send()` 的锁保护。这导致即使 `AgentConn` 有锁，也无法防止并发写入。

**修复建议**:

统一所有 WebSocket 写入操作通过 `AgentConn.Send()` 方法进行，不直接调用 `conn.WriteJSON()`。

---

## 汇总统计

| 严重性 | 数量 | 问题编号 |
|--------|------|----------|
| 严重   | 2    | C1, C2   |
| 中等   | 12   | M1-M12   |
| 低     | 12   | L1-L12   |
| **合计** | **26** | |

## 优先修复建议

1. **立即修复 (C1, C2)**: WebSocket 并发写入竞争和 CheckOrigin 问题，可能导致连接损坏和安全漏洞。
2. **短期修复 (M1-M5)**: Token 暴露、WritePingData 逻辑错误、资源清理、限速器 bug、Setup 竞争条件。
3. **中期修复 (M6-M12)**: TLS 默认配置、前端 Token 存储、DoS 防护、并发安全、信息泄露。
4. **长期优化 (L1-L12)**: 资源泄漏、性能优化、安全加固。

## 设计初衷符合度总结

| 原则 | 状态 | 说明 |
|------|------|------|
| S1 纯只读架构 | 符合 | 无控制通道 |
| S2 强制 TLS | 符合 | 拒绝明文连接 |
| S3 非 root 运行 | 符合 | systemd 以 probe 用户运行 |
| S4 无远程执行 | 符合 | 无 exec.Command |
| S5 单管理员 | 符合 | 管理 API 正确保护 |
| S6 SSRF 防护 | 基本符合 | Webhook 已防护，邮件通知未防护 |
| S7 最小权限采集 | 符合 | 仅采集非 root 数据 |
| S8 配置文件权限 | 符合 | 600 权限 |
