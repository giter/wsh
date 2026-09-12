"use strict";

let connCache = [];
let folderCache = [];

async function refreshConnections() {
    connCache = await RPC.call("connections.list");
    try {
        folderCache = await RPC.call("folders.list");
    } catch (e) {
        folderCache = [];
    }
    renderTree();
}

function renderTree() {
    const tree = document.getElementById("tree");
    tree.innerHTML = "";
    tree.appendChild(
        treeBranch(
            "连接",
            [
                { type: "action", label: "＋ 新建连接", onClick: () => openConnEditor(null) },
                { type: "action", label: "连接管理", onClick: () => openConnectionsPage() },
                ...folderTreeNodes(),
            ],
            true,
        ),
    );
    tree.appendChild(treeLeaf({ icon: "📁", label: "文件传输", onClick: () => openSftpPage() }));
    tree.appendChild(treeLeaf({ icon: "🔗", label: "端口隧道", onClick: () => openTunnelsPage() }));
}

// folderTreeNodes builds one collapsible branch per folder plus an "未分组"
// branch for connections without a folder.
function folderTreeNodes() {
    const nodes = [];
    for (const f of folderCache) {
        nodes.push({
            type: "folder",
            label: f.name,
            conns: connCache.filter((c) => c.folderId === f.id),
        });
    }
    const ungrouped = connCache.filter((c) => !c.folderId);
    if (ungrouped.length) {
        nodes.push({ type: "folder", label: "未分组", conns: ungrouped });
    }
    return nodes;
}

function treeBranch(label, children, open = false) {
    const node = document.createElement("div");
    node.className = "tree-node";

    const row = document.createElement("div");
    row.className = "tree-row branch";
    row.innerHTML = `<span class="caret ${open ? "open" : ""}">▶</span><span class="lbl">${esc(label)}</span>`;
    const kids = document.createElement("div");
    kids.className = "tree-children";
    kids.style.display = open ? "block" : "none";

    row.addEventListener("click", () => {
        const isOpen = kids.style.display !== "none";
        kids.style.display = isOpen ? "none" : "block";
        row.querySelector(".caret").classList.toggle("open", !isOpen);
    });

    children.forEach((ch) => {
        if (ch.type === "conn") {
            const r = document.createElement("div");
            r.className = "tree-row conn";
            const color = ch.conn.color || "#34d399";
            r.innerHTML = `<span class="dot" style="background:${color}"></span><span class="lbl">${esc(ch.label)}</span>`;
            r.title = `${ch.conn.user}@${ch.conn.host}:${ch.conn.port}`;
            r.addEventListener("click", (e) => {
                e.stopPropagation();
                ch.onClick();
            });
            kids.appendChild(r);
        } else if (ch.type === "folder") {
            // Nested folder branch: connections grouped under it.
            const sub = treeBranch(
                ch.label,
                ch.conns.map((c) => ({
                    type: "conn",
                    conn: c,
                    label: c.name,
                    onClick: () => openTerminal(c),
                })),
                false,
            );
            kids.appendChild(sub);
        } else {
            const r = document.createElement("div");
            r.className = "tree-row action";
            r.innerHTML = `<span class="lbl">${esc(ch.label)}</span>`;
            r.addEventListener("click", (e) => {
                e.stopPropagation();
                ch.onClick();
            });
            kids.appendChild(r);
        }
    });

    node.appendChild(row);
    node.appendChild(kids);
    return node;
}

function treeLeaf({ icon, label, onClick }) {
    const r = document.createElement("div");
    r.className = "tree-row";
    r.innerHTML = `<span class="caret"></span><span class="lbl">${icon} ${esc(label)}</span>`;
    r.addEventListener("click", onClick);
    return r;
}

function openTerminal(conn) {
    // Reuse an existing tab for the same connection if it is still open.
    const existing = Tabs.get(conn.id);
    if (existing) {
        Tabs.select(conn.id);
        return;
    }
    makeTerminalPage(conn.id, conn.name);
}

/* ============================================================
 * Connections management page
 * ============================================================ */
function openConnectionsPage() {
    const existing = Tabs.get("conns-page");
    if (existing) {
        Tabs.select("conns-page");
        return;
    }

    const page = document.createElement("div");
    page.className = "tab-page";
    page.style.overflow = "auto";
    const box = document.createElement("div");
    box.className = "page";
    box.innerHTML = `<h2>连接管理</h2>
    <div class="toolbar">
      <button class="btn primary" id="conn-add">＋ 新建连接</button>
      <button class="btn" id="folder-add">＋ 新建文件夹</button>
      <span class="muted" id="conn-count"></span>
    </div>
    <div class="list" id="conn-list"></div>`;
    page.appendChild(box);
    const tab = Tabs.add("conns-page", "连接管理", page, null);
    box.querySelector("#conn-add").addEventListener("click", () => openConnEditor(null));
    box.querySelector("#folder-add").addEventListener("click", () => addFolder());
    renderConnList(box.querySelector("#conn-list"), box.querySelector("#conn-count"));
}

async function renderConnList(listEl, countEl) {
    let conns, folders;
    try {
        [conns, folders] = await Promise.all([
            RPC.call("connections.list"),
            RPC.call("folders.list"),
        ]);
    } catch (e) {
        return;
    }
    connCache = conns;
    folderCache = folders;
    if (countEl) countEl.textContent = `${conns.length} 台服务器 · ${folders.length} 个文件夹`;
    renderTree();
    listEl.innerHTML = "";
    if (!conns.length && !folders.length) {
        listEl.innerHTML = `<div class="empty">还没有保存的连接。点击「新建连接」开始。</div>`;
        return;
    }
    const byFolder = (fid) => conns.filter((c) => (c.folderId || "") === (fid || ""));
    for (const f of folders) {
        listEl.appendChild(folderSection(f, byFolder(f.id)));
    }
    const ungrouped = byFolder("");
    if (ungrouped.length || !folders.length) {
        listEl.appendChild(folderSection(null, ungrouped));
    }
}

// folderSection renders one folder block (header + connection rows) in the
// connections management page. A null folder is the "未分组" section.
function folderSection(f, conns) {
    const sec = document.createElement("div");
    sec.className = "conn-folder";
    const head = document.createElement("div");
    head.className = "folder-head";
    const title = document.createElement("span");
    title.className = "folder-title";
    title.textContent = f ? f.name : "未分组";
    head.appendChild(title);
    if (f) {
        head.appendChild(button("btn small", "重命名", () => renameFolder(f)));
        head.appendChild(button("btn small danger", "删除", () => deleteFolder(f)));
    }
    sec.appendChild(head);
    if (!conns.length) {
        const empty = document.createElement("div");
        empty.className = "empty";
        empty.textContent = "（空）";
        sec.appendChild(empty);
        return sec;
    }
    conns.forEach((c) => sec.appendChild(connRow(c)));
    return sec;
}

function connRow(c) {
    const row = document.createElement("div");
    row.className = "row";
    const color = c.color || "#34d399";
    row.innerHTML = `
      <span class="dot" style="background:${color}"></span>
      <div class="row-main">
        <div class="row-title">${esc(c.name)}</div>
        <div class="row-sub">${esc(c.user)}@${esc(c.host)}:${c.port}</div>
      </div>
      <div class="row-actions">
        <button class="btn small" data-act="conn">连接</button>
        <button class="btn small" data-act="move">移动</button>
        <button class="icon-btn" title="编辑" data-act="edit">✎</button>
        <button class="icon-btn danger" title="删除" data-act="del">🗑</button>
      </div>`;
    row.querySelector('[data-act="conn"]').addEventListener("click", () => openTerminal(c));
    row.querySelector('[data-act="move"]').addEventListener("click", () => moveConn(c));
    row.querySelector('[data-act="edit"]').addEventListener("click", () => openConnEditor(c));
    row.querySelector('[data-act="del"]').addEventListener("click", () => deleteConn(c));
    return row;
}

async function addFolder() {
    const name = prompt("文件夹名称：");
    if (!name || !name.trim()) return;
    await RPC.call("folders.save", { name: name.trim() });
    renderConnList(document.querySelector("#conn-list"), document.querySelector("#conn-count"));
}

async function renameFolder(f) {
    const name = prompt("重命名文件夹：", f.name);
    if (!name || !name.trim()) return;
    await RPC.call("folders.save", { id: f.id, name: name.trim() });
    renderConnList(document.querySelector("#conn-list"), document.querySelector("#conn-count"));
}

async function deleteFolder(f) {
    const body = document.createElement("div");
    body.innerHTML = `<p>删除文件夹 <b>${esc(f.name)}</b>？</p><p class="muted" style="margin-top:6px">文件夹内的连接不会被删除，只会移回「未分组」。</p>`;
    const del = button("btn danger", "删除", async () => {
        await RPC.call("folders.delete", { id: f.id });
        Modal.close();
        renderConnList(document.querySelector("#conn-list"), document.querySelector("#conn-count"));
    });
    Modal.open("删除文件夹", body, [button("btn", "取消", () => Modal.close()), del]);
}

// moveConn moves a connection into another folder via a small modal.
function moveConn(c) {
    const body = document.createElement("div");
    const sel = document.createElement("select");
    sel.innerHTML =
        `<option value="">未分组</option>` +
        folderCache.map((f) => `<option value="${f.id}">${esc(f.name)}</option>`).join("");
    sel.value = c.folderId || "";
    const lbl = document.createElement("label");
    lbl.className = "field";
    const s = document.createElement("span");
    s.textContent = "移动到";
    lbl.appendChild(s);
    lbl.appendChild(sel);
    body.appendChild(lbl);
    const save = button("btn primary", "保存", async () => {
        await RPC.call("connections.save", { ...c, folderId: sel.value });
        Modal.close();
        renderConnList(document.querySelector("#conn-list"), document.querySelector("#conn-count"));
    });
    Modal.open(`移动「${c.name}」`, body, [button("btn", "取消", () => Modal.close()), save]);
}

async function deleteConn(c) {
    const body = document.createElement("div");
    body.innerHTML = `<p>删除连接 <b>${esc(c.name)}</b>？</p><p class="muted" style="margin-top:6px">该操作无法撤销，其依赖的隧道也会一并移除。</p>`;
    const del = button("btn danger", "删除", async () => {
        await RPC.call("connections.delete", { id: c.id });
        Modal.close();
        refreshConnections();
        const t = Tabs.get(c.id);
        if (t) Tabs.close(c.id);
    });
    Modal.open("删除连接", body, [button("btn", "取消", () => Modal.close()), del]);
}

/* ---------- Connection editor modal ---------- */
function openConnEditor(existing) {
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
    };
    const name = input();
    const host = input("example.com");
    const port = input();
    port.value = String((settingsCache && settingsCache.defaultPort) || 22);
    const user = input("root");
    user.value = (settingsCache && settingsCache.defaultUser) || "";
    const password = input();
    password.type = "password";
    password.autocomplete = "off";
    const savePw = document.createElement("input");
    savePw.type = "checkbox";
    const keyPath = input("可选：/path/to/id_rsa");
    const folderSel = document.createElement("select");
    folderSel.innerHTML =
        `<option value="">未分组</option>` +
        folderCache.map((f) => `<option value="${f.id}">${esc(f.name)}</option>`).join("");

    if (editing) {
        name.value = existing.name;
        host.value = existing.host;
        port.value = existing.port;
        user.value = existing.user;
        savePw.checked = existing.savePassword;
        keyPath.value = existing.privateKeyPath || "";
        folderSel.value = existing.folderId || "";
    }

    f("名称", name);
    f("文件夹", folderSel);
    f("主机", host);
    f("端口", port);
    f("用户", user);
    f("密码", password);
    const ck = document.createElement("label");
    ck.className = "check-row";
    ck.appendChild(savePw);
    ck.appendChild(document.createTextNode("保存密码（加密落盘）"));
    body.appendChild(ck);
    f("私钥路径", keyPath);

    const status = document.createElement("div");
    status.className = "status-msg";
    body.appendChild(status);

    const testBtn = button("btn", "测试连接", async () => {
        status.classList.remove("err");
        status.textContent = "测试中…";
        try {
            await RPC.call("connections.test", {
                id: editing ? existing.id : "",
                host: host.value,
                port: parseInt(port.value || "22", 10),
                user: user.value,
                password: password.value,
                keyPath: keyPath.value,
            });
            status.textContent = "连接成功 ✓";
        } catch (e) {
            status.classList.add("err");
            status.textContent = e.message;
        }
    });

    const save = button("btn primary", "保存", async () => {
        status.classList.remove("err");
        const payload = {
            id: editing ? existing.id : "",
            name: name.value.trim(),
            host: host.value.trim(),
            port: parseInt(port.value || "22", 10),
            user: user.value.trim(),
            folderId: folderSel.value,
            password: password.value,
            savePassword: savePw.checked,
            privateKeyPath: keyPath.value.trim(),
        };
        try {
            await RPC.call("connections.save", payload);
            Modal.close();
            refreshConnections();
        } catch (e) {
            status.classList.add("err");
            status.textContent = e.message;
        }
    });

    Modal.open(editing ? "编辑连接" : "新建连接", body, [button("btn", "取消", () => Modal.close()), testBtn, save]);
}
