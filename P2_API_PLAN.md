# P2 实施计划：面板运维与展示完善（后端 API）

> **依据**: `COMPETITOR_ANALYSIS.md` §5.1 F2/F3/F7/F9/F10 + §6 P2 优先级
> **范围**: 会话管理（F7）+ 临时分享令牌（F9）+ 备份导出（F10）+ 流量配额类型（F2）+ 过期提前通知（F3）
> **原则**: 全部改动不触碰 Agent-Server 协议与控制通道（S1/S4/S10 合规）；数据库变更为"新增表 + 新增列"（AutoMigrate 原生支持），无破坏性迁移、无数据回填

## 0. 现状审计结论

| 能力 | 现状 | 缺陷 |
|------|------|------|
| 会话 | JWT 无状态 12h，HttpOnly Cookie | 无法列出活跃会话、无法撤销（登出仅清 Cookie）、JWT 签发后到过期前始终有效 |
| 分享页 | 8 字符 share_id 永久有效 | 无过期时间，"限时分享给朋友"需手动删除 |
| 备份 | GET /db/backup 手动 VACUUM INTO 快照 | 无升级自动备份；站点设置/Agent 元数据无法单独迁移 |
| 流量配额 | traffic_quota_bytes 单值，使用率恒按 rx+tx | VPS 常见计费口径（仅上行/取上下行较大者等）无法表达 |
| 到期提醒 | expires_at 存在，告警规则 metric=expire_days 需手动配置 | 无主动通知，管理员须记得自建规则 |

## 1. 任务总览

| 编号 | 任务 | 类型 | 落点 |
|------|------|------|------|
| W1.1 | Session 模型 + AutoMigrate | 后端 | model/models.go、repository/sqlite.go |
| W1.2 | SessionRepository（CRUD/撤销/清理） | 后端 | repository/repo_session.go |
| W1.3 | JWT 增加 session 绑定（jti claim） | 后端 | pkg/auth.go |
| W1.4 | AuthRequired / /auth/me / WS 握手校验会话 | 后端 | api/middleware.go、handler_auth.go、handler_dashboard_ws.go |
| W1.5 | 登录创建会话、登出撤销会话 | 后端 | api/handler_auth.go |
| W1.6 | 会话列表/撤销 API + TOTP 变更联动撤销 | 后端 | api/handler_auth.go、router.go |
| W1.7 | 会话过期清理任务 + 测试 | 后端+测试 | service/session.go、repo_session_test.go |
| W2.1 | SharePage.ExpiresAt 字段 + 过期校验 | 后端 | model/models.go、handler_share.go |
| W2.2 | 分享页前端表单/列表过期时间 | 前端 | SharePageManagement.tsx |
| W2.3 | 过期访问测试 | 测试 | handler_share_test.go |
| W3.1 | 版本变更自动备份 | 后端 | main.go（启动时） |
| W3.2 | 设置导出/导入 API（2FA 保护） | 后端 | api/handler_settings.go |
| W3.3 | 导出导入测试（不含敏感字段） | 测试 | handler_settings_test.go |
| W4.1 | Agent.TrafficQuotaType 五种口径 | 后端 | model/models.go、service/alert.go |
| W4.2 | 配额口径统一计算函数 + API 透传 | 后端 | handler_agent_api.go、service/monitor.go |
| W4.3 | 前端表单下拉 + 卡片百分比口径 | 前端 | AgentManagement.tsx、ServerCard.tsx |
| W4.4 | 五种口径计算单测 | 测试 | quota_test.go |
| W5.1 | 到期通知设置项 + 服务（每日摘要） | 后端 | service/settings.go、service/expire_notify.go |
| W5.2 | 设置页表单 + 测试 | 前端+测试 | Settings.tsx、expire_notify_test.go |
| V | go build/test + tsc + vite build | 验证 | — |

## 2. W1 会话管理（F7：会话列表 + 远程注销）

### 2.1 数据模型

```
表: sessions
SessionID  string  uniqueIndex  ← 32 字节随机 hex，与 JWT jti 一致
AdminID    int64   index
IP         string
UserAgent  string
CreatedAt / LastSeenAt / ExpiresAt  time
RevokedAt  *time.Time              ← nil=未撤销
```

### 2.2 JWT 绑定

- `GenerateToken(adminID, sessionID)`：sessionID 写入 `RegisteredClaims.ID`（jti）
- 兼容性：升级部署后旧 Token（无 jti 或 sessions 无记录）一律 401，需重新登录（一次性代价，部署窗口内可接受）

### 2.3 校验链路（三处全部收口）

1. **AuthRequired**：JWT 通过后查 session——不存在/已撤销/已过期 → 401 清 Cookie；通过则节流更新 LastSeenAt（距上次 > 60s 才写库）
2. **/auth/me**（公开路由）：同样校验 session，否则出现"/me 已认证但其余 API 全 401"的割裂
3. **WS /ws/dashboard 握手**：同样校验（已建立的连接不主动断，与现有 JWT 过期行为一致）

### 2.4 API 设计

| 端点 | 说明 |
|------|------|
| `GET /api/v1/auth/sessions` | 当前管理员全部会话，`current` 标记当前会话 |
| `DELETE /api/v1/auth/sessions/:sessionId` | 撤销指定会话（盗号踢出） |
| `POST /api/v1/auth/sessions/revoke-others` | 撤销除当前外全部 |

- 登录成功 → 创建 session；登出 → 撤销当前 session（真正失效，非仅清 Cookie）
- **TOTP enable/disable → 撤销除当前外全部会话**（认证配置变更后旧会话不应存活）

### 2.5 清理

每日清理 `expires_at < now-7d` 或 `revoked_at < now-7d` 的会话（与审计日志同模式）。

## 3. W2 临时分享令牌（F9：分享页过期）

- `SharePage.ExpiresAt *time.Time`（nil=永久，AutoMigrate ADD COLUMN）
- 创建/更新 API 接收 `expires_at`（RFC3339）；公开端点 `GET /public/share/:shareId` 过期 → 404（与不存在同响应，不泄露存在性）
- 管理列表/详情透传 `expires_at`，前端表单提供"永久/自定义时间"，列表列显示剩余天数/已过期

## 4. W3 备份导出（F10）

### 4.1 版本变更自动备份

- 启动时将当前程序版本写入 `system_settings.app_version`；与已存值不一致 → 自动 `VACUUM INTO` 到 `backup/`（文件名 `auto-before-{old→new}-{ts}.db`），随后更新版本记录
- 自动备份保留最近 5 份，超出滚动删除（手动备份逻辑不变）

### 4.2 设置导出/导入（迁移场景）

| 端点 | 保护 | 说明 |
|------|------|------|
| `GET /api/v1/settings/export` | RequireSensitive2FA | JSON：站点设置 + 标签 + Agent 元数据（**不含 token/密码/TOTP**） |
| `POST /api/v1/settings/import` | RequireSensitive2FA | 设置覆盖；标签按名称合并；Agent 元数据按 hostname 匹配更新（不存在跳过） |

## 5. W4 流量配额类型（F2）

- `Agent.TrafficQuotaType string`，默认 `sum`（AutoMigrate ADD COLUMN，存量数据即 sum=现行为）

| 类型 | 口径 | 语义 |
|------|------|------|
| sum | rx + tx | 合计计费（现行为，默认） |
| up | tx | 仅上行 |
| down | rx | 仅下行 |
| max | max(rx, tx) | 上下行取大 |
| min | min(rx, tx) | 上下行取小 |

- 统一函数 `CalcQuotaUsedBytes(quotaType, rx, tx)`，告警引擎（metric_traffic_quota）与前端百分比共用同一口径
- API：Agent 创建/更新接收 `traffic_quota_type`（白名单校验）；列表/详情/公开端透传

## 6. W5 过期提前通知（F3）

- 设置项：`expire_notify_enabled`（默认 false）、`expire_notify_lead_days`（默认 7，1-90）、`expire_notify_channel_id`（默认 0）
- `ExpireNotifyService`：每日 09:00 检查全部 Agent `expires_at`，剩余 `0 < days ≤ lead` → 发送**每日一条汇总通知**（列出各机器剩余天数；含已过期机器）
- 防重发：`expire_notify_last_sent_date` 存 settings，重启不重发
- 未配置渠道（channel_id=0）时跳过发送

## 7. 验收标准

1. 登录后可在"会话管理"看到当前全部活跃会话（IP/UA/时间），可单踢/全踢；被踢会话立即 401
2. 登出后旧 Token 复用请求返回 401（真正撤销）
3. TOTP 启用/停用后其他设备会话全部失效
4. 分享页设置过期时间后，到期访问返回 404；管理列表可见剩余天数
5. 升级版本重启后 `backup/` 出现自动备份；导出 JSON 无 token/密码字段；导入后设置与 Agent 元数据恢复
6. 配额类型切换 sum→max 后，卡片百分比与告警使用率同口径变化
7. 到期提醒：设置渠道 + 提前天数后，每日一条汇总通知，不重复
8. `go build ./... && go test ./...` 通过；前端 `tsc --noEmit` + `vite build` 通过；Agent 侧零改动
