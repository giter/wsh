"use strict";

/* ============================================================
 * RPC client over WebSocket
 * ============================================================ */
const RPC = {
    id: 0,
    pending: new Map(),
    ws: null,

    connect() {
        const proto = location.protocol === "https:" ? "wss" : "ws";
        this.ws = new WebSocket(`${proto}://${location.host}/ws`);
        this.ws.onmessage = (ev) => {
            let msg;
            try {
                msg = JSON.parse(ev.data);
            } catch {
                return;
            }
            if (msg.type) {
                this.handlePush(msg);
                return;
            }
            const p = this.pending.get(msg.id);
            if (p) {
                this.pending.delete(msg.id);
                if (msg.ok) p.resolve(msg.data);
                else p.reject(new Error(msg.error || "请求失败"));
            }
        };
        return new Promise((res, rej) => {
            this.ws.onopen = res;
            this.ws.onerror = () => rej(new Error("无法连接后端服务"));
        });
    },

    call(method, params = {}) {
        return new Promise((resolve, reject) => {
            const id = ++this.id;
            this.pending.set(id, { resolve, reject });
            this.ws.send(JSON.stringify({ id, method, params }));
        });
    },

    // Inbound pushes from the backend (terminal output / exit).
    handlePush(msg) {
        if (msg.type && msg.type.indexOf("zmodem") === 0) {
            console.log("[zmodem] push:", JSON.stringify(msg));
        }
        if (msg.type === "terminal.data") {
            const t = Tabs.bySession(msg.sessionId);
            if (!t) return;
            if (t.term) t.term.write(msg.data || "");
            else (t.pending = t.pending || []).push(msg.data || ""); // buffer until the terminal is ready
        } else if (msg.type === "terminal.exit") {
            const t = Tabs.bySession(msg.sessionId);
            if (t) {
                if (t.term) t.term.writeln("\r\n\x1b[90m[连接已关闭]\x1b[0m");
                t.exited = true;
            }
        } else if (msg.type === "ui.navigate") {
            navigateTo(msg.page);
        } else if (msg.type === "ui.new-connection") {
            openConnEditor(null);
        } else if (msg.type === "ui.open-settings") {
            openSettings();
        } else if (msg.type === "zmodem.send-file") {
            showZmodemBar(msg.sessionId);
        } else if (msg.type === "zmodem.download-start") {
            // A download frame arrived; dismiss any pending upload banner.
            const t = Tabs.bySession(msg.sessionId);
            if (t && t.page) {
                const bar = t.page.querySelector(".zmodem-bar");
                if (bar) bar.remove();
            }
        } else if (msg.type === "zmodem.receive") {
            const t = Tabs.bySession(msg.sessionId);
            if (t && t.term && msg.name) {
                t.term.writeln(`\r\n\x1b[90m[已接收 ${msg.name}，正在保存…]\x1b[0m`);
            }
            if (msg.name) triggerDownload(msg.name, base64Decode(msg.data || ""));
        }
    },
};

// navigateTo switches the visible page on native menu requests.
function navigateTo(page) {
    switch (page) {
        case "connections":
            openConnectionsPage();
            break;
        case "sftp":
            openSftpPage();
            break;
        case "tunnels":
            openTunnelsPage();
            break;
        case "home":
        default:
            if (Tabs.items.length === 0) Tabs.showEmpty();
            else Tabs.select(Tabs.items[0].id);
    }
}

/* ============================================================
 * Modal helper
 * ============================================================ */
const Modal = {
    root: document.getElementById("modal-root"),

    open(title, bodyEl, actions = []) {
        this.close();
        const backdrop = document.createElement("div");
        backdrop.className = "modal-backdrop";
        const modal = document.createElement("div");
        modal.className = "modal";
        const h = document.createElement("h3");
        h.textContent = title;
        modal.appendChild(h);
        modal.appendChild(bodyEl);
        if (actions.length) {
            const act = document.createElement("div");
            act.className = "modal-actions";
            actions.forEach((b) => act.appendChild(b));
            modal.appendChild(act);
        }
        backdrop.appendChild(modal);
        backdrop.addEventListener("mousedown", (e) => {
            if (e.target === backdrop) this.close();
        });
        this.root.appendChild(backdrop);
        return modal;
    },

    close() {
        while (this.root.firstChild) this.root.removeChild(this.root.firstChild);
    },
};

/* ============================================================
 * Tab manager
 * ============================================================ */
const Tabs = {
    items: [], // {id, title, page, term, exited, onClose}
    active: null,
    barEl: document.getElementById("tabbar"),
    contentEl: document.getElementById("content"),
    _seq: 0,

    uid(prefix) {
        return `${prefix}-${++this._seq}`;
    },

    add(id, title, page, onClose) {
        const tab = { id, title, page, onClose, exited: false };
        this.items.push(tab);
        if (page) this.contentEl.appendChild(page);
        this.renderBar();
        this.select(id);
        return tab;
    },

    get(id) {
        return this.items.find((t) => t.id === id);
    },

    // bySession finds the tab whose live terminal session matches the id used
    // in backend pushes. Tabs are keyed by connId, while terminal sessions
    // carry their own random id returned from terminal.open.
    bySession(sessionId) {
        return this.items.find((t) => t.sessionId === sessionId);
    },

    select(id) {
        const t = this.get(id);
        if (!t) return;
        this.active = id;
        this.items.forEach((it) => {
            const show = it.id === id;
            if (it.page) it.page.style.display = show ? "flex" : "none";
        });
        this.renderBar();
    },

    close(id) {
        const idx = this.items.findIndex((t) => t.id === id);
        if (idx < 0) return;
        const t = this.items[idx];
        if (t.onClose) t.onClose(t);
        if (t.page) this.contentEl.removeChild(t.page);
        this.items.splice(idx, 1);
        if (this.active === id) {
            const next = this.items[Math.min(idx, this.items.length - 1)];
            this.active = next ? next.id : null;
        }
        this.renderBar();
        if (this.active) {
            const t2 = this.get(this.active);
            if (t2 && t2.page) t2.page.style.display = "flex";
        } else {
            this.showEmpty();
        }
    },

    renderBar() {
        this.barEl.innerHTML = "";
        this.items.forEach((t) => {
            const el = document.createElement("div");
            el.className = "tab" + (t.id === this.active ? " active" : "");
            const title = document.createElement("span");
            title.className = "tab-title";
            title.textContent = t.title;
            title.title = t.title;
            const close = document.createElement("button");
            close.className = "tab-close";
            close.textContent = "✕";
            close.addEventListener("click", (e) => {
                e.stopPropagation();
                this.close(t.id);
            });
            el.appendChild(title);
            el.appendChild(close);
            el.addEventListener("click", () => this.select(t.id));
            this.barEl.appendChild(el);
        });
    },

    showEmpty() {
        this.contentEl.innerHTML = "";
        const c = document.createElement("div");
        c.className = "center-box";
        c.innerHTML = "<div style='font-size:28px'>🖥️</div><div>从左侧选择连接或功能开始</div>";
        this.contentEl.appendChild(c);
    },
};



/* ============================================================
 * Boot
 * ============================================================ */
(async function boot() {
    try {
        await RPC.connect();
    } catch (e) {
        document.getElementById("content").innerHTML =
            `<div class="center-box"><div class="err">${esc(e.message)}</div></div>`;
        return;
    }
    Tabs.showEmpty();
    document.getElementById("btn-new-conn").addEventListener("click", () => openConnEditor(null));
    await loadSettings();
    refreshConnections().catch(() => {});
})();
