// RPC client: JSON over the WebSocket served by the Go backend.
//
// Requests are promise-based; backend pushes (terminal output, menu events, ...)
// are delivered to subscribers registered with `on(type, handler)`.

class RpcClient {
    constructor() {
        this.nextId = 0;
        this.pending = new Map();
        this.handlers = new Map();
        this.ws = null;
    }

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
                this.dispatch(msg);
                return;
            }
            const p = this.pending.get(msg.id);
            if (!p) return;
            this.pending.delete(msg.id);
            if (msg.ok) p.resolve(msg.data);
            else p.reject(new Error(msg.error || "请求失败"));
        };

        return new Promise((resolve, reject) => {
            this.ws.onopen = () => resolve();
            this.ws.onerror = () => reject(new Error("无法连接后端服务"));
        });
    }

    call(method, params = {}) {
        return new Promise((resolve, reject) => {
            if (!this.ws || this.ws.readyState !== WebSocket.OPEN) {
                reject(new Error("后端连接已断开"));
                return;
            }
            const id = ++this.nextId;
            this.pending.set(id, { resolve, reject });
            this.ws.send(JSON.stringify({ id, method, params }));
        });
    }

    dispatch(msg) {
        const set = this.handlers.get(msg.type);
        if (!set) return;
        for (const fn of set) {
            try {
                fn(msg);
            } catch (err) {
                console.error("[rpc] push handler failed:", err);
            }
        }
    }

    // on subscribes to a push message type and returns an unsubscribe function.
    on(type, fn) {
        let set = this.handlers.get(type);
        if (!set) {
            set = new Set();
            this.handlers.set(type, set);
        }
        set.add(fn);
        return () => set.delete(fn);
    }
}

export const rpc = new RpcClient();
