#!/bin/bash
# Server Probe Agent 一键卸载脚本
# 完全移除 Agent 端: 二进制、配置、setcap 权限、用户、systemd 服务
#
# 用法:
#   curl -fsSL https://raw.githubusercontent.com/YCJE/ServerProbe/master/scripts/uninstall-agent.sh | bash
#   或: bash uninstall-agent.sh

set -e

RED='\033[0;31m'
GREEN='\033[0;32m'
YELLOW='\033[1;33m'
NC='\033[0m'

info()  { echo -e "${GREEN}[INFO]${NC} $1"; }
warn()  { echo -e "${YELLOW}[WARN]${NC} $1"; }
error() { echo -e "${RED}[ERROR]${NC} $1"; exit 1; }

SERVICE_NAME="probe-agent"
INSTALL_DIR="/usr/local/bin"
CONFIG_DIR="/etc/probe-agent"
SERVICE_FILE="/etc/systemd/system/${SERVICE_NAME}.service"
SERVICE_SYMLINK="/etc/systemd/system/multi-user.target.wants/${SERVICE_NAME}.service"

while [[ $# -gt 0 ]]; do
    case $1 in
        --help)
            echo "用法: uninstall-agent.sh"
            echo ""
            echo "完全卸载 Server Probe Agent，包括:"
            echo "  - systemd 服务"
            echo "  - 二进制文件"
            echo "  - 配置目录"
            echo "  - setcap 权限"
            echo "  - 系统用户 probe"
            exit 0
            ;;
        *) error "未知参数: $1";;
    esac
done

if [ "$EUID" -ne 0 ]; then
    error "请以 root 用户运行此脚本"
fi

echo ""
echo "========================================"
echo -e "${YELLOW}  Server Probe Agent 卸载程序${NC}"
echo "========================================"
echo ""
echo "将删除以下内容:"
echo "  - systemd 服务: ${SERVICE_NAME}"
echo "  - 二进制文件:   ${INSTALL_DIR}/probe-agent"
echo "  - 配置目录:     ${CONFIG_DIR}"
echo "  - setcap 权限:  CAP_NET_RAW"
echo "  - 系统用户:     probe"
echo ""
# 从 /dev/tty 读取用户输入，兼容 curl | bash 管道方式
read -p "确认卸载? (y/N): " confirm < /dev/tty
if [ "$confirm" != "y" ] && [ "$confirm" != "Y" ]; then
    info "已取消卸载"
    exit 0
fi

echo ""

# 1. 停止并禁用 systemd 服务
info "停止 ${SERVICE_NAME} 服务..."
if systemctl is-active --quiet "${SERVICE_NAME}" 2>/dev/null; then
    systemctl stop "${SERVICE_NAME}"
    info "服务已停止"
else
    info "服务未运行"
fi

info "禁用 ${SERVICE_NAME} 服务..."
if systemctl is-enabled --quiet "${SERVICE_NAME}" 2>/dev/null; then
    systemctl disable "${SERVICE_NAME}" 2>/dev/null || true
    info "服务已禁用"
else
    info "服务未启用"
fi

# 2. 删除 systemd 服务文件
info "删除 systemd 服务文件..."
if [ -f "$SERVICE_FILE" ]; then
    rm -f "$SERVICE_FILE"
    info "已删除: $SERVICE_FILE"
fi
if [ -L "$SERVICE_SYMLINK" ]; then
    rm -f "$SERVICE_SYMLINK"
fi
systemctl daemon-reload 2>/dev/null || true
systemctl reset-failed "${SERVICE_NAME}" 2>/dev/null || true

# 3. 删除二进制文件 (先移除 setcap 权限)
info "移除 setcap 权限..."
if [ -f "${INSTALL_DIR}/probe-agent" ]; then
    setcap -r "${INSTALL_DIR}/probe-agent" 2>/dev/null || true
    info "已移除 CAP_NET_RAW 权限"
fi

info "删除二进制文件..."
if [ -f "${INSTALL_DIR}/probe-agent" ]; then
    rm -f "${INSTALL_DIR}/probe-agent"
    info "已删除: ${INSTALL_DIR}/probe-agent"
else
    info "二进制文件不存在，跳过"
fi

# 4. 删除配置目录
info "删除配置目录..."
if [ -d "$CONFIG_DIR" ]; then
    rm -rf "$CONFIG_DIR"
    info "已删除: $CONFIG_DIR"
else
    info "配置目录不存在，跳过"
fi

# 5. 删除系统用户和组
info "删除系统用户..."
if id probe &>/dev/null; then
    userdel -r probe 2>/dev/null || userdel probe 2>/dev/null || true
    info "已删除用户: probe"
else
    info "用户不存在，跳过"
fi
# 尝试删除组 (如果 userdel 没有自动删除)
if getent group probe &>/dev/null; then
    groupdel probe 2>/dev/null || true
fi

# 6. 清理临时构建文件
info "清理临时文件..."
rm -rf /tmp/server-probe-build 2>/dev/null || true
rm -f /tmp/probe-agent /tmp/probe-agent.sha256 2>/dev/null || true

# 7. 清理 Go 环境 (可选，仅当脚本安装的 Go 且无其他 Go 程序使用)
if [ -f /etc/profile.d/go.sh ] && [ ! -f "${INSTALL_DIR}/probe-server" ]; then
    info "检测到 Go 环境配置文件..."
    read -p "是否删除脚本安装的 Go 环境? (y/N): " del_go < /dev/tty
    if [ "$del_go" = "y" ] || [ "$del_go" = "Y" ]; then
        rm -rf /usr/local/go
        rm -f /etc/profile.d/go.sh
        info "已删除 Go 环境"
    else
        info "保留 Go 环境"
    fi
fi

echo ""
echo "========================================"
echo -e "${GREEN}  Server Probe Agent 卸载完成！${NC}"
echo "========================================"
echo ""
echo "Server Probe Agent 已从系统中完全移除"
echo ""
