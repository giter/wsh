#!/usr/bin/env bash
# 构建 wsh 桌面程序。
#
# 用法：
#   ./build.sh             # 默认构建 Windows 版 wsh.exe（交叉编译）
#   ./build.sh windows     # 同上
#   ./build.sh darwin      # 打包 macOS 版 wsh.app（必须在 macOS 上运行）
#   ./build.sh dmg         # 在 darwin 的基础上再打一个 wsh.dmg 发行镜像（必须在 macOS 上运行）
#
# 依赖：
#   前端（两者都需要）：bun，见 frontend/
#   Windows 交叉编译（Debian/Ubuntu）：
#     sudo apt install gcc-mingw-w64-x86-64
#    macOS 打包：Xcode Command Line Tools（clang / sips / iconutil）+ 系统自带 hdiutil
set -euo pipefail
cd "$(dirname "$0")"

TARGET="${1:-windows}"

# 1) 前端：构建到 web/，随后被 go:embed 打进二进制。
build_frontend() {
  if command -v bun >/dev/null 2>&1; then
    echo ">> 构建前端 (bun) ..."
    (cd frontend && bun install --frozen-lockfile && bun run build)
  else
    echo "警告：未找到 bun，跳过前端构建，直接使用 web/ 中已有的产物" >&2
    echo "      安装：https://bun.sh" >&2
  fi
}

# 交叉编译 Windows amd64（Wails v3 需要 CGO，需安装 mingw-w64 交叉工具链）。
build_windows() {
  OUT="wsh.exe"

  if ! command -v x86_64-w64-mingw32-gcc >/dev/null 2>&1; then
    echo "错误：未找到 x86_64-w64-mingw32-gcc，请先安装 mingw-w64 交叉工具链：" >&2
    echo "  Debian/Ubuntu: sudo apt install gcc-mingw-w64-x86-64" >&2
    exit 1
  fi

  # Windows 资源（应用图标）：编译成 .syso，go build 会自动链接。
  if command -v x86_64-w64-mingw32-windres >/dev/null 2>&1; then
    echo ">> 编译 Windows 资源 (图标) ..."
    x86_64-w64-mingw32-windres -O coff -o rsrc_windows_amd64.syso wsh.rc
  else
    echo "警告：未找到 x86_64-w64-mingw32-windres，跳过图标资源编译" >&2
  fi

  echo ">> 交叉编译 Windows amd64 (CGO + mingw) ..."
  CGO_ENABLED=0 GOOS=windows GOARCH=amd64 \
    go build -trimpath -ldflags "-H windowsgui" -o "$OUT" .

  echo ">> 完成：$OUT"
}

# 由 build/icon.png 生成 macOS 的 .icns（sips 缩放 + iconutil 打包）。
build_icns() {
  local src="build/icon.png" dest="$1"
  [ -f "$src" ] || return 1
  command -v sips >/dev/null 2>&1 || return 1
  command -v iconutil >/dev/null 2>&1 || return 1

  local work iconset entry size
  work="$(mktemp -d)"
  iconset="$work/icon.iconset"
  mkdir -p "$iconset"

  # iconutil 要求的文件名 → 像素尺寸（@2x 为两倍像素）。
  local spec="icon_16x16.png:16 icon_16x16@2x.png:32 \
icon_32x32.png:32 icon_32x32@2x.png:64 \
icon_128x128.png:128 icon_128x128@2x.png:256 \
icon_256x256.png:256 icon_256x256@2x.png:512 \
icon_512x512.png:512 icon_512x512@2x.png:1024"

  local status=0
  for entry in $spec; do
    size="${entry##*:}"
    sips -z "$size" "$size" "$src" --out "$iconset/${entry%%:*}" >/dev/null \
      || status=1
  done

  iconutil -c icns "$iconset" -o "$dest" || status=1

  # 必须先记录成败再清理：函数返回值取自最后一条命令，
  # 若让 rm -rf 收尾，iconutil 的失败会被它的成功状态掩盖掉。
  rm -rf "$work"
  return "$status"
}

# 打包 macOS .app：CGO 链接 Cocoa / WebKit，无法从其它平台交叉编译，
# 因此这一步只能在 macOS 上执行。
build_darwin() {
  local app="wsh.app"
  local bin="wsh"
  local bundle_id="com.wsh.app"
  local version="1.0.0"
  # 默认按本机架构构建（Apple Silicon 为 arm64，Intel 为 amd64），
  # 可用 GOARCH=... ./build.sh darwin 覆盖。
  local arch="${GOARCH:-$(go env GOARCH)}"

  echo ">> 编译 macOS 可执行文件 (CGO, $arch) ..."
  CGO_ENABLED=1 GOOS=darwin GOARCH="$arch" \
    go build -trimpath -ldflags "-s -w" -o "$bin" .

  echo ">> 组装 $app ..."
  rm -rf "$app"
  mkdir -p "$app/Contents/MacOS" "$app/Contents/Resources"
  cp "$bin" "$app/Contents/MacOS/$bin"

  # 应用图标：由 build/icon.png 生成 .icns；工具缺失时退化为系统默认图标。
  local icon_file=""
  if build_icns "$app/Contents/Resources/icon.icns"; then
    icon_file="icon.icns"
    echo "   已生成应用图标 icon.icns"
  else
    echo "警告：缺少 sips / iconutil 或 build/icon.png，使用系统默认图标" >&2
  fi

  # Info.plist：可执行文件名、图标、最低系统版本等，macOS 启动 .app 必需。
  cat > "$app/Contents/Info.plist" <<PLIST
<?xml version="1.0" encoding="UTF-8"?>
<!DOCTYPE plist PUBLIC "-//Apple//DTD PLIST 1.0//EN" "http://www.apple.com/DTDs/PropertyList-1.0.dtd">
<plist version="1.0">
<dict>
	<key>CFBundleName</key>
	<string>wsh</string>
	<key>CFBundleDisplayName</key>
	<string>wsh</string>
	<key>CFBundleIdentifier</key>
	<string>$bundle_id</string>
	<key>CFBundleExecutable</key>
	<string>$bin</string>
	<key>CFBundleIconFile</key>
	<string>$icon_file</string>
	<key>CFBundlePackageType</key>
	<string>APPL</string>
	<key>CFBundleVersion</key>
	<string>$version</string>
	<key>CFBundleShortVersionString</key>
	<string>$version</string>
	<key>LSMinimumSystemVersion</key>
	<string>11.0</string>
	<key>NSHighResolutionCapable</key>
	<true/>
	<key>NSPrincipalClass</key>
	<string>NSApplication</string>
</dict>
</plist>
PLIST

  # Apple Silicon 上运行二进制至少需要 ad-hoc 签名；顺手清掉隔离属性，
  # 让本地构建的 .app 可以直接双击打开。
  if command -v codesign >/dev/null 2>&1; then
    codesign --force --deep --sign - "$app" >/dev/null 2>&1 \
      || echo "警告：ad-hoc 签名失败，未签名时首次打开需右键 →「打开」" >&2
  fi
  xattr -cr "$app" 2>/dev/null || true

  echo ">> 完成：${app}（双击运行，或 open ${app}）"
}

# 打包 macOS 发行镜像 wsh.dmg：镜像内是 wsh.app 加一个指向 /Applications 的
# 快捷方式，双击挂载后把应用拖进 Applications 即可安装。
build_dmg() {
  build_darwin

  local app="wsh.app" out="wsh.dmg" volname="wsh"
  local work
  work="$(mktemp -d)"

  echo ">> 组装 $out ..."
  # ditto 会连同扩展属性一起复制，保证 ad-hoc 签名在拷贝后依然有效。
  if command -v ditto >/dev/null 2>&1; then
    ditto "$app" "$work/$app"
  else
    cp -R "$app" "$work/$app"
  fi
  ln -s /Applications "$work/Applications"

  # UDZO = 压缩只读镜像；-ov 允许覆盖旧文件。
  if ! hdiutil create -volname "$volname" -srcfolder "$work" -ov \
    -format UDZO "$out" >/dev/null; then
    rm -rf "$work"
    echo "错误：hdiutil 创建 $out 失败" >&2
    return 1
  fi
  rm -rf "$work"

  echo ">> 完成：${out}（双击挂载，把 ${app} 拖入 Applications）"
}

case "$TARGET" in
  windows) ;;
  darwin|dmg)
    # 提前报错，避免在非 macOS 上白白跑一遍前端构建。
    if [ "$(uname -s)" != "Darwin" ]; then
      echo "错误：$TARGET 打包需要 CGO 链接 Cocoa / WebKit，只能在 macOS 上运行。" >&2
      echo "      请在 macOS 上执行 ./build.sh $TARGET" >&2
      exit 1
    fi
    ;;
  *)
    echo "用法：$0 [windows|darwin|dmg]" >&2
    exit 1
    ;;
esac

build_frontend
"build_$TARGET"
