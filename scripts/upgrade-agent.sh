#!/bin/bash
# Server Probe Agent 升级脚本
# 用法: ./upgrade-agent.sh [--version <版本>]

set -e

# 颜色输出
RED='\033[0;31m'
GREEN='\033[0;32m'
YELLOW='\033[1;33m'
NC='\033[0m'

info()  { echo -e "${GREEN}[INFO]${NC} $1"; }
warn()  { echo -e "${YELLOW}[WARN]${NC} $1"; }
error() { echo -e "${RED}[ERROR]${NC} $1"; exit 1; }

# 参数
VERSION=""
INSTALL_DIR="/usr/local/bin"
CONFIG_DIR="/etc/probe-agent"
SERVICE_NAME="probe-agent"
BACKUP_DIR="/tmp/probe-agent-backup"

while [[ $# -gt 0 ]]; do
    case $1 in
        --version) VERSION="$2"; shift 2;;
        --help)
            echo "用法: upgrade-agent.sh [--version <版本>]"
            echo ""
            echo "选项:"
            echo "  --version    指定版本 (默认: latest)"
            exit 0
            ;;
        *) error "未知参数: $1";;
    esac
done

# 检查 root 权限
if [ "$EUID" -ne 0 ]; then
    error "请以 root 用户运行此脚本"
fi

# 检查 Agent 是否已安装
if [ ! -f "${INSTALL_DIR}/probe-agent" ]; then
    error "Agent 未安装, 请先运行 install-agent.sh"
fi

# 检查配置文件
if [ ! -f "${CONFIG_DIR}/config.yml" ]; then
    error "配置文件不存在: ${CONFIG_DIR}/config.yml"
fi

# 读取当前版本
CURRENT_VERSION=$("${INSTALL_DIR}/probe-agent" --version 2>/dev/null || echo "unknown")
info "当前版本: ${CURRENT_VERSION}"

# 读取 Server URL
SERVER_URL=$(grep '^server:' "${CONFIG_DIR}/config.yml" | awk '{print $2}' | tr -d '"')
if [ -z "$SERVER_URL" ]; then
    error "无法从配置文件读取 server URL"
fi

info "Server URL: ${SERVER_URL}"

# 检测系统
OS=$(uname -s | tr '[:upper:]' '[:lower:]')
ARCH=$(uname -m)

case $ARCH in
    x86_64)  ARCH="amd64";;
    aarch64) ARCH="arm64";;
    armv7l)  ARCH="armv7";;
    *) error "不支持的架构: $ARCH";;
esac

# 确定下载 URL
if [ -n "$VERSION" ]; then
    AGENT_URL="${SERVER_URL}/download/agent/${OS}/${ARCH}?version=${VERSION}"
else
    AGENT_URL="${SERVER_URL}/download/agent/${OS}/${ARCH}"
fi

TMP_FILE="/tmp/probe-agent-new"

# 下载新版本
info "下载新版本 Agent..."
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

# 备份当前版本
info "备份当前版本..."
cp "${INSTALL_DIR}/probe-agent" "${BACKUP_DIR}"
chmod +x "${BACKUP_DIR}"

# 停止服务
info "停止 Agent 服务..."
systemctl stop "${SERVICE_NAME}" || warn "服务未运行"

# 安装新版本
info "安装新版本..."
chmod +x "$TMP_FILE"
mv "$TMP_FILE" "${INSTALL_DIR}/probe-agent"

# 重新设置 setcap
if command -v setcap &> /dev/null; then
    setcap cap_net_raw+ep "${INSTALL_DIR}/probe-agent" 2>/dev/null || true
fi

# 启动服务
info "启动 Agent 服务..."
systemctl start "${SERVICE_NAME}"

# 等待启动
sleep 2
if systemctl is-active --quiet "${SERVICE_NAME}"; then
    NEW_VERSION=$("${INSTALL_DIR}/probe-agent" --version 2>/dev/null || echo "unknown")
    info "升级成功！"
    info "版本: ${CURRENT_VERSION} -> ${NEW_VERSION}"
    rm -f "${BACKUP_DIR}"
else
    error "启动失败, 正在回滚..."
    cp "${BACKUP_DIR}" "${INSTALL_DIR}/probe-agent"
    chmod +x "${INSTALL_DIR}/probe-agent"
    if command -v setcap &> /dev/null; then
        setcap cap_net_raw+ep "${INSTALL_DIR}/probe-agent" 2>/dev/null || true
    fi
    systemctl start "${SERVICE_NAME}"
    error "升级失败, 已回滚到旧版本"
fi

info "查看状态: systemctl status ${SERVICE_NAME}"
