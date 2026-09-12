"use strict";

function makeTerminalPage(connId, connName) {
    const page = document.createElement("div");
    page.className = "tab-page";

    const wrap = document.createElement("div");
    wrap.className = "term-wrap";

    const connecting = document.createElement("div");
    connecting.className = "center-box";
    connecting.innerHTML = `<div class="spinner"></div><div>正在连接 ${connName} …</div>`;
    wrap.appendChild(connecting);

    const fitAddon = new FitAddon.FitAddon();
    const term = new Terminal({
        fontFamily: '"Cascadia Code", "JetBrains Mono", Consolas, monospace',
        fontSize: 13,
        theme: {
            background: "#111118",
            foreground: "#e7e7f0",
            cursor: "#34d399",
            selectionBackground: "rgba(52,211,153,.25)",
        },
        scrollback: 5000,
    });

    page.appendChild(wrap);

    const tab = Tabs.add(connId, connName, page, (t) => {
        if (t.sessionId && !t.exited) RPC.call("terminal.close", { sessionId: t.sessionId }).catch(() => {});
    });

    const onReady = (sessionId) => {
        tab.sessionId = sessionId;
        wrap.innerHTML = "";
        term.loadAddon(fitAddon);
        term.open(wrap);
        fitAddon.fit();
        tab.term = term;
        // Replay any output pushed before the terminal finished opening.
        if (tab.pending && tab.pending.length) {
            tab.pending.forEach((d) => term.write(d));
            delete tab.pending;
        }

        term.onData((data) => {
            RPC.call("terminal.input", { sessionId, data }).catch(() => {});
        });
        term.onResize(() => {
            const { cols, rows } = fitAddon.proposeDimensions();
            if (cols && rows) RPC.call("terminal.resize", { sessionId, cols, rows }).catch(() => {});
        });

        const prevActive = tab.id;
        setTimeout(() => {
            if (Tabs.active === prevActive) term.focus();
        }, 60);
    };

    const onError = (message, needPassword) => {
        connecting.innerHTML = "";
        const e = document.createElement("div");
        e.className = "center-box";
        e.innerHTML = `<div class="err">连接失败</div><div class="muted">${esc(message)}</div>`;
        connecting.appendChild(e);
        if (needPassword) {
            promptPassword(message, (pw) => {
                if (pw === null) return; // user cancelled
                connecting.innerHTML = `<div class="spinner"></div><div>正在连接 ${connName} …</div>`;
                connect(pw);
            });
        }
    };

    const connect = (password) => {
        RPC.call("terminal.open", { connId, password: password || "" })
            .then((res) => {
                if (res && res.needPassword) {
                    onError(res.message || "需要密码", true);
                    return;
                }
                onReady(res.sessionId);
            })
            .catch((err) => onError(err.message, false));
    };

    connect("");
    return tab;
}

// promptPassword shows a modal asking for the connection password and invokes
// callback(pw); callback(null) means the user dismissed the dialog.
function promptPassword(message, callback) {
    const pw = document.createElement("input");
    pw.type = "password";
    pw.placeholder = "密码";
    pw.autocomplete = "off";

    const status = document.createElement("div");
    status.className = "status-msg err";
    status.textContent = message;

    const field = document.createElement("label");
    field.className = "field";
    field.appendChild(pw);

    const submit = () => {
        Modal.close();
        callback(pw.value);
    };
    const cancel = () => {
        Modal.close();
        callback(null);
    };

    pw.addEventListener("keydown", (e) => {
        if (e.key === "Enter") {
            e.preventDefault();
            submit();
        }
    });

    const body = document.createElement("div");
    body.appendChild(status);
    body.appendChild(field);
    body.appendChild(containerHBox(button("btn", "取消", cancel), button("btn primary", "连接", submit)));

    Modal.open("需要密码", body, []);
    setTimeout(() => pw.focus(), 40);
}

// showZmodemBar shows a clickable banner on the terminal page when the remote
// rz is waiting for a file. The file picker must be opened from a real user
// gesture (the WebSocket push itself is not one), so we surface a button
// instead of calling input.click() directly.
function showZmodemBar(sessionId) {
    console.log("[zmodem] showZmodemBar sessionId =", sessionId);
    const t = Tabs.bySession(sessionId);
    if (!t || !t.page) {
        console.log("[zmodem] !!! no tab/page found for session", sessionId);
        return;
    }
    const old = t.page.querySelector(".zmodem-bar");
    if (old) old.remove();

    const bar = document.createElement("div");
    bar.className = "zmodem-bar";
    const label = document.createElement("span");
    label.textContent = "远端 rz 正在等待接收文件";
    const pick = button("btn primary", "选择文件…", () => {
        console.log("[zmodem] 用户点击“选择文件…”，准备打开文件选择框");
        const input = document.createElement("input");
        input.type = "file";
        input.onchange = async () => {
            const file = input.files[0];
            if (!file) return;
            console.log("[zmodem] 已选择文件:", file.name, file.size);
            bar.remove();
            if (t.term) t.term.writeln(`\r\n\x1b[90m[正在发送 ${file.name} …]\x1b[0m`);
            try {
                // 分块上传：避免把整个文件 base64 后塞进单条 WebSocket 消息，
                // 大文件在 WebView2 里会卡住。每块 1 MiB。
                const CHUNK = 1 << 20;
                const total = Math.ceil(file.size / CHUNK);
                await RPC.call("zmodem.sendBegin", {
                    sessionId,
                    name: file.name,
                    size: file.size,
                });
                for (let i = 0; i < total; i++) {
                    const slice = file.slice(i * CHUNK, Math.min((i + 1) * CHUNK, file.size));
                    const buf = await slice.arrayBuffer();
                    await RPC.call("zmodem.sendChunk", {
                        sessionId,
                        index: i,
                        data: base64Encode(buf),
                    });
                }
                await RPC.call("zmodem.sendEnd", { sessionId });
                if (t.term) t.term.writeln("\r\n\x1b[90m[发送中，等待远端完成…]\x1b[0m");
            } catch (e) {
                console.log("[zmodem] 发送失败:", e);
                if (t.term) t.term.writeln(`\r\n\x1b[90m[发送失败：${e.message}]\x1b[0m`);
            }
        };
        input.click(); // within a user gesture
        console.log("[zmodem] input.click() 已调用");
    });
    const cancel = button("btn", "取消", () => {
        bar.remove();
        RPC.call("zmodem.cancel", { sessionId }).catch(() => {});
    });
    bar.appendChild(label);
    bar.appendChild(pick);
    bar.appendChild(cancel);
    t.page.appendChild(bar);
}
