"use strict";

function openTunnelsPage() {
    const existing = Tabs.get("tunnels-page");
    if (existing) {
        Tabs.select("tunnels-page");
        return;
    }

    const page = document.createElement("div");
    page.className = "tab-page";
    page.style.overflow = "auto";
    const box = document.createElement("div");
    box.className = "page";
    box.innerHTML = `<h2>端口隧道</h2>
    <div class="toolbar"><button class="btn primary" id="tunnel-add">＋ 新建隧道</button></div>
    <div class="list" id="tunnel-list"></div>`;
    page.appendChild(box);
    const tab = Tabs.add("tunnels-page", "端口隧道", page, null);
    box.querySelector("#tunnel-add").addEventListener("click", () => openTunnelEditor(null));
    renderTunnelList(box.querySelector("#tunnel-list"));
}

async function renderTunnelList(listEl) {
    let tunnels;
    try {
        tunnels = await RPC.call("tunnels.list");
    } catch (e) {
        return;
    }
    listEl.innerHTML = "";
    if (!tunnels.length) {
        listEl.innerHTML = `<div class="empty">还没有隧道。点击「新建隧道」创建端口转发规则。</div>`;
        return;
    }
    tunnels.forEach((t) => {
        const row = document.createElement("div");
        row.className = "row";
        const state = t.running ? "on" : "off";
        const dirLabel = t.direction === "remote" ? "远程转发" : "本地转发";
        const flow = t.direction === "remote"
            ? `${esc(t.remoteAddress)}:${t.remotePort} → ${esc(t.localAddress)}:${t.localPort}`
            : `${esc(t.localAddress)}:${t.localPort} → ${esc(t.remoteAddress)}:${t.remotePort}`;
        const remarkHtml = t.remark
            ? `<div class="row-sub remark">${esc(t.remark)}</div>`
            : "";
        row.innerHTML = `
      <span class="dot" style="background:${t.running ? "var(--accent)" : "var(--faint)"}"></span>
      <div class="row-main">
        <div class="row-title">${flow}</div>
        <div class="row-sub">${esc(t.name)} · ${dirLabel} · 通过 ${esc(t.connectionName)}</div>
        ${remarkHtml}
      </div>
      <span class="badge ${state}">${t.running ? "转发中" : "已停止"}</span>
      <div class="row-actions">
        <button class="btn small" data-act="toggle">${t.running ? "停止" : "启动"}</button>
        <button class="icon-btn" title="编辑" data-act="edit">✎</button>
        <button class="icon-btn danger" title="删除" data-act="del">🗑</button>
      </div>`;
        row.querySelector('[data-act="toggle"]').addEventListener("click", async () => {
            if (t.running) await RPC.call("tunnels.stop", { id: t.id });
            else {
                try {
                    await RPC.call("tunnels.start", { id: t.id });
                } catch (e) {
                    alert(e.message);
                }
            }
            renderTunnelList(listEl);
        });
        row.querySelector('[data-act="edit"]').addEventListener("click", () => openTunnelEditor(t));
        row.querySelector('[data-act="del"]').addEventListener("click", async () => {
            await RPC.call("tunnels.delete", { id: t.id });
            renderTunnelList(listEl);
        });
        listEl.appendChild(row);
    });
}

function openTunnelEditor(existing) {
    const editing = !!existing;
    const body = document.createElement("div");
    const f = (label, input) => {
        const l = document.createElement("label");
        l.className = "field";
        const s = document.createElement("span");
        s.textContent = label;
        l.appendChild(s);
        l.appendChild(input);
        body.appendChild(l);
        return s;
    };
    const name = input();
    const connSel = document.createElement("select");
    const direction = document.createElement("select");
    direction.innerHTML = `
        <option value="local">本地转发（本地监听 → 远程）</option>
        <option value="remote">远程转发（远程监听 → 本地）</option>`;
    const localAddr = input();
    localAddr.value = "127.0.0.1";
    const localPort = input();
    localPort.value = String((settingsCache && settingsCache.tunnelLocalPort) || 8080);
    const remoteAddr = input();
    remoteAddr.value = "127.0.0.1";
    const remotePort = input();
    remotePort.value = String((settingsCache && settingsCache.tunnelRemotePort) || 80);
    const remark = input("备注（可选）");

    if (editing) {
        name.value = existing.name;
        direction.value = existing.direction || "local";
        localAddr.value = existing.localAddress;
        localPort.value = existing.localPort;
        remoteAddr.value = existing.remoteAddress;
        remotePort.value = existing.remotePort;
        remark.value = existing.remark || "";
    }

    RPC.call("connections.list").then((conns) => {
        connCache = conns;
        connSel.innerHTML = conns.map((c) => `<option value="${c.id}">${esc(c.name)}</option>`).join("");
        if (editing) connSel.value = existing.connectionId;
        else if (conns.length) connSel.value = conns[0].id;
    });

    f("名称", name);
    f("连接", connSel);
    f("方向", direction);
    const localAddrLbl = f("本地监听地址", localAddr);
    const localPortLbl = f("本地监听端口", localPort);
    const remoteAddrLbl = f("远程目标地址", remoteAddr);
    const remotePortLbl = f("远程目标端口", remotePort);
    f("备注", remark);

    const updateLabels = () => {
        if (direction.value === "remote") {
            localAddrLbl.textContent = "本地目标地址";
            localPortLbl.textContent = "本地目标端口";
            remoteAddrLbl.textContent = "远程监听地址";
            remotePortLbl.textContent = "远程监听端口";
        } else {
            localAddrLbl.textContent = "本地监听地址";
            localPortLbl.textContent = "本地监听端口";
            remoteAddrLbl.textContent = "远程目标地址";
            remotePortLbl.textContent = "远程目标端口";
        }
    };
    direction.addEventListener("change", updateLabels);
    updateLabels();

    const status = document.createElement("div");
    status.className = "status-msg";
    body.appendChild(status);

    const save = button("btn primary", "保存", async () => {
        status.classList.remove("err");
        const payload = {
            id: editing ? existing.id : "",
            name: name.value.trim(),
            connectionId: connSel.value,
            direction: direction.value,
            localAddress: localAddr.value.trim(),
            localPort: parseInt(localPort.value || "0", 10),
            remoteAddress: remoteAddr.value.trim(),
            remotePort: parseInt(remotePort.value || "0", 10),
            remark: remark.value.trim(),
        };
        try {
            await RPC.call("tunnels.save", payload);
            Modal.close();
            renderTunnelList(document.querySelector("#tunnel-list"));
        } catch (e) {
            status.classList.add("err");
            status.textContent = e.message;
        }
    });

    Modal.open(editing ? "编辑隧道" : "新建隧道", body, [button("btn", "取消", () => Modal.close()), save]);
}
