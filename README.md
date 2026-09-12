# FyneShell

基于 [Fyne](https://fyne.io) 的跨平台 SSH 客户端。

## 功能

- **连接管理** — 保存多台服务器，一键新建 / 编辑 / 删除 / 测试连接
- **密码保存** — 密码使用本机密钥 AES-GCM 加密后落盘，配置被拷走也无法解出明文
- **终端** — 打开 SSH 远程 shell，随窗口自动调整 PTY 大小
- **文件传输** — 双栏 SFTP 浏览器，支持上传 / 下载单个文件
- **隧道管理** — 本地端口转发到远程地址，可启动 / 停止

## 构建

需要 Go 1.21+ 与 C 编译器（Fyne 桌面端依赖 CGO/OpenGL）。

```bash
# Linux 还需系统开发库
sudo apt-get install libgl1-mesa-dev xorg-dev libxkbcommon-dev

CGO_ENABLED=1 go build -o fyneshell .
```

### 交叉编译 Windows（.exe）

Fyne 的 GLFW/OpenGL 后端需要 CGO，所以编译 Windows 版必须安装 MinGW 交叉编译器：

```bash
# 安装 MinGW-w64 交叉编译器
sudo apt-get install gcc-mingw-w64-x86-64

# 方式一：运行脚本
./build_windows.sh

# 方式二：手动交叉编译
CGO_ENABLED=1 GOOS=windows GOARCH=amd64 \
  CC=x86_64-w64-mingw32-gcc \
  go build -ldflags "-H windowsgui" -o FyneShell.exe .
```

生成的 `FyneShell.exe` 可直接拷贝到 Windows 上运行，无需在 Windows 上重新编译。
在 Windows 本地开发时也可直接用 `go build` 构建（需安装 Go + MSVC 或 MinGW）。

> 说明：`-H windowsgui` 让 GUI 程序不弹出控制台窗口。代码本身无平台特定调用，
> 完全跨平台（存储用 `os.UserConfigDir`，终端组件 `fyne-io/terminal` 官方支持 Windows）。

## 使用

配置数据保存在系统配置目录下 `fyneshell/config.json`（密码为加密后的密文），
机器密钥保存在同目录 `secret.key`。

### 连接管理

- 点击「新建」添加服务器（名称、主机、端口、用户、密码、可选私钥）
- 勾选「保存密码」后密码才会加密落盘
- 「测试连接」校验凭据

### 传输

- 选择连接后出现远程浏览器
- 左侧本地、右侧远程；选中文件点「上传」或「下载」

### 隧道

- 新建规则：本地端口 → 远程主机端口，经由某台已保存的连接
- 点击「启动」开始转发，连接断开后自动停止
