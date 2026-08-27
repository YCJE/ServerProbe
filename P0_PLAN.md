# P0 实施计划：小时级 Rollup + 极值列 / 重连 Jitter

> **依据**: `COMPETITOR_ANALYSIS.md` §6 P0 优先级（方案 A）
> **状态**: 计划文档，未实施
> **原则**: 全部改动不触碰 Agent-Server 协议与控制通道（S1/S4/S10 合规）；数据库变更为"新增表 + 新增设置"，不修改现有表结构，无破坏性迁移

---

## 0. 背景与目标

**现状**:
- 历史数据单层：`metric_records` 每 5 分钟一条均值记录，默认保留 4 天（`settings.go:29`），最长查询范围 3 天
- 只有均值没有极值：5 分钟均值 CPU 50% 掩盖了其中 4 分钟 90% 的尖峰
- Agent 重连已有指数退避（`agent/internal/reporter/ws.go:523`，5s→60s，成功后重置），但**无 jitter**——Server 重启时全体 Agent 同步重试形成脉冲

**目标**:
1. 新增小时级聚合表：支持 7d/30d/90d/1y 长范围查询，长历史存储量恒定（每 Agent 每年仅 ~8760 行）
2. 新增极值列（min/max）：长范围图表能展示真实峰值
3. 重连间隔加 ±20% jitter，消除同步重连脉冲
4. 5 分钟层数据保留期默认 4 天 → 30 天（新安装），存量安装保持用户已设值

**非目标**:
- 不做多级 rollup（1m 层）、TDigest 百分位（P0 仅方案 A，p95/p99 属远期）
- 不改 Agent 上报协议、不改环形缓冲逻辑
- 不做前端 min/max 区带渲染（本轮仅透传字段，图表增强留到下一轮）

---

## 1. 任务总览

| 编号 | 任务 | 类型 | 预估 |
|------|------|------|------|
| A1 | `MetricRecordHourly` 数据模型 + AutoMigrate | 后端 | 0.5h |
| A2 | Hourly repository（upsert / 范围查询 / 清理 / 按 Agent 删除） | 后端 | 1h |
| A3 | 聚合服务新增小时 rollup 任务（含回填与离线占位） | 后端 | 2.5h |
| A4 | 保留策略拆分（新设置项 `retention_days_hourly`） | 后端 | 1h |
| A5 | 历史 API 双表路由（管理端 + 公开端点） | 后端 | 1.5h |
| A6 | 删除 Agent 联动清理小时表 | 后端 | 0.5h |
| A7 | 前端：新增时间范围 + 类型 + 设置项 | 前端 | 1.5h |
| A8 | 后端测试（rollup 正确性 / upsert 幂等 / 路由） | 测试 | 1.5h |
| B1 | 重连间隔加 jitter | Agent | 0.5h |
| B2 | jitter 单元测试 | 测试 | 0.5h |

---

## 2. P0-A：小时级 Rollup

### A1 数据模型

**文件**: `server/internal/model/models.go`（新增）、`server/internal/repository/sqlite.go:67`（AutoMigrate 清单追加）

```
表: metric_records_hourly
索引: uniqueIndex(agent_id, timestamp)  ← 幂等 upsert 的冲突键
```

字段设计（对齐 `MetricRecord` 语义，×10 整数缩放规则不变）:

| 字段 | 类型 | 语义（该小时内 12 条 5 分钟记录的聚合） |
|------|------|------|
| agent_id, timestamp | int64 | timestamp = 小时起始时间（对齐整点，`ts - ts%3600`） |
| cpu_usage / cpu_min / cpu_max | int ×10 | 均值 / 最小 / 最大 |
| mem_usage / mem_min / mem_max | float | 同上 |
| load_1 / load_1_max | int ×10 | 均值 / 最大（load_5/15 仅均值，与现表对齐） |
| net_rx / net_tx | int64 | 均值 |
| net_rx_max / net_tx_max | int64 | 峰值速率（网络尖峰是丢包前兆，价值最高） |
| mem_total/used, swap_total/used, disk_usage, uptime, process_count, tcp/udp_conns | 同现表 | 取小时内最后一条记录（与 5 分钟聚合口径一致） |
| ping_data | text | **按目标名对齐求均值**（见 A3） |
| offline | int | 多数规则：offline_samples ≥ sample_count/2 → 1 |
| sample_count / offline_samples | int | 该小时实际参与的 5 分钟记录数 / 其中离线占位数 |

> 现有 `metric_records` 表**完全不动**，无列变更、无数据迁移，AutoMigrate 仅建新表。

### A2 Repository

**文件**: `server/internal/repository/repo_record.go`（扩展 `RecordRepository`）

复用 `repo_traffic.go:24` 的原生 SQL upsert 模式（GORM OnConflict 对 SQLite 支持有限）:

- `UpsertHourly(rec *model.MetricRecordHourly)`：`ON CONFLICT(agent_id, timestamp) DO UPDATE SET <全部指标列>`——整行覆盖语义（小时记录由 12 条 5 分钟行全量重算，不是累加）
- `GetHourlyByAgentAndTimeRange(agentID, start, end)`
- `GetLastHourlyTimestamp(agentID)`：rollup 增量起点（启动时读一次）
- `CleanupHourlyExpired(retentionDays)`：与 `CleanupExpired` 同构
- `DeleteHourlyByAgentID(agentID)`
- 小时表写入量极低（每 Agent 每小时 1 条），**不走 BatchWriter**，直接 GORM/原生 SQL

### A3 聚合任务（核心）

**文件**: `server/internal/service/aggregation.go`

**触发方式**：不新增独立 ticker。在现有 5 分钟 `aggregate()` 末尾追加 `rollupHourly()` 调用——幂等 upsert 允许任意频率重跑，5 分钟粒度的检查成本可忽略。

**算法**（每 Agent 独立）:

```
lastRolled := 内存缓存（启动时 = GetLastHourlyTimestamp(agent)）
currentCompleteHour := now - now%3600 - 3600   // 上一个已完整结束的小时
for hour := lastRolled + 3600; hour <= currentCompleteHour; hour += 3600:
    rows := SELECT * FROM metric_records
            WHERE agent_id=? AND timestamp >= hour AND timestamp < hour+3600
    if len(rows) == 0:
        写 offline=1, sample_count=0 占位行        ← 修复离线时段时间线空洞
    else:
        对 12 条行等权聚合（5 分钟行本身是等宽窗口的均值，等权正确）:
        - 均值列 = mean(rows)，min/max 列 = min/max(rows)
        - offline 过滤：均值计算排除 offline=1 的行（其指标为零值），
          sample_count = 在线行数，offline_samples = 离线行数
        - mem_total/disk/uptime/process 等取最后一条在线行
        - ping_data：按 target 字符串分组，对每组 avg_latency/jitter/loss 求均值后重组 JSON
    UpsertHourly(...)
更新 lastRolled
```

**崩溃安全**: lastRolled 只从数据库读取（启动时），内存缓存仅做增量优化；任何小时缺失都会在下次循环补算（upsert 覆盖）。

**首次回填**: 启动时若 `GetLastHourlyTimestamp` 无记录，从 `metric_records` 中该 Agent 的最早记录时间开始 rollup。存量安装默认只有 4 天数据 → 每 Agent 约 96 小时，10 Agent 共 ~960 行，一次补算秒级完成。

**已知边界（写入计划备注，实施时验证）**: 现 5 分钟聚合对"环形缓冲已删除的离线 Agent"不写占位行（`aggregate()` 中 `rb == nil` 直接 continue），因此长离线时段的 5 分钟源数据有空洞；小时层 `len(rows)==0 → offline 占位` 恰好治愈该空洞的时间线显示。

### A4 保留策略拆分

**文件**: `server/internal/service/settings.go`、`server/internal/api/handler_settings.go`

| 设置键 | 默认 | 范围 | 语义 |
|--------|------|------|------|
| `retention_days`（现有） | 4 → **30**（仅新安装） | 1–3650（不变） | 5 分钟层保留期 |
| `retention_days_hourly`（新增） | 730 | 30–3650 | 小时层保留期 |

- **存量安装迁移策略**：已持久化的 `retention_days` 值原样保留（无法区分"用户显式设 4"与"默认 4"，宁可不猜）；设置页 UI 加提示建议调至 30
- `SettingsResponse` / `UpdateSettings` 请求体各加一个字段；校验范围 30 ≤ hourly ≤ 3650
- `StartCleanupTask` 改为同一任务里清理两张表（`cmd/server/main.go:138` 接线不变）

### A5 历史 API 双表路由

**文件**: `server/internal/api/handler_server.go`（`HandleGetServerHistory:370` + 公开端点 `publicHistoryPoint` 附近）

路由规则（显式映射，不做动态推断）:

| range 参数 | 数据源 | 粒度 | 点数（降采样前） |
|-----------|--------|------|----------------|
| 1h / 6h / 12h / 1d / 2d / 3d | `metric_records` | 5 min | ≤ 864 |
| **7d / 30d / 90d / 1y**（新增） | `metric_records_hourly` | 1 h | 168 / 720 / 2160 / 8760 |

- `switch rangeStr` 扩展四个 case，选表后共用现有降采样逻辑（`maxChartPoints` 抽稀保留首尾，`handler_server.go:407` 已实现）
- `historyPoint` 响应结构新增可选字段 `cpu_min/cpu_max/mem_min/mem_max/net_rx_max/net_tx_max/load1_max`（5 分钟层查询时为零值/省略）
- 公开端点同步路由，且继续过滤探测目标 IP 等敏感字段（沿用现白名单逻辑）
- `offline` → `online` 换算沿用 `1 - r.Offline`

### A6 删除 Agent 联动

**文件**: 删除 Agent 的 handler（现调用 `DeleteByAgentID` 处）

追加 `DeleteHourlyByAgentID`，防止孤儿数据撑大库文件。

### A7 前端

**文件**:
- `frontend/src/types/index.ts`：`TimeRange` 联合类型加 `'7d' | '30d' | '90d' | '1y'`；历史点类型加可选 min/max 字段
- `frontend/src/pages/ServerDetail.tsx:28` 与 `PublicServerDetail.tsx`：`TIME_RANGES` 追加 4 项（公开页可只加 7d/30d）
- `frontend/src/lib/api.ts`：如 range 参数为字符串类型则无需改；核对 `getServerHistory` 签名
- 设置页（站点设置 → 数据管理）：新增"小时数据保留天数"输入项，与现有保留天数并列
- 默认历史范围逻辑（站点设置 default_history_range）若其枚举含新范围则同步

### A8 测试

| 用例 | 断言 |
|------|------|
| upsert 幂等 | 同一小时重算两次，行数不变、指标为最新值 |
| rollup 数值 | 12 条构造行 → 均值/min/max/offline_samples 精确匹配 |
| 离线混入 | 12 行中 6 行 offline → offline=1、均值仅用 6 行在线数据 |
| 空洞占位 | 无 5 分钟行的小时 → offline=1 且 sample_count=0 |
| ping 均值 | 两个目标交错出现 → 按目标名分组各自平均 |
| 路由 | range=7d 走小时表、range=3d 走 5 分钟表、未知 range 回退 1h |
| 清理 | 过期行删除、未过期保留 |

---

## 3. P0-B：重连 Jitter

### B1 实现

**文件**: `agent/internal/reporter/ws.go:523`（`getReconnectInterval`）

现有退避序列 5s → 10s → 20s → 40s → 60s（封顶）已符合 NodeGet 同类设计，仅追加:

```go
// 退避基础上加 ±20% 随机抖动，避免 Server 重启后全体 Agent 同步重试
jitter := 0.8 + 0.4*rand.Float64()
interval = time.Duration(float64(interval) * jitter)
```

- 成功连接后 `reconnectAttempts = 0` 重置已存在（`ws.go:161`），无需改
- `cmd/agent/main.go:142` 的线性重试是本地 Token 持久化（1/2/3s），无群体脉冲问题，**不改**
- 配置同步器按自身 interval 周期运行，Agent 启动时刻天然错峰，**不改**

### B2 测试

- `getReconnectInterval` 连续采样 N=1000 次：第 n 次采样落在 `[0.8×base_n, 1.2×base_n]` 区间且不超过 60s 封顶；均值的方差 > 0（证明抖动生效）
- attempts 计数与重置行为回归验证

---

## 4. 实施顺序

```
第 1 步  A1 模型 + A2 repository（含单测）      ← 纯新增，可独立合入
第 2 步  A3 rollup 任务 + 回填（含单测）
第 3 步  A4 设置项 + A6 删除联动
第 4 步  A5 API 路由（管理端 + 公开）
第 5 步  A7 前端范围/类型/设置页
第 6 步  B1/B2 Agent jitter（与 1-5 无依赖，可并行）
第 7 步  验证：tsc + vite build / go build + vet + test / Agent 构建
```

## 5. 风险与回滚

| 风险 | 缓解 |
|------|------|
| rollup 每小时读 12 行 × N Agent，5 分钟一次检查 | 单次查询走 (agent_id, timestamp) 索引，量级 < 1k 行/次；内存 lastRolled 跳过已处理小时，实际仅整点后首个 tick 有负载 |
| 存量安装 5 分钟层仍只保 4 天，7d+ 范围中间有空洞 | 前端图表已支持数据空洞断线；设置页提示用户调大保留期；空洞小时由 offline 占位补齐时间线 |
| ping_data 按目标名聚合后目标集合变化（小时内增删探测目标） | 分组各自平均，新目标从出现时刻起算，语义可接受 |
| `DefaultRetentionDays` 4→30 的存储增量 | 30d × 288 点 × 10 Agent ≈ 86k 行（数 MB 级），SQLite 无压力；仅影响新安装 |
| 回滚 | 两张表解耦：停用小时路由即回到现状；`DROP TABLE metric_records_hourly` 无损现有功能 |

## 6. 验收标准

1. 管理端/公开端详情页可选 7d/30d/90d/1y，图表正常渲染且无卡顿（点数受 maxChartPoints 约束）
2. 1y 范围查询响应 < 500ms（10 Agent 规模）
3. Agent 离线 24h 后在线率时间线在小时粒度上仍连续（offline 占位生效）
4. 重启 Server 后 Agent 重连时间在日志中呈现离散分布（jitter 生效）
5. 删除 Agent 后两张历史表均无该 Agent 残留
6. `go test ./...`、`go vet ./...`、前端 `tsc && vite build` 全绿
