// AI reasoning pane (ReasonPane) and the smart input bar (SmartInput).
export default {
    en: {
        // ReasonPane header / empty states
        "reason.title": "AI Reasoning",
        "reason.clear": "Clear",
        "reason.collapse": "Collapse (Ctrl+Shift+A to reopen)",
        "reason.notConfigured":
            "No AI provider configured. The local safety engine still works: blocked-zone commands are rejected outright, and caution-zone commands are dry-run here for confirmation.",
        "reason.notConfiguredHint": "Configure an OpenAI-compatible endpoint or local Ollama in Options → AI Reasoning.",
        "reason.empty": "Nothing here yet. Type a natural-language request below, or let the terminal raise an error for auto-diagnosis.",

        // DryRunCard (shadow dry-run)
        "reason.dryRun": "Shadow Dry-Run",
        "reason.needsConfirm": "This command needs confirmation",
        "reason.autoInitiated": "Initiated by AI",
        "reason.cancel": "Cancel",
        "reason.once": "Just once",
        "reason.onceTip": "Allow this run only",
        "reason.allowSession": "Allow this session",
        "reason.allowSessionTip": "Don't ask again for this command in this session",
        "reason.alwaysAllow": "Always allow",
        "reason.alwaysAllowTip": "Always allow this command; remove it in Options → AI Reasoning",
        "reason.allowHint":
            "\"Allow this session\" and \"Always allow\" apply to this exact command line only; no other command is loosened.",

        // Folded card summaries (cardSummary)
        "reason.aiRequest": "AI request",
        "reason.aiFallback": "AI reasoning",
        "reason.failedSummary": "⚠ {title} failed",
        "reason.cancelledSummary": "Cancelled · {target}",
        "reason.analysisFailedSummary": "⚠ {cmd} · result analysis failed",
        "reason.summaryRunning": "⏳ {cmd} · running…",
        "reason.summaryWaiting": "🟡 {cmd} · waiting for input",
        "reason.summaryStreaming": "🟡 {cmd} · streaming",
        "reason.summaryTimeout": "🟡 {cmd} · captured first {secs}s of output",
        "reason.summaryBlocked": "⛔ {cmd} · blocked",
        "reason.summaryConfirm": "⚠ {cmd} · awaiting confirmation",
        "reason.summaryError": "⚠ {cmd} · run failed",
        "reason.summaryStalled": "🟡 {cmd} · stopped at an input prompt, result unconfirmed",
        "reason.summaryDone": "🟢 {cmd} · {secs}s",
        "reason.summaryDefault": "🤖 {target}",

        // ReasonCard
        "reason.blockedPill": "Blocked",
        "reason.myQuestion": "My question",
        "reason.analyzeRca": "Analyze root cause",
        "reason.thinking": "Thinking…",
        "reason.cancelled": "Cancelled",
        "reason.warnPill": "⚠ Risk noted",
        "reason.failed": "Failed",
        "reason.rawReply": "Thought process (raw model reply)",
        "reason.retry": "Retry",

        // GeneratedCommand
        "reason.generatedCommand": "Generated command",
        "reason.fillTab": "Fill input (Tab)",
        "reason.fillTabTip": "Or just press Tab",
        "reason.runNow": "🚀 Run now (Ctrl+↵)",
        "reason.running": "Running…",
        "reason.viewOutput": "View raw output",
        "reason.viewOutputTip": "Scroll the terminal and highlight this command's output",
        "reason.outputGone": "This output has scrolled out of the terminal buffer and can't be located",

        // ExecStatus
        "reason.stop": "Stop (Ctrl+C)",
        "reason.stopTip": "Send Ctrl+C to the terminal",
        "reason.confirmedWaiting": "Confirmed, waiting for output…",
        "reason.autoWaiting": "Safe-zone command auto-executed, waiting for output…",
        "reason.sentWaiting": "Sent, waiting for output…",
        "reason.waitingPassword": "🟡 Waiting for input (may need a password) · continue in the terminal",
        "reason.streamingStatus": "🟡 Streaming · {el}s",
        "reason.blockedByEngine": "⛔ Blocked by the local safety engine{reason}",
        "reason.needConfirmExec": "⚠ Caution-zone command — confirm in the shadow dry-run above",
        "reason.execFailed": "Run failed{reason}",
        "reason.truncatedTail": " (output truncated)",
        "reason.timeoutStatus": "🟡 Command still running, captured first {secs}s of output{tail}",
        "reason.stoppedAtPrompt": "🟡 Command stopped at an input prompt (e.g. password); result unconfirmed",
        "reason.doneConfirmed": "🟢 Confirmed run ({secs}s){tail}",
        "reason.doneAuto": "⚡ Auto-executed (safe zone, {secs}s){tail}",
        "reason.done": "🟢 Executed ({secs}s){tail}",

        // ResultAnalysis
        "reason.nextStep": "Next troubleshooting step",
        "reason.analysisTitle": "🤖 AI result analysis",
        "reason.analyzing": "Analyzing…",
        "reason.retryAnalysis": "Retry analysis",
        "reason.followups": "Quick follow-ups",
        "reason.followupsHint": "Click to run — still gated by the local safety engine",

        // SmartInput
        "smart.passthrough": "Passthrough",
        "smart.passthroughHint": "A full-screen app is running; keyboard input goes straight to the remote (Esc exits full-screen)",
        "smart.placeholder": "Type a command, or describe what you want…",
        "smart.placeholderNoAI": "Type a command (natural language available once AI is configured)",
        "smart.modeTip":
            "Parsing mode (affects how input is read, never whether it runs):\n" +
            "Auto: Chinese or question-like input goes to AI, the rest runs as a command;\n" +
            "Command only: everything is sent to the shell as-is;\n" +
            "AI only: the model always generates the command.\n" +
            "Whether anything executes, and whether it needs confirmation, is always decided by the local safety engine.",
        "smart.modeAuto": "Auto",
        "smart.modeShell": "Command only",
        "smart.modeAI": "AI only",
        "smart.astEngine": "Local AST safety engine",
        "smart.escToInterrupt": "Press Esc to interrupt",
        "smart.thinking": "Thinking…",
        "smart.noAIExecuted": "No AI provider configured; sent as-is as a command",
        "smart.aiNoResult": "AI returned no usable result",
        "smart.aiNoCommand": "AI didn't produce a runnable command — try rephrasing",
        "smart.blockedDisabled": "Blocked-zone command; execution is disabled",
        "smart.aiInterrupted": "AI generation interrupted",
        "smart.ctrlCSent": "Sent Ctrl+C to the terminal",
        "smart.runningHint": "Command running · Esc sends Ctrl+C to stop",
        "smart.hintAI": "Tab fills the latest AI command · Ctrl+Enter runs it directly",
        "smart.hintEnter": "Enter runs the command",
        "smart.willAsk": "Will ask AI to generate a command (Enter to generate · Ctrl+Enter to generate and run)",
        "smart.blockedZone": "Red zone · blocked{reason} (cannot run)",
        "smart.cautionZone": "Will run as a command · yellow zone · confirmation required{reason}",
        "smart.safeZone": "Will run as a command · green zone · safe (Enter to run)",
    },
    zh: {
        // ReasonPane header / empty states
        "reason.title": "AI 推理",
        "reason.clear": "清空",
        "reason.collapse": "收起（Ctrl+Shift+A 可重新打开）",
        "reason.notConfigured": "未配置 AI 提供方。本地安全引擎仍然工作：高危命令会被直接阻断，黄区命令会在此处推演确认。",
        "reason.notConfiguredHint": "在「选项 → AI 推理」中配置 OpenAI 兼容接口或本地 Ollama。",
        "reason.empty": "暂无推理内容。在底部输入自然语言，或让终端抛出错误后自动诊断。",

        // DryRunCard (shadow dry-run)
        "reason.dryRun": "影子推演",
        "reason.needsConfirm": "该命令需要确认",
        "reason.autoInitiated": "AI 自动发起",
        "reason.cancel": "取消",
        "reason.once": "仅此一次",
        "reason.onceTip": "只放行这一次",
        "reason.allowSession": "本会话允许",
        "reason.allowSessionTip": "本会话内不再询问这条命令",
        "reason.alwaysAllow": "始终允许",
        "reason.alwaysAllowTip": "永久放行这条命令，可在「选项 → AI 推理」中移除",
        "reason.allowHint": "「本会话允许」和「始终允许」只对这条命令本身生效（完全相同的一行），不会放宽其他命令。",

        // Folded card summaries (cardSummary)
        "reason.aiRequest": "AI 请求",
        "reason.aiFallback": "AI 推理",
        "reason.failedSummary": "⚠ {title}失败",
        "reason.cancelledSummary": "已取消 · {target}",
        "reason.analysisFailedSummary": "⚠ {cmd} · 结果分析失败",
        "reason.summaryRunning": "⏳ {cmd} · 执行中…",
        "reason.summaryWaiting": "🟡 {cmd} · 等待输入",
        "reason.summaryStreaming": "🟡 {cmd} · 持续监听中",
        "reason.summaryTimeout": "🟡 {cmd} · 已截取前 {secs}s 输出",
        "reason.summaryBlocked": "⛔ {cmd} · 已阻断",
        "reason.summaryConfirm": "⚠ {cmd} · 待确认",
        "reason.summaryError": "⚠ {cmd} · 执行失败",
        "reason.summaryStalled": "🟡 {cmd} · 停在输入提示，未确认结果",
        "reason.summaryDone": "🟢 {cmd} · {secs}s",
        "reason.summaryDefault": "🤖 {target}",

        // ReasonCard
        "reason.blockedPill": "已阻断",
        "reason.myQuestion": "我的提问",
        "reason.analyzeRca": "分析根因",
        "reason.thinking": "思考中…",
        "reason.cancelled": "已取消",
        "reason.warnPill": "⚠ 有风险提示",
        "reason.failed": "失败",
        "reason.rawReply": "思考过程（模型原始回复）",
        "reason.retry": "重试",

        // GeneratedCommand
        "reason.generatedCommand": "生成指令",
        "reason.fillTab": "填入 (Tab)",
        "reason.fillTabTip": "也可以直接按 Tab",
        "reason.runNow": "🚀 立即执行 (Ctrl+↵)",
        "reason.running": "执行中…",
        "reason.viewOutput": "查看原始输出",
        "reason.viewOutputTip": "滚动终端并高亮这条命令的输出",
        "reason.outputGone": "这段输出已流出终端缓冲区，无法定位",

        // ExecStatus
        "reason.stop": "停止 (Ctrl+C)",
        "reason.stopTip": "向终端发送 Ctrl+C",
        "reason.confirmedWaiting": "已确认，等待输出…",
        "reason.autoWaiting": "绿区命令已自动执行，等待输出…",
        "reason.sentWaiting": "已下发，等待输出…",
        "reason.waitingPassword": "🟡 等待输入（可能需要密码）· 在终端输入后继续",
        "reason.streamingStatus": "🟡 持续监听中 (Streaming…) · {el}s",
        "reason.blockedByEngine": "⛔ 已被本地安全引擎阻断{reason}",
        "reason.needConfirmExec": "⚠ 黄区命令，需确认 — 见上方影子推演",
        "reason.execFailed": "执行失败{reason}",
        "reason.truncatedTail": "（输出过长，已截断）",
        "reason.timeoutStatus": "🟡 命令仍在运行，已截取前 {secs}s 输出{tail}",
        "reason.stoppedAtPrompt": "🟡 命令停在输入提示（如密码），未能确认执行结果",
        "reason.doneConfirmed": "🟢 已确认执行（{secs}s）{tail}",
        "reason.doneAuto": "⚡ 已自动执行（绿区 · {secs}s）{tail}",
        "reason.done": "🟢 已执行（{secs}s）{tail}",

        // ResultAnalysis
        "reason.nextStep": "下一步排查",
        "reason.analysisTitle": "🤖 AI 结果分析",
        "reason.analyzing": "分析中…",
        "reason.retryAnalysis": "重试分析",
        "reason.followups": "快捷追问",
        "reason.followupsHint": "点击即执行，仍需通过本地安全引擎",

        // SmartInput
        "smart.passthrough": "透传模式",
        "smart.passthroughHint": "全屏应用运行中，键盘输入已直接转发给远端（Esc 退出全屏后恢复）",
        "smart.placeholder": "输入命令，或描述你的意图…",
        "smart.placeholderNoAI": "输入命令（配置 AI 后可用自然语言）",
        "smart.modeTip":
            "解析模式（只决定输入怎么理解，不决定是否执行）：\n" +
            "自动判别：中文或疑问句交给 AI 生成命令，其余按命令执行；\n" +
            "仅当命令：一律按 shell 命令原样发送；\n" +
            "仅交给 AI：一律由模型生成命令。\n" +
            "执行与否、是否需要确认，始终由本地安全引擎决定。",
        "smart.modeAuto": "自动判别",
        "smart.modeShell": "仅当命令",
        "smart.modeAI": "仅交给 AI",
        "smart.astEngine": "本地 AST 安全引擎",
        "smart.escToInterrupt": "按 Esc 中断",
        "smart.thinking": "流式思考中…",
        "smart.noAIExecuted": "未配置 AI 提供方，已按原样作为命令执行",
        "smart.aiNoResult": "AI 未返回可用结果",
        "smart.aiNoCommand": "AI 没有给出可执行的命令，请换一种说法",
        "smart.blockedDisabled": "高危命令已禁用执行",
        "smart.aiInterrupted": "已中断 AI 生成",
        "smart.ctrlCSent": "已向终端发送 Ctrl+C",
        "smart.runningHint": "命令运行中 · Esc 发送 Ctrl+C 停止",
        "smart.hintAI": "Tab 填入最新 AI 指令 · Ctrl+Enter 直接执行",
        "smart.hintEnter": "Enter 执行命令",
        "smart.willAsk": "将交给 AI 生成指令（Enter 生成 · Ctrl+Enter 生成并执行）",
        "smart.blockedZone": "红区 · 已阻断{reason}（无法执行）",
        "smart.cautionZone": "将作为命令执行 · 黄区 · 需确认{reason}",
        "smart.safeZone": "将作为命令执行 · 绿区 · 安全（Enter 执行）",
    },
};
