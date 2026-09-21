#!/usr/bin/env bash
# 交叉编译 Windows 版（Wails v3 需要 CGO，需安装 mingw-w64 交叉工具链）。
#
# 用法：
#   ./build.sh            # 构建前端 + 生成 wsh.exe
#
# 依赖（Debian/Ubuntu）：
#   sudo apt install gcc-mingw-w64-x86-64
#   bun（前端构建，见 frontend/）
set -euo pipefail
cd "$(dirname "$0")"

OUT="wsh.exe"

# 1) 前端：构建到 web/，随后被 go:embed 打进二进制。
if command -v bun >/dev/null 2>&1; then
  echo ">> 构建前端 (bun) ..."
  (cd frontend && bun install --frozen-lockfile && bun run build)
else
  echo "警告：未找到 bun，跳过前端构建，直接使用 web/ 中已有的产物" >&2
  echo "      安装：https://bun.sh" >&2
fi

# 2) 后端：交叉编译 Windows amd64。
if ! command -v x86_64-w64-mingw32-gcc >/dev/null 2>&1; then
  echo "错误：未找到 x86_64-w64-mingw32-gcc，请先安装 mingw-w64 交叉工具链：" >&2
  echo "  Debian/Ubuntu: sudo apt install gcc-mingw-w64-x86-64" >&2
  exit 1
fi

# 2.5) Windows 资源（应用图标）：编译成 .syso，go build 会自动链接。
if command -v x86_64-w64-mingw32-windres >/dev/null 2>&1; then
  echo ">> 编译 Windows 资源 (图标) ..."
  x86_64-w64-mingw32-windres -O coff -o rsrc_windows_amd64.syso wsh.rc
else
  echo "警告：未找到 x86_64-w64-mingw32-windres，跳过图标资源编译" >&2
fi

echo ">> 交叉编译 Windows amd64 (CGO + mingw) ..."
CGO_ENABLED=1 GOOS=windows GOARCH=amd64 \
  CC=x86_64-w64-mingw32-gcc \
  go build -trimpath -ldflags "-H windowsgui" -o "$OUT" .

echo ">> 完成：$OUT"
