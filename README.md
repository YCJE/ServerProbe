# Server Probe - 服务器探针监控系统

> 安全优先、只读架构的服务器监控探针系统

[![Go Version](https://img.shields.io/badge/Go-1.23+-00ADD8)](https://go.dev/)
[![License](https://img.shields.io/badge/License-MIT-blue)](LICENSE)

## 目录

- [特性](#特性)
- [架构](#架构)
- [安装、升级与卸载](#安装升级与卸载)
  - [安装 Server](#安装-server)
  - [安装 Agent](#安装-agent)
  - [升级](#升级)
  - [卸载](#卸载)
- [配置域名和 HTTPS 证书](#配置域名和-https-证书)
- [功能使用指南](#功能使用指南)
- [JWT 配置说明](#jwt-配置说明)
- [Server 配置详解](#server-配置详解)
- [Agent 配置详解](#agent-配置详解)
- [日常运维](#日常运维)
- [从源码构建](#从源码构建)
- [FAQ](#faq)

## 特性

### 监控与数据采集

- **实时监控**: CPU/内存/磁盘/网络，3 秒粒度，WebSocket 实时推送
- **网络探测**: ICMP/TCP/HTTP Ping，自动降级，每目标独立延迟统计（平均/最小/最大/抖动/丢包）
- **延迟格子图**: NodeGet 风格的每目标延迟可视化，颜色分级 + 高度映射，IPv4/IPv6 自动分组
- **CPU 温度采集**: 读取 `/sys/class/thermal`，详情页展示温度
- **服务器元数据**: 位置/国家/ISP/到期时间/价格/月流量配额，支持国旗徽章、流量进度条、月成本汇总
- **进程监控**: Top N 进程列表（按 CPU/内存排序）
- **NTP 时间偏移**: 检测系统时钟偏差

### 面板与视图

- **三种视图**: 卡片视图 / 表格视图 / 离线世界地图（ECharts 气泡标注）
- **多维筛选**: 名称搜索、标签筛选、地区筛选、IPv4/IPv6/双栈筛选
- **URL 参数持久化**: 筛选状态写入 URL，刷新和分享链接均保持视图
- **标签系统**: 独立标签表，自定义颜色，卡片徽章统一着色
- **公开仪表盘**: 无需登录的只读公开页（敏感字段已过滤）
- **分享页**: 可配置展示内容的独立分享页面

### 告警与通知

- **告警规则**: CPU/内存/磁盘/离线/服务状态/SSL 证书/流量配额/到期天数，8 种指标
- **告警历史**: FIRING 触发落盘、RESOLVED 恢复回填，时间线展示，30 天自动清理
- **状态机去重**: PENDING → FIRING → RESOLVED，静默期内不重复通知
- **通知渠道**: Webhook/Telegram/Email，配置脱敏返回，支持测试发送

### 服务监控与安全

- **服务监控**: HTTP/TCP 服务可用性检测，独立告警
- **SSL 证书监控**: 证书到期天数检测与告警
- **TOTP 两步验证**: RFC 6238 标准，二维码绑定，验证码一次性消费防重放
- **只读架构**: Agent 仅采集系统指标，不接收任何控制指令
- **强制 TLS**: 全程加密通信，拒绝明文连接
- **非 root 运行**: Agent 以 `probe` 用户运行，最小权限
- **无远程执行**: Agent 不包含任何命令执行/终端/文件操作能力
- **SSRF 防护**: Webhook 通知与 Ping 目标内置 SSRF 防护层
- **单管理员**: 无多租户攻击面，JWT + HttpOnly Cookie + 登录限速
- **单二进制部署**: 前端内嵌，一个文件即可运行

## 架构

```
┌──────────────┐     WSS (TLS)     ┌──────────────┐     HTTPS      ┌────────────┐
│   Agent      │ <---------------> │   Server     │ <------------> │  Browser   │
│  (Collector) │   上报 + 心跳      │ (Backend +   │   JWT Cookie   │  (Panel)   │
│              │                   │   Frontend)  │                │            │
└──────────────┘                   └──────────────┘                └────────────┘
```

- **Agent**: 部署在被监控服务器，采集系统指标并上报
- **Server**: 接收数据，提供 Web 面板和 API，内嵌 React 前端
- **Browser**: 管理员通过浏览器访问监控面板

---

## 安装、升级与卸载

本章涵盖完整的生命周期管理：安装 → 升级 → 卸载。所有操作均提供一键脚本，升级保留全部配置和数据，卸载有交互式确认防误操作。

### 安装 Server

#### 前提条件

- Linux 服务器 (Ubuntu/Debian/CentOS 等)
- root 权限
- 开放一个端口 (默认 8443)

#### 方式一: 一键脚本安装 (推荐)

此方式会自动安装 Go 和 Node.js，从源码编译，无需预编译二进制。

```bash
curl -fsSL https://raw.githubusercontent.com/YCJE/ServerProbe/master/scripts/install-server.sh | bash -s -- --port 8443
```

脚本会自动完成:
1. 安装 Go 和 Node.js (如未安装)
2. 克隆代码仓库并编译
3. 创建 `probe-server` 系统用户
4. 生成 JWT 密钥和配置文件
5. 安装 systemd 服务并启动

安装完成后会显示访问地址，首次访问需要在浏览器中设置管理员账号。

#### 方式二: 手动安装

如果你已经有编译好的二进制文件，可以手动安装:

**第 1 步: 安装二进制**

```bash
# 将二进制放到 /usr/local/bin/
cp probe-server /usr/local/bin/probe-server
chmod +x /usr/local/bin/probe-server
```

**第 2 步: 创建系统用户**

```bash
useradd -r -s /usr/sbin/nologin -d /var/lib/probe-server probe-server
```

**第 3 步: 创建目录**

```bash
mkdir -p /etc/probe-server /var/lib/probe-server
chown probe-server:probe-server /var/lib/probe-server
```

**第 4 步: 生成 JWT 密钥**

```bash
# 生成随机密钥
JWT_SECRET=$(head -c 32 /dev/urandom | base64 | tr -d '/+=' | head -c 32)
echo "你的 JWT 密钥: $JWT_SECRET"
```

**第 5 步: 创建配置文件**

```bash
cat > /etc/probe-server/config.yml << EOF
listen: ":8443"
data_dir: "/var/lib/probe-server"
jwt_secret: "${JWT_SECRET}"
tls:
  auto: true
  cert_file: ""
  key_file: ""
EOF

chmod 600 /etc/probe-server/config.yml
chown probe-server:probe-server /etc/probe-server/config.yml
```

**第 6 步: 创建 systemd 服务**

```bash
cat > /etc/systemd/system/probe-server.service << 'EOF'
[Unit]
Description=Server Probe Server
After=network.target
Wants=network-online.target

[Service]
Type=simple
User=probe-server
Group=probe-server
ExecStart=/usr/local/bin/probe-server --config /etc/probe-server/config.yml
Restart=always
RestartSec=5
NoNewPrivileges=true
ProtectSystem=strict
ProtectHome=true
PrivateTmp=true
ReadWritePaths=/var/lib/probe-server /etc/probe-server
LimitNOFILE=65536

[Install]
WantedBy=multi-user.target
EOF
```

**第 7 步: 启动服务**

```bash
systemctl daemon-reload
systemctl enable probe-server
systemctl start probe-server

# 检查状态
systemctl status probe-server
```

**第 8 步: 访问面板**

浏览器打开 `https://你的服务器IP:8443`，首次访问会要求设置管理员账号和密码。

> 浏览器会提示证书不安全，因为使用的是自签名证书。点击"高级" -> "继续前往"即可。如需使用正式证书，请参考下面的[域名配置](#配置域名和-https-证书)章节。

#### 方式三: Docker 安装

```bash
# 克隆仓库
git clone https://github.com/YCJE/ServerProbe.git
cd ServerProbe

# 启动
docker compose up -d

# 查看日志
docker compose logs -f
```

默认监听 443 端口。如需修改，编辑 `docker-compose.yml` 中的端口映射。

### 安装 Agent

#### 前提条件

- Server 已安装并运行

#### 方式一: 两步式添加 + 一键安装 (推荐)

面板采用「先添加、后安装」的两步流程（Komari 风格）:

1. 登录 Server 面板，进入 **Agent 管理** 页面
2. 点击 **添加服务器**，填写基本信息和元数据（可选：位置/国家/ISP/到期时间/价格/月流量配额）
3. 创建成功后，页面会显示一键安装命令，复制到被监控服务器上以 root 执行:

```bash
curl -fsSL https://raw.githubusercontent.com/YCJE/ServerProbe/master/scripts/install-agent.sh | bash -s -- --server https://your-server.com:8443 --token YOUR_TOKEN
```

Agent 首次连接时自动绑定主机指纹并回填系统信息（主机名/系统/架构），同时记录 IPv4/IPv6 出口 IP，此后按指纹严格校验，防止 Token 泄露后被其他主机冒用。

已添加的服务器可在列表中点击 **安装命令** 随时重新获取命令（用于重装或迁移）。

#### 方式二: 注册码 (兼容旧流程)

在 **Agent 管理** 页面点击 **生成注册码**，然后:

```bash
curl -fsSL https://raw.githubusercontent.com/YCJE/ServerProbe/master/scripts/install-agent.sh | bash -s -- --server https://your-server.com:8443 --code YOUR_CODE
```

参数说明:
- `--server`: Server 地址，**必须以 `https://` 开头**
- `--token`: 后台添加服务器后生成的 Token（推荐，与 `--code` 二选一）
- `--code`: 在 Server 面板中生成的注册码（与 `--token` 二选一，15 分钟有效）
- `--secure-tls`: 启用 TLS 证书验证（Server 使用受信任 CA 签发的证书时使用；默认跳过以兼容自签名证书）

脚本会自动:
1. 安装 Go (如未安装)
2. 从源码编译 Agent
3. 创建 `probe` 系统用户
4. 生成配置文件 (权限 600)
5. 设置 ICMP Ping 权限 (setcap)
6. 安装 systemd 服务并启动

#### 手动安装

```bash
# 1. 安装二进制
cp probe-agent /usr/local/bin/probe-agent
chmod +x /usr/local/bin/probe-agent

# 2. 创建用户
useradd -r -s /usr/sbin/nologin probe

# 3. 创建配置
mkdir -p /etc/probe-agent
cat > /etc/probe-agent/config.yml << 'EOF'
server: "https://your-server.com:8443"
token: "YOUR_TOKEN"        # Token 直连 (推荐)
# register_code: "YOUR_CODE"  # 或使用注册码 (二选一)
report_interval: 3
config_sync_interval: 3600
ping_method: "auto"
EOF
chmod 600 /etc/probe-agent/config.yml
chown probe:probe /etc/probe-agent/config.yml

# 4. 设置 ICMP 权限
setcap cap_net_raw+ep /usr/local/bin/probe-agent

# 5. 创建 systemd 服务
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

# 6. 启动
systemctl daemon-reload
systemctl enable probe-agent
systemctl start probe-agent
```

#### 验证 Agent 连接

```bash
# 查看 Agent 日志
journalctl -u probe-agent -f

# 正常日志应显示:
# "注册成功，保存 Token"
# "Agent 已启动，开始监控"
```

在 Server 面板中应该能看到该 Agent 上线，卡片上会显示国旗徽章、标签和延迟格子图。

### 升级

升级脚本仅更新二进制文件，**保留所有配置和数据**，无需卸载重装。升级失败会自动回滚到旧版本。

**升级 Server:**

```bash
curl -fsSL https://raw.githubusercontent.com/YCJE/ServerProbe/master/scripts/upgrade-server.sh | bash
```

**升级 Agent:**

```bash
curl -fsSL https://raw.githubusercontent.com/YCJE/ServerProbe/master/scripts/upgrade-agent.sh | bash
```

可选参数:
- `--release`: 从 Release 下载预编译二进制（默认从源码编译）
- `--from-source`: 强制从源码编译
- `--version <版本号>`: 指定升级到的版本

升级过程:
1. 检查已安装状态，备份当前二进制
2. 编译/下载新版本
3. 替换二进制（Agent 会重新设置 setcap ICMP 权限）
4. 重启 systemd 服务
5. 失败自动回滚备份

> 升级 Agent 后无需重新配置，Token 和指纹绑定保持不变。

> 如果 `curl` 无法解析 `raw.githubusercontent.com`，可使用 ghproxy 加速:
> ```bash
> curl -fsSL https://ghproxy.com/https://raw.githubusercontent.com/YCJE/ServerProbe/master/scripts/upgrade-server.sh -o /tmp/upgrade.sh && bash /tmp/upgrade.sh
> ```

### 卸载

如需完全移除 Server Probe，可使用一键卸载脚本。**两个卸载脚本都有交互式确认 (`y/N`)，防止误操作。**

**卸载 Server:**

```bash
curl -fsSL https://raw.githubusercontent.com/YCJE/ServerProbe/master/scripts/uninstall-server.sh | bash
```

清理内容:
- 停止并禁用 systemd 服务
- 删除二进制文件 `/usr/local/bin/probe-server`
- 删除配置目录 `/etc/probe-server`
- 删除数据目录 `/var/lib/probe-server`
- 删除系统用户 `probe-server`
- 可选清理 Go 环境

如需保留数据（例如仅重装），使用 `--keep-data` 参数:

```bash
curl -fsSL https://raw.githubusercontent.com/YCJE/ServerProbe/master/scripts/uninstall-server.sh | bash -s -- --keep-data
```

**卸载 Agent:**

```bash
curl -fsSL https://raw.githubusercontent.com/YCJE/ServerProbe/master/scripts/uninstall-agent.sh | bash
```

清理内容:
- 停止并禁用 systemd 服务
- 移除 setcap CAP_NET_RAW 权限
- 删除二进制文件 `/usr/local/bin/probe-agent`
- 删除配置目录 `/etc/probe-agent`
- 删除系统用户 `probe`
- 可选清理 Go 环境

> 卸载 Agent 只是移除被监控机器上的程序。如需同时在面板中删除该服务器记录，请在 **Agent 管理** 页面中删除，否则会残留离线记录。

> 如果 `curl` 无法解析 `raw.githubusercontent.com`，可以先下载脚本再执行:
> ```bash
> # 方法一: 使用 ghproxy 加速
> curl -fsSL https://ghproxy.com/https://raw.githubusercontent.com/YCJE/ServerProbe/master/scripts/uninstall-server.sh -o /tmp/uninstall.sh && bash /tmp/uninstall.sh
>
> # 方法二: 手动下载后上传
> # 1. 在能访问 GitHub 的电脑上下载脚本
> # 2. 上传到服务器
> # 3. 执行: bash uninstall-server.sh
> ```

---

## 配置域名和 HTTPS 证书

默认情况下 Server 使用自签名证书，浏览器会报警告。生产环境建议配置域名和正式证书。

### 使用 Nginx 反向代理 + Let's Encrypt

这是最推荐的方式，免费获取正式证书。

**第 1 步: 安装 Nginx 和 Certbot**

```bash
# Ubuntu/Debian
apt update
apt install -y nginx certbot python3-certbot-nginx

# CentOS
yum install -y nginx certbot python3-certbot-nginx
```

**第 2 步: 配置 DNS**

在你的域名服务商处，添加 A 记录:
- 记录类型: A
- 主机记录: probe (或你喜欢的子域名)
- 记录值: 你的服务器 IP

**第 3 步: 修改 Server 监听端口**

编辑 `/etc/probe-server/config.yml`，将端口改为本地端口 (不对外暴露):

```yaml
listen: "127.0.0.1:8443"
```

重启 Server:

```bash
systemctl restart probe-server
```

**第 4 步: 配置 Nginx 反向代理**

> **重要**: 只写一个监听 80 的 server 块，**不要预先写 443 块**。Certbot 申请证书时会自动创建 443 块并配置证书。预先写 443 块会导致 Certbot 重复添加，产生 `conflicting server name` 冲突。

```bash
cat > /etc/nginx/conf.d/probe.conf << 'EOF'
server {
    listen 80;
    server_name probe.yourdomain.com;

    # Certbot 验证用
    location /.well-known/acme-challenge/ {
        root /var/www/html;
    }

    # 反向代理到 Server (证书申请前先用 HTTP)
    location / {
        proxy_pass https://127.0.0.1:8443;
        proxy_set_header Host $host;
        proxy_set_header X-Real-IP $remote_addr;
        proxy_set_header X-Forwarded-For $proxy_add_x_forwarded_for;
        proxy_set_header X-Forwarded-Proto $scheme;

        # WebSocket 支持
        proxy_http_version 1.1;
        proxy_set_header Upgrade $http_upgrade;
        proxy_set_header Connection "upgrade";

        # 超时设置
        proxy_read_timeout 86400;
        proxy_send_timeout 86400;
    }
}
EOF
```

测试配置并重载:

```bash
nginx -t
systemctl reload nginx
```

此时通过 `http://probe.yourdomain.com` 即可访问 (HTTP，浏览器会提示不安全，下一步申请证书后自动变为 HTTPS)。

> 如果使用反向代理，建议在 Server 环境变量中设置 `TRUSTED_PROXIES`（如 `TRUSTED_PROXIES=127.0.0.1,::1`），这样限速中间件才能按真实客户端 IP 生效。

**第 5 步: 申请 SSL 证书**

```bash
# 确保域名已解析到本服务器
certbot --nginx -d probe.yourdomain.com
```

按提示操作，Certbot 会自动:
1. 验证域名所有权
2. 申请 SSL 证书
3. 修改 Nginx 配置: 将 80 端口重定向到 443，自动添加 443 server 块和证书路径
4. 重载 Nginx

**第 6 步: 验证**

```bash
# 测试 Nginx 配置 (不应有 conflicting server name 警告)
nginx -t

# 重载
systemctl reload nginx
```

现在通过 `https://probe.yourdomain.com` 访问了，浏览器不会再报警告。

**第 7 步: 设置证书自动续期**

```bash
# 测试续期
certbot renew --dry-run

# Certbot 会自动添加 cron 定时任务，无需手动配置
```

> **如果出现 `conflicting server name` 警告**: 说明 `probe.conf` 中有多个重复的 server 块。执行 `cat -n /etc/nginx/conf.d/probe.conf` 查看内容，只保留 Certbot 管理的 server 块 (含 `# managed by Certbot` 注释的行)，删除多余的 server 块，然后 `nginx -t && systemctl reload nginx`。

### 使用自有证书

如果你已经有 SSL 证书，可以直接配置到 Server:

**第 1 步: 上传证书**

```bash
# 将证书放到服务器上
cp your-cert.pem /etc/probe-server/cert.pem
cp your-key.pem /etc/probe-server/key.pem
chown probe-server:probe-server /etc/probe-server/*.pem
chmod 600 /etc/probe-server/*.pem
```

**第 2 步: 修改配置文件**

编辑 `/etc/probe-server/config.yml`:

```yaml
listen: ":8443"
data_dir: "/var/lib/probe-server"
jwt_secret: "你的JWT密钥"
tls:
  auto: false
  cert_file: "/etc/probe-server/cert.pem"
  key_file: "/etc/probe-server/key.pem"
```

**第 3 步: 重启服务**

```bash
systemctl restart probe-server
```

---

## 功能使用指南

### 系统设置

**系统设置** 页面集中管理站点信息与数据加载参数，保存后即时生效（无需重启）:

- **站点信息**: 站点标题（浏览器标签页与登录页）、站点描述、公告（面板顶部横幅）、自定义页脚
- **默认历史范围**: 服务器详情页初次加载时的图表时间范围（1h/6h/12h/1d/2d/3d）
- **离线判定宽限期**: Agent 断开后多久判定为离线（30–86400 秒，默认 90 秒）
- **数据保留天数**: 历史数据保留时长（1–3650 天，默认 4 天），每天自动清理过期数据
- **最大图表数据点**: 图表单次加载的最大数据点数（100–2000，默认 800），过大可能加载变慢
- **数据库管理**: 查看数据库体积/记录数统计、下载一致性备份快照、执行 VACUUM 压缩、按天数清理历史数据

### 两步验证 (TOTP)

为管理员账户启用动态口令两步验证，防止密码泄露导致面板被入侵:

1. 进入 **系统状态** 页面的 **两步验证** 区域
2. 点击 **生成密钥**，使用 Google Authenticator / Microsoft Authenticator 等扫码导入
3. 输入认证器上的 6 位验证码完成绑定
4. 此后登录需输入密码 + 动态验证码两步完成

安全细节:
- 验证码一次性消费，同一 30 秒窗口内不可重放
- 登录接口限速（每 IP 每分钟 5 次），暴力破解动态码会被拦截
- 停用两步验证需再次确认密码，防止会话被劫持后直接关闭

### 告警与告警历史

**告警管理** 页面分为两个 Tab:

- **告警规则**: 支持 8 种指标 —— CPU 使用率 / 内存使用率 / 磁盘使用率 / Agent 离线 / 服务状态 / SSL 证书到期 / 流量配额使用率 / 服务器到期天数。每条规则可绑定通知渠道，支持手动测试发送
- **告警历史**: 时间线展示 FIRING（触发）→ RESOLVED（恢复）完整记录，包含触发时间、恢复时间、指标值，支持按状态/服务器/规则筛选，历史保留 30 天

### 标签管理

**标签管理** 页面维护独立标签表:

- 自定义标签名称与颜色（`#RRGGBB` 格式，内置色板可选）
- 在 Agent 元数据编辑中为服务器打标签，卡片与表格视图统一显示彩色徽章
- 公开仪表盘同步展示标签颜色

### Ping 目标与延迟格子图

**探测目标** 页面管理 Ping 探测目标:

- 每个目标可指定探测方式（ICMP/TCP/HTTP）、排序、启用状态
- **IP 版本标注**: 每个目标可标注 IPv4/IPv6（默认自动按名称/地址识别），用于延迟格子图准确分组
- 目标变更实时推送到所有在线 Agent，无需重启

服务器卡片和详情页的**延迟格子图**按 IPv4/IPv6 分组展示每个目标:
- 颜色分级: 绿色（快）→ 黄色 → 红色（慢/超时）
- 高度映射延迟大小，悬停显示详细统计（平均/最小/最大延迟、抖动、丢包率）

### 仪表盘视图与筛选

- **卡片视图**: 国旗徽章 + 名称 + 虚拟化标签 + 资源进度条 + 延迟格子图
- **表格视图**: 状态灯 + 名称 + CPU/内存/磁盘进度条 + 网速 + 月流量 + 平均延迟 + 到期时间 + 运行时长
- **地图视图**: 离线世界地图，按服务器位置显示气泡标注
- **筛选**: 搜索、标签、地区、IP 栈（v4/v6/双栈）四种维度，筛选状态写入 URL 可直接分享
- 顶部汇总栏: 在线/离线数、平均负载、总网速、本月总流量、月成本合计（按币种分组，年付折算为月）

### 服务器元数据

在 **Agent 管理** 页面编辑服务器元数据:

- 位置/国家代码（国旗徽章）、ISP
- 到期时间（到期倒计时与告警）
- 价格与计费周期（月成本汇总）
- 月流量配额（流量使用进度条与配额告警）
- IPv4/IPv6 出口 IP（Agent 连接时自动记录，也可手动修正）

---

## JWT 配置说明

JWT (JSON Web Token) 用于管理员登录认证。安装脚本会自动生成随机密钥，你也可以手动配置。

### 自动生成 (推荐)

安装脚本会在配置文件中自动生成 32 位随机密钥:

```bash
# 查看当前 JWT 密钥
grep jwt_secret /etc/probe-server/config.yml
```

### 手动生成

如果你想自己生成密钥:

```bash
# 方法 1: 使用 openssl
openssl rand -base64 32

# 方法 2: 使用 /dev/urandom
head -c 32 /dev/urandom | base64 | tr -d '/+=' | head -c 32

# 方法 3: 使用 uuid
cat /proc/sys/kernel/random/uuid | tr -d '-' | head -c 32
```

将生成的密钥填入配置文件:

```yaml
jwt_secret: "你生成的密钥"
```

重启服务生效:

```bash
systemctl restart probe-server
```

### 注意事项

- **密钥保密**: 不要泄露 JWT 密钥，任何拿到密钥的人可以伪造管理员 Token
- **更换密钥**: 更换密钥后所有已登录用户需要重新登录
- **密钥长度**: 建议至少 32 字符
- **文件权限**: 配置文件权限为 600，只有 root 和 probe-server 用户可读

---

## Server 配置详解

配置文件路径: `/etc/probe-server/config.yml`

```yaml
# 监听地址和端口
# ":8443" 表示监听所有网卡的 8443 端口
# "127.0.0.1:8443" 表示只监听本地 (配合 Nginx 反向代理使用)
listen: ":8443"

# 数据目录 (SQLite 数据库和 JWT 密钥存放在此)
data_dir: "/var/lib/probe-server"

# JWT 签名密钥 (安装时自动生成,可手动更换)
jwt_secret: "your-random-secret"

# TLS 证书配置
tls:
  auto: true              # true: 自动生成自签证书; false: 使用下面的证书
  cert_file: ""           # 证书文件路径 (auto: false 时必填)
  key_file: ""            # 私钥文件路径 (auto: false 时必填)
```

> 数据聚合周期固定为 5 分钟，实时数据使用内存环形缓冲（最近 6 小时，每 3 秒一个点），历史数据保留天数在 **系统设置** 页面配置（默认 4 天，范围 1–3650 天），均无需修改配置文件。

### 环境变量

| 变量 | 说明 | 默认 |
|------|------|------|
| `TRUSTED_PROXIES` | 信任的反代 IP 列表（逗号分隔），设置后限速按真实客户端 IP 生效 | 空（不信任任何代理） |
| `COOKIE_SECURE` | 登录 Cookie 的 Secure 标记 | 生产模式 (`GIN_MODE=release`) 自动开启 |

修改配置后重启生效:

```bash
systemctl restart probe-server
```

## Agent 配置详解

配置文件路径: `/etc/probe-agent/config.yml`

```yaml
# Server 地址 (必须 https:// 开头)
server: "https://your-server.com:8443"

# Token (后台添加服务器后生成,推荐直连方式)
token: "xxxxxxxxxxxxxxxxxxxxxxxxxxxxxxxx"

# 注册码 (兼容旧流程,首次注册用,注册成功后自动清除并替换为 Token)
# 与 token 二选一
# register_code: "ABC123XY"

# 数据上报间隔,秒
report_interval: 3

# 配置同步间隔,秒 (从 Server 拉取 Ping 目标列表)
config_sync_interval: 3600

# Ping 方式
# auto: 自动选择 (ICMP -> TCP -> HTTP)
# icmp: 强制 ICMP
# tcp:  强制 TCP
# http: 强制 HTTP
ping_method: "auto"

# 跳过 TLS 证书验证 (Server 使用自签名证书时使用;受信任 CA 证书请设为 false)
insecure_tls: true

# 允许 Ping 私有网段地址 (默认 false)
# 安全机制: 回环/链路本地/多播地址永远禁止探测;私有网段 (RFC1918) 默认禁止,
# 防止 Server 被攻破后 Agent 被当作内网探测跳板 (SSRF)。
# 如需监控内网目标 (如网关 192.168.1.1), 改为 true
allow_private_targets: false
```

修改配置后重启生效:

```bash
systemctl restart probe-agent
```

### Ping 目标安全限制

Agent 对 Server 下发的 Ping 目标执行安全校验:

- **永远禁止**: 回环地址 (127.0.0.1, ::1)、链路本地地址 (169.254.x.x, fe80::)、未指定地址 (0.0.0.0)、多播/广播地址
- **默认禁止**: 私有网段 (10.x, 172.16-31.x, 192.168.x, IPv6 ULA)，需通过 `allow_private_targets: true` 显式开启
- **数量上限**: 单轮最多探测 20 个目标，超出部分丢弃并记录日志

---

## 日常运维

### 服务管理

```bash
# Server
systemctl start probe-server       # 启动
systemctl stop probe-server        # 停止
systemctl restart probe-server     # 重启
systemctl status probe-server      # 状态
journalctl -u probe-server -f      # 实时日志

# Agent
systemctl start probe-agent        # 启动
systemctl stop probe-agent         # 停止
systemctl restart probe-agent      # 重启
systemctl status probe-agent       # 状态
journalctl -u probe-agent -f       # 实时日志
```

### 备份数据

```bash
# 停止 Server
systemctl stop probe-server

# 备份 SQLite 数据库
cp /var/lib/probe-server/data.db /backup/data-$(date +%Y%m%d).db

# 备份配置
cp /etc/probe-server/config.yml /backup/config-$(date +%Y%m%d).yml

# 重启
systemctl start probe-server
```

### 防火墙配置

```bash
# 仅开放必要端口 (以 ufw 为例)
ufw allow 8443/tcp    # Server 端口
ufw enable

# 或使用 firewalld
firewall-cmd --permanent --add-port=8443/tcp
firewall-cmd --reload
```

---

## 从源码构建

### 环境要求

- Go 1.23+
- Node.js 20+
- npm

### 构建步骤

```bash
# 1. 克隆仓库
git clone https://github.com/YCJE/ServerProbe.git
cd ServerProbe

# 2. 构建前端
cd server/frontend
npm install
npm run build          # 输出到 server/web/

# 3. 构建 Server
cd ../..
cd server
CGO_ENABLED=0 go build -ldflags "-s -w" -o ../bin/probe-server ./cmd/server

# 4. 构建 Agent
cd ../agent
CGO_ENABLED=0 go build -ldflags "-s -w" -o ../bin/probe-agent ./cmd/agent
```

### 交叉编译

```bash
# Linux amd64
CGO_ENABLED=0 GOOS=linux GOARCH=amd64 go build -o probe-server ./cmd/server

# Linux arm64
CGO_ENABLED=0 GOOS=linux GOARCH=arm64 go build -o probe-server ./cmd/server
```

---

## FAQ

### Q: 浏览器提示证书不安全?

Server 默认使用自签名 TLS 证书。解决方案:

1. **生产环境**: 配置 Nginx 反向代理 + Let's Encrypt 免费证书 (参考上面的[域名配置](#配置域名和-https-证书))
2. **临时方案**: 浏览器点击"高级" -> "继续前往"
3. **自有证书**: 修改配置文件使用自己的证书

### Q: 一键安装脚本下载失败?

脚本默认从源码构建，需要网络连接。如果 GitHub 访问慢，可以:

1. 手动克隆仓库: `git clone https://github.com/YCJE/ServerProbe.git`
2. 手动构建 (参考[从源码构建](#从源码构建))
3. 将二进制传到服务器手动安装 (参考[手动安装](#方式二-手动安装))

### Q: Agent 无法连接 Server?

排查步骤:

```bash
# 1. 检查 Agent 日志
journalctl -u probe-agent -f

# 2. 确认 Server 地址以 https:// 开头
grep server /etc/probe-agent/config.yml

# 3. 测试网络连通性
curl -k https://your-server.com:8443

# 4. 检查 Server 是否运行
systemctl status probe-server

# 5. 检查防火墙
ufw status
```

### Q: ICMP Ping 不工作?

Agent 默认尝试 ICMP Ping，需要 `CAP_NET_RAW` 权限。安装脚本会自动设置。如果不可用:

```bash
# 手动设置
setcap cap_net_raw+ep /usr/local/bin/probe-agent

# 或降级为 TCP Ping (修改配置)
# ping_method: "tcp"
```

### Q: 延迟格子图中目标分组错误?

延迟格子图按 IPv4/IPv6 分组展示。默认按目标名称/地址自动识别（如名称含 "6" 或地址含 ":" 判定为 IPv6）。如果自动识别不准，在 **探测目标** 页面编辑该目标，显式指定 IP 版本为 IPv4 或 IPv6。

### Q: 如何修改上报间隔?

编辑 `/etc/probe-agent/config.yml`，修改 `report_interval` 后重启:

```bash
systemctl restart probe-agent
```

注意: Server 端会校验上报频率，过快会被拒绝。建议保持默认 3 秒。

### Q: 数据存储在哪里?

- 实时数据: 内存环形缓冲 (最近 6 小时,每 3 秒一个点)
- 历史数据: SQLite (`/var/lib/probe-server/data.db`)，每 5 分钟聚合一次
- 告警历史: 同一 SQLite 数据库，保留 30 天
- 历史数据保留天数在 **系统设置** 页面配置（默认 4 天，范围 1–3650 天），每天自动清理过期数据

### Q: 支持哪些操作系统?

- Agent: Linux (amd64/arm64/armv7)
- Server: Linux (amd64/arm64/armv7)
- 浏览器: Chrome/Firefox/Safari/Edge 最新版

### Q: 忘记管理员密码怎么办?

**方法一: 命令行重置 (推荐)**

```bash
# 停止 Server
systemctl stop probe-server

# 删除管理员账户 (需要重新设置)
sqlite3 /var/lib/probe-server/data.db "DELETE FROM admins;"

# 重启 Server
systemctl start probe-server
```

重启后访问面板，会自动跳转到"初始化设置"页面，重新设置管理员账号和密码。

**方法二: 如果没有 sqlite3 命令**

```bash
# 停止 Server
systemctl stop probe-server

# 直接删除数据库文件 (会丢失所有数据,包括 Agent 信息和历史记录)
rm /var/lib/probe-server/data.db

# 重启 Server
systemctl start probe-server
```

> 注意: 方法二会丢失所有数据，仅在没有重要数据时使用。

### Q: 换了手机如何重新绑定两步验证?

两步验证绑定在管理员账户上，换手机需先解绑再重新绑定:

1. 登录面板（需要旧手机上的动态码），进入 **系统状态** → **两步验证**
2. 输入密码确认后停用
3. 重新生成密钥，用新手机扫码绑定

如果旧手机已丢失无法登录，用上面的密码重置方法删除管理员账户，重新设置后重新绑定。

### Q: 如何查看 Server 版本?

当前版本暂未提供 `--version` 命令行参数。可通过以下方式确认版本:

```bash
# 查看二进制文件的修改时间（大致对应升级时间）
ls -l /usr/local/bin/probe-server

# 查看服务启动日志
journalctl -u probe-server --no-pager | tail -20
```

也可以在 GitHub Releases 页面对照升级时间确认当前运行版本。

## 安全

请查阅 [SECURITY.md](SECURITY.md) 了解安全设计原则和漏洞报告流程。

## License

[MIT](LICENSE)
