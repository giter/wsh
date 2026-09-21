#!/usr/bin/env bash
# 构建并启动 wsh（Go 后端 + Web 前端，Wails v3 原生窗口）。
#
# 用法：
#   ./run.sh                # 构建并启动（弹出原生窗口）
#   ./run.sh --no-open      # 只起服务，不弹窗（headless 调试）
set -euo pipefail
cd "$(dirname "$0")"

echo ">> 构建 ..."
go build -o wsh .

echo ">> 启动 ..."
if [[ "${1:-}" == "--no-open" ]]; then
  ./wsh -no-open
else
  ./wsh
fi
