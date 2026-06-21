#!/bin/bash
# Server Probe Agent 一键安装脚本
# 用法: curl -fsSL https://your-server.com/install.sh | bash -s -- --server https://your-server.com --code ABC123XY

set -e

# 颜色输出
RED='\033[0;31m'
GREEN='\033[0;32m'
YELLOW='\033[1;33m'
NC='\033[0m'

info()  { echo -e "${GREEN}[INFO]${NC} $1"; }
warn()  { echo -e "${YELLOW}[WARN]${NC} $1"; }
error() { echo -e "${RED}[ERROR]${NC} $1"; exit 1; }

# 解析参数
SERVER_URL=""
REGISTER_CODE=""
INSTALL_DIR="/usr/local/bin"
CONFIG_DIR="/etc/probe-agent"
SERVICE_NAME="probe-agent"

while [[ $# -gt 0 ]]; do
    case $1 in
        --server) SERVER_URL="$2"; shift 2;;
        --code) REGISTER_CODE="$2"; shift 2;;
        --help)
            echo "用法: install-agent.sh --server <URL> --code <注册码>"
            exit 0
            ;;
        *) error "未知参数: $1";;
    esac
done

if [ -z "$SERVER_URL" ]; then
    error "必须指定 --server 参数"
fi

if [ -z "$REGISTER_CODE" ]; then
    error "必须指定 --code 参数"
fi

# 检查 root 权限
if [ "$EUID" -ne 0 ]; then
    error "请以 root 用户运行此脚本"
fi

# 检测系统
info "检测系统信息..."
OS=$(uname -s | tr '[:upper:]' '[:lower:]')
ARCH=$(uname -m)

case $ARCH in
    x86_64)  ARCH="amd64";;
    aarch64) ARCH="arm64";;
    armv7l)  ARCH="armv7";;
    *) error "不支持的架构: $ARCH";;
esac

info "系统: $OS/$ARCH"

# 下载 Agent 二进制
AGENT_URL="${SERVER_URL}/download/agent/${OS}/${ARCH}"
TMP_FILE="/tmp/probe-agent"

info "下载 Agent..."
if command -v curl &> /dev/null; then
    curl -fsSL -o "$TMP_FILE" "$AGENT_URL" || error "下载失败"
else
    wget -qO "$TMP_FILE" "$AGENT_URL" || error "下载失败"
fi

# 校验 SHA256
SHA256_URL="${AGENT_URL}.sha256"
if command -v curl &> /dev/null; then
    curl -fsSL -o "${TMP_FILE}.sha256" "$SHA256_URL" 2>/dev/null || true
else
    wget -qO "${TMP_FILE}.sha256" "$SHA256_URL" 2>/dev/null || true
fi

if [ -f "${TMP_FILE}.sha256" ]; then
    info "校验文件完整性..."
    echo "$(cat ${TMP_FILE}.sha256)  ${TMP_FILE}" | sha256sum -c - || error "校验失败"
fi

# 安装二进制
info "安装 Agent..."
chmod +x "$TMP_FILE"
mv "$TMP_FILE" "${INSTALL_DIR}/probe-agent"

# 创建 probe 用户
if ! id probe &>/dev/null; then
    info "创建 probe 用户..."
    useradd -r -s /usr/sbin/nologin probe
fi

# 创建配置目录
mkdir -p "$CONFIG_DIR"

# 生成配置文件
info "生成配置文件..."
cat > "${CONFIG_DIR}/config.yml" << EOF
server: "${SERVER_URL}"
register_code: "${REGISTER_CODE}"
report_interval: 3
config_sync_interval: 3600
ping_method: "auto"
EOF

chmod 600 "${CONFIG_DIR}/config.yml"
chown probe:probe "${CONFIG_DIR}/config.yml"

# 尝试 setcap（ICMP Ping）
info "配置 ICMP 权限..."
if command -v setcap &> /dev/null; then
    if setcap cap_net_raw+ep "${INSTALL_DIR}/probe-agent" 2>/dev/null; then
        info "ICMP Ping 已启用 (CAP_NET_RAW)"
    else
        warn "setcap 失败，将尝试 unprivileged ICMP 或降级到 TCP Ping"
    fi
else
    warn "setcap 不可用，将尝试 unprivileged ICMP 或降级到 TCP Ping"
fi

# 安装 systemd service
info "安装 systemd 服务..."
cat > "/etc/systemd/system/${SERVICE_NAME}.service" << 'EOF'
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

# 启动服务
info "启动 Agent..."
systemctl daemon-reload
systemctl enable "${SERVICE_NAME}"
systemctl start "${SERVICE_NAME}"

info "安装完成！"
info "查看状态: systemctl status ${SERVICE_NAME}"
info "查看日志: journalctl -u ${SERVICE_NAME} -f"
