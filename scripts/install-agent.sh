#!/bin/bash
# Server Probe Agent 安装脚本
# 支持两种模式:
#   1. 从源码构建 (默认,无需 Release)
#   2. 下载预编译二进制 (需要 GitHub Release)
#
# 用法:
#   从源码构建:  ./install-agent.sh --server https://your-server.com:8443 --code ABC123XY
#   下载二进制:  ./install-agent.sh --server https://your-server.com:8443 --code ABC123XY --release

set -e

RED='\033[0;31m'
GREEN='\033[0;32m'
YELLOW='\033[1;33m'
NC='\033[0m'

info()  { echo -e "${GREEN}[INFO]${NC} $1"; }
warn()  { echo -e "${YELLOW}[WARN]${NC} $1"; }
error() { echo -e "${RED}[ERROR]${NC} $1"; exit 1; }

# 参数
SERVER_URL=""
REGISTER_CODE=""
INSTALL_DIR="/usr/local/bin"
CONFIG_DIR="/etc/probe-agent"
SERVICE_NAME="probe-agent"
FROM_SOURCE=true
VERSION=""
REPO_URL="https://github.com/YCJE/ServerProbe.git"
DOWNLOAD_BASE="https://github.com/YCJE/ServerProbe/releases/latest/download"

while [[ $# -gt 0 ]]; do
    case $1 in
        --server) SERVER_URL="$2"; shift 2;;
        --code) REGISTER_CODE="$2"; shift 2;;
        --version) VERSION="$2"; shift 2;;
        --release) FROM_SOURCE=false; shift;;
        --from-source) FROM_SOURCE=true; shift;;
        --help)
            echo "用法: install-agent.sh --server <URL> --code <注册码> [选项]"
            echo ""
            echo "选项:"
            echo "  --server <URL>    Server 地址 (必须 https://)"
            echo "  --code <注册码>   注册码"
            echo "  --version <版本>  指定版本 (仅 --release 模式)"
            echo "  --release         从 GitHub Release 下载二进制"
            echo "  --from-source     从源码构建 (默认)"
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

TMP_FILE="/tmp/probe-agent"

if [ "$FROM_SOURCE" = true ]; then
    # ========== 从源码构建 ==========
    info "从源码构建 Agent..."

    # 检查 Go
    if ! command -v go &> /dev/null; then
        info "安装 Go..."
        GO_VERSION="1.23.4"
        GO_URL="https://go.dev/dl/go${GO_VERSION}.${OS}-${ARCH}.tar.gz"
        if command -v curl &> /dev/null; then
            curl -fsSL "$GO_URL" | tar -C /usr/local -xzf -
        else
            wget -qO- "$GO_URL" | tar -C /usr/local -xzf -
        fi
        export PATH=$PATH:/usr/local/go/bin
        echo 'export PATH=$PATH:/usr/local/go/bin' >> /etc/profile.d/go.sh
        info "Go ${GO_VERSION} 安装完成"
    fi

    # 检查 git
    if ! command -v git &> /dev/null; then
        info "安装 git..."
        if command -v apt-get &> /dev/null; then
            apt-get update -qq && apt-get install -y git
        elif command -v yum &> /dev/null; then
            yum install -y git
        else
            error "请手动安装 git"
        fi
        info "git 安装完成"
    fi

    # 克隆并构建
    BUILD_DIR="/tmp/server-probe-build"
    rm -rf "$BUILD_DIR"
    info "克隆代码仓库..."
    git clone --depth 1 "$REPO_URL" "$BUILD_DIR"

    info "构建 Agent 二进制..."
    cd "$BUILD_DIR/agent"
    CGO_ENABLED=0 go build -ldflags "-s -w" -o "$TMP_FILE" ./cmd/agent

    rm -rf "$BUILD_DIR"
    info "构建完成"
else
    # ========== 下载预编译二进制 ==========
    if [ -n "$VERSION" ]; then
        DOWNLOAD_BASE="https://github.com/YCJE/ServerProbe/releases/download/${VERSION}"
    fi

    AGENT_URL="${DOWNLOAD_BASE}/probe-agent-${OS}-${ARCH}"
    info "下载 Agent..."
    if command -v curl &> /dev/null; then
        curl -fsSL -o "$TMP_FILE" "$AGENT_URL" || error "下载失败: $AGENT_URL"
    else
        wget -qO "$TMP_FILE" "$AGENT_URL" || error "下载失败: $AGENT_URL"
    fi

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
insecure_tls: true
EOF

chmod 600 "${CONFIG_DIR}/config.yml"
chown probe:probe "${CONFIG_DIR}/config.yml"

# setcap (ICMP Ping)
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

# systemd service
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

sleep 2
if systemctl is-active --quiet "${SERVICE_NAME}"; then
    info "Agent 启动成功！"
else
    error "Agent 启动失败，请检查日志: journalctl -u ${SERVICE_NAME} -e"
fi

echo ""
info "安装完成！"
info "查看状态: systemctl status ${SERVICE_NAME}"
info "查看日志: journalctl -u ${SERVICE_NAME} -f"
echo ""
echo "卸载命令:"
echo "  curl -fsSL https://raw.githubusercontent.com/YCJE/ServerProbe/master/scripts/uninstall-agent.sh | bash"
