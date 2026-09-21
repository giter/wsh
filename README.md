# wsh

基于 Go + Web 的跨平台 SSH 客户端。后端用 Go 实现，界面是内嵌的 Web 前端
（ZED 风格：左侧连接树 + 顶部多标签页），终端用 xterm.js 渲染，
由 Wails v3 封装为原生桌面窗口。

## 功能

- **连接管理** — 保存多台服务器，一键新建 / 编辑 / 删除 / 测试连接
- **密钥管理** — 提交并保存用户私钥（加密落盘），显示对应公钥与指纹；连接可引用密钥登录
- **优先密钥登录** — 配置了密钥的连接登录时先用密钥认证，失败再回退到密码
- **密码保存** — 密码使用本机密钥 AES-GCM 加密后落盘，配置被拷走也无法解出明文
- **多窗口配置** — 「选项」「密钥管理」在独立的窗口中打开，与主窗口各司其职
- **自绘窗口** — Windows/Linux 下窗口无系统边框，标题栏（品牌 / 菜单 / 窗口按钮）由
  前端绘制，整体风格与界面一致；macOS 保留系统边框与全局菜单
- **终端** — 多标签页 xterm.js 终端，随窗口自动调整 PTY 大小；连接失败时弹窗输入密码重试
- **文件传输** — 双栏 SFTP 浏览器，支持上传 / 下载文件
- **隧道管理** — 本地端口转发到远程地址，可启动 / 停止

## 架构

```
Wails v3 桌面窗口 (跨平台 WebView 容器，窗口/托盘/菜单等桌面能力)
  └─ Go 后端 (复用 SSH / SFTP / 隧道 / 存储逻辑)
       └─ 本地 HTTP + WebSocket 服务 (仅绑定 127.0.0.1)
            └─ Web 前端 (go:embed 进单二进制，离线可跑)
                 ├─ 左侧连接树
                 ├─ 顶部多标签
                 └─ xterm.js 终端 / SFTP 双栏 / 隧道管理
```

前后端通过 WebSocket 交换 JSON 消息（RPC），终端输出以流式推送，键盘输入走请求。
前端为原生 HTML/CSS/JS，**零 npm 依赖**，静态资源打包进可执行文件；
Wails 只充当窗口外壳，加载本地服务的 URL，业务逻辑与协议完全不变。

## 构建

Wails v3 需要 **Go 1.25+** 与平台 WebView 运行时（不再是纯 Go / 无 CGO）：

- **Windows** — 自带 WebView2 Runtime（Win10/11 已内置）；交叉编译需 mingw-w64
- **Linux** — 需 `libwebkit2gtk-4.1-dev`、`libgtk-3-dev`、`build-essential`、`pkg-config`
- **macOS** — 需 Xcode Command Line Tools（自带 WKWebView）

```bash
# Linux (Debian/Ubuntu 先装依赖)
sudo apt install libgtk-3-dev libwebkit2gtk-4.1-dev build-essential pkg-config

# 构建并启动（弹出原生窗口）
./run.sh

# 只构建二进制
go build -o wsh .
```

### Windows（.exe）

Windows 版依赖 WebView2 Runtime（Win10/11 自带），交叉编译需要 mingw 工具链：

```bash
sudo apt install gcc-mingw-w64-x86-64   # 交叉工具链
./build_windows.sh                      # 生成 wsh.exe
```

> 生成的 `wsh.exe` 在 Windows 上启动后直接显示原生窗口，
> 无需打开浏览器。

## 使用

启动后程序在 `127.0.0.1` 随机端口起一个本地服务，并在 Wails 原生窗口
中加载前端界面（`-no-open` 可只起服务不弹窗，便于调试）。
配置数据保存在系统配置目录下 `wsh/config.json`（密码与私钥均为加密后的密文），
机器密钥保存在同目录 `secret.key`。

菜单栏（自绘，位于标题栏内）：

- **文件** — 新建连接（`Ctrl/Cmd+N`）、连接管理（`Ctrl/Cmd+M`）、退出（`Ctrl/Cmd+Q`）
- **选项** — 选项（`Ctrl/Cmd+Shift+,`）、密钥管理（`Ctrl/Cmd+Shift+K`），均在独立窗口中打开
- **视图** — 连接 / 文件传输 / 端口隧道 快速切换（`Ctrl/Cmd+1/2/3`）
- **帮助** — 关于 wsh、开发者工具（DevTools）

> Windows/Linux 下窗口是无边框的（frameless），系统标题栏和菜单栏都不再使用，
> 由前端绘制一整套标题栏（品牌 + 菜单 + 最小化/最大化/关闭），见 `web/chrome.js`。
> 窗口拖动/边缘缩放通过 Wails 的 `wails:drag` / `wails:resize:<edge>` 消息实现，
> 窗口按钮、打开配置窗口、退出、DevTools 通过 RPC（`window.control` / `app.action`）
> 转发给 Go 侧。macOS 保留系统边框与全局菜单。

- 左侧「连接」树里点某台服务器即打开一个终端标签页
- 连接失败（无密码 / 密码错误）会弹窗让你输入密码后重试
- 「文件传输」标签页可浏览本地与远程目录并上传 / 下载
- 「端口隧道」标签页可新建、启动、停止端口转发规则
- 「密钥管理」窗口可提交私钥（粘贴或从文件读取，支持加密私钥 + 口令），
  复制公钥到服务器 `authorized_keys`；连接编辑器里选择密钥后，登录会优先使用该密钥
