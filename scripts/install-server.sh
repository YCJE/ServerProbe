#!/bin/bash
# Server Probe Server 安装脚本
# 支持两种模式:
#   1. 从源码构建 (默认,无需 Release)
#   2. 下载预编译二进制 (需要 GitHub Release)
#
# 用法:
#   从源码构建:  ./install-server.sh --port 8443
#   下载二进制:  ./install-server.sh --port 8443 --release

set -e

RED='\033[0;31m'
GREEN='\033[0;32m'
YELLOW='\033[1;33m'
NC='\033[0m'

info()  { echo -e "${GREEN}[INFO]${NC} $1"; }
warn()  { echo -e "${YELLOW}[WARN]${NC} $1"; }
error() { echo -e "${RED}[ERROR]${NC} $1"; exit 1; }

# 默认参数
PORT=8443
INSTALL_DIR="/usr/local/bin"
CONFIG_DIR="/etc/probe-server"
DATA_DIR="/var/lib/probe-server"
SERVICE_NAME="probe-server"
FROM_SOURCE=true
VERSION=""
REPO_URL="https://github.com/YCJE/ServerProbe.git"
DOWNLOAD_BASE="https://github.com/YCJE/ServerProbe/releases/latest/download"

while [[ $# -gt 0 ]]; do
    case $1 in
        --port) PORT="$2"; shift 2;;
        --data-dir) DATA_DIR="$2"; shift 2;;
        --version) VERSION="$2"; shift 2;;
        --release) FROM_SOURCE=false; shift;;
        --from-source) FROM_SOURCE=true; shift;;
        --help)
            echo "用法: install-server.sh [选项]"
            echo ""
            echo "选项:"
            echo "  --port <端口>       监听端口 (默认: 8443)"
            echo "  --data-dir <目录>   数据目录 (默认: /var/lib/probe-server)"
            echo "  --version <版本>    指定版本 (仅 --release 模式)"
            echo "  --release           从 GitHub Release 下载二进制"
            echo "  --from-source       从源码构建 (默认)"
            exit 0
            ;;
        *) error "未知参数: $1";;
    esac
done

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

TMP_FILE="/tmp/probe-server"

if [ "$FROM_SOURCE" = true ]; then
    # ========== 从源码构建 ==========
    info "从源码构建 Server..."

    # 检查依赖
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

    if ! command -v node &> /dev/null; then
        info "安装 Node.js..."
        if command -v apt-get &> /dev/null; then
            curl -fsSL https://deb.nodesource.com/setup_20.x | bash -
            apt-get install -y nodejs
        elif command -v yum &> /dev/null; then
            curl -fsSL https://rpm.nodesource.com/setup_20.x | bash -
            yum install -y nodejs
        else
            error "请手动安装 Node.js 20+"
        fi
        info "Node.js 安装完成"
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

    # 克隆仓库
    BUILD_DIR="/tmp/server-probe-build"
    rm -rf "$BUILD_DIR"
    info "克隆代码仓库..."
    git clone --depth 1 "$REPO_URL" "$BUILD_DIR"

    # 构建前端
    info "构建前端..."
    cd "$BUILD_DIR/server/frontend"
    npm install
    npm run build

    # 构建后端
    info "构建 Server 二进制..."
    cd "$BUILD_DIR/server"
    CGO_ENABLED=0 go build -ldflags "-s -w" -o "$TMP_FILE" ./cmd/server

    # 清理
    rm -rf "$BUILD_DIR"
    info "构建完成"
else
    # ========== 下载预编译二进制 ==========
    if [ -n "$VERSION" ]; then
        DOWNLOAD_BASE="https://github.com/YCJE/ServerProbe/releases/download/${VERSION}"
    fi

    SERVER_URL="${DOWNLOAD_BASE}/probe-server-${OS}-${ARCH}"
    info "下载 Server..."
    if command -v curl &> /dev/null; then
        curl -fsSL -o "$TMP_FILE" "$SERVER_URL" || error "下载失败: $SERVER_URL"
    else
        wget -qO "$TMP_FILE" "$SERVER_URL" || error "下载失败: $SERVER_URL"
    fi

    # 校验 SHA256
    SHA256_URL="${SERVER_URL}.sha256"
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
info "安装 Server..."
chmod +x "$TMP_FILE"
mv "$TMP_FILE" "${INSTALL_DIR}/probe-server"

# 创建用户
if ! id probe-server &>/dev/null; then
    info "创建 probe-server 用户..."
    useradd -r -s /usr/sbin/nologin -d "$DATA_DIR" probe-server
fi

# 创建目录
mkdir -p "$CONFIG_DIR" "$DATA_DIR"
chown probe-server:probe-server "$DATA_DIR"

# 生成 JWT 密钥
JWT_SECRET=$(head -c 32 /dev/urandom | base64 | tr -d '/+=' | head -c 32)
if [ -z "$JWT_SECRET" ]; then
    JWT_SECRET=$(cat /proc/sys/kernel/random/uuid | tr -d '-' | head -c 32)
fi

# 生成配置文件
info "生成配置文件..."
cat > "${CONFIG_DIR}/config.yml" << EOF
listen: ":${PORT}"
data_dir: "${DATA_DIR}"
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

chmod 600 "${CONFIG_DIR}/config.yml"
chown probe-server:probe-server "${CONFIG_DIR}/config.yml"

# 安装 systemd service
info "安装 systemd 服务..."
cat > "/etc/systemd/system/${SERVICE_NAME}.service" << EOF
[Unit]
Description=Server Probe Server
After=network.target
Wants=network-online.target

[Service]
Type=simple
User=probe-server
Group=probe-server
WorkingDirectory=${DATA_DIR}
ExecStart=${INSTALL_DIR}/probe-server --config ${CONFIG_DIR}/config.yml
Restart=always
RestartSec=5
NoNewPrivileges=true
ProtectSystem=strict
ProtectHome=true
PrivateTmp=true
ReadWritePaths=${DATA_DIR} ${CONFIG_DIR}
LimitNOFILE=65536

[Install]
WantedBy=multi-user.target
EOF

# 启动服务
info "启动 Server..."
systemctl daemon-reload
systemctl enable "${SERVICE_NAME}"
systemctl start "${SERVICE_NAME}"

sleep 2
if systemctl is-active --quiet "${SERVICE_NAME}"; then
    info "Server 启动成功！"
else
    error "Server 启动失败，请检查日志: journalctl -u ${SERVICE_NAME} -e"
fi

SERVER_IP=$(hostname -I 2>/dev/null | awk '{print $1}')
if [ -z "$SERVER_IP" ]; then
    SERVER_IP="localhost"
fi

echo ""
echo "========================================"
echo -e "${GREEN}Server Probe 安装完成！${NC}"
echo "========================================"
echo ""
echo "访问地址: https://${SERVER_IP}:${PORT}"
echo ""
echo "首次访问需要在浏览器中设置管理员账号"
echo ""
echo "配置文件: ${CONFIG_DIR}/config.yml"
echo "数据目录: ${DATA_DIR}"
echo "JWT 密钥: 已自动生成 (在配置文件中)"
echo ""
echo "常用命令:"
echo "  查看状态: systemctl status ${SERVICE_NAME}"
echo "  查看日志: journalctl -u ${SERVICE_NAME} -f"
echo "  重启服务: systemctl restart ${SERVICE_NAME}"
echo "  停止服务: systemctl stop ${SERVICE_NAME}"
echo ""
echo "卸载命令:"
echo "  curl -fsSL https://raw.githubusercontent.com/YCJE/ServerProbe/master/scripts/uninstall-server.sh | bash"
echo ""
echo -e "${YELLOW}注意: Server 使用自签名 TLS 证书, 浏览器会提示不安全连接, 请手动信任。${NC}"
echo -e "${YELLOW}如需使用域名和正式证书, 请参考 README.md 中的 Nginx 反向代理配置。${NC}"
echo ""
