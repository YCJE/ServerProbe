# 安全策略 (Security Policy)

## 安全设计原则

本项目遵循以下 8 项核心安全原则:

### S1: 只读架构 - 无控制帧

Server 到 Agent 的 WebSocket 通信仅包含以下 4 种消息类型,不包含任何控制/执行指令:

| 消息类型 | 方向 | 用途 |
|---------|------|------|
| `register_ok` | Server → Agent | 注册成功 |
| `register_fail` | Server → Agent | 注册失败 |
| `config_update` | Server → Agent | 配置更新通知 |
| `heartbeat_ack` | Server → Agent | 心跳确认 |

Agent 不会执行任何来自 Server 的命令、脚本或代码。

### S2: 强制 TLS - 不可关闭

- Server 强制启用 TLS,无关闭选项
- Agent 拒绝 `http://` 和 `ws://` 连接,仅接受 `https://` 和 `wss://`
- 无证书时自动生成自签名证书 (ECDSA P-256, 365 天)
- TLS 配置不可通过配置文件关闭

### S3: 非 root 运行

- Agent 以 `probe` 系统用户运行 (非 root)
- systemd service 包含安全加固:
  - `NoNewPrivileges=true`
  - `ProtectSystem=strict`
  - `ProtectHome=true`
  - `PrivateTmp=true`
  - `ReadWritePaths=/etc/probe-agent`

### S4: 无远程执行能力

Agent 代码中不包含以下任何能力:

- `os/exec` / `exec.Command` / `exec.CommandContext` - 无命令执行
- `pty` / `term` / `shell` / `terminal` - 无终端会话
- `os.OpenFile` (写模式) / `os.Remove` / `os.Rename` - 无文件系统操作 (配置文件写入除外)
- Cron 任务执行器 - 无定时任务执行

### S5: 单管理员认证

- 无多租户设计,消除越权攻击面
- JWT + HttpOnly + Secure + SameSite=Strict Cookie
- 登录限速: 5 次/分钟
- 无默认密码,首次启动强制设置
- 密码策略: 最小 12 位,必须包含大小写字母 + 数字
- 密码哈希: bcrypt cost = 12

### S6: SSRF 防护

Webhook 通知内置多层 SSRF 防护:

- **私有 IP 过滤**: 拒绝 10/8、172.16/12、192.168/16、127/8、169.254/16、::1、fc00::/7
- **DNS 重绑定防护**: 自定义 Dialer 强制使用预解析 IP
- **重定向检测**: `CheckRedirect` 中再次执行 SSRF 检查
- **响应体限制**: 最多读取 1KB,不反射给用户
- **超时限制**: 10 秒超时
- **TLS 验证**: 强制验证,不可关闭

### S7: 最小权限采集

所有采集器仅读取 `/proc` 文件系统和系统调用,不需要 root 权限:

- CPU: `/proc/stat`, `/proc/cpuinfo`, `/proc/loadavg`
- 内存: `/proc/meminfo`
- 磁盘: `statfs` 系统调用
- 网络: `/proc/net/dev`, `/proc/net/tcp`, `/proc/net/udp`
- 系统: `uname` 系统调用, `/proc/uptime`, `/proc/loadavg`

ICMP Ping 通过 `setcap cap_net_raw+ep` 授予最小 capability,而非 root 运行。

### S8: 配置文件权限控制

- 配置文件权限: `chmod 600`
- 配置文件属主: `chown probe:probe` (Agent) / `chown probe-server:probe-server` (Server)
- JWT 密钥在安装时随机生成

## 威胁模型

| 威胁 | 防护措施 |
|------|---------|
| 通过 Server 向 Agent 下发命令 | 协议无控制帧,Agent 无执行功能 |
| 截获 Agent-Server 通信 | 强制 TLS,Agent 拒绝明文连接 |
| 多租户越权 | 单管理员设计,无多租户攻击面 |
| Webhook SSRF | 私有 IP 过滤 + DNS 重绑定防护 + 重定向检测 |
| 暴力破解管理员密码 | 登录限速 5 次/分钟 |
| Token 盗用到其他机器 | 主机指纹绑定校验 |
| 伪造 Agent 上报数据 | 数据合理性校验 (CPU 0-100%, 延迟 0-60000ms 等) |
| CSRF 攻击 | SameSite=Strict Cookie |
| 注册码重复使用 | 一次性消费,15 分钟过期 |
| Agent root 运行 | 非 root + 最小 capabilities |
| 公开分享页泄露 | 白名单字段,无 IP/Token/配置 |
| WebSocket 未授权访问 | WS 端点强制 JWT 认证 |

## 数据合理性校验

Server 对 Agent 上报的数据执行以下校验,超范围数据将被丢弃并记录告警:

| 校验项 | 规则 | 异常处理 |
|--------|------|---------|
| CPU 使用率 | 0-100% | 丢弃 + 告警 |
| 内存使用率 | 0-100%, used ≤ total | 丢弃 |
| 磁盘使用率 | 0-100% | 丢弃 |
| 延迟 | 0-60000ms | 丢弃 |
| 丢包率 | 0-100% | 丢弃 |
| 上报频率 | 每 3 秒 ±1 秒 | 过快拒绝,过慢标记离线 |
| 数据大小 | 单次 ≤ 10KB | 超大拒绝 |

## 安全审计

代码中不包含以下高危模式 (可通过全局搜索验证):

```bash
# 验证 Agent 无命令执行
grep -r "os/exec" agent/
grep -r "exec.Command" agent/

# 验证 Server 不主动连接 Agent
grep -r "net.Dial" server/ | grep -v "ssrf"

# 验证无硬编码密钥
grep -r "password.*=.*\"" --include="*.go" | grep -v test
grep -r "secret.*=.*\"" --include="*.go" | grep -v test
```

## 报告漏洞

如果您发现了安全漏洞,请按以下流程报告:

1. **不要** 在 GitHub Issue 中公开披露
2. 发送邮件至: 请在 GitHub Issues 中查看维护者联系方式,或通过 GitHub Security Advisory 报告
3. 邮件标题: `[SECURITY] Server Probe - 简要描述`
4. 邮件内容请包含:
   - 漏洞描述
   - 复现步骤
   - 影响范围
   - 建议的修复方案 (如有)

### 响应时间

- 确认收到: 24 小时内
- 初步评估: 72 小时内
- 修复发布: 视严重程度,7-30 天内

### 披露政策

- 漏洞修复并发布后,我们会在 GitHub Security Advisory 中公开披露
- 感谢报告者 (如愿意公开姓名)

## 依赖安全

- Go 依赖通过 `go mod` 管理,定期更新
- 前端依赖通过 `npm audit` 检查
- CI/CD 流程中包含依赖扫描

## 合规说明

本系统的设计符合以下安全实践:

- 最小权限原则 (Principle of Least Privilege)
- 纵深防御 (Defense in Depth)
- 安全默认配置 (Secure by Default)
- 失败安全 (Fail Safe)
