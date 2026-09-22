# wsh 「AI-Terminal-X」改进落地计划

本文件把朋友的改进清单（三栏架构 + Smart Input + AI 推理窗格 + 本地 AST 安全引擎 +
跳板机 + 资源探针）对照当前代码库逐条拆解，形成可独立交付、可验证的里程碑。

原则：**先做本地确定性能力（纯 Go、可单测），再做前端布局，最后接 AI 推理**。
安全能力绝不依赖远程 LLM，LLM 能力在未配置时必须优雅降级（不阻塞原有终端体验）。

---

## 一、现状与差距分析

### 1.1 当前架构（已核实）

| 层 | 位置 | 说明 |
| --- | --- | --- |
| 后端 RPC | `internal/server/rpc.go` | `{id, method, params}` 请求 / `{id, ok, data, error}` 响应；推送是带 `type` 字段的 JSON |
| 终端会话 | `internal/server/terminal.go` | `WebSession`：`x/crypto/ssh` 的 `Session` + PTY，`readLoop` 读输出，`HandleInput` 写输入 |
| SSH 层 | `ssh/client.go`、`ssh/pool.go` | `Dial` / `DialAdhoc`，连接池按 `connId` 复用；`InsecureIgnoreHostKey` |
| 存储 | `storage/store.go`、`storage/crypto.go` | `config.json`（连接/隧道/密钥）+ `settings.json`（全局选项），密码/私钥 AES-GCM 落盘 |
| 前端 | `frontend/src/` | React + Vite，`web/` 为构建产物并 `go:embed` |
| 布局 | `frontend/src/App.jsx`、`styles.css` | `#app-body` = flex 行：`#sidebar`(260px) + `#main`（`#quick-connect` + `#tabbar` + `#content`） |
| 终端渲染 | `components/TerminalTab.jsx` | `@xterm/xterm` + `@xterm/addon-fit` |

### 1.2 清单与现状的对应关系

| 清单项 | 当前状态 | 结论 |
| --- | --- | --- |
| 三栏布局（左会话 / 中终端 / 右推理） | 已有左 + 中，缺右 | 新增右侧可折叠面板 `#reason-pane` |
| 底部 Smart Input | 无（输入靠 xterm 原生） | 新增常驻输入条 + `terminal.exec` |
| 多标签 + 快速连接 | **已实现**（`TabBar` + `QuickConnectBar`） | 无需重做 |
| 跳板机拓扑 | 无（仅直连） | 新增 `JumpHostIDs` + 链式拨号 |
| 节点资源探针 | 无 | 新增 `host.stats` + 后台 goroutine |
| AST 安全引擎 | 无 | **新增 `internal/safety`（mvdan/sh）** |
| 数据脱敏网关 | 无 | **新增 `internal/sanitize`** |
| 输出流嗅探 | 无 | 新增 `internal/server/sniffer.go` |
| AI 推理 / CoT / RCA | 无 | 新增 `internal/ai` + 右侧卡片 |
| 影子推演确认面板 | 无 | 复用安全引擎的黄/红区 + 确认令牌 |
| WebGL 渲染 | 未启用 | 加 `@xterm/addon-webgl`，保留 canvas 回退 |

### 1.3 必须修正的清单描述（诚实偏差说明）

1. **`io.MultiWriter` 分流不必要。** 输出侧的唯一收口点已经是 `WebSession.readLoop` →
   `routeOutput`；在这里做嗅探即可，不必再包一层 `MultiWriter`（那会与 zmodem 分支抢数据流）。
2. **PTY 交互式 shell 拿不到「每条命令的退出码」。** 当前是在 `$SHELL` 里跑交互式 PTY，
   没有 per-command `$?`。清单里的「自动侦测 Exit Code != 0」在不侵入远端环境的前提下
   只能**用输出特征启发式**近似（`FATAL` / `Segmentation fault` / `Permission denied` …）。
   精确退出码需要注入 shell 集成脚本（写 `PROMPT_COMMAND`），违反「零侵入」，故做为**可选项**而非默认。
3. **单框双轨「自然语言 vs Shell」不能只靠 AST 是否解析成功判断。**
   `帮我找出占用 8080 端口的进程` 会被 POSIX 语法解析成一条合法命令（`帮我找出占用` 当命令名），
   所以判据必须是：**AST 解析成功 且 首词是可识别的命令/内建 且 不含 CJK**，否则走 AI 意图分支。
   另外提供显式 `Mode` 开关与 `/ai`、`/sh` 前缀兜底，避免误判。
4. **清单 POC 的 `CheckSecurityAST` 实现有缺陷**，不能照抄：
   - `strings.Contains(val, "r")` 会把任意含字母 r 的**文件名**当成递归标志（`rm -rf report/`）；
   - `arg.Lit()` 对引号/变量/通配符返回空串，`$TARGET`、`"$DIR"` 会被漏掉；
   - 不递归，`bash -c "rm -rf /"`、`sudo rm -rf /`、`$(...)`、`sh -c` 全部绕过；
   - `sudo rm -rf /` 的首词是 `sudo` 而不是 `rm`，直接漏判；
   - 没有覆盖 `dd of=/dev/sda`、`> /dev/sda` 重定向等清单自己列出的高危项。
   → 本计划用**完整 AST 遍历 + 正确的 flag 解析 + 递归下钻 + 重定向检查**重写（见 M1）。
5. **WebGL addon 是新增依赖**（`@xterm/addon-webgl`），需保留 `canvas` 回退，否则某些 WebView2/
   WebKitGTK 环境会白屏。
6. **`wails` 主包在本机无法编译**（缺 `pkg-config` / GTK 依赖），GUI 端到端验证需在目标平台进行；
   纯 Go 包（`ssh`、`storage`、`internal/server`）可正常 `go test`。这是**既有**限制，非本次引入。

---

## 二、实施进度

| 里程碑 | 内容 | 状态 |
| --- | --- | --- |
| M1 | 本地 AST 安全引擎 `internal/safety` | ✅ 已完成 |
| M2 | 数据脱敏网关 `internal/sanitize` | ✅ 已完成 |
| M3 | 安全引擎接入 RPC / 终端执行路径 | ✅ 已完成 |
| M4 | 输出流嗅探（错误特征 + 备用屏感知） | ✅ 已完成 |
| M5 | 前端三栏布局 + 底部 Smart Input + 风险高亮 | ✅ 已完成 |
| M6 | AI 推理层（Provider / CoT / RCA / NL→命令） | ✅ 已完成 |
| M7 | 影子推演确认面板与授权令牌 | ✅ 已完成（令牌随 M3 实现：`safety.confirm` + 一次性令牌；确认面板 UI 随 M5 实现） |
| M8 | 跳板机拓扑（存储 + 链式拨号 + UI） | ✅ 已完成 |
| M9 | 节点资源探针 | ✅ 已完成 |
| M10 | WebGL 渲染 + 文档 | ✅ 已完成 |

状态图例：⬜ 待开始 · 🚧 进行中 · ✅ 已完成

### 验证结果（本地实测）

| 项 | 命令 | 结果 |
| --- | --- | --- |
| Go 单元/集成测试 | `go test ./ssh/... ./storage/... ./internal/...` | ✅ 全部通过（safety / sanitize / ai / server / ssh） |
| 前端构建 | `cd frontend && bun run build` | ✅ 通过（60 modules，产物入 `web/`） |
| 整包交叉编译 | `CGO_ENABLED=0 GOOS=windows GOARCH=amd64 go build .` | ✅ 通过（含 `main.go`） |
| Linux 本地 `go build .` | 同上但本机目标 | ⚠️ 因缺 `pkg-config`/GTK 失败（**既有**环境限制，非本次引入） |
| GUI 端到端 | `./run.sh` | ⚠️ 未执行（需 Ubuntu 桌面或 Windows 环境） |

---

## 三、里程碑详情

### M1 本地 AST 安全引擎（`internal/safety`）

**目标**：Go 后端在毫秒级把命令解析为 AST，输出三级风险判定（绿/黄/红），完全不依赖 LLM 与正则。
**依赖**：`github.com/mvdan/sh/v3/syntax`（已验证 goproxy.cn 可拉取，latest v3.14.1）。

```go
type Level int
const (
    LevelSafe    Level = iota // 绿区：只读/低风险，直接放行
    LevelCaution              // 黄区：需二次确认（影子推演）
    LevelBlocked              // 红区：硬阻断，禁止下发
)

type Finding struct {
    Level   Level  `json:"level"`
    Rule    string `json:"rule"`    // 规则 ID，如 "rm.root"
    Reason  string `json:"reason"`  // 中文说明
    Command string `json:"command"` // 触发命令，如 "rm"
}

type Result struct {
    Level      Level     `json:"level"`
    Findings   []Finding `json:"findings"`
    ParseError string    `json:"parseError,omitempty"` // 语法不完整（正在输入）时按原样交给 shell
}

func Analyze(cmd string) Result
func Classify(cmd string) Level
```

**判定规则**
- 红区（硬阻断）：`rm -rf /`、`rm -rf /*`、`rm -rf ~`、`rm -rf /etc` 等系统目录；
  `dd of=/dev/sd*|/dev/nvme*`；重定向 `> /dev/sd*`；`mkfs.* /dev/*`；fork bomb `:(){ :|:& };:`。
- 黄区（需确认）：`rm -r/-rf <普通路径>`、`iptables -F/-X`、`nft flush ruleset`、
  `systemctl stop|restart|disable|mask`、`shutdown|reboot|halt|poweroff`、`kill -9 -1`、
  `crontab -r`、`userdel`、`docker system prune`、`> file` 截断覆盖。
- 绿区：其余（`ls`/`cat`/`grep`/`ss`/`lsof` …）。

**关键实现点**
- 正确解析短选项簇：`-rf`、`-fr`、`-r -f`、`--recursive`、`--force`。
- 递归下钻：`sudo`/`env`/`nohup`/`time` 等前缀剥离；`bash -c`/`sh -c`/`eval` 的内层脚本文本
  重新 `Parse` 后继续分析；命令替换 `$(...)`、反引号、子 shell、管道各段都遍历。
- 非字面量参数（`$VAR`、引号、通配符）：无法静态确定路径时**保守升级**为黄区，而非漏过。
- `syntax.Walk` 天然覆盖上述嵌套结构，但 `bash -c` 的**字符串内容**必须显式二次解析。

**验收**：`internal/safety/safety_test.go` 表驱动覆盖红/黄/绿三区、绕过手法、非字面量、
语法错误不 panic；`go test ./internal/safety/...` 通过。

---

### M2 数据脱敏网关（`internal/sanitize`）

**目标**：终端文本 / 用户输入在上送 LLM 前就地脱敏，纯内存正则，零外发。

```go
func Mask(s string) string        // IPv4/IPv6 → [IP_MASKED]，密钥类 → [SENSITIVE_DATA]
func MaskBytes(b []byte) []byte
```

**规则**：IPv4（校验八位组 0-255）、IPv6（严格：至少 2 个 `:` 且十六进制段，避免误伤时间戳
`18:00:12`）、`user:pass@host`、`password=`/`passwd=`/`secret=`/`token=`/`api_key=`、
`Authorization: Bearer xxx`、JWT（`eyJ...`）、AWS `AKIA[0-9A-Z]{16}`、PEM 私钥块整体替换。

**验收**：`internal/sanitize/sanitize_test.go` 必须包含「时间戳 / 版本号 / 正常日志不被误伤」
与「各类密钥被替换」两组用例。`Regexp` 在包初始化时编译一次。

---

### M3 安全引擎接入 RPC / 终端执行路径

**新增 RPC**
- `safety.check {command}` → `safety.Result`：输入框实时高亮/红色边框/禁用回车用。
- `terminal.exec {sessionId, command, force}` → 分类后写入 PTY（`command + "\n"`）：
  - 红区 → 返回错误，**不写入**；
  - 黄区且未带 `force` → `{confirm:true, findings:[...]}`，由右侧影子推演面板确认后带 `force` 重发；
  - 绿区 → 直接写入。
- `terminal.input` 保持原意（**原始按键透传**，TUI 模式用），不做拦截。

**说明**：把守的是「Smart Input 提交」这个动作；键盘直敲（xterm 原生输入）保持零侵入透传，
这是「终端原体验」与「安全护栏」的折中，符合清单的「零侵入与透传优先」。

---

### M4 输出流嗅探（`internal/server/sniffer.go`）

- 在 `WebSession.routeOutput` 的 `pushOutput` 前挂一个**只读旁路**（不消费数据，不干扰 zmodem）。
- 剥离 ANSI 转义后再匹配错误特征，推送两类事件：
  - `terminal.mode {sessionId, altScreen}`：识别 `\x1b[?1049h/l`、`\x1b[?47h/l`、`\x1b[?1047h/l`，
    前端据此把底部 Smart Input 切为「纯透传模式」，避免快捷键冲突。
  - `terminal.error {sessionId, kind, severity, excerpt}`：`FATAL`、`Segmentation fault`、
    `Permission denied`、`command not found`、`No such file or directory`、`panic:`、
    `out of memory`、`Address already in use` 等。
- 去抖与去重：同一会话同特征在时间窗内只推一次，避免刷屏时轰炸前端。

---

### M5 前端三栏布局 + 底部 Smart Input

- 布局：`#app-body` 内新增右侧 `#reason-pane`（默认收起，可拖拽/快捷键切换），`#main` 内
  `#content` 下方新增 `#smart-input`。
- Smart Input：输入即调 `safety.check` 高亮（红框禁用回车）；`Enter` 走 `terminal.exec`；
  NL 意图命中时展示 **Preview 预览行**（`Enter` 执行 / `Tab` 填入输入框）；`Esc` 清空。
- alt-screen 时自动切换为透传模式并提示。
- 右侧窗格：CoT 流式卡片（可折叠）、错误 RCA 卡片、修复按钮（把命令填回输入框）、影子推演确认。

---

### M6 AI 推理层（`internal/ai`）

- Provider 抽象：`openai`（兼容 OpenAI/Azure/任意兼容端点）与 `ollama`（本地离线）。
- 设置项新增：`aiProvider` / `aiBaseURL` / `aiModel` / `aiAPIKey`（**密文落盘**，复用
  `storage.EncryptPassword`）/ `aiOffline`。未配置 → 全部 AI 功能静默禁用，终端不受影响。
- 流式：`ai.ask {sessionId, prompt}` → `ai.chunk` / `ai.done` 推送，右侧渲染 CoT。
- NL→命令：要求模型输出**结构化单条命令**，经 `safety.Analyze` 复核后才可执行（LLM 不参与放行决策）。
- RCA：把嗅探到的错误 + 脱敏上下文（含 `sanitize.Mask`）送模型，回填错误卡片。

---

### M7 影子推演确认面板与授权令牌

- `safety.confirm {command}` 生成一次性令牌（内存、短时效、绑定命令哈希与会话），
  `terminal.exec` 的 `force` 改为携带该令牌，避免前端简单布尔被滥用；令牌消费即失效。

---

### M8 跳板机拓扑

- `storage.Connection` 增 `JumpHostIDs []string`（指向其它连接），`connView`/`saveConnParams` 同步。
- 拨号：`sshclient.DialVia(jumps, target)`：逐级 `bastion.Dial("tcp", nextAddr)` 得到 `net.Conn`，
  再用 `ssh.NewClientConn` 建客户端；检测环路并限制深度。
- UI：连接编辑器可选多级跳板；侧栏树展示 `Local → Bastion A → Target` 链路。

---

### M9 节点资源探针

- `internal/server/probe.go`：为「已打开终端」的会话起低频 goroutine，用**独立 SSH channel**
  （不影响交互式 PTY）执行只读采集命令，解析 CPU/内存/磁盘，推送 `host.stats`。
- 侧栏在主机行旁渲染迷你仪表盘；无会话时不采集，避免无谓连接。

---

### M10 WebGL 渲染 + 文档

- 加 `@xterm/addon-webgl`，加载失败回退 canvas；`README.md` 增补 AI 配置、安全引擎、
  跳板机、探针的说明。

---

## 四、验证策略

| 范围 | 方式 | 现状 |
| --- | --- | --- |
| `internal/safety`、`internal/sanitize`、`ssh`、`storage`、`internal/server` | `go test ./...` | 可用 |
| 前端 | `cd frontend && bun run build` | 可用（bun 已安装） |
| GUI 端到端 | `./run.sh`（Wails 原生窗口） | **本机不可用**：缺 `pkg-config`/GTK，需在 Ubuntu 桌面或 Windows 上跑 |
| Windows 产物 | `./build.sh`（mingw 交叉编译） | 需 `gcc-mingw-w64-x86-64` |

> 说明：`go build ./...` 在本机因 Wails v3 需要 `pkg-config` 而失败（既有问题）；
> 因此 CI/本地以「非 Wails 包 `go test` + 前端 `bun run build`」作为回归门槛。

---

## 五、实施记录（实际落地情况）

### 5.1 新增 / 修改文件

**后端**

| 文件 | 作用 |
| --- | --- |
| `internal/safety/safety.go` | AST 遍历、三级判定、嵌套下钻（`bash -c`/`eval`/`find -exec`） |
| `internal/safety/rules.go` | 命令前缀剥离（sudo/env/timeout/ssh/xargs…）与规则表 |
| `internal/sanitize/sanitize.go` | 脱敏网关（顺序化规则 + 边界校验） |
| `internal/ai/ai.go` `provider.go` `format.go` | Provider 抽象、OpenAI/Ollama 流式解析、CoT 结构化解析 |
| `internal/server/safety.go` | `safety.check` / `safety.confirm` / `terminal.exec` + 一次性令牌 |
| `internal/server/sniffer.go` | 输出旁路：备用屏识别 + 错误特征 + 最近输出环形缓冲 |
| `internal/server/ai.go` | `ai.status` / `ai.ask` 流式推送 + 脱敏上下文拼装 |
| `internal/server/probe.go` | 主机采样 goroutine + `df`/`/proc` 输出解析 |
| `storage/models.go` `store.go` | `JumpHostIDs`、AI 设置项、`Connection(id)` 查找、内存 Store |
| `ssh/client.go` `pool.go` | `JumpChain` / `DialChain` 链式拨号，池接入跳板解析 |

**前端**

| 文件 | 作用 |
| --- | --- |
| `components/SmartInput.jsx` | 纯输入框（不再预览命令）+ 风险高亮 + 透传模式；解析模式为「自动判别 / 仅当命令 / 仅交给 AI」，只决定输入怎么理解；Tab 填入最新 AI 指令，Ctrl+Enter 一键执行；状态行常驻占位并说明当前输入的走向，避免终端重排 |
| `components/ReasonPane.jsx` | 对话承载区：用户提问卡（`ask`）/ CoT / RCA / 影子推演 / 阻断提示；命令卡片带「填入/立即执行」、执行状态、原地结果分析与快捷追问 |
| `lib/intent.js` | Shell vs 自然语言判据 + 风险/严重度标签 |
| `state/store.jsx` | 推理卡片、AI 请求关联（含卡片内嵌字段）、统一执行入口、命令输出绑定、提问卡、主机采样 |
| `App.jsx` `TerminalTab.jsx` `Sidebar.jsx` `SettingsPage.jsx` `ConnectionDialog.jsx` `styles.css` | 三栏布局、推送订阅、AI 设置、跳板机选择、仪表盘 |

### 5.2 实际新增的 RPC

| 方法 | 参数 → 返回 |
| --- | --- |
| `safety.check` | `{command}` → `safety.Result` |
| `safety.confirm` | `{sessionId, command}` → `{token, result}` |
| `terminal.exec` | `{sessionId, command, confirmToken, trackId}` → `{written, blocked, confirm, result}` |
| `ai.status` | → `{configured, provider, model, autoAnalyze, noContext, name}` |
| `ai.ask` | `{sessionId, prompt, kind, excerpt}` → `{requestId, kind}` |
| `probes.snapshot` | → `map[connId]HostStats` |

`ai.ask` 的 `kind` 为 `command`（自然语言→命令）、`diagnose`（报错根因）或
`result`（解读刚执行命令的输出，返回 `suggestions` 供卡片生成快捷追问按钮）。

推送：`terminal.mode`、`terminal.error`、`terminal.output`、`ai.delta`、`ai.done`、`host.stats`。

`terminal.exec` 带 `trackId` 时，后端会把这条命令的输出片段随 `terminal.output`
（`{trackId, command, text, durationMs, truncated, timedOut}`）推回，前端据此把输出
绑到发起该命令的推理卡片上，自动触发结果分析，形成「提问 → 生成 → 执行 → 解读 →
追问」的闭环。

### 5.3 实施中修正的细节 (与清单原文的差异)

1. **依赖路径修正**：清单写的 `github.com/mvdan/sh/v3/syntax` 不存在，实际模块路径是
   `mvdan.cc/sh/v3/syntax`（`go get` 会直接报 `module declares its path as`）。
2. **清单 POC 的 `CheckSecurityAST` 未采用**，换成 `syntax.Walk` + 前缀剥离 + 递归下钻；
   已用测试固定住清单会漏判的绕过手法（`sudo rm -rf /`、`bash -c`、`eval`、
   `xargs`、`find -exec`、`ssh host rm -rf /`、命令替换、子 shell）。
3. **退出码侦测降级为输出特征启发式**：交互式 PTY 不提供 per-command `$?`，
   精确实现需注入 `PROMPT_COMMAND`（入侵远端环境），与「零侵入」冲突。
   同理，命令输出片段的**结束点**也是推断的：`sniffer` 在「输出静默 ≥400ms」
   且「尾部形似 shell 提示符」时才收尾，否则再给 1.5s 宽限，超 30s 或 32KB
   则截断交付。误判只影响喂给模型的上下文长度，不会影响命令本身。
4. **黄区确认用一次性令牌而非布尔开关**：令牌绑定会话 + 命令哈希，2 分钟过期、消费即失效。
5. **脱敏规则刻意保守**：IPv6 要求具备 `::` 或 8 段（否则会误伤 `18:00:12`），
   IPv4 校验八位组与词边界（否则误伤 `nginx/1.18.0`、`/var/log/1.2.3.4.log`）。
   测试里专门有一组「必须保持不变」的用例。
6. **AI 上下文开关用反向字段 `AINoContext`**：Go 布尔零值为 false，
   反向命名才能让「默认发送上下文」在不迁移历史配置的情况下成立。
7. **跳板机的认证边界**：提示只针对目标主机；跳板机必须自己已存凭据或可用密钥，
   否则报明确的「no authentication method」而非静默超时。
8. **资源探针只采样池化连接**：quick-connect 会话自持 client（不入池），不参与采样。
9. **WebGL 加失败回退**：`@xterm/addon-webgl` 在某些 WebView2/WebKitGTK 环境不可用，
   加载异常时保留 canvas 渲染，避免白屏。
