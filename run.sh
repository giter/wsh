#!/usr/bin/env bash
# 构建并启动 wsh（Go 后端 + Web 前端，Wails v3 原生窗口）。
#
# 用法：
#   ./run.sh                # 构建并启动（弹出原生窗口）
#   ./run.sh --no-open      # 只起服务，不弹窗（headless 调试）
set -euo pipefail
cd "$(dirname "$0")"

# 前端：构建到 web/，随后被 go:embed 打进二进制。
if command -v bun >/dev/null 2>&1; then
  echo ">> 构建前端 (bun) ..."
  (cd frontend && bun install --frozen-lockfile && bun run build)
else
  echo "警告：未找到 bun，跳过前端构建，直接使用 web/ 中已有的产物" >&2
fi

echo ">> 构建后端 ..."
go build -o wsh .

echo ">> 启动 ..."
if [[ "${1:-}" == "--no-open" ]]; then
  ./wsh -no-open
else
  ./wsh
fi
