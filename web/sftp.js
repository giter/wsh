"use strict";

function openSftpPage() {
    const existing = Tabs.get("sftp-page");
    if (existing) {
        Tabs.select("sftp-page");
        return;
    }

    const page = document.createElement("div");
    page.className = "tab-page";
    const box = document.createElement("div");
    box.className = "page";
    box.style.padding = "12px 16px";
    page.appendChild(box);

    const state = { connId: null, local: [], localPath: "", remote: [], remotePath: "/" };

    box.innerHTML = `
    <div class="toolbar">
      <label class="muted">连接</label>
      <select id="sftp-conn" style="width:220px"></select>
      <button class="btn" id="sftp-upload">⇧ 上传文件</button>
      <span class="muted" id="sftp-status">选择连接以浏览远程文件</span>
    </div>
    <div class="sftp-body">
      <div class="sftp-pane">
        <div class="pane-head">
          <span class="muted">本地</span>
          <span class="pane-path" id="local-path"></span>
          <button class="icon-btn" title="上级" id="local-up">↑</button>
        </div>
        <div class="pane-list" id="local-list"></div>
      </div>
      <div class="sftp-pane">
        <div class="pane-head">
          <span class="muted">远程</span>
          <span class="pane-path" id="remote-path"></span>
          <button class="icon-btn" title="上级" id="remote-up">↑</button>
          <button class="icon-btn" title="新建目录" id="remote-mkdir">＋</button>
        </div>
        <div class="pane-list" id="remote-list"></div>
      </div>
    </div>`;

    const tab = Tabs.add("sftp-page", "文件传输", page, null);
    const $ = (id) => box.querySelector(id);

    const connSel = $("#sftp-conn");
    const populate = async () => {
        const conns = await RPC.call("connections.list");
        connCache = conns;
        connSel.innerHTML =
            `<option value="">选择连接…</option>` +
            conns.map((c) => `<option value="${c.id}">${esc(c.name)}</option>`).join("");
    };
    populate();

    const localPathEl = $("#local-path"),
        remotePathEl = $("#remote-path");
    const localListEl = $("#local-list"),
        remoteListEl = $("#remote-list");

    function escPath(p) {
        return p;
    }

    function localRow(entry) {
        const r = document.createElement("div");
        r.className = "sftp-entry";
        const icon = entry.isDir ? "📁" : "📄";
        r.innerHTML =
            `<span>${icon}</span><span class="entry-name">${esc(entry.name)}</span>` +
            (entry.isDir ? "" : `<span class="entry-size">${fmtSize(entry.size)}</span>`);
        if (entry.isDir) {
            r.addEventListener("dblclick", () => navigateLocal(entry.name));
        }
        localListEl.appendChild(r);
    }

    function remoteRow(entry) {
        const r = document.createElement("div");
        r.className = "sftp-entry";
        const icon = entry.isDir ? "📁" : "📄";
        r.innerHTML =
            `<span>${icon}</span><span class="entry-name">${esc(entry.name)}</span>` +
            (entry.isDir ? "" : `<span class="entry-size">${fmtSize(entry.size)}</span>`);
        if (entry.isDir) {
            r.addEventListener("dblclick", () => navigateRemote(entry.name));
        } else {
            const dl = button("btn small", "下载", () => download(entry.name));
            r.appendChild(dl);
        }
        remoteListEl.appendChild(r);
    }

    async function loadLocal(path) {
        try {
            const res = await RPC.call("fs.list", { path }); // empty => home dir
            state.localPath = res.path;
            state.local = res.entries;
            localPathEl.textContent = res.path;
            renderLocal();
        } catch (e) {
            status("读取本地失败：" + e.message, true);
        }
    }

    async function loadRemote(path) {
        if (!state.connId) {
            remoteListEl.innerHTML = `<div class="empty">请选择连接</div>`;
            return;
        }
        try {
            const res = await RPC.call("sftp.list", { connId: state.connId, path });
            state.remote = res.entries;
            state.remotePath = res.path;
            remotePathEl.textContent = res.path;
            renderRemote();
        } catch (e) {
            status("读取远程失败：" + e.message, true);
        }
    }

    function renderLocal() {
        localListEl.innerHTML = "";
        {
            const up = document.createElement("div");
            up.className = "sftp-entry";
            up.innerHTML = `<span>📁</span><span class="entry-name">..</span>`;
            up.addEventListener("click", () => navigateLocal(".."));
            localListEl.appendChild(up);
        }
        state.local.forEach(localRow);
    }

    function renderRemote() {
        remoteListEl.innerHTML = "";
        if (state.remotePath !== "/") {
            const up = document.createElement("div");
            up.className = "sftp-entry";
            up.innerHTML = `<span>📁</span><span class="entry-name">..</span>`;
            up.addEventListener("click", () => navigateRemote(".."));
            remoteListEl.appendChild(up);
        }
        state.remote.forEach(remoteRow);
    }

    function navigateLocal(name) {
        if (name === "..") loadLocal(dirOf(state.localPath));
        else loadLocal(joinLocal(state.localPath, name));
    }
    function navigateRemote(name) {
        if (name === "..") {
            if (state.remotePath === "/") return;
            const parts = state.remotePath.split("/").filter(Boolean);
            parts.pop();
            state.remotePath = "/" + parts.join("/");
        } else {
            state.remotePath = (state.remotePath === "/" ? "" : state.remotePath) + "/" + name;
        }
        loadRemote(state.remotePath);
    }

    function status(msg, isErr) {
        const el = $("#sftp-status");
        el.textContent = msg;
        el.style.color = isErr ? "var(--danger)" : "";
    }

    async function upload() {
        if (!state.connId) return status("请先选择连接", true);
        const input = document.createElement("input");
        input.type = "file";
        input.onchange = async () => {
            const file = input.files[0];
            if (!file) return;
            status(`上传 ${file.name} …`);
            const buf = await file.arrayBuffer();
            const b64 = base64Encode(buf);
            try {
                await RPC.call("sftp.upload", {
                    connId: state.connId,
                    remoteDir: state.remotePath,
                    name: file.name,
                    data: b64,
                });
                status(`已上传 ${file.name}`);
                loadRemote(state.remotePath);
            } catch (e) {
                status(e.message, true);
            }
        };
        input.click();
    }

    async function download(name) {
        if (!state.connId) return;
        const remote = (state.remotePath === "/" ? "" : state.remotePath) + "/" + name;
        status(`下载 ${name} …`);
        try {
            const res = await RPC.call("sftp.download", { connId: state.connId, path: remote });
            const bytes = base64Decode(res.data);
            triggerDownload(name, bytes);
            status(`已下载 ${name}`);
        } catch (e) {
            status(e.message, true);
        }
    }

    connSel.addEventListener("change", async () => {
        state.connId = connSel.value;
        if (!state.connId) {
            remoteListEl.innerHTML = "";
            return;
        }
        status("连接中…");
        try {
            await loadRemote("/");
            status("");
        } catch (e) {
            status(e.message, true);
        }
    });

    $("#local-up").addEventListener("click", () => navigateLocal(".."));
    $("#sftp-upload").addEventListener("click", () => upload());
    $("#remote-up").addEventListener("click", () => navigateRemote(".."));
    $("#remote-mkdir").addEventListener("click", async () => {
        const name = prompt("目录名：");
        if (!name) return;
        try {
            await RPC.call("sftp.mkdir", { connId: state.connId, parent: state.remotePath, name });
            loadRemote(state.remotePath);
        } catch (e) {
            status(e.message, true);
        }
    });

    // initial local dir (home)
    loadLocal("").catch(() => {});
}
