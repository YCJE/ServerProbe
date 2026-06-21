# 服务器探针系统 - 开发任务分解

> **版本**: v1.0
> **日期**: 2026-06-21
> **状态**: 待审核

---

## 开发原则

- **TDD（测试驱动开发）**：每个里程碑遵循"先写测试 → 实现 → 测试通过"的循环
- **安全优先**：所有代码必须符合 spec.md 第 2 节的 8 条安全原则
- **里程碑顺序**：M1→M2→M3→M4→M5→M6，M1-M4 为 MVP，M5-M6 为完整版本

---

## M1: 基础架构 + 核心采集（2 周）

**目标**：Agent 能采集系统指标，Server 能存储数据。

### M1-T1: 项目初始化

| 属性 | 内容 |
|------|------|
| 类型 | 代码 |
| 依赖 | 无 |
| 描述 | Go module 初始化，创建 server/ 和 agent/ 两个子项目目录结构，初始化共享 model 包 |

子任务：
- 创建 `server/` 目录结构（cmd/internal/frontend/web）
- 创建 `agent/` 目录结构（cmd/internal）
- 初始化 `server/go.mod` 和 `agent/go.mod`
- 创建共享 `model/` 包（agent.go, metric.go, alert.go, notify.go）
- 配置 `.gitignore`、`Makefile`、`VERSION` 文件

### M1-T2: CPU 采集器（TDD）

| 属性 | 内容 |
|------|------|
| 类型 | TDD |
| 依赖 | M1-T1 |
| 描述 | 读 `/proc/stat` 计算 CPU 使用率，读 `/proc/cpuinfo` 获取型号和核心数，读 `/proc/loadavg` 获取负载 |

子任务：
- 编写 CPU 采集器单元测试（使用 mock `/proc/stat` 数据）
- 实现 `agent/internal/collector/cpu.go`
- 测试 CPU 使用率计算逻辑
- 测试 CPU 型号和核心数解析
- 测试负载平均值解析

### M1-T3: 内存采集器（TDD）

| 属性 | 内容 |
|------|------|
| 类型 | TDD |
| 依赖 | M1-T1 |
| 描述 | 读 `/proc/meminfo` 解析 MemTotal/MemAvailable/SwapTotal/SwapFree |

子任务：
- 编写内存采集器单元测试
- 实现 `agent/internal/collector/memory.go`
- 测试 MemTotal/MemAvailable 解析
- 测试 SwapTotal/SwapFree 解析
- 测试内存使用率计算

### M1-T4: 磁盘采集器（TDD）

| 属性 | 内容 |
|------|------|
| 类型 | TDD |
| 依赖 | M1-T1 |
| 描述 | 系统调用 statfs 遍历挂载点获取磁盘使用率 |

子任务：
- 编写磁盘采集器单元测试
- 实现 `agent/internal/collector/disk.go`
- 测试 statfs 系统调用
- 测试挂载点遍历
- 测试磁盘使用率计算

### M1-T5: 网络采集器（TDD）

| 属性 | 内容 |
|------|------|
| 类型 | TDD |
| 依赖 | M1-T1 |
| 描述 | 读 `/proc/net/dev` 计算网卡速率差值，读 `/proc/net/tcp` 和 `/proc/net/udp` 统计连接数 |

子任务：
- 编写网络采集器单元测试
- 实现 `agent/internal/collector/network.go`
- 测试网卡流量速率差值计算
- 测试 TCP/UDP 连接数统计
- 测试首次调用与后续调用的差值逻辑

### M1-T6: 系统信息采集器（TDD）

| 属性 | 内容 |
|------|------|
| 类型 | TDD |
| 依赖 | M1-T1 |
| 描述 | uname 系统调用获取系统信息，读 `/proc/uptime` 获取运行时间，遍历 `/proc` 统计进程数 |

子任务：
- 编写系统信息采集器单元测试
- 实现 `agent/internal/collector/system.go`
- 测试 uname 系统调用
- 测试运行时间解析
- 测试进程数统计

### M1-T7: SQLite 数据层（TDD）

| 属性 | 内容 |
|------|------|
| 类型 | TDD |
| 依赖 | M1-T1 |
| 描述 | SQLite 连接管理，GORM AutoMigrate，repository 层 CRUD |

子任务：
- 编写 SQLite 数据层单元测试
- 实现 `server/internal/repository/sqlite.go`
- 实现 GORM AutoMigrate（所有表结构）
- 实现 `repo_agent.go`（Agent 元数据 CRUD）
- 实现 `repo_alert.go`（告警规则 CRUD）
- 实现 `repo_notify.go`（通知渠道 CRUD）
- 实现 `repo_record.go`（历史聚合数据 CRUD）

### M1-T8: 环形缓冲（TDD）

| 属性 | 内容 |
|------|------|
| 类型 | TDD |
| 依赖 | M1-T1 |
| 描述 | 内存环形缓冲，支持并发安全写入读取，每 Agent 一个实例 |

子任务：
- 编写环形缓冲单元测试（覆盖率 ≥ 90%）
- 实现 `server/internal/repository/ringbuffer.go`
- 测试写入/读取/覆盖逻辑
- 测试并发安全（sync.Mutex 或 channel）
- 测试预分配 3600 点容量

### M1 验收标准

- [ ] Agent 能采集 CPU/内存/磁盘/网络/系统信息并输出到 stdout（JSON）
- [ ] Server 能初始化 SQLite 并通过 repository 层 CRUD
- [ ] 环形缓冲单元测试覆盖率 ≥ 90%
- [ ] 所有采集器不需要 root 权限

---

## M2: Agent-Server 通信 + 注册上线（2 周）

**目标**：Agent 能注册到 Server 并维持 WebSocket 连接，数据能上报。

### M2-T1: WebSocket 服务端（TDD）

| 属性 | 内容 |
|------|------|
| 类型 | TDD |
| 依赖 | M1-T7, M1-T8 |
| 描述 | Server 端 WS 端点 `/api/v1/agent/report`，连接管理，消息分发 |

子任务：
- 编写 WebSocket 服务端单元测试
- 实现 `server/internal/api/handler_agent.go`
- 实现 Agent 连接池（`map[agentID]*Conn`）
- 实现消息分发（按 type 字段路由到不同 handler）
- 实现心跳超时检测（90 秒无心跳标记离线）

### M2-T2: WebSocket 客户端（TDD）

| 属性 | 内容 |
|------|------|
| 类型 | TDD |
| 依赖 | M1-T2 至 M1-T6 |
| 描述 | Agent 端 WS 客户端，自动重连，心跳维持，数据上报 |

子任务：
- 编写 WebSocket 客户端单元测试
- 实现 `agent/internal/reporter/ws.go`
- 实现自动重连（指数退避 1s→2s→4s→...→60s 上限）
- 实现 `agent/internal/reporter/heartbeat.go`（每 30 秒心跳）
- 实现 `agent/internal/reporter/upload.go`（每 3 秒打包采集数据发送）

### M2-T3: 注册流程（TDD）

| 属性 | 内容 |
|------|------|
| 类型 | TDD |
| 依赖 | M2-T1 |
| 描述 | 注册码生成/验证/一次性消费，Token 签发，主机指纹绑定 |

子任务：
- 编写注册流程单元测试
- 实现 `server/internal/service/agent_registry.go`
- 实现注册码生成（随机 8 位）
- 实现注册码验证和一次性消费
- 实现注册码 15 分钟过期
- 实现注册码数量限制（最多 5 个未使用）
- 实现 Token 生成（随机 32 字节）
- 实现 Token 持久化和主机指纹绑定
- 实现 `agent/internal/register/register.go`（首次启动用注册码注册）

### M2-T4: 强制 TLS

| 属性 | 内容 |
|------|------|
| 类型 | 代码 |
| 依赖 | M1-T1 |
| 描述 | Server TLS 配置，Agent 拒绝明文连接 |

子任务：
- 实现 `server/internal/pkg/tls.go`
- Server TLS 配置，无证书时自动生成自签证书
- Agent 拒绝 `http://` 和 `ws://` 连接
- Agent 仅接受 `https://` 和 `wss://`

### M2-T5: 数据上报协议（TDD）

| 属性 | 内容 |
|------|------|
| 类型 | TDD |
| 依赖 | M2-T1, M2-T2 |
| 描述 | report/heartbeat 消息的序列化/反序列化 |

子任务：
- 编写数据上报协议单元测试
- 实现 report 消息的 JSON 序列化/反序列化
- 实现 heartbeat 消息的 JSON 序列化/反序列化
- 实现 ping_result 消息的 JSON 序列化/反序列化
- 实现 register 消息的 JSON 序列化/反序列化

### M2-T6: 配置拉取（TDD）

| 属性 | 内容 |
|------|------|
| 类型 | TDD |
| 依赖 | M2-T4 |
| 描述 | Agent 定时 HTTPS GET 拉取探测目标配置 |

子任务：
- 编写配置拉取单元测试
- 实现 `agent/internal/config/sync.go`
- 实现每小时 HTTPS GET `/api/v1/agent/config`
- 实现本地缓存，拉取失败用缓存
- 实现 `server/internal/service/config_sync.go`

### M2-T7: 数据合理性校验（TDD）

| 属性 | 内容 |
|------|------|
| 类型 | TDD |
| 依赖 | M2-T1 |
| 描述 | 上报数据范围校验，频率限制 |

子任务：
- 编写数据合理性校验单元测试
- 实现 CPU 使用率校验（0-100%）
- 实现内存使用率校验（0-100%，used ≤ total）
- 实现磁盘使用率校验（0-100%）
- 实现延迟校验（0-60000ms）
- 实现丢包率校验（0-100%）
- 实现上报频率限制（每 3 秒 ±1 秒）
- 实现数据大小限制（单次上报 ≤ 10KB）

### M2 验收标准

- [ ] Agent 能通过注册码注册并获取 Token
- [ ] Agent 能维持 WebSocket 长连接，3 秒上报一次数据
- [ ] 注册码一次性消费，15 分钟过期
- [ ] 全程 TLS 加密，明文连接被拒绝
- [ ] 断线自动重连（指数退避：1s→2s→4s→...→60s 上限）

---

## M3: 实时监控 + 前端面板（3 周）

**目标**：浏览器面板能实时展示服务器状态。

### M3-T1: 前端项目初始化

| 属性 | 内容 |
|------|------|
| 类型 | 代码 |
| 依赖 | M2 完成 |
| 描述 | React + Vite + shadcn/ui + Tailwind + ECharts + Zustand + React Router |

子任务：
- 初始化 Vite + React 18 + TypeScript 项目
- 配置 Tailwind CSS
- 安装和配置 shadcn/ui
- 安装 ECharts、Zustand、React Router
- 配置 ESLint、Prettier
- 创建页面结构（仪表盘、服务器详情、告警、通知、设置、分享）

### M3-T2: 登录页（TDD）

| 属性 | 内容 |
|------|------|
| 类型 | TDD |
| 依赖 | M3-T1 |
| 描述 | 密码登录，限速，JWT Cookie |

子任务：
- 编写登录页测试
- 实现登录表单组件
- 实现密码登录 API 调用
- 实现限速提示（5 次/分钟）
- 实现 JWT Cookie 存储（HttpOnly + Secure + SameSite=Strict）
- 实现可选 TOTP 两步验证界面

### M3-T3: 仪表盘页

| 属性 | 内容 |
|------|------|
| 类型 | 代码 |
| 依赖 | M3-T1, M3-T2 |
| 描述 | 服务器卡片网格，含延迟/丢包率展示 |

子任务：
- 实现概览栏（在线/离线数、平均 CPU、平均内存）
- 实现服务器卡片组件
- 卡片展示：在线状态、运行时间、CPU/内存/磁盘进度条、上下行速度
- 卡片展示：三网延迟和丢包率
- 丢包率 > 0 橙色标注，> 20% 红色标注
- 离线服务器延迟/丢包显示 `---`
- 点击卡片跳转详情页

### M3-T4: 服务器详情页

| 属性 | 内容 |
|------|------|
| 类型 | 代码 |
| 依赖 | M3-T3 |
| 描述 | CPU/内存实时折线图，延迟丢包融合图 |

子任务：
- 实现 CPU 占用率实时折线图（ECharts）
- 实现内存占用率实时折线图
- 实现延迟与丢包率融合图（上半部分延迟折线图三网三条线，下半部分丢包率柱状图，共享 X 轴）
- 实现时间范围切换（1H/6H/12H/1D/2D）
- 实现磁盘使用率、网络流量、网络连接信息卡片
- 实现系统信息卡片
- 实现鼠标悬停显示详细数据

### M3-T5: WebSocket 实时推送（TDD）

| 属性 | 内容 |
|------|------|
| 类型 | TDD |
| 依赖 | M3-T1 |
| 描述 | Server→浏览器 WS 推送，前端 Zustand store |

子任务：
- 编写 WebSocket 实时推送测试
- 实现 Server 端 `/ws/dashboard` 端点
- 实现 JWT 认证 WS 端点
- 实现每 3 秒推送所有在线 Agent 的实时数据
- 实现前端 Zustand store 管理实时数据状态
- 实现 React 组件重渲染和 ECharts 图表更新

### M3-T6: 时间范围切换

| 属性 | 内容 |
|------|------|
| 类型 | 代码 |
| 依赖 | M3-T4, M3-T5 |
| 描述 | 1H/6H/12H/1D/2D，混合数据源拼接 |

子任务：
- 实现 1H 数据源（环形缓冲，3 秒/点，WebSocket 实时）
- 实现 6H 数据源（环形缓冲 + SQLite 聚合，混合拼接）
- 实现 12H/1D/2D 数据源（SQLite 聚合，5 分钟/点，定时轮询）
- 实现时间范围切换 UI 组件

### M3-T7: 主题系统

| 属性 | 内容 |
|------|------|
| 类型 | 代码 |
| 依赖 | M3-T1 |
| 描述 | 浅色/深色/跟随系统，localStorage 存储，CSS 变量切换 |

子任务：
- 实现浅色/深色/跟随系统主题
- 实现 localStorage 存储主题选择
- 实现 CSS 变量切换
- 实现 shadcn/ui 组件自动适配
- 实现主题切换 UI 组件

### M3-T8: Go embed 前端

| 属性 | 内容 |
|------|------|
| 类型 | 代码 |
| 依赖 | M3-T1 至 M3-T7 |
| 描述 | 前端构建产物内嵌到 Server 二进制 |

子任务：
- 实现 `server/internal/pkg/embed.go`
- 配置 Vite 构建输出到 `web/` 目录
- 实现 Go embed 引用 `web/` 目录
- 实现静态文件服务路由

### M3-T9: 面板 WS 认证（TDD）

| 属性 | 内容 |
|------|------|
| 类型 | TDD |
| 依赖 | M3-T5 |
| 描述 | JWT 认证 WS 端点 |

子任务：
- 编写 WS 认证测试
- 实现 WS 连接时 JWT 验证
- 实现无 Token 连接拒绝
- 实现过期 Token 刷新

### M3 验收标准

- [ ] 管理员能登录面板
- [ ] 仪表盘卡片展示 CPU/内存/磁盘/网络/延迟/丢包率
- [ ] 服务器详情页有 CPU/内存实时折线图和延迟丢包融合图
- [ ] 5 档时间范围切换正常，1H/6H 实时刷新
- [ ] 主题切换正常
- [ ] 单二进制部署（前端内嵌）

---

## M4: 网络探测(Ping) + 历史数据（2 周）

**目标**：Agent 执行三网 Ping 探测，历史数据落盘和查询。

### M4-T1: ICMP Ping 采集器（TDD）

| 属性 | 内容 |
|------|------|
| 类型 | TDD |
| 依赖 | M1-T1 |
| 描述 | pro-bing privileged 模式，10 包采样，完整统计 |

子任务：
- 编写 ICMP Ping 采集器单元测试
- 实现 `agent/internal/collector/ping.go`
- 实现 privileged 模式（需 CAP_NET_RAW）
- 实现 10 个包采样，发包间隔 0.5 秒，总超时 15 秒
- 实现 DNS 预解析排除 DNS 时间
- 实现报告 avg_latency, min_latency, max_latency, jitter(StdDevRtt), loss, packets_sent, packets_recv

### M4-T2: Unprivileged ICMP 降级（TDD）

| 属性 | 内容 |
|------|------|
| 类型 | TDD |
| 依赖 | M4-T1 |
| 描述 | Linux 3.0+ SOCK_DGRAM ICMP socket，无需 root |

子任务：
- 编写 unprivileged ICMP 单元测试
- 实现 unprivileged ICMP 模式
- 实现降级逻辑检测

### M4-T3: TCP Ping 采集器（TDD）

| 属性 | 内容 |
|------|------|
| 类型 | TDD |
| 依赖 | M4-T1 |
| 描述 | 5 次采样，每次超时 5 秒，间隔 0.5 秒 |

子任务：
- 编写 TCP Ping 采集器单元测试
- 实现 TCP Ping 逻辑
- 实现 5 次采样，5 秒超时，0.5 秒间隔
- 实现 DNS 预解析
- 实现报告 5 次平均延迟和失败比例

### M4-T4: HTTP Ping 采集器（TDD）

| 属性 | 内容 |
|------|------|
| 类型 | TDD |
| 依赖 | M4-T1 |
| 描述 | 3 次采样，每次超时 10 秒，排除 DNS 时间 |

子任务：
- 编写 HTTP Ping 采集器单元测试
- 实现 HTTP Ping 逻辑
- 实现 3 次采样，10 秒超时
- 实现自定义 DialContext 排除 DNS 时间
- 实现状态码 2xx-3xx 为成功

### M4-T5: Ping 方法自动选择（TDD）

| 属性 | 内容 |
|------|------|
| 类型 | TDD |
| 依赖 | M4-T1 至 M4-T4 |
| 描述 | privileged→unprivileged→TCP 降级逻辑 |

子任务：
- 编写 Ping 方法自动选择单元测试
- 实现优先 privileged ICMP
- 实现降级 unprivileged ICMP
- 实现降级 TCP Ping
- 实现运行时探测方式标注

### M4-T6: 探测目标配置同步（TDD）

| 属性 | 内容 |
|------|------|
| 类型 | TDD |
| 依赖 | M2-T6 |
| 描述 | Agent 拉取目标列表，本地缓存 |

子任务：
- 编写探测目标配置同步单元测试
- 实现 Agent 每小时拉取服务端配置的探测目标列表
- 实现本地缓存，拉取失败用缓存
- 实现默认每 60 秒执行一轮完整探测
- 实现探测间隔可配置（30 秒或 120 秒）

### M4-T7: 数据聚合落盘（TDD）

| 属性 | 内容 |
|------|------|
| 类型 | TDD |
| 依赖 | M1-T7, M1-T8 |
| 描述 | 每 5 分钟将环形缓冲实时数据聚合为一个点写入 SQLite |

子任务：
- 编写数据聚合落盘单元测试
- 实现每 5 分钟聚合定时器
- 实现聚合策略：CPU/内存取平均值，网络取累计值
- 实现写入 SQLite metric_records 表

### M4-T8: 历史数据查询 API（TDD）

| 属性 | 内容 |
|------|------|
| 类型 | TDD |
| 依赖 | M4-T7 |
| 描述 | 按时间范围查询聚合数据 |

子任务：
- 编写历史数据查询 API 单元测试
- 实现 `GET /api/v1/servers/:id/history?range=1h|6h|12h|1d|2d`
- 实现 1h/6h 从环形缓冲读取
- 实现 12h+ 从 SQLite 读取
- 实现 6h 混合数据源拼接

### M4-T9: 历史数据清理（TDD）

| 属性 | 内容 |
|------|------|
| 类型 | TDD |
| 依赖 | M4-T7 |
| 描述 | 定时清理超期数据（默认 90 天） |

子任务：
- 编写历史数据清理单元测试
- 实现定时清理任务
- 实现默认 90 天保留期
- 实现清理日志记录

### M4 验收标准

- [ ] ICMP Ping 10 包采样，报告完整统计（平均/最小/最大/抖动/丢包率）
- [ ] Ping 方法自动降级正常
- [ ] 历史数据每 5 分钟落盘，查询 API 正常
- [ ] 超期数据自动清理

---

## M5: 告警 + 通知 + 安全加固（2 周）

**目标**：告警引擎、通知渠道、安全审计全部就绪。

### M5-T1: 告警规则 CRUD（TDD）

| 属性 | 内容 |
|------|------|
| 类型 | TDD |
| 依赖 | M1-T7 |
| 描述 | API + 前端管理页 |

子任务：
- 编写告警规则 CRUD 单元测试
- 实现告警规则 API（GET/POST/PUT/DELETE `/api/v1/alerts`）
- 实现前端告警管理页
- 实现规则字段：name, metric, operator, threshold, duration, enabled, notify_channel_id

### M5-T2: 告警状态机（TDD）

| 属性 | 内容 |
|------|------|
| 类型 | TDD |
| 依赖 | M5-T1 |
| 描述 | OK→PENDING→FIRING→RESOLVED |

子任务：
- 编写告警状态机单元测试
- 实现 `server/internal/service/alert.go`
- 实现 OK → PENDING（超阈值但未达 duration）
- 实现 PENDING → FIRING（达到 duration）
- 实现 FIRING → RESOLVED（恢复正常）
- 实现进入 FIRING 时发送告警通知
- 实现进入 RESOLVED 时发送恢复通知

### M5-T3: 通知去重 + 静默期（TDD）

| 属性 | 内容 |
|------|------|
| 类型 | TDD |
| 依赖 | M5-T2 |
| 描述 | 同一告警静默期内不重复 |

子任务：
- 编写通知去重单元测试
- 实现同一告警 FIRING 状态下静默期内不重复发送
- 实现默认静默 60 分钟
- 实现静默期可配置

### M5-T4: Webhook 通知 + SSRF 防护（TDD）

| 属性 | 内容 |
|------|------|
| 类型 | TDD |
| 依赖 | M5-T3 |
| 描述 | 内网过滤/重定向检测/响应体限制 |

子任务：
- 编写 Webhook 通知 + SSRF 防护单元测试
- 实现 `server/internal/pkg/ssrf.go`
- 实现内网地址过滤（10/8, 172.16/12, 192.168/16, 127/8, 169.254/16, ::1, fc00::/7）
- 实现 DNS 重绑定防护（自定义 Dialer 强制使用预解析 IP）
- 实现重定向检测（CheckRedirect 中再次 SSRF 检查）
- 实现响应体限制（最多读 1KB，不反射给用户）
- 实现超时 10 秒，强制 TLS 验证
- 实现 `server/internal/service/notify.go` Webhook 发送

### M5-T5: Telegram 通知（TDD）

| 属性 | 内容 |
|------|------|
| 类型 | TDD |
| 依赖 | M5-T4 |
| 描述 | Bot API 发送 |

子任务：
- 编写 Telegram 通知单元测试
- 实现 Telegram Bot API 发送
- 实现消息格式化

### M5-T6: 邮件通知（TDD）

| 属性 | 内容 |
|------|------|
| 类型 | TDD |
| 依赖 | M5-T4 |
| 描述 | SMTP 发送 |

子任务：
- 编写邮件通知单元测试
- 实现 SMTP 发送
- 实现邮件模板

### M5-T7: TOTP 两步验证（TDD）

| 属性 | 内容 |
|------|------|
| 类型 | TDD |
| 依赖 | M2 完成 |
| 描述 | TOTP 密钥生成/验证 |

子任务：
- 编写 TOTP 单元测试
- 实现 `server/internal/pkg/auth.go` TOTP 功能
- 实现 TOTP 密钥生成
- 实现 TOTP 验证
- 实现前端 TOTP 绑定和验证界面

### M5-T8: 公开分享页

| 属性 | 内容 |
|------|------|
| 类型 | 代码 |
| 依赖 | M3 完成 |
| 描述 | 白名单字段，无敏感信息 |

子任务：
- 实现分享页 API（`GET /api/v1/public/:share_id`）
- 实现分享页前端组件
- 实现白名单字段（仅展示非敏感信息，无 IP/Token/配置）
- 实现分享页管理界面

### M5-T9: 安全审计

| 属性 | 内容 |
|------|------|
| 类型 | 代码 |
| 依赖 | M5-T1 至 M5-T8 |
| 描述 | 按 spec.md 安全审计清单逐项审计 |

子任务：
- 全局搜索 `os/exec`、`exec.Command`，确认零匹配
- 全局搜索 `pty`、`shell`、`terminal`，确认 Agent 侧零匹配
- 确认所有 WebSocket 帧类型有明确定义，无"通用命令"帧
- 确认 TLS 配置不可关闭，Agent 拒绝明文连接
- 确认 JWT Cookie 配置为 HttpOnly + Secure + SameSite=Strict
- 确认登录限速生效
- 确认 Webhook 通知经过 SSRF 防护
- 确认 Agent systemd service 以 probe 用户运行
- 确认配置文件权限 600
- 确认注册码一次性 + 15 分钟有效期
- 确认主机指纹校验生效
- 确认数据合理性校验生效
- 确认公开分享页不包含敏感信息
- 确认所有 WebSocket 端点需要认证

### M5 验收标准

- [ ] 告警规则能正确触发和恢复
- [ ] Webhook 通知经过 SSRF 防护，内网地址被拒绝
- [ ] 通知去重和静默期生效
- [ ] TOTP 两步验证可用
- [ ] 安全审计清单全部通过

---

## M6: 部署 + 发布 + 文档（1 周）

**目标**：可发布的生产版本。

### M6-T1: Docker 镜像构建

| 属性 | 内容 |
|------|------|
| 类型 | 代码 |
| 依赖 | M5 完成 |
| 描述 | 多阶段构建，多架构 |

子任务：
- 编写 Dockerfile（多阶段：Node 构建前端 → Go 构建后端 → Alpine 运行时）
- 配置多架构构建（linux/amd64, linux/arm64）
- 安全加固：read_only, no-new-privileges, cap_drop ALL
- 编写 docker-compose.yml

### M6-T2: Server 一键安装脚本

| 属性 | 内容 |
|------|------|
| 类型 | 代码 |
| 依赖 | M5 完成 |
| 描述 | 检测架构/下载/配置/systemd |

子任务：
- 编写 `install-server.sh`
- 实现架构检测
- 实现二进制下载（校验 SHA256）
- 实现配置文件生成
- 实现 systemd service 安装
- 实现首次启动密码设置

### M6-T3: Agent 一键安装脚本

| 属性 | 内容 |
|------|------|
| 类型 | 代码 |
| 依赖 | M5 完成 |
| 描述 | 含注册码/创建用户/setcap/systemd |

子任务：
- 编写 `install-agent.sh`
- 实现参数解析（--server, --code）
- 实现系统和架构检测
- 实现二进制下载（校验 SHA256）
- 实现 probe 用户创建
- 实现配置文件生成（权限 600）
- 实现 setcap cap_net_raw+ep
- 实现 systemd service 安装（User=probe, 安全加固）
- 实现服务启动

### M6-T4: Agent 升级脚本

| 属性 | 内容 |
|------|------|
| 类型 | 代码 |
| 依赖 | M6-T3 |
| 描述 | 手动升级流程 |

子任务：
- 编写 `upgrade-agent.sh`
- 实现停止服务
- 实现备份旧版
- 实现替换新版
- 实现重新 setcap
- 实现启动服务

### M6-T5: GitHub Release CI/CD

| 属性 | 内容 |
|------|------|
| 类型 | 代码 |
| 依赖 | M6-T1 至 M6-T4 |
| 描述 | 自动构建发布 |

子任务：
- 编写 GitHub Actions workflow
- 实现自动构建所有平台二进制
- 实现生成 SHA256 校验文件
- 实现自动构建多架构 Docker 镜像
- 实现自动发布 GitHub Release

### M6-T6: 用户文档

| 属性 | 内容 |
|------|------|
| 类型 | 文档 |
| 依赖 | M6-T1 至 M6-T4 |
| 描述 | 安装/配置/使用/FAQ |

子任务：
- 编写安装指南（Docker、二进制）
- 编写配置说明
- 编写使用手册
- 编写 FAQ

### M6-T7: 安全文档

| 属性 | 内容 |
|------|------|
| 类型 | 文档 |
| 依赖 | M5-T9 |
| 描述 | SECURITY.md，安全报告流程 |

子任务：
- 编写 SECURITY.md
- 编写安全报告流程
- 编写安全最佳实践指南

### M6 验收标准

- [ ] Docker 部署成功
- [ ] 一键安装脚本在 Ubuntu/Debian/CentOS 上测试通过
- [ ] GitHub Release 包含所有平台二进制和 SHA256 校验
- [ ] 文档覆盖安装、配置、使用

---

## 任务依赖关系图

```
M1-T1 (项目初始化)
  ├── M1-T2 (CPU采集器)
  ├── M1-T3 (内存采集器)
  ├── M1-T4 (磁盘采集器)
  ├── M1-T5 (网络采集器)
  ├── M1-T6 (系统信息采集器)
  ├── M1-T7 (SQLite数据层)
  └── M1-T8 (环形缓冲)
        │
        ▼
M2-T1 (WebSocket服务端) ←── M1-T7, M1-T8
M2-T2 (WebSocket客户端) ←── M1-T2 至 M1-T6
M2-T3 (注册流程) ←── M2-T1
M2-T4 (强制TLS) ←── M1-T1
M2-T5 (数据上报协议) ←── M2-T1, M2-T2
M2-T6 (配置拉取) ←── M2-T4
M2-T7 (数据合理性校验) ←── M2-T1
        │
        ▼
M3-T1 (前端项目初始化) ←── M2 完成
M3-T2 (登录页) ←── M3-T1
M3-T3 (仪表盘页) ←── M3-T1, M3-T2
M3-T4 (服务器详情页) ←── M3-T3
M3-T5 (WebSocket实时推送) ←── M3-T1
M3-T6 (时间范围切换) ←── M3-T4, M3-T5
M3-T7 (主题系统) ←── M3-T1
M3-T8 (Go embed前端) ←── M3-T1 至 M3-T7
M3-T9 (面板WS认证) ←── M3-T5
        │
        ▼
M4-T1 (ICMP Ping采集器) ←── M1-T1
M4-T2 (Unprivileged ICMP) ←── M4-T1
M4-T3 (TCP Ping采集器) ←── M4-T1
M4-T4 (HTTP Ping采集器) ←── M4-T1
M4-T5 (Ping方法自动选择) ←── M4-T1 至 M4-T4
M4-T6 (探测目标配置同步) ←── M2-T6
M4-T7 (数据聚合落盘) ←── M1-T7, M1-T8
M4-T8 (历史数据查询API) ←── M4-T7
M4-T9 (历史数据清理) ←── M4-T7
        │
        ▼
M5-T1 (告警规则CRUD) ←── M1-T7
M5-T2 (告警状态机) ←── M5-T1
M5-T3 (通知去重+静默期) ←── M5-T2
M5-T4 (Webhook通知+SSRF防护) ←── M5-T3
M5-T5 (Telegram通知) ←── M5-T4
M5-T6 (邮件通知) ←── M5-T4
M5-T7 (TOTP两步验证) ←── M2 完成
M5-T8 (公开分享页) ←── M3 完成
M5-T9 (安全审计) ←── M5-T1 至 M5-T8
        │
        ▼
M6-T1 (Docker镜像构建) ←── M5 完成
M6-T2 (Server一键安装脚本) ←── M5 完成
M6-T3 (Agent一键安装脚本) ←── M5 完成
M6-T4 (Agent升级脚本) ←── M6-T3
M6-T5 (GitHub Release CI/CD) ←── M6-T1 至 M6-T4
M6-T6 (用户文档) ←── M6-T1 至 M6-T4
M6-T7 (安全文档) ←── M5-T9
```

---

## 技能使用规划

在实现阶段，将按以下规划使用用户指定的技能：

| 里程碑 | 技能 | 用途 |
|--------|------|------|
| M3 | `frontend-design` | 创建高质量前端界面 |
| M3 | `frontend-skill` | 视觉强烈的面板设计 |
| M3 | `shadcn` | shadcn/ui 组件管理 |
| M3 | `vercel-composition-patterns` | React 组件组合模式 |
| M3 | `vercel-react-best-practices` | React/Next.js 性能优化 |
| M3 | `chart-visualization` | ECharts 图表可视化 |
| M3 | `canvas-design` | 视觉设计 |
| M3 | `web-artifacts-builder` | 复杂前端组件构建 |
| M3 | `web-design-guidelines` | UI 代码审查 |
| M5 | `security-best-practices` | 安全最佳实践审查 |
| M3/M5 | `webapp-testing` | Web 应用测试 |
| M3/M5 | `dogfood` | 探索性测试和 QA |
| M3 | `vercel-react-native-skills` | 移动端适配参考 |
