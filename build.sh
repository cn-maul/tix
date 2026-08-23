#!/usr/bin/env bash
# ============================================
#   tix 工单系统 - Linux 编译
#   Usage:
#     ./build.sh                  # 默认: 构建前端 + 编译当前平台
#     ./build.sh linux amd64      # 交叉编译到 linux/amd64
#     ./build.sh windows amd64    # 交叉编译到 windows/amd64
# ============================================
set -euo pipefail

cd "$(dirname "$0")"

BINARY="tix"
GOOS="${1:-}"
GOARCH="${2:-}"

# 如果指定了目标平台，拼入文件名
if [ -n "$GOOS" ] && [ -n "$GOARCH" ]; then
    BINARY="tix-${GOOS}-${GOARCH}"
    if [ "$GOOS" = "windows" ]; then
        BINARY="${BINARY}.exe"
    fi
    export GOOS GOARCH
    echo "  目标平台: ${GOOS}/${GOARCH}"
fi

echo "============================================"
echo "  tix 工单系统 - 编译"
echo "============================================"
echo ""

# ---- 确保 pnpm 可用（nvm 环境脚本内不自动加载）----
# pnpm 11 默认会在运行脚本前检查依赖状态并触发子进程 install（无 TTY 时可能失败），
# 构建流程本身已保证依赖完整，这里显式关闭该校验。
export pnpm_config_verify_deps_before_run=false
if ! command -v pnpm >/dev/null 2>&1; then
    export NVM_DIR="${NVM_DIR:-$HOME/.config/nvm}"
    if [ -s "$NVM_DIR/nvm.sh" ]; then
        # shellcheck disable=SC1091
        . "$NVM_DIR/nvm.sh" >/dev/null 2>&1
    fi
fi
if ! command -v pnpm >/dev/null 2>&1; then
    echo "[失败] 未找到 pnpm，请先安装：npm install -g pnpm"
    exit 1
fi

# ---- [1/2] 构建前端 ----
echo "[1/2] 构建前端 ..."
if [ ! -d "web/node_modules/vite" ]; then
    echo "    安装依赖 ..."
    pnpm --dir web install --ignore-scripts --force
fi
pnpm --dir web build
echo "    前端构建完成"
echo ""

# ---- [2/2] 编译 Go 二进制 ----
echo "[2/2] 编译 Go 二进制 ..."
CGO_ENABLED=0 go build -ldflags="-s -w" -o "$BINARY" .
echo ""

echo "[成功] 已生成 ${BINARY}"
echo "       $(ls -lh "$BINARY" | awk '{print $5}')  单文件，前端已嵌入，纯静态，零依赖"
echo ""
echo "启动方式："
echo "  ./${BINARY}                     # 默认 :8881"
echo "  ./${BINARY} -addr :8888 -db /path/to/tix.db"
echo "  PORT=8888 ./${BINARY}"
echo ""
echo "开发模式（前后端分离，热更新）："
echo "  pnpm --dir web dev  # Vite :5173 (代理 /api 到 :8881)"
echo "  go run .            # Go 后端 :8881"