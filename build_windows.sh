#!/usr/bin/env bash
# 交叉编译 FyneShell 的 Windows 版本。
#
# 前置条件（Debian/Ubuntu）：
#   sudo apt-get install gcc-mingw-w64-x86-64
#
# 用法：
#   ./build_windows.sh            # 生成 FyneShell.exe
set -euo pipefail

cd "$(dirname "$0")"

OUT="FyneShell.exe"

echo ">> 交叉编译 Windows amd64 ..."
CGO_ENABLED=1 \
GOOS=windows \
GOARCH=amd64 \
CC=x86_64-w64-mingw32-gcc \
go build -trimpath -ldflags "-H windowsgui" -o "$OUT" .

echo ">> 完成：$OUT"
file "$OUT" 2>/dev/null || true
