# 竞品深度分析报告：Komari 与 NodeGet

> **日期**: 2026-08-28
> **分析对象**: Komari (commit `e31a032`, 2026-08-26)、NodeGet (commit `079ad12`, 2026-07-29)
> **分析方法**: 浅克隆两仓库源码，逐目录代码审查 + 文档核对，所有结论附文件路径证据
> **目的**: 找出可借鉴的功能与设计（含数据库重构方向），分析优缺点；所有借鉴建议均已通过 `SECURITY.md` 的功能准入门槛（S1-S10 + 残余能力清单）过滤
> **状态**: 设计分析，未修改任何代码

---

## 1. 三项目架构总览

| 维度 | 本项目 (ServerProbe) | Komari | NodeGet |
|------|---------------------|--------|---------|
| 语言 | Go (Server+Agent) + React | Go + 独立前端仓库 (komari-web) | Rust workspace + Vue (前后端分离) |
| 数据库 | SQLite (GORM) | SQLite 主库 + 独立 metric store (SQLite/MySQL/PG) | SQLite + PostgreSQL (SeaORM) |
| 实时数据 | 内存环形缓冲 | 内存 + metric store | mpsc channel 批量缓冲 |
| 通信协议 | WS + JSON，Server→Agent 仅 4 种数据帧 | v1 JSON + v2 JSON-RPC，含 exec/terminal/ping 等控制帧 | WS + JSON-RPC 2.0，12 种任务类型下发 |
| 认证 | 单管理员 + JWT Cookie + TOTP | 多用户 (admin/client/guest) + OAuth/OIDC + 2FA | Token 体系 (超级令牌/子令牌/RBAC) |
| Agent 权限 | 非 root + setcap，无执行能力 | 有 exec/terminal/网络测试 | 有 execute/webshell/selfupdate，但每任务独立 allow_* 开关 (Agent 侧校验) |
| 扩展机制 | 无 | 插件系统 (JS runtime, 含 AllowExec 能力) + 主题市场 | JS Worker + 插件 + 主题生态 |

## 2. 安全性验证：竞品的"控制通道"事实

这是本报告最重要的结论，直接验证本项目"只读安全探针"定位的差异化价值。

### 2.1 Komari 的控制通道

| 能力 | 证据 |
|------|------|
| 远程命令执行 | `web/rpc/jsonrpc/admin.system.go` 的 `adminExec()`；`database/models/task.go` 的 `tasks`/`task_results` 表存储 command/result/exit_code |
| Web 终端 | `web/api/terminal/request.go`（`agent.terminal.request` 事件）、`establish.go`（WS 升级 + 转发）、`forward.go` |
| 网络测试下发 | `protocol/v2/networktest.go`：`networkTest.nextTrace`/`iperf3`/`meshTrace`（traceroute、iperf3 打流、Agent 互测） |
| 插件执行 | `database/models/plugin.go`：`PluginPermissions.AllowExec`/`AllowAllFileAccess`/`AllowListen` |
| 协议层 | `protocol/v2/jsonrpc.go` 定义 `MethodAgentExec`/`MethodAgentTerminal`/`MethodAgentPull` 等 12 个方法 |

Komari 的安全策略是**纵深防御**（多角色鉴权 + 敏感操作 2FA + 会话审计），不是**架构隔离**。Server 沦陷 = 全体 Agent RCE，与 Nezha 同构。

### 2.2 NodeGet 的控制通道

| 能力 | 证据 |
|------|------|
| 命令执行 | `agent/src/tasks/execute.rs`（含进程组管理、SIGTERM/SIGKILL、输出截断） |
| WebShell/PTY | `agent/src/tasks/pty.rs`（PTY 终端 + WS 双向转发，活跃会话上限 8） |
| 自更新 | `crates/ng-task/src/types/mod.rs` 的 `SelfUpdate` 任务 + `server/src/rpc_nodeget.rs` 的 `self_update` RPC |
| Agent 配置读写 | `read_config`/`edit_config` RPC（Server 可改 Agent 本地配置） |
| 任意 HTTP/DNS 出站 | `HttpRequest`/`Dns`/`HttpPing`/`Ip` 任务（Agent 按指令访问任意 URL/域名） |

NodeGet 宣称的"极致的网络安全性：对外网络请求除 Agent-Server 通信外只有 NTP"（`README.md`）**在代码层面不完全属实**：Agent 还会按任务访问 HTTP/HTTPS 目标、Cloudflare、ipinfo.io、DNS 服务器（`agent/src/tasks/ip.rs`、`http_request.rs`）。此外 Agent 存在 `ignore_cert` 配置项可跳过 TLS 证书校验（`agent/src/rpc/multi_server.rs`），与本项目曾存在的 `insecure_tls` 同病。

NodeGet 的可取之处是**权限模型**：12 种任务类型每种有独立 `allow_*` 开关，全部默认 `false`，且**校验在 Agent 侧执行**（`crates/ng-task/src/types/mod.rs`、`agent/src/tasks/mod.rs`）——这与本项目"SSRF 过滤在 Agent 侧执行"（`agent/internal/collector/ping.go`）是同一设计哲学。

### 2.3 结论

| 项目 | Server 沦陷时对 Agent 的最坏能力 |
|------|-------------------------------|
| Komari | 任意命令执行 + 终端 + 插件代码执行（完整 RCE） |
| NodeGet | 命令执行 + WebShell + 自更新（若对应 allow_* 开启）+ 任意 HTTP/DNS 出站 |
| **本项目** | **仅限有界配置参数（S9 钳制）+ 数据读写，无任何控制通道** |

本项目的核心差异化不是营销话术，是架构事实。后续所有借鉴决策以此为准绳。

---

## 3. 数据库设计深度对比（重点）

### 3.1 三方存储架构对比

| 维度 | 本项目 | Komari | NodeGet |
|------|--------|--------|---------|
| 原始高频数据 | 内存环形缓冲（不落盘） | metric store 原始层（保留期可配） | `dynamic_monitoring` 表（1s 间隔落盘） |
| 聚合数据 | `metric_records` 单层（5 分钟均值） | 多级 rollup：1m→7d → 5m→30d → 1h→1y | `dynamic_monitoring_summary` 摘要表 |
| 静态信息 | `agents` 表（心跳时更新） | `clients` 表 | `static_monitoring` 表 + `data_hash` 去重 |
| 流量统计 | `traffic_records`（按日 upsert） | 从 metric store 区间查询计算 | `total_received`/`total_transmitted` 累计值 |
| 数据保留 | 可配置天数（1-3650），定时清理 | RollupPolicy 每层独立保留期 | 未见明确自动清理迁移 |
| 主键策略 | int64 自增 + agent_id 外键 | UUID 字符串主键 | int 代理键 (`monitoring_uuid` 表映射 UUID) |

### 3.2 Komari metric store：多级 rollup（最值得借鉴的设计）

**位置**: `pkg/metric/rollup.go`、`internal/metricstore/compaction.go`、`pkg/metric/percentile.go`

**设计**:
```
原始点 → [1m 桶, 保留 7d] → [5m 桶, 保留 30d] → [1h 桶, 保留 1y]
```
- 每个 rollup 桶保存 `count/sum/sumSq/min/max/first/last` 七个统计量——**聚合的聚合仍可继续推导**（两个相邻 1m 桶可无损合并为 2m 桶）
- `sumSq`（平方和）使任意层级都能计算标准差
- **TDigest 摘要**保存百分位分布——任意时间范围可查询 p95/p99 延迟，而非只有均值
- 桶宽 + 保留期由 `RollupPolicy` 统一声明，`Compact()`/`CleanupExpired()` 定期维护

**优点**:
1. 长历史与存储量的矛盾被结构性解决：1 年数据只需 ~8760 点/agent（1h 粒度）
2. 任意时间范围查询自动路由到合适粒度（查 30 天用 5m 桶，查 1 天用 1m 桶），扫描量恒定
3. p95/p99 延迟成为一等公民——对 VPS 监控（延迟抖动敏感）价值极高，均值会掩盖抖动
4. 聚合可合并性使"调整分层策略"不需要重算历史

**缺点**:
1. 复杂度高：rollup_transfer/digest_codec/hot_rollup 等十余个文件，查询需按范围选层
2. TDigest 序列化/反序列化有额外成本
3. 写入路径变长（原始层 + 分钟 rollup + 粗粒度 rollup 三级刷写）

### 3.3 NodeGet 数据层：三表分离 + 工程细节

**位置**: `crates/ng-db/migration/src/m20260113_*` ~ `m20260708_*`

**设计**:
- `static_monitoring`（静态信息）+ `dynamic_monitoring`（原始动态）+ `dynamic_monitoring_summary`（扁平列摘要表）三表分离
- `monitoring_uuid` 表：UUID → 小整数代理键，索引和存储开销更小
- `static_monitoring.data_hash` 唯一索引 + `on_conflict_do_nothing`：**静态信息未变化时不重复写入**（5 分钟一次的静态上报，99% 是重复数据）
- `timestamp`（Agent 上报时间）与 `storage_time`（Server 入库时间）分离：可检测时钟漂移和延迟入库
- `dynamic_monitoring_summary` 摘要表是**扁平列**（cpu_usage、used_memory 等直接列名）——仪表盘高频查询不碰 JSON 解析

**优点**:
1. 摘要表扁平列设计对 SQLite 极友好（无 JSON 解析、列裁剪、索引覆盖）
2. data_hash 去重显著降低静态数据写入量
3. 双时间戳是正确性设计，成本几乎为零
4. 代理键把 36 字节 UUID 缩为 2 字节 SMALLINT，索引体积降一个数量级

**缺点**:
1. 三表 + JSONB 列意味着查询详情需 join 或二次解析
2. 未见到明确的数据清理策略（`storage_time` 索引已建，清理逻辑可能在 service 层）
3. PostgreSQL 特性（JSONB lz4 压缩）在 SQLite 上不适用

### 3.4 本项目现状评估

```
Agent 3s 上报 → WS → 内存环形缓冲（实时面板读这里）
                      ↓ 每 5 分钟
              metric_records 单层聚合（均值）→ N 天后删除
                      ↓ 每 5 分钟
              traffic_records 按日 upsert
```

**优点**（不必妄自菲薄）:
- 环形缓冲扛高频 + 5 分钟落盘，写放大极低，SQLite 压力小
- ×10 整数缩放（CPU/Load）已是正经的存储优化
- offline 占位记录设计（反转语义兼容旧行）解决了在线率时间线，这是竞品都没有直接对应的
- traffic_records 按日聚合直接可查月流量，比 Komari 的区间计算简单可靠

**短板**:
1. **单层聚合**：保留 90 天 = 90×288 = 25920 点/agent；想要 1 年历史只能拉长保留期，存储线性膨胀，且长范围查询扫描量大
2. **只有均值**：延迟抖动、CPU 尖峰被抹平——5 分钟均值 50% CPU 掩盖了其中 4 分钟 90% 的情况
3. 查询历史只有一张表一个粒度，前端"max chart points"只能靠丢弃采样点

### 3.5 数据库重构建议（方向，待评审）

**方案 A：最小改动——metric_records 增加 rollup 层（推荐起步）**

保留现有 5 分钟层不动，新增 `metric_records_hourly`（小时聚合表）：
- 字段 = 现有列 + `cpu_min`/`cpu_max`、`net_rx_max` 等极值补充列
- 聚合服务每小时把 12 个 5 分钟点合并写入；查询层按时间范围自动选表
- 保留策略：5 分钟层保 30 天，小时层保 2 年（~17.5k 点/agent，SQLite 毫无压力）
- 优点：改动集中在聚合服务 + 查询路由，无迁移风险；缺点：粒度跨度大（5min→1h），中间查询（如 6h 范围想要 1m 粒度）无解

**方案 B：完整 Komari 式多级 rollup**

原始层（可选落盘）→ 1m → 5m → 1h 三层，每桶存 count/sum/min/max（sumSq 可选，TDigest 暂缓）。
- 优点：任意范围最优粒度、可计算 p95 延迟；缺点：实现复杂度高，个人规模（十几台）收益边际递减

**方案 C：NodeGet 式摘要表拆分**

把 `metric_records` 拆为"摘要扁平表（高频查询）+ 详情表（disk/ping JSON）"。
- 优点：仪表盘查询更瘦；缺点：本项目单表已经够快（十几台规模），拆表徒增复杂度

**建议**：A 起步，需要 p95/p99 时再升级到 B 的简化版（不带 TDigest）。C 不建议。

**顺带可借鉴的零成本细节**（无论选哪个方案）:
- NodeGet 的 `storage_time` 双时间戳列——区分"Agent 声称的时间"与"实际入库时间"
- NodeGet 的 SQLite 批量写入子批次计算 `999 / num_columns`（`crates/ng-monitoring/src/monitoring_buffer.rs`）——本项目 BatchWriter 若单批超 999 参数会报错，值得核对
- Komari 的批量 flush 参数（500ms 间隔 / 1000 条批量 / 10000 channel 容量，满则丢弃并记日志）可作为本项目 BatchWriter 调参参考

---

## 4. 通信协议对比

| 维度 | 本项目 | Komari | NodeGet |
|------|--------|--------|---------|
| 协议形态 | WS + 固定 JSON 帧 | v1 JSON + v2 JSON-RPC（演进中） | JSON-RPC 2.0（前后端 + Agent 统一） |
| Server→Agent 帧 | 4 种（register_ok/fail、config_update、heartbeat_ack） | 12+ 方法（含 exec/terminal/ping/pull/event） | 任务下发 + RPC 响应 |
| 离线消息 | Agent 周期拉取配置，天然无队列 | v2 事件队列（TTL 5min，ping 事件 3s，上限 128 条，重连补投） | 任务注册每分钟重注册 |
| 重连策略 | 固定间隔重试 | — | **指数退避 1s→60s + ±20% jitter**（`agent/src/rpc/multi_server.rs`） |
| TLS | 强制 + （待实现）指纹固化 | 可配置 | 可配置 + `ignore_cert` 开关 |

**可借鉴**:
1. **指数退避 + jitter**（NodeGet）：固定间隔重连在 Server 重启风暴时会形成同步重连脉冲，jitter 是标准解法，Agent 侧改动极小
2. Komari 的事件 TTL 分级（普通 5min / ping 3s）提示了"若未来加推送型配置，需按消息类型设 TTL"——本项目当前拉取式架构无此需求，不必引入

**不必借鉴**: JSON-RPC 统一协议（Komari/NodeGet 为多方法扩展性设计，本项目帧类型固定 4 种正是安全特性，协议表达力越弱攻击面越小）

---

## 5. 功能借鉴清单

### 5.1 可直接借鉴（零安全代价，推荐排期）

| # | 功能 | 来源 | 证据 | 说明 |
|---|------|------|------|------|
| F1 | **告警 ratio 机制** | Komari | `database/models/notification.go` LoadNotification.ratio | 窗口内 N% 采样超阈值才触发，而非单点超阈即报——天然抑制瞬时尖峰告警风暴。本项目 AlertRule 已有 Duration，语义近似但 ratio 表达更直观 |
| F2 | **流量配额类型** | Komari | `clients.traffic_limit_type`: sum/max/min/up/down | 本项目 traffic_quota_bytes 是单值；加一个类型字段即可支持"取上下行较大者计费"等 VPS 常见计费口径 |
| F3 | **过期提前通知** | Komari | settings: expire_notification_lead_days=7 | 本项目已有 expire_days 告警规则，可补"提前 N 天"配置项 |
| F4 | **流量报告（日/周/月报）** | Komari | `utils/notifier/traffic_report.go`（cron 0 0 0 * * * 等） | 基于已有 traffic_records 即可实现，纯服务端定时聚合 + 通知 |
| F5 | **通知渠道扩展 + 模板** | Komari | `utils/messageSender/`（bark/serverchan3/telegram/email/webhook）+ `sender.go` 模板 `{{event}}/{{client}}/{{time}}` | 本项目已有 webhook/telegram/email 三渠道；bark（iOS 推送）实现成本极低；统一模板变量系统值得抄 |
| F6 | **审计日志** | Komari | `database/auditlog/log.go` | 管理员操作（登录/改配置/删 Agent）落库，安全闭环的最后一块 |
| F7 | **会话列表 + 远程注销** | Komari | `sessions` 表（user_agent/ip/expires + admin 管理接口） | 单管理员也有"我被盗号了想踢掉所有会话"的需求 |
| F8 | **敏感操作 2FA 再验证** | Komari | `RequireSensitive2FA`（exec/terminal/2fa 禁用） | 已有 TOTP，把"改认证配置/删 Agent/清数据"标记为敏感操作需二次验证，改动小 |
| F9 | **临时分享令牌** | Komari | settings: tempory_share_token + expire_at | 私有站点场景下限时分享给朋友看，比永久公开页安全 |
| F10 | **备份导出/恢复** | Komari | `dbcore.go` backupOnVersionUpgrade + zip 打包下载/分块上传恢复 | SQLite 单文件天然适合；升级前自动备份尤其值得抄 |
| F11 | **指数退避 + jitter 重连** | NodeGet | `multi_server.rs` BASE_BACKOFF=1s, MAX=60s, ±20% | Agent 侧小改动 |
| F12 | **GeoIP 自动识别** | Komari | `utils/geoip/`（mmdb/ip-api/geojs/ipinfo 多 provider） | 本项目 country_code 目前纯手工填；mmdb 本地文件方案无 SSRF 风险（注意：走外部 HTTP API 的 provider 需过 SSRF 防护） |
| F13 | **静态页公开面板 + 受限 Token** | NodeGet | docs/guide/theme/index.md（StatusShow 纯静态 + site_tokens 受限读权限） | 前后端分离的公开页生态：静态托管 + 只读受限 token 访问 WS。本项目已有公开页，可评估"导出为纯静态站"的可能性（远期） |

### 5.2 需改造后借鉴（通过功能准入门槛后可实现）

| # | 功能 | 来源 | 改造约束 |
|---|------|------|---------|
| F14 | **GPU 监控** | Komari `gpu_records`（device_index/mem/util/temp）、NodeGet gpu_data | 须走 NVML/ROCm 库调用（cgo 或 IPC），**不得** exec nvidia-smi；NVML 只读查询无安全代价。注意：引入 cgo 会破坏单二进制交叉编译优势，需权衡 |
| F15 | **带宽测速** | Komari networkTest.iperf3 | iperf3 打流 = Agent 主动产生大流量，属"流量出口"能力。若做必须：显式 opt-in + 单次时长硬上限 + 频率硬上限（S9 式钳制）。默认关闭 |
| F16 | **温度采集** | Komari Record.temp | 本项目 agent 已有 thermal.go（`agent/internal/collector/thermal.go`），核对是否已上报展示即可，几乎零成本 |
| F17 | **地图视图** | NodeGet map.md（KV 存经纬度 + JS Worker 刷新） | 纯前端展示 + agents 表加经纬度字段即可，无通道变化。优先级低（个人十几台机器地图意义有限） |
| F18 | **多语言 i18n** | Komari（language cookie + html lang 替换） | 纯前端工作，无安全影响，优先级看受众 |
| F19 | **主题配置表单**（user_preferences_form） | NodeGet docs/dev/theme | 主题声明 JSON schema → 面板自动渲染配置 UI。本项目单内置主题，做主题生态前不值得；**低成本替代**：Komari 的 custom_head/custom_body HTML 注入（已通过站点设置部分实现） |

### 5.3 永不借鉴（违反 S1/S4/S10，写入 Non-Goals）

| 功能 | 来源 | 排除理由 |
|------|------|---------|
| Web 终端 | Komari terminal/、NodeGet pty.rs | S4 无远程执行 |
| 远程命令执行/批量执行 | Komari adminExec、NodeGet execute.rs | S4 |
| Agent 带内自更新 | NodeGet self_update | S10 |
| Server 改写 Agent 配置 | NodeGet edit_config | S9 之外的控制能力（本项目仅允许下发有界探测参数） |
| Agent 任意 HTTP/DNS 出站 | NodeGet http_request/dns 任务 | Agent 沦为流量跳板，超出残余能力清单 |
| 插件执行能力 | Komari PluginPermissions.AllowExec | S4 |
| iperf3 mesh 互测 | Komari networkTest.meshTrace | Agent 间任意组网探测 |
| 多主控 Agent | NodeGet multi_server | 一台 Agent 信任多台 Server = 攻击面翻倍，且与指纹固化（S2 TOFU）冲突 |
| 多用户/多租户 | Komari users+OAuth/OIDC | S5 单管理员是攻击面裁剪决策，不是功能缺失 |

### 5.4 特别说明：NodeGet 权限模型的部分借鉴价值

NodeGet 的"每能力独立 allow_* 开关 + **Agent 侧校验**"模式（`crates/ng-task/src/types/mod.rs`）值得吸收为**设计思想**而非功能：本项目即使永不加控制任务，也应在 Agent 配置中固化"能力清单"概念——例如 `allow_private_targets` 已是这个模式的一个实例（`agent/cmd/agent/main.go` → `NewPingCollector(..., cfg.AllowPrivateTargets)`）。未来任何新增的可配置行为，都应默认关闭、Agent 侧校验、通过 S9 式硬边界。

---

## 6. 综合优先级建议

按"价值 ÷ 成本 × 安全合规"排序：

| 优先级 | 事项 | 类型 |
|--------|------|------|
| P0 | 数据库小时级 rollup（方案 A）+ 极值列（min/max） | 数据层重构 |
| P0 | 指数退避 + jitter 重连 | Agent 小改动 |
| P1 | 审计日志 + 敏感操作 2FA 再验证 | 安全闭环 |
| P1 | 通知模板系统 + bark 渠道 + 流量日报 | 通知增强 |
| P1 | S2 指纹固化（TOFU）落地 | 安全（已在 spec 中立项） |
| P2 | 会话管理 + 临时分享令牌 + 备份导出 | 面板运维 |
| P2 | 流量配额类型 + 过期提前通知 | 展示完善 |
| P3 | GPU（NVML）/ 温度核对 / GeoIP mmdb / i18n | 采集与展示 |
| 远期 | p95/p99 延迟（rollup 升级）、静态公开页生态 | 视需求 |

---

## 7. 附录：证据文件索引

**Komari** (G:\TraeSOLO\Project_2\_competitor_analysis\komari)
- 数据库模型: `database/models/models.go`、`task.go`、`notification.go`、`theme.go`、`plugin.go`
- Metric store: `pkg/metric/rollup.go`、`percentile.go`、`internal/metricstore/compaction.go`、`definitions.go`
- 协议: `protocol/v1/report.go`、`protocol/v2/jsonrpc.go`、`protocol/v2/networktest.go`
- 控制通道: `web/api/terminal/`、`web/rpc/jsonrpc/admin.system.go`、`database/tasks/tasks.go`
- 认证: `web/api/Auth.go`、`web/router/router.go`
- 通知: `utils/notifier/`、`utils/messageSender/`
- 设置: `internal/config/settings.go`

**NodeGet** (G:\TraeSOLO\Project_2\_competitor_analysis\nodeget)
- 迁移/表结构: `crates/ng-db/migration/src/m20260113_*` ~ `m20260708_*`
- 任务体系: `crates/ng-task/src/types/mod.rs`、`agent/src/tasks/`（execute.rs/pty.rs/http_request.rs/ip.rs）
- 通信: `agent/src/rpc/multi_server.rs`、`server/src/rpc_nodeget.rs`
- 缓冲: `crates/ng-monitoring/src/monitoring_buffer.rs`
- 文档: `docs/guide/features/`（map/ping-map/cost/snippet/batch-exec）、`docs/dev/theme/`

**本项目对照**
- 数据模型: `server/internal/model/models.go`
- 聚合: `server/internal/service/aggregation.go`（5 分钟 ticker + 清理任务）
- Agent 安全: `agent/internal/collector/ping.go`（SSRF 过滤）、`agent/cmd/agent/main.go`（参数钳制）
- 设计原则: `SECURITY.md`（S1-S10 + 残余能力清单 + 功能准入门槛）、`spec.md` §2
