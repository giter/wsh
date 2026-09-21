# wsh

基于 Go + Web 的跨平台 SSH 客户端。后端用 Go 实现，界面是内嵌的 Web 前端
（ZED 风格：左侧连接树 + 顶部多标签页），终端用 xterm.js 渲染，
由 Wails v3 封装为原生桌面窗口。

## 功能

- **会话管理器**（左侧）— 文件夹 / 连接树 + 选中连接的属性面板，新建 / 重命名 / 删除都在这里
- **多标签会话** — 主区域是多标签页（终端 / 文件传输 / 端口隧道），标签栏末尾的 ＋ 可新建连接
- **快速连接** — 顶部地址栏直接输 `ssh://user@host:port` 回车即连，不保存、适合临时用
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
Wails v3 桌面窗口 (跨平台 WebView 容器，窗口/菜单等桌面能力)
  └─ Go 后端 (复用 SSH / SFTP / 隧道 / 存储逻辑)
       └─ 本地 HTTP + WebSocket 服务 (仅绑定 127.0.0.1)
            └─ React 前端 (构建产物 go:embed 进单二进制，离线可跑)
                 ├─ 左侧会话管理器 (连接树 + 属性)
                 ├─ 顶部快速连接地址栏 + 多标签会话
                 └─ xterm.js 终端 / SFTP 双栏 / 隧道管理
```

前后端通过 WebSocket 交换 JSON 消息（RPC），终端输出以流式推送，键盘输入走请求。
前端是 **React + Vite**（源码在 `frontend/`，构建产物输出到 `web/` 并被 `go:embed`
打包进可执行文件，运行时不需要 Node）；Wails 只充当窗口外壳，加载本地服务的 URL。

> `web/` 是构建产物，**不入库**（见 `.gitignore`）。所以 `go build` 前需要先构建
> 前端，否则窗口里会显示一个“前端未构建”的提示页。`./run.sh` 和 `./build.sh`
> 会自动先跑前端构建。

## 构建

需要 **Go 1.25+**、**bun**（构建前端）与平台 WebView 运行时（不再是纯 Go / 无 CGO）：

- **Windows** — 自带 WebView2 Runtime（Win10/11 已内置）；交叉编译需 mingw-w64
- **Linux** — 需 `libwebkit2gtk-4.1-dev`、`libgtk-3-dev`、`build-essential`、`pkg-config`
- **macOS** — 需 Xcode Command Line Tools（自带 WKWebView）

```bash
# Linux (Debian/Ubuntu 先装依赖)
sudo apt install libgtk-3-dev libwebkit2gtk-4.1-dev build-essential pkg-config
# 前端工具链
curl -fsSL https://bun.sh/install | bash

# 构建前端 + 后端并启动（弹出原生窗口）
./run.sh

# 手动：先构建前端，再编译后端
cd frontend && bun install && bun run build && cd ..
go build -o wsh .
```

### 前端开发（HMR 调试）

```bash
./dev.sh        # 后端(固定端口 17777) + Vite 开发服务器(5173)
```

然后在浏览器打开 <http://localhost:5173>：改前端代码即时热更新，无需重编译 Go。
Vite 会把 `/ws` 代理到后端。想连自绘标题栏一起看，用
<http://localhost:5173/?win=main>（拖动/缩放需要 Wails 运行时，浏览器里不生效，
但菜单与窗口按钮的界面可以调）。

单独跑时也可以用：

```bash
go run . -no-open -port 17777        # 后端，固定端口
cd frontend && bun install && bun run dev
```

前端源码结构：

```
frontend/src/
  main.jsx / App.jsx      入口与整体布局（标题栏 / 快速连接 / 侧边栏 / 标签页）
  lib/rpc.js              WebSocket RPC 客户端（请求 + 推送订阅）
  lib/windowChrome.js     无边框窗口的拖动 / 缩放与窗口按钮
  lib/format.js           格式化 / 路径 / base64 / 快速连接地址解析
  state/store.jsx         全局状态（设置、连接、密钥、标签页、弹窗）
  components/             各页面与组件（终端、SFTP、隧道、密钥…）
```

> 改完前端跑 `cd frontend && bun run build` 刷新 `web/`，或直接用 `./run.sh`。

### Windows（.exe）

Windows 版依赖 WebView2 Runtime（Win10/11 自带），交叉编译需要 mingw 工具链：

```bash
sudo apt install gcc-mingw-w64-x86-64   # 交叉工具链
./build.sh                              # 构建前端 + 生成 wsh.exe
```

> 生成的 `wsh.exe` 在 Windows 上启动后直接显示原生窗口，
> 无需打开浏览器。

## 使用

启动后程序在 `127.0.0.1` 起一个本地服务（默认随机端口，`-port` 可固定），
并在 Wails 原生窗口中加载前端界面（`-no-open` 可只起服务不弹窗，便于调试）。
配置数据保存在系统配置目录下 `wsh/config.json`（密码与私钥均为加密后的密文），
机器密钥保存在同目录 `secret.key`。

菜单栏（自绘，位于标题栏内）：

- **文件** — 新建连接（`Ctrl/Cmd+N`）、退出（`Ctrl/Cmd+Q`）
- **选项** — 选项（`Ctrl/Cmd+Shift+,`）、密钥管理（`Ctrl/Cmd+Shift+K`），均在独立窗口中打开
- **视图** — 连接 / 文件传输 / 端口隧道 快速切换（`Ctrl/Cmd+1/2/3`）
- **帮助** — 关于 wsh、开发者工具（DevTools）

> Windows/Linux 下窗口是无边框的（frameless），系统标题栏和菜单栏都不再使用，
> 由前端绘制一整套标题栏（品牌 + 菜单 + 最小化/最大化/关闭），见
> `frontend/src/lib/windowChrome.js`。窗口拖动/边缘缩放通过 Wails 的
> `wails:drag` / `wails:resize:<edge>` 消息实现，窗口按钮、打开配置窗口、退出、
> DevTools 通过 RPC（`window.control` / `app.action`）转发给 Go 侧。
> macOS 保留系统边框与全局菜单。

- 左侧「会话管理器」里单击连接选中（下方显示属性），双击即打开一个终端标签页；
  文件夹重命名 / 删除在鼠标悬停时出现在行尾
- 顶部「快速连接」地址栏可直接输地址临时连接（不保存），支持的写法：
  `ssh://user@host:port`、`user@host:port`、`host:port`、`user@host`、`host`；
  没写用户名时用设置里的默认用户。认证先试已托管的密钥，再试密码（没密码会弹窗输入）
- 连接失败（无密码 / 密码错误）会弹窗让你输入密码后重试
- 「文件传输」标签页可浏览本地与远程目录并上传 / 下载
- 「端口隧道」标签页可新建、启动、停止端口转发规则
- 「密钥管理」窗口可提交私钥（粘贴或从文件读取，支持加密私钥 + 口令），
  复制公钥到服务器 `authorized_keys`；连接编辑器里选择密钥后，登录会优先使用该密钥
