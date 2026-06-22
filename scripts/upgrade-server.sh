#!/bin/bash
# Server Probe Server 升级脚本
# 仅更新二进制文件，保留配置、数据、用户、服务
#
# 用法:
#   curl -fsSL https://raw.githubusercontent.com/YCJE/ServerProbe/master/scripts/upgrade-server.sh | bash
#   或: bash upgrade-server.sh [--release]

set -e

RED='\033[0;31m'
GREEN='\033[0;32m'
YELLOW='\033[1;33m'
NC='\033[0m'

info()  { echo -e "${GREEN}[INFO]${NC} $1"; }
warn()  { echo -e "${YELLOW}[WARN]${NC} $1"; }
error() { echo -e "${RED}[ERROR]${NC} $1"; exit 1; }

FROM_SOURCE=true
VERSION=""
REPO_URL="https://github.com/YCJE/ServerProbe.git"
DOWNLOAD_BASE="https://github.com/YCJE/ServerProbe/releases/latest/download"
INSTALL_DIR="/usr/local/bin"
SERVICE_NAME="probe-server"
TMP_FILE="/tmp/probe-server-upgrade"

while [[ $# -gt 0 ]]; do
    case $1 in
        --release) FROM_SOURCE=false; shift;;
        --from-source) FROM_SOURCE=true; shift;;
        --version) VERSION="$2"; shift 2;;
        --help)
            echo "用法: upgrade-server.sh [选项]"
            echo ""
            echo "仅更新二进制文件，保留所有配置和数据"
            echo ""
            echo "选项:"
            echo "  --release         从 GitHub Release 下载二进制"
            echo "  --from-source     从源码构建 (默认)"
            echo "  --version <版本>  指定版本 (仅 --release 模式)"
            exit 0
            ;;
        *) error "未知参数: $1";;
    esac
done

if [ "$EUID" -ne 0 ]; then
    error "请以 root 用户运行此脚本"
fi

# 检查是否已安装
if [ ! -f "${INSTALL_DIR}/probe-server" ]; then
    error "Server 未安装，请先运行安装脚本"
fi

if ! systemctl list-unit-files | grep -q "${SERVICE_NAME}"; then
    error "未找到 ${SERVICE_NAME} 服务，请先运行安装脚本"
fi

echo ""
echo "========================================"
echo -e "${YELLOW}  Server Probe Server 升级程序${NC}"
echo "========================================"
echo ""

# 检测系统
OS=$(uname -s | tr '[:upper:]' '[:lower:]')
ARCH=$(uname -m)
case $ARCH in
    x86_64)  ARCH="amd64";;
    aarch64) ARCH="arm64";;
    armv7l)  ARCH="armv7";;
    *) error "不支持的架构: $ARCH";;
esac

info "系统: $OS/$ARCH"

# 备份当前二进制
info "备份当前二进制..."
cp "${INSTALL_DIR}/probe-server" "${INSTALL_DIR}/probe-server.bak"
info "已备份到: ${INSTALL_DIR}/probe-server.bak"

# 构建或下载新二进制
if [ "$FROM_SOURCE" = true ]; then
    info "从源码构建 Server..."

    # 确保 Go 可用
    if ! command -v go &> /dev/null; then
        if [ -f /usr/local/go/bin/go ]; then
            export PATH=$PATH:/usr/local/go/bin
        else
            error "Go 未安装，请使用 --release 模式或先安装 Go"
        fi
    fi

    # 确保 Node.js 可用
    if ! command -v node &> /dev/null; then
        error "Node.js 未安装，请使用 --release 模式或先安装 Node.js"
    fi

    # 确保 git 可用
    if ! command -v git &> /dev/null; then
        error "git 未安装，请使用 --release 模式或先安装 git"
    fi

    BUILD_DIR="/tmp/server-probe-build"
    rm -rf "$BUILD_DIR"
    info "克隆代码仓库..."
    git clone --depth 1 "$REPO_URL" "$BUILD_DIR"

    info "构建前端..."
    cd "$BUILD_DIR/server/frontend"
    npm install --silent 2>/dev/null
    npm run build 2>/dev/null

    info "构建 Server 二进制..."
    cd "$BUILD_DIR/server"
    CGO_ENABLED=0 go build -ldflags "-s -w" -o "$TMP_FILE" ./cmd/server

    rm -rf "$BUILD_DIR"
    info "构建完成"
else
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
fi

# 替换二进制
info "更新二进制文件..."
chmod +x "$TMP_FILE"
mv "$TMP_FILE" "${INSTALL_DIR}/probe-server"

# 确保 probe-server 用户有执行权限
chown probe-server:probe-server "${INSTALL_DIR}/probe-server" 2>/dev/null || true

# 重启服务
info "重启服务..."
systemctl restart "${SERVICE_NAME}"

sleep 2
if systemctl is-active --quiet "${SERVICE_NAME}"; then
    info "升级成功！服务已重启"
else
    warn "服务启动失败，正在回滚..."
    cp "${INSTALL_DIR}/probe-server.bak" "${INSTALL_DIR}/probe-server"
    systemctl restart "${SERVICE_NAME}"
    sleep 2
    if systemctl is-active --quiet "${SERVICE_NAME}"; then
        info "已回滚到旧版本"
    else
        error "回滚失败，请检查日志: journalctl -u ${SERVICE_NAME} -e"
    fi
    error "升级失败，已回滚"
fi

# 清理备份
rm -f "${INSTALL_DIR}/probe-server.bak"

echo ""
echo "========================================"
echo -e "${GREEN}  Server Probe Server 升级完成！${NC}"
echo "========================================"
echo ""
echo "配置和数据已保留，无需重新设置"
echo ""
echo "查看状态: systemctl status ${SERVICE_NAME}"
echo "查看日志: journalctl -u ${SERVICE_NAME} -f"
echo ""
