#!/usr/bin/env bash
# 前端开发模式：Go 后端（固定端口）+ Vite 开发服务器（HMR）。
#
# 用法：
#   ./dev.sh
#
# 然后在浏览器打开 http://localhost:5173 调试界面；Vite 会把 /ws 代理到
# 下面这个固定端口的 Go 后端。修改前端代码即时热更新，无需重新编译 Go。
set -euo pipefail
cd "$(dirname "$0")"

PORT="${PORT:-17777}"

echo ">> 启动后端（127.0.0.1:${PORT}，无窗口）..."
go run . -no-open -port "$PORT" &
BACK_PID=$!
trap 'kill "$BACK_PID" 2>/dev/null || true' EXIT INT TERM

echo ">> 启动前端开发服务器 (http://localhost:5173) ..."
cd frontend
bun install --frozen-lockfile
bun run dev
