#!/bin/bash
# Server Probe Server 一键卸载脚本
# 完全移除 Server 端: 二进制、配置、数据、用户、systemd 服务
#
# 用法:
#   curl -fsSL https://raw.githubusercontent.com/YCJE/ServerProbe/master/scripts/uninstall-server.sh | bash
#   或: bash uninstall-server.sh [--keep-data]

set -e

RED='\033[0;31m'
GREEN='\033[0;32m'
YELLOW='\033[1;33m'
NC='\033[0m'

info()  { echo -e "${GREEN}[INFO]${NC} $1"; }
warn()  { echo -e "${YELLOW}[WARN]${NC} $1"; }
error() { echo -e "${RED}[ERROR]${NC} $1"; exit 1; }

KEEP_DATA=false
SERVICE_NAME="probe-server"
INSTALL_DIR="/usr/local/bin"
CONFIG_DIR="/etc/probe-server"
DATA_DIR="/var/lib/probe-server"
SERVICE_FILE="/etc/systemd/system/${SERVICE_NAME}.service"
SERVICE_SYMLINK="/etc/systemd/system/multi-user.target.wants/${SERVICE_NAME}.service"

while [[ $# -gt 0 ]]; do
    case $1 in
        --keep-data) KEEP_DATA=true; shift;;
        --help)
            echo "用法: uninstall-server.sh [选项]"
            echo ""
            echo "选项:"
            echo "  --keep-data   保留数据目录 (默认: 删除所有数据)"
            echo "  --help        显示帮助"
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
echo -e "${YELLOW}  Server Probe Server 卸载程序${NC}"
echo "========================================"
echo ""

if [ "$KEEP_DATA" = true ]; then
    echo "模式: 保留数据目录"
else
    echo "模式: 完全删除 (包括所有数据)"
fi
echo ""
echo "将删除以下内容:"
echo "  - systemd 服务: ${SERVICE_NAME}"
echo "  - 二进制文件:   ${INSTALL_DIR}/probe-server"
echo "  - 配置目录:     ${CONFIG_DIR}"
if [ "$KEEP_DATA" = false ]; then
    echo "  - 数据目录:     ${DATA_DIR}"
    echo "  - 系统用户:     probe-server"
fi
echo ""
read -p "确认卸载? (y/N): " confirm
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

# 3. 删除二进制文件
info "删除二进制文件..."
if [ -f "${INSTALL_DIR}/probe-server" ]; then
    rm -f "${INSTALL_DIR}/probe-server"
    info "已删除: ${INSTALL_DIR}/probe-server"
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

# 5. 删除数据目录 (除非 --keep-data)
if [ "$KEEP_DATA" = false ]; then
    info "删除数据目录..."
    if [ -d "$DATA_DIR" ]; then
        rm -rf "$DATA_DIR"
        info "已删除: $DATA_DIR"
    else
        info "数据目录不存在，跳过"
    fi
else
    info "保留数据目录: $DATA_DIR"
fi

# 6. 删除系统用户和组
if [ "$KEEP_DATA" = false ]; then
    info "删除系统用户..."
    if id probe-server &>/dev/null; then
        userdel -r probe-server 2>/dev/null || userdel probe-server 2>/dev/null || true
        info "已删除用户: probe-server"
    else
        info "用户不存在，跳过"
    fi
    # 尝试删除组 (如果 userdel 没有自动删除)
    if getent group probe-server &>/dev/null; then
        groupdel probe-server 2>/dev/null || true
    fi
else
    info "保留用户: probe-server"
fi

# 7. 清理临时构建文件
info "清理临时文件..."
rm -rf /tmp/server-probe-build 2>/dev/null || true
rm -f /tmp/probe-server /tmp/probe-server.sha256 2>/dev/null || true

# 8. 清理 Go 环境 (可选，仅当脚本安装的 Go 且无其他 Go 程序使用)
if [ -f /etc/profile.d/go.sh ] && [ ! -f "${INSTALL_DIR}/probe-agent" ]; then
    info "检测到 Go 环境配置文件..."
    read -p "是否删除脚本安装的 Go 环境? (y/N): " del_go
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
echo -e "${GREEN}  Server Probe Server 卸载完成！${NC}"
echo "========================================"
echo ""
if [ "$KEEP_DATA" = true ]; then
    echo "数据目录已保留: $DATA_DIR"
    echo "如需彻底删除，请手动执行: rm -rf $DATA_DIR"
    echo ""
fi
echo "Server Probe Server 已从系统中完全移除"
echo ""
