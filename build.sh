#!/usr/bin/env bash
# 交叉编译 Windows 版（Wails v3 需要 CGO，需安装 mingw-w64 交叉工具链）。
#
# 用法：
#   ./build_windows.sh            # 生成 FyneShell.exe
#
# 依赖（Debian/Ubuntu）：
#   sudo apt install gcc-mingw-w64-x86-64
set -euo pipefail
cd "$(dirname "$0")"

OUT="FyneShell.exe"

if ! command -v x86_64-w64-mingw32-gcc >/dev/null 2>&1; then
  echo "错误：未找到 x86_64-w64-mingw32-gcc，请先安装 mingw-w64 交叉工具链：" >&2
  echo "  Debian/Ubuntu: sudo apt install gcc-mingw-w64-x86-64" >&2
  exit 1
fi

echo ">> 交叉编译 Windows amd64 (CGO + mingw) ..."
CGO_ENABLED=1 GOOS=windows GOARCH=amd64 \
  CC=x86_64-w64-mingw32-gcc \
  go build -trimpath -ldflags "-H windowsgui" -o "$OUT" .

echo ">> 完成：$OUT"
