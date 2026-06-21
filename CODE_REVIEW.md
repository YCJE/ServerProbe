# Server Probe 全量代码审查报告

审查日期: 2026-06-22
审查范围: 后端 (server/internal/)、Agent (agent/internal/)、前端 (server/frontend/src/)、部署文件

---

## 一、严重问题 (Critical)

### C1. Agent 注册流程字段映射错误，导致重复注册和版本信息丢失

**文件**: `server/internal/api/handler_agent.go` 第 118-125 行
**文件**: `agent/internal/reporter/ws.go` 第 117-125 行
**文件**: `shared/model/message.go` 第 4-18 行

**问题描述**:
`handleRegister` 中存在两处严重的字段映射错误:

```go
req := service.RegisterAgentRequest{
    Code:            msg.Code,
    Hostname:        msg.Hostname,
    OS:              msg.OS,
    AgentVersion:    msg.OS,            // BUG: AgentVersion 被赋值为 OS
    HostFingerprint: msg.Token,         // BUG: HostFingerprint 取自 Token，但注册时 Token 为空
}
```

1. `AgentVersion` 被设置为 `msg.OS`（如 "linux/amd64"），而非实际 Agent 版本号。
2. `HostFingerprint` 取自 `msg.Token`，但 Agent 注册时 `msg.Token` 为空字符串。
3. `WSMessage` 结构体中根本没有 `AgentVersion`、`Arch`、`HostFingerprint` 字段。
4. Agent 端 `register()` 方法也没有传递 `Arch` 和 `HostFingerprint`。

**影响**:
- 由于 `HostFingerprint` 永远为空，`GetByFingerprint` 永远找不到已有 Agent，同一台机器每次注册都会创建新 Agent 记录。
- 注册码被消耗但旧 Agent 记录残留，导致数据混乱。
- Agent 版本信息错误，无法进行版本管理和升级提示。

**修复建议**:
1. 在 `WSMessage` 中增加 `AgentVersion`、`Arch`、`HostFingerprint` 字段。
2. Agent 端 `register()` 传递完整信息（包括主机指纹，可用 machine-id 或 MAC 地址生成）。
3. 修正 `handleRegister` 中的字段映射。

---

### C2. Agent 重连后无法上报数据（协议设计缺陷）

**文件**: `agent/internal/reporter/ws.go` 第 100-111 行、第 170-205 行
**文件**: `server/internal/api/handler_agent.go` 第 82-114 行

**问题描述**:
Agent 重连流程存在根本性缺陷:

1. Agent 首次连接时发送 `register` 消息，Server 设置 `registered=true`。
2. Agent 重连时，由于 `c.token != ""` 且 `c.registerCode == ""`，跳过注册直接连接。
3. Agent 开始发送 `report` 消息，但 Server 端 `registered` 仍为 `false`。
4. Server 的 `handleReport` 检查 `if !registered || agentID == 0 { return }`，直接丢弃所有数据。

```go
// agent ws.go - 重连时不注册
if c.registerCode != "" && c.token == "" {
    if err := c.register(); err != nil { ... }
}
// 重连后直接开始上报，但 server 不知道这个连接属于谁
```

```go
// server handler_agent.go - 未注册的连接上报数据被静默丢弃
func (h *AgentHandler) handleReport(...) {
    if !registered || agentID == 0 {
        return  // 重连后的所有 report 都走这里
    }
}
```

**影响**: Agent 网络中断后重连，将永远无法上报数据，直到 Agent 进程重启（重新带上注册码）。

**修复建议**:
增加 `authenticate` 消息类型，Agent 重连时用已有 Token 认证:
```go
case sharedmodel.MsgTypeAuthenticate:
    h.handleAuthenticate(conn, &msg, &agentID, &registered)
```
Agent 端在 `Connect()` 后，如果有 Token 则发送认证消息而非注册消息。

---

### C3. DataValidator 并发数据竞争

**文件**: `server/internal/service/validator.go` 第 11-13 行、第 81-93 行

**问题描述**:
`DataValidator.lastReportTime` 是普通 `map`，没有任何互斥保护，但被多个 Agent WebSocket goroutine 并发读写:

```go
type DataValidator struct {
    lastReportTime map[int64]time.Time // 无 mutex 保护
}

func (v *DataValidator) CheckReportFrequency(agentID int64) error {
    // 并发读写 map，会导致 panic: concurrent map read and map write
    if lastTime, ok := v.lastReportTime[agentID]; ok { ... }
    v.lastReportTime[agentID] = now
}
```

**影响**: 多 Agent 并发上报时必然触发 `concurrent map read and map write` panic，导致 Server 崩溃。

**修复建议**:
```go
type DataValidator struct {
    mu             sync.RWMutex
    lastReportTime map[int64]time.Time
}

func (v *DataValidator) CheckReportFrequency(agentID int64) error {
    v.mu.Lock()
    defer v.mu.Unlock()
    // ...
}
```

---

### C4. WebSocket 写操作并发不安全（Agent 和 Dashboard 均存在）

**文件**: `server/internal/api/handler_agent.go` 第 70-80 行
**文件**: `server/internal/api/handler_dashboard_ws.go` 第 72-78 行、第 114-134 行

**问题描述**:
gorilla/websocket 文档明确要求: "Connections support one concurrent reader and one concurrent writer." 但两个 WebSocket handler 中，ping goroutine 和主循环/推送逻辑都直接调用 `conn.WriteMessage` / `conn.WriteJSON`，没有同步保护。

`handler_agent.go`:
```go
// ping goroutine 写
go func() {
    for range pingTicker.C {
        if err := conn.WriteMessage(websocket.PingMessage, nil); err != nil { ... }
    }
}()
// 主循环也写
_ = conn.WriteJSON(response)  // 与 ping goroutine 并发写
```

`handler_dashboard_ws.go` 注释说"加锁写入"但实际未加锁:
```go
// 加锁写入，避免与 ping 协程竞争  <-- 注释存在但代码未实现
if err := conn.WriteMessage(websocket.TextMessage, data); err != nil { ... }
```

**影响**: 并发写会导致 WebSocket 帧损坏，连接断开。`MonitorService.AgentConn.Send` 方法有 mutex 保护，但 handler 中直接使用 `conn.WriteJSON` 绕过了这层保护。

**修复建议**:
使用统一的写锁或写通道:
```go
type wsWriter struct {
    mu sync.Mutex
    conn *websocket.Conn
}
func (w *wsWriter) WriteJSON(v interface{}) error {
    w.mu.Lock()
    defer w.mu.Unlock()
    return w.conn.WriteJSON(v)
}
```

---

### C5. Agent 连接替换导致新连接被误杀（竞态条件）

**文件**: `server/internal/service/monitor.go` 第 48-74 行、第 77-90 行

**问题描述**:
当同一 Agent 重新连接时，`RegisterConnection` 关闭旧连接，但旧连接的 handler goroutine 仍在运行:

1. 新连接 B 到达，`RegisterConnection` 关闭旧连接 A，将 B 存入 map。
2. 旧连接 A 的 handler `ReadMessage` 返回错误，退出循环。
3. 旧连接 A 的 `defer` 调用 `UnregisterConnection(agentID)`。
4. `UnregisterConnection` 从 map 中取出连接 B（当前映射），关闭 B，删除 B。

```go
func (m *MonitorService) UnregisterConnection(agentID int64) {
    m.mu.Lock()
    defer m.mu.Unlock()
    if conn, ok := m.connections[agentID]; ok {
        conn.Conn.Close()  // 关闭的是新连接 B！
        delete(m.connections, agentID)
    }
}
```

**影响**: Agent 重连后新连接立即被旧连接的清理逻辑杀死，Agent 反复断连重连。

**修复建议**:
`UnregisterConnection` 应检查当前连接是否就是要注销的连接:
```go
func (m *MonitorService) UnregisterConnection(agentID int64, conn *websocket.Conn) {
    m.mu.Lock()
    defer m.mu.Unlock()
    if current, ok := m.connections[agentID]; ok && current.Conn == conn {
        current.Conn.Close()
        delete(m.connections, agentID)
    }
}
```

---

### C6. WritePingData 导致数据点重复

**文件**: `server/internal/service/monitor.go` 第 147-165 行

**问题描述**:
```go
func (m *MonitorService) WritePingData(agentID int64, pingData []sharedmodel.PingResult) error {
    points := rb.Latest(1)
    if len(points) > 0 {
        points[0].PingData = pingData
        rb.Write(points[0])  // 写入新位置，而非更新原位置
    }
    return nil
}
```

`rb.Write` 总是写入 ring buffer 的下一个位置，而不是更新已有数据点。每次 Ping 结果上报都会在 ring buffer 中创建一个重复的数据点（相同时间戳，不同 PingData）。

**影响**: Ring buffer 中数据点数量翻倍，历史数据查询返回重复记录，图表渲染异常。

**修复建议**:
在 `RingBuffer` 中增加 `UpdateLatest` 方法，或直接修改最新数据点的 PingData 字段。

---

### C7. CORS 配置允许任意源携带凭证（CSRF 风险）

**文件**: `server/internal/api/middleware.go` 第 100-114 行

**问题描述**:
```go
c.Header("Access-Control-Allow-Origin", c.GetHeader("Origin"))  // 反射任意 Origin
c.Header("Access-Control-Allow-Credentials", "true")             // 允许携带凭证
```

此配置允许任何网站发起带 Cookie 的跨域请求。虽然 Cookie 设置了 `SameSite=Strict`（现代浏览器会拦截），但:
1. 旧浏览器不支持 SameSite 时仍可被利用。
2. 前端同时通过 `Authorization` header 发送 Token，CORS 策略允许跨域读取响应。

**修复建议**:
维护允许的 Origin 白名单，仅反射白名单中的 Origin:
```go
var allowedOrigins = map[string]bool{
    "https://your-domain.com": true,
}
origin := c.GetHeader("Origin")
if allowedOrigins[origin] {
    c.Header("Access-Control-Allow-Origin", origin)
    c.Header("Access-Control-Allow-Credentials", "true")
}
```

---

### C8. TOTP 两步验证被跳过（认证绕过）

**文件**: `server/internal/api/handler_auth.go` 第 70-83 行

**问题描述**:
```go
if admin.TOTPEnabled {
    if req.TOTPCode == "" {
        c.JSON(http.StatusOK, LoginResponse{Success: false, NeedTOTP: true, ...})
        return
    }
    // TODO: 验证 TOTP（M5 实现）
    // 暂时跳过 TOTP 验证
}
```

当管理员启用了 TOTP 两步验证，攻击者只需在 `totp_code` 字段填入任意非空字符串即可绕过验证直接登录。

**影响**: 两步验证形同虚设，攻击者只需密码即可登录。

**修复建议**:
在 TOTP 验证实现之前，应拒绝所有 TOTP 登录请求，而非跳过验证:
```go
if admin.TOTPEnabled {
    if req.TOTPCode == "" {
        c.JSON(http.StatusOK, LoginResponse{Success: false, NeedTOTP: true, ...})
        return
    }
    if !validateTOTP(admin.TOTPSecret, req.TOTPCode) {
        c.JSON(http.StatusUnauthorized, LoginResponse{Success: false, Message: "验证码错误"})
        return
    }
}
```

---

### C9. SQLite 未配置 WAL 模式，并发写入将失败

**文件**: `server/internal/repository/sqlite.go` 第 22-55 行

**问题描述**:
SQLite 默认使用 `journal_mode=DELETE`，不支持并发写入。多个 goroutine（聚合服务、心跳检查、Agent 状态更新）同时写入时会报 `database is locked` 错误。没有设置 `busy_timeout`。

**修复建议**:
```go
db, err := gorm.Open(sqlite.Open(dbPath + "?_journal_mode=WAL&_busy_timeout=5000"), &gorm.Config{
    Logger: logger.Default.LogMode(logger.Warn),
})
```

---

### C10. 登录限速器内存泄漏

**文件**: `server/internal/api/middleware.go` 第 56-97 行

**问题描述**:
```go
var rateLimiter = &loginRateLimiter{
    attempts: make(map[string][]time.Time),
}
```

`rateLimiter.attempts` map 为全局变量，只清理单个 IP 的过期记录，从不删除已不活跃 IP 的 key。长时间运行后，每个曾尝试登录的 IP 都会永久占据 map 空间。

**修复建议**:
增加定期清理逻辑，或使用带 TTL 的缓存库（如 `golang.org/x/time/rate` 或 `github.com/patrickmn/go-cache`）。

---

## 二、中等问题 (Medium)

### M1. JWT Token 同时存储在 Cookie 和 localStorage

**文件**: `server/internal/api/handler_auth.go` 第 94-99 行
**文件**: `server/frontend/src/lib/api.ts` 第 19-31 行

**问题描述**:
后端将 Token 设置到 HttpOnly Cookie，同时又在响应体中返回 Token。前端将 Token 存入 localStorage 并通过 `Authorization` header 发送。localStorage 中的 Token 可被 XSS 攻击窃取。

**修复建议**:
选择一种认证方式。推荐仅使用 HttpOnly Cookie（防 XSS），移除响应体中的 Token 和 localStorage 存储。前端 API 请求仅依赖 Cookie（`credentials: 'include'`），移除 `Authorization` header 逻辑。

---

### M2. WebSocket CheckOrigin 允许所有来源

**文件**: `server/internal/api/handler_agent.go` 第 36-40 行
**文件**: `server/internal/api/handler_dashboard_ws.go` 第 27-32 行

**问题描述**:
```go
CheckOrigin: func(r *http.Request) bool {
    return true // 允许所有来源（生产环境应限制）
}
```

注释已指出问题但未修复。恶意网站可建立 WebSocket 连接到 Server。

**修复建议**:
验证 Origin 头是否在白名单中，或至少验证 Origin 与 Server 同源。

---

### M3. Agent 删除未清理关联数据（资源泄漏）

**文件**: `server/internal/api/handler_agent_api.go` 第 117-135 行

**问题描述**:
`HandleDeleteAgent` 仅删除数据库中的 Agent 记录，未清理:
1. 内存中的 RingBuffer
2. 活跃的 WebSocket 连接
3. 历史监控记录（metric_records 表）
4. 告警状态（alert engine 的 states map）

**修复建议**:
```go
func (h *AgentAPIHandler) HandleDeleteAgent(c *gin.Context) {
    // ...
    h.monitor.UnregisterConnection(agentID)      // 关闭连接
    h.monitor.RemoveRingBuffer(agentID)           // 清理 ring buffer
    h.recordRepo.DeleteByAgentID(agentID)         // 清理历史记录
    h.agentRepo.Delete(agentID)
}
```

---

### M4. SSRF 防护存在 DNS Rebinding 漏洞

**文件**: `server/internal/pkg/ssrf.go` 第 67-99 行、第 22-46 行

**问题描述**:
`CheckURL` 和 `SendWebhook` 中的 `Transport.DialContext` 都会进行 DNS 解析和 IP 检查，但两次解析之间存在时间窗口。攻击者可利用 DNS rebinding: 第一次解析返回公网 IP（通过检查），第二次解析返回内网 IP（实际连接）。

虽然 `DialContext` 中有二次检查，但 `CheckURL` 中的 `net.LookupIP` 和 `DialContext` 中的 `net.DefaultResolver.LookupIP` 可能返回不同结果。

**修复建议**:
在 `CheckURL` 中解析 IP 并缓存，`DialContext` 使用缓存的 IP 直接连接，不再二次解析:
```go
// CheckURL 解析后返回 IP，SendWebhook 使用该 IP
func CheckURL(rawURL string) (string, error) {
    // 解析并验证 IP，返回安全的 IP 地址
}
```

---

### M5. 注册码生成存在模偏差

**文件**: `server/internal/service/agent_registry.go` 第 180-190 行

**问题描述**:
```go
const charset = "ABCDEFGHIJKLMNOPQRSTUVWXYZ0123456789"  // 36 个字符
bytes[i] = charset[bytes[i]%byte(len(charset))]         // 256 % 36 = 4，前 4 个字符概率略高
```

**修复建议**:
使用拒绝采样:
```go
maxVal := 256 - (256 % len(charset))
for {
    if _, err := rand.Read(bytes[i:i+1]); err != nil { return "", err }
    if int(bytes[i]) < maxVal {
        bytes[i] = charset[bytes[i]%byte(len(charset))]
        break
    }
}
```

---

### M6. 配置文件 cert_file 路径处理逻辑错误

**文件**: `server/cmd/server/main.go` 第 56-58 行

**问题描述**:
```go
if cfg.TLS.CertFile != "" {
    *certDir = filepath.Dir(cfg.TLS.CertFile)  // 将 certDir 设为证书文件所在目录
}
```

但后续代码用 `certDir` 来定位自签证书目录，逻辑混乱。如果配置了 `cert_file`，后面又检查 `cfg.TLS.CertFile != "" && cfg.TLS.KeyFile != ""` 来决定是否使用指定证书。`certDir` 的修改实际上没有意义，反而可能导致自签证书路径错误。

**修复建议**:
分离"指定证书路径"和"自签证书目录"两个概念，不要互相覆盖。

---

### M7. RingBuffer 容量硬编码，未使用配置

**文件**: `server/internal/service/monitor.go` 第 66 行、第 110 行

**问题描述**:
```go
m.ringBuffers[agentID] = repository.NewRingBuffer(3600)  // 硬编码 3600
```

配置文件中有 `ring_buffer.size` 字段，但 `MonitorService` 未接收此配置，始终使用 3600。

**修复建议**:
`NewMonitorService` 接收 `ringBufferSize` 参数。

---

### M8. AggregationService.Stop 和 AlertEngine.Stop 可多次调用导致 panic

**文件**: `server/internal/service/aggregation.go` 第 57-62 行
**文件**: `server/internal/service/alert.go` 第 72-77 行

**问题描述**:
```go
func (s *AggregationService) Stop() {
    if s.ticker != nil { s.ticker.Stop() }
    close(s.stopCh)  // 第二次调用会 panic: close of closed channel
}
```

**修复建议**:
使用 `sync.Once`:
```go
type AggregationService struct {
    // ...
    stopOnce sync.Once
}
func (s *AggregationService) Stop() {
    s.stopOnce.Do(func() {
        if s.ticker != nil { s.ticker.Stop() }
        close(s.stopCh)
    })
}
```

---

### M9. 告警状态仅存内存，重启后丢失

**文件**: `server/internal/service/alert.go` 第 20-28 行

**问题描述**:
`AlertEngine.states` 仅存储在内存中。Server 重启后所有告警状态重置为 OK，可能导致:
1. 已触发告警在重启后重新走 PENDING -> FIRING 流程，产生重复通知。
2. FIRING 状态的告警静默期被重置。

**修复建议**:
将告警状态持久化到数据库，重启时恢复。

---

### M10. 前端类型定义与后端 API 响应不匹配

**文件**: `server/frontend/src/types/index.ts`

**问题描述**:
1. `LoginResponse` 定义有 `expires_at` 字段，后端返回的是 `success`、`message`、`need_totp`、`token`。
2. `ServerListResponse` 定义有 `total` 字段，后端只返回 `{servers: []}`。
3. `HistoryData` 定义为 `{agent_id, range, points}`，后端返回 `{source, records}`。
4. `DashboardItem` 定义有 `disk_usage` 字段，后端 `DashboardItem` 结构体没有此字段。

**影响**: 前端类型检查通过但运行时数据缺失，可能导致 `undefined` 错误或图表不显示。

**修复建议**:
对齐前后端类型定义，或使用 OpenAPI/Swagger 自动生成前端类型。

---

### M11. 前端未处理 TOTP 两步验证流程

**文件**: `server/frontend/src/pages/Login.tsx` 第 38-57 行

**问题描述**:
后端 `HandleLogin` 可能返回 `{success: false, need_totp: true}`，但前端 `login` 函数直接 `throw new Error(message)`，没有处理 `need_totp` 响应。用户无法完成两步验证登录。

**修复建议**:
在 Login 组件中检查 `need_totp` 响应，显示 TOTP 输入框，二次提交时携带 `totp_code`。

---

### M12. Agent 配置拉取 Token 通过 URL 明文传递

**文件**: `agent/internal/config/sync.go` 第 77 行

**问题描述**:
```go
url := s.serverURL + "/api/v1/agent/config?token=" + s.token
```

Token 通过 URL query 参数传递，会被:
1. Web 服务器访问日志记录。
2. 中间代理/CDN 缓存。
3. 浏览器历史记录（如果通过浏览器访问）。

**修复建议**:
通过 `Authorization` header 或 POST body 传递 Token。

---

### M13. Dashboard WebSocket Token 通过 URL 传递

**文件**: `server/frontend/src/lib/websocket.ts` 第 40 行

**问题描述**:
```typescript
const wsUrl = `${this.url}?token=${encodeURIComponent(token)}`
```

WebSocket 协议不支持自定义 header（浏览器 API 限制），Token 只能通过 URL 传递。但 URL 会被记录在服务器访问日志中。

**修复建议**:
1. 使用短期一次性 Ticket 替代长期 Token: 前端先请求 `/api/v1/ws-ticket` 获取短期 ticket，再用 ticket 连接 WebSocket。
2. 或在连接建立后通过消息发送 Token 认证。

---

### M14. 前端 setup-status 请求失败时默认 needsSetup=true

**文件**: `server/frontend/src/store/useServerStore.ts` 第 117-122 行

**问题描述**:
```typescript
catch (err) {
    console.error('checkSetupStatus failed:', err)
    set({ needsSetup: true, authLoading: false })  // 网络错误也跳转到 setup
}
```

网络抖动或 Server 维护时，已部署系统的前端会误判为"需要初始化"，跳转到 Setup 页面。

**修复建议**:
网络错误时应保持当前状态或显示错误提示，不应改变 `needsSetup`。

---

### M15. upgrade-agent.sh 从 Server 下载 Agent，但 Server 未实现下载端点

**文件**: `scripts/upgrade-agent.sh` 第 78-81 行

**问题描述**:
```bash
AGENT_URL="${SERVER_URL}/download/agent/${OS}/${ARCH}"
```

后端路由中没有 `/download/agent/:os/:arch` 端点。升级脚本会下载失败。

**修复建议**:
1. 在后端实现 Agent 二进制下载端点（需要认证和版本管理）。
2. 或修改升级脚本从 GitHub Release 下载。

---

### M16. Dockerfile 硬编码 amd64 架构

**文件**: `Dockerfile` 第 24 行

**问题描述**:
```dockerfile
RUN CGO_ENABLED=0 GOOS=linux GOARCH=amd64 go build -o /bin/probe-server ./server/cmd/server
```

不支持 ARM64 Docker 镜像构建。

**修复建议**:
使用 `docker buildx` 多架构构建，或在 Dockerfile 中使用 `ARG TARGETARCH`:
```dockerfile
ARG TARGETARCH
RUN CGO_ENABLED=0 GOOS=linux GOARCH=${TARGETARCH} go build ...
```

---

### M17. HandleCheckSetup 泄露内部错误详情

**文件**: `server/internal/api/handler_auth.go` 第 183 行

**问题描述**:
```go
c.JSON(http.StatusInternalServerError, gin.H{"error": "检查失败", "detail": err.Error()})
```

向客户端返回内部错误详情，可能泄露数据库路径等敏感信息。

**修复建议**:
仅返回通用错误消息，详情记录在服务端日志中。

---

### M18. 日志中打印注册码

**文件**: `server/internal/service/agent_registry.go` 第 58 行

**问题描述**:
```go
log.Printf("生成注册码: %s, 名称: %s, 有效期 15 分钟", code, displayName)
```

注册码是敏感凭证，明文打印到日志中。如果日志被泄露，攻击者可在有效期内使用该注册码注册恶意 Agent。

**修复建议**:
日志中只显示注册码的部分内容: `log.Printf("生成注册码: %s****, 名称: %s", code[:4], displayName)`。

---

## 三、低级问题 (Low)

### L1. 未使用的导入和变量

**文件**: `agent/internal/collector/cpu.go` 第 4 行、第 234 行
- `bytes` 包导入但未使用，通过 `var _ = bytes.Buffer{}` 绕过编译检查。

**文件**: `server/internal/api/router.go` 第 115 行
- `var _ = websocket.ErrBadHandshake` 强制使用 websocket 包，但实际不需要。

**文件**: `server/internal/service/alert.go` 第 18 行
- `validator *DataValidator` 字段从未使用。

**文件**: `server/internal/api/handler_agent.go` 第 277 行
- `_ = agent` 获取了 agent 但未使用。

---

### L2. 硬编码 Agent 版本号

**文件**: `agent/cmd/agent/main.go` 第 43 行
```go
sysCollector := collector.NewSystemCollector(fileReader, "v1.0.0")
```
应通过 `-ldflags "-X main.Version=xxx"` 在构建时注入版本号。

---

### L3. WebSocket 重连未实现指数退避

**文件**: `agent/internal/reporter/ws.go` 第 288-292 行
```go
func (c *WSClient) getReconnectInterval() time.Duration {
    return 5 * time.Second  // 固定 5 秒，注释说"指数退避"但未实现
}
```

---

### L4. 前端 formatBytes 处理负数和零不当

**文件**: `server/frontend/src/lib/utils.ts` 第 2-8 行
```typescript
export function formatBytes(bytes: number, decimals = 2): string {
  if (bytes === 0 || bytes == null) return '0 B'
  const k = 1024
  const i = Math.floor(Math.log(bytes) / Math.log(k))  // 负数输入返回 NaN
```
应增加 `if (bytes < 0) return '0 B'` 或 `Math.abs(bytes)`。

---

### L5. 前端缺少 React ErrorBoundary

**文件**: `server/frontend/src/App.tsx`
没有 ErrorBoundary 组件，未捕获的渲染错误会导致白屏。

---

### L6. 前端 checkSetupStatus 在多个组件重复调用

**文件**: `App.tsx` 第 42-44 行、`Login.tsx` 第 20-22 行、`Setup.tsx` 第 21-23 行
三个组件都调用 `checkSetupStatus`，导致重复请求。

---

### L7. 前端 API 401 时强制跳转可能丢失表单数据

**文件**: `server/frontend/src/lib/api.ts` 第 54-61 行
```typescript
if (response.status === 401) {
    clearToken()
    window.location.href = '/login'  // 硬跳转，丢失 React 状态
}
```
应使用 React Router 的 `navigate` 而非 `window.location.href`。

---

### L8. install-server.sh 安装 Go 不校验完整性

**文件**: `scripts/install-server.sh` 第 83-86 行
直接 `curl | tar` 下载 Go，不校验 SHA256。如果 CDN 被劫持，可注入恶意 Go 工具链。

---

### L9. docker-compose.yml version 字段已弃用

**文件**: `docker-compose.yml` 第 1 行
```yaml
version: '3.8'
```
新版 Docker Compose 已弃用 `version` 字段，可删除。

---

### L10. CI/CD 缺少测试步骤

**文件**: `.github/workflows/release.yml`
发布流程中没有运行 `go test` 和前端测试，直接构建发布。

---

### L11. Agent WebSocket 端点无速率限制

**文件**: `server/internal/api/handler_agent.go` 第 46 行
`/api/v1/agent/report` WebSocket 端点没有连接速率限制，攻击者可发起大量连接消耗资源。

---

### L12. 前端 alert() 使用不当

**文件**: `server/frontend/src/pages/AgentManagement.tsx` 第 77、90、101 行
使用 `alert()` 显示错误，阻塞 UI 且体验差。应使用 Toast 通知。

---

### L13. SQLite 数据库文件未设置权限

**文件**: `server/internal/repository/sqlite.go` 第 24 行
```go
if err := os.MkdirAll(dataDir, 0755); err != nil {  // 目录权限 0755
```
数据目录权限 0755，数据库文件默认权限 0644，其他用户可读取。包含 JWT 密钥和 Agent Token 等敏感数据。

**修复建议**:
目录权限设为 0700，并通过 `PRAGMA` 或文件系统操作确保数据库文件权限为 0600。

---

### L14. cert.pem 文件权限过宽

**文件**: `server/internal/pkg/tls.go` 第 106 行
```go
certOut, err := os.Create(certPath)  // 默认权限 0666
```
虽然证书是公开信息，但应设为 0644 以明确权限。私钥文件已正确使用 0600。

---

### L15. Agent 端 config Syncer 每次创建新 HTTP Client

**文件**: `agent/internal/config/sync.go` 第 80-87 行
`sync()` 方法每次调用都创建新的 `http.Client` 和 `http.Transport`，浪费资源且无法复用连接。

**修复建议**:
在 `NewSyncer` 中创建一次 Client 并复用。

---

### L16. 前端 Layout 侧边栏底部信息可能重叠

**文件**: `server/frontend/src/components/Layout.tsx` 第 118 行
```tsx
<div className="absolute bottom-0 left-0 w-56 ...">
```
使用 `absolute` 定位，当导航项过多时可能与导航内容重叠。应使用 `flex` 布局将底部信息推到底部。

---

### L17. HandleListRegisterCodes 只返回未使用的注册码

**文件**: `server/internal/service/agent_registry.go` 第 149-151 行
```go
func (s *AgentRegistryService) ListRegisterCodes() ([]model.RegisterCode, error) {
    return s.registerRepo.ListUnused()  // 只返回未使用的
}
```
前端无法查看已使用的注册码历史，无法追溯哪个注册码注册了哪个 Agent。

---

### L18. Agent 上报数据时未更新数据库 LastSeen

**文件**: `server/internal/service/monitor.go` 第 103-144 行
`WriteMetricData` 只写入 RingBuffer，`UpdateHeartbeat` 只更新内存中的 `LastSeen`。数据库中的 `last_seen` 字段仅在 `RegisterConnection` 和 `UnregisterConnection` 时更新。Agent 长期在线时，数据库中的 `last_seen` 不会更新。

---

### L19. 前端 ServerDetail 历史数据轮询间隔过长

**文件**: `server/frontend/src/pages/ServerDetail.tsx` 第 96-104 行
非实时范围的历史数据每 5 分钟才刷新一次，用户可能看到过期数据。

---

### L20. Dockerfile ENTRYPOINT 仅支持 Server

**文件**: `Dockerfile` 第 44 行
```dockerfile
ENTRYPOINT ["probe-server"]
```
同一镜像中构建了 probe-agent，但 ENTRYPOINT 只能运行 Server。如需运行 Agent 需覆盖 ENTRYPOINT。

---

## 四、功能完整性检查总结

| 功能项 | 状态 | 说明 |
|--------|------|------|
| Agent 注册流程 | 不完整 | 字段映射错误 (C1)，重连失败 (C2) |
| Dashboard WebSocket 推送 | 基本可用 | 写并发不安全 (C4)，数据重复 (C6) |
| 前端显示 Agent 数据 | 基本可用 | 类型不匹配 (M10)，缺 disk_usage |
| Agent 管理页面 | 基本可用 | 删除不清理关联数据 (M3) |
| 登录/Setup 流程 | 基本可用 | TOTP 绕过 (C8)，前端未处理 TOTP (M11) |
| 历史数据查询 | 基本可用 | Ping 数据重复 (C6) |
| 告警系统 | 不完整 | 状态仅内存 (M9)，validator 未使用 (L1) |
| 配置文件读写 | 基本可用 | cert_file 路径处理错误 (M6) |

---

## 五、安全检查总结

| 安全项 | 状态 | 说明 |
|--------|------|------|
| JWT 认证 | 需改进 | Token 同时在 Cookie 和 localStorage (M1) |
| WebSocket 认证 | 需改进 | Token 通过 URL 传递 (M12, M13) |
| SSRF 防护 | 需改进 | DNS Rebinding 漏洞 (M4) |
| SQL 注入 | 安全 | GORM 参数化查询 |
| XSS 防护 | 安全 | React 默认转义 |
| 密码安全 | 安全 | bcrypt cost=12，强度验证 |
| 注册码安全 | 需改进 | 模偏差 (M5)，日志泄露 (M18) |
| TLS 配置 | 安全 | 强制 TLS 1.2+，自签证书私钥 0600 |
| 文件权限 | 需改进 | 数据库文件权限过宽 (L13) |
| systemd 安全 | 良好 | NoNewPrivileges, ProtectSystem 等已配置 |
| CORS | 不安全 | 反射任意 Origin (C7) |
| 并发安全 | 不安全 | DataValidator (C3)，WebSocket 写 (C4) |
