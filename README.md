# Server Probe - 服务器探针监控系统

> 安全优先、只读架构的服务器监控探针系统

[![Go Version](https://img.shields.io/badge/Go-1.23+-00ADD8)](https://go.dev/)
[![License](https://img.shields.io/badge/License-MIT-blue)](LICENSE)

## 目录

- [特性](#特性)
- [架构](#架构)
- [安装 Server](#安装-server)
  - [方式一: 一键脚本安装 (推荐)](#方式一-一键脚本安装-推荐)
  - [方式二: 手动安装](#方式二-手动安装)
  - [方式三: Docker 安装](#方式三-docker-安装)
- [配置域名和 HTTPS 证书](#配置域名和-https-证书)
  - [使用 Nginx 反向代理 + Let's Encrypt](#使用-nginx-反向代理--lets-encrypt)
  - [使用自有证书](#使用自有证书)
- [JWT 配置说明](#jwt-配置说明)
- [安装 Agent](#安装-agent)
- [Server 配置详解](#server-配置详解)
- [Agent 配置详解](#agent-配置详解)
- [日常运维](#日常运维)
- [从源码构建](#从源码构建)
- [FAQ](#faq)

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
│   Agent      │ <---------------> │   Server     │ <------------> │  Browser   │
│  (Collector) │   上报 + 心跳      │ (Backend +   │   JWT Cookie   │  (Panel)   │
│              │                   │   Frontend)  │                │            │
└──────────────┘                   └──────────────┘                └────────────┘
```

- **Agent**: 部署在被监控服务器,采集系统指标并上报
- **Server**: 接收数据,提供 Web 面板和 API,内嵌 React 前端
- **Browser**: 管理员通过浏览器访问监控面板

---

## 安装 Server

### 前提条件

- Linux 服务器 (Ubuntu/Debian/CentOS 等)
- root 权限
- 开放一个端口 (默认 8443)

### 方式一: 一键脚本安装 (推荐)

此方式会自动安装 Go 和 Node.js,从源码编译,无需预编译二进制。

```bash
curl -fsSL https://raw.githubusercontent.com/YCJE/ServerProbe/master/scripts/install-server.sh | bash -s -- --port 8443
```

脚本会自动完成:
1. 安装 Go 和 Node.js (如未安装)
2. 克隆代码仓库并编译
3. 创建 `probe-server` 系统用户
4. 生成 JWT 密钥和配置文件
5. 安装 systemd 服务并启动

安装完成后会显示访问地址,首次访问需要在浏览器中设置管理员账号。

### 方式二: 手动安装

如果你已经有编译好的二进制文件,可以手动安装:

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
aggregation:
  interval: 300
  retention_days: 90
ring_buffer:
  size: 3600
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

浏览器打开 `https://你的服务器IP:8443`,首次访问会要求设置管理员账号和密码。

> 浏览器会提示证书不安全,因为使用的是自签名证书。点击"高级" -> "继续前往"即可。如需使用正式证书,请参考下面的[域名配置](#配置域名和-https-证书)章节。

### 方式三: Docker 安装

```bash
# 克隆仓库
git clone https://github.com/YCJE/ServerProbe.git
cd ServerProbe

# 启动
docker compose up -d

# 查看日志
docker compose logs -f
```

默认监听 443 端口。如需修改,编辑 `docker-compose.yml` 中的端口映射。

---

## 配置域名和 HTTPS 证书

默认情况下 Server 使用自签名证书,浏览器会报警告。生产环境建议配置域名和正式证书。

### 使用 Nginx 反向代理 + Let's Encrypt

这是最推荐的方式,免费获取正式证书。

**第 1 步: 安装 Nginx 和 Certbot**

```bash
# Ubuntu/Debian
apt update
apt install -y nginx certbot python3-certbot-nginx

# CentOS
yum install -y nginx certbot python3-certbot-nginx
```

**第 2 步: 配置 DNS**

在你的域名服务商处,添加 A 记录:
- 记录类型: A
- 主机记录: probe (或你喜欢的子域名)
- 记录值: 你的服务器 IP

**第 3 步: 修改 Server 监听端口**

编辑 `/etc/probe-server/config.yml`,将端口改为本地端口 (不对外暴露):

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

如果你已经有 SSL 证书,可以直接配置到 Server:

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

## JWT 配置说明

JWT (JSON Web Token) 用于管理员登录认证。安装脚本会自动生成随机密钥,你也可以手动配置。

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

- **密钥保密**: 不要泄露 JWT 密钥,任何拿到密钥的人可以伪造管理员 Token
- **更换密钥**: 更换密钥后所有已登录用户需要重新登录
- **密钥长度**: 建议至少 32 字符
- **文件权限**: 配置文件权限为 600,只有 root 和 probe-server 用户可读

---

## 安装 Agent

### 前提条件

- Server 已安装并运行
- 在 Server 面板中已生成注册码 (首次登录后在"Agent 管理"页面生成)

### 一键安装

在被监控的服务器上执行:

```bash
curl -fsSL https://raw.githubusercontent.com/YCJE/ServerProbe/master/scripts/install-agent.sh | bash -s -- --server https://your-server.com:8443 --code YOUR_CODE
```

参数说明:
- `--server`: Server 地址,**必须以 `https://` 开头**
- `--code`: 在 Server 面板中生成的注册码

脚本会自动:
1. 安装 Go (如未安装)
2. 从源码编译 Agent
3. 创建 `probe` 系统用户
4. 生成配置文件 (权限 600)
5. 设置 ICMP Ping 权限 (setcap)
6. 安装 systemd 服务并启动

### 手动安装

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
register_code: "YOUR_CODE"
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

### 验证 Agent 连接

```bash
# 查看 Agent 日志
journalctl -u probe-agent -f

# 正常日志应显示:
# "注册成功，保存 Token"
# "Agent 已启动，开始监控"
```

在 Server 面板中应该能看到该 Agent 上线。

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

# 数据聚合配置
aggregation:
  interval: 300           # 聚合间隔,秒 (300 = 5 分钟)
  retention_days: 90      # 历史数据保留天数

# 环形缓冲区 (实时数据缓存)
ring_buffer:
  size: 3600              # 缓存点数 (3600 = 1 小时,每 3 秒一个点)
```

修改配置后重启生效:

```bash
systemctl restart probe-server
```

## Agent 配置详解

配置文件路径: `/etc/probe-agent/config.yml`

```yaml
# Server 地址 (必须 https:// 开头)
server: "https://your-server.com:8443"

# 注册码 (首次注册用,注册成功后自动清除并替换为 Token)
register_code: "ABC123XY"

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
```

修改配置后重启生效:

```bash
systemctl restart probe-agent
```

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

### 卸载

如需完全移除 Server Probe，可使用一键卸载脚本:

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

如需保留数据，使用 `--keep-data` 参数:

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

两个卸载脚本都有交互式确认 (`y/N`)，防止误操作。

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

### 升级

升级脚本仅更新二进制文件，保留所有配置和数据，无需卸载重装。升级失败会自动回滚到旧版本。

**升级 Server:**

```bash
curl -fsSL https://raw.githubusercontent.com/YCJE/ServerProbe/master/scripts/upgrade-server.sh | bash
```

**升级 Agent:**

```bash
curl -fsSL https://raw.githubusercontent.com/YCJE/ServerProbe/master/scripts/upgrade-agent.sh | bash
```

> 如果 `curl` 无法解析 `raw.githubusercontent.com`，可使用 ghproxy 加速:
> ```bash
> curl -fsSL https://ghproxy.com/https://raw.githubusercontent.com/YCJE/ServerProbe/master/scripts/upgrade-server.sh -o /tmp/upgrade.sh && bash /tmp/upgrade.sh
> ```

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

脚本默认从源码构建,需要网络连接。如果 GitHub 访问慢,可以:

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

Agent 默认尝试 ICMP Ping,需要 `CAP_NET_RAW` 权限。安装脚本会自动设置。如果不可用:

```bash
# 手动设置
setcap cap_net_raw+ep /usr/local/bin/probe-agent

# 或降级为 TCP Ping (修改配置)
# ping_method: "tcp"
```

### Q: 如何修改上报间隔?

编辑 `/etc/probe-agent/config.yml`,修改 `report_interval` 后重启:

```bash
systemctl restart probe-agent
```

注意: Server 端会校验上报频率,过快会被拒绝。建议保持默认 3 秒。

### Q: 数据存储在哪里?

- 实时数据: 内存环形缓冲 (最近 1 小时)
- 历史数据: SQLite (`/var/lib/probe-server/data.db`)
- 默认保留 90 天

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

重启后访问面板,会自动跳转到"初始化设置"页面,重新设置管理员账号和密码。

**方法二: 如果没有 sqlite3 命令**

```bash
# 停止 Server
systemctl stop probe-server

# 直接删除数据库文件 (会丢失所有数据,包括 Agent 信息和历史记录)
rm /var/lib/probe-server/data.db

# 重启 Server
systemctl start probe-server
```

> 注意: 方法二会丢失所有数据,仅在没有重要数据时使用。

### Q: 如何查看 Server 版本?

```bash
probe-server --version
```

## 安全

请查阅 [SECURITY.md](SECURITY.md) 了解安全设计原则和漏洞报告流程。

## License

[MIT](LICENSE)
