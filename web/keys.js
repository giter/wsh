"use strict";

// keyCache mirrors the managed keys returned by the backend. Only public
// metadata is kept here; the private material never reaches the browser.
let keyCache = [];

async function refreshKeys() {
    keyCache = await RPC.call("keys.list");
    return keyCache;
}

// renderKeysPage builds the key management page for the dedicated window opened
// from the native menu ("#keys"). Users submit their own private keys here;
// connections then reference a key by ID and use it first at login.
function renderKeysPage(container) {
    const page = document.createElement("div");
    page.className = "config-page";

    const head = document.createElement("header");
    head.className = "config-head";
    head.innerHTML = `
      <div class="config-head-icon">🔑</div>
      <div class="config-head-text">
        <h2>密钥管理</h2>
        <p>提交私钥，连接登录时优先使用密钥认证</p>
      </div>
      <div class="spacer"></div>`;
    head.appendChild(button("btn primary", "＋ 提交密钥", () => openKeyEditor(null)));
    page.appendChild(head);

    const body = document.createElement("div");
    body.className = "config-body";
    const listEl = document.createElement("div");
    listEl.id = "key-list";
    listEl.style.cssText = "display:flex;flex-direction:column;gap:12px;";
    body.appendChild(listEl);
    page.appendChild(body);

    const foot = document.createElement("footer");
    foot.className = "config-foot";
    const countEl = document.createElement("div");
    countEl.className = "muted";
    countEl.id = "key-count";
    foot.appendChild(countEl);
    page.appendChild(foot);

    container.appendChild(page);
    renderKeyList(listEl, countEl);
}

async function renderKeyList(listEl, countEl) {
    let keys;
    try {
        keys = await refreshKeys();
    } catch (e) {
        listEl.innerHTML = `<div class="empty-state err">${esc(e.message)}</div>`;
        return;
    }
    if (countEl) countEl.textContent = `共 ${keys.length} 个密钥`;
    listEl.innerHTML = "";
    if (!keys.length) {
        listEl.innerHTML = `<div class="empty-state">
          <div class="es-icon">🔑</div>
          <div class="es-title">还没有密钥</div>
          <div class="es-hint">点击右上角「提交密钥」导入你的私钥</div>
        </div>`;
        return;
    }
    keys.forEach((k) => listEl.appendChild(keyRow(k)));
}

function keyRow(k) {
    const card = document.createElement("div");
    card.className = "card key-card";

    const head = document.createElement("div");
    head.className = "key-card-head";
    const name = document.createElement("div");
    name.className = "key-name";
    name.innerHTML = `${esc(k.name)}${k.hasPassphrase ? ' <span class="badge on">已加密</span>' : ""}`;
    const actions = document.createElement("div");
    actions.className = "key-actions";
    actions.appendChild(button("btn small", "复制公钥", () => copyText(k.publicKey)));
    actions.appendChild(button("btn small", "编辑", () => openKeyEditor(k)));
    actions.appendChild(button("btn small danger", "删除", () => deleteKey(k)));
    head.appendChild(name);
    head.appendChild(actions);
    card.appendChild(head);

    const meta = document.createElement("div");
    meta.className = "key-meta";
    meta.innerHTML = `<span class="tag mono">${esc(k.keyType || "未知类型")}</span>` +
        `<span class="mono key-fp">${esc(k.fingerprint || "")}</span>`;
    card.appendChild(meta);

    const pub = document.createElement("div");
    pub.className = "key-pub mono";
    pub.textContent = k.publicKey || "（无公钥）";
    card.appendChild(pub);

    if (k.comment) {
        const c = document.createElement("div");
        c.className = "key-comment";
        c.textContent = k.comment;
        card.appendChild(c);
    }
    return card;
}

// copyText copies to the clipboard, falling back to a hidden textarea when the
// async Clipboard API is unavailable or denied inside the webview.
function copyText(text) {
    if (!text) return;
    if (navigator.clipboard && navigator.clipboard.writeText) {
        navigator.clipboard.writeText(text).catch(() => fallbackCopy(text));
        return;
    }
    fallbackCopy(text);
}
function fallbackCopy(text) {
    const ta = document.createElement("textarea");
    ta.value = text;
    ta.style.position = "fixed";
    ta.style.opacity = "0";
    document.body.appendChild(ta);
    ta.select();
    try {
        document.execCommand("copy");
    } catch (e) {
        /* ignore */
    }
    document.body.removeChild(ta);
}

// reRenderKeys refreshes the list on the key management page, if it is open.
function reRenderKeys() {
    const listEl = document.querySelector("#key-list");
    if (listEl) renderKeyList(listEl, document.querySelector("#key-count"));
}

// openKeyEditor shows the form for submitting a new key or editing an existing
// one. On edit, leaving the private-key box empty keeps the stored material.
function openKeyEditor(existing) {
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

    const name = input("例如：生产服务器");
    if (editing) name.value = existing.name;
    const comment = input("可选备注");
    if (editing) comment.value = existing.comment || "";

    const keyArea = document.createElement("textarea");
    keyArea.rows = 8;
    keyArea.spellcheck = false;
    keyArea.placeholder = "-----BEGIN OPENSSH PRIVATE KEY-----\n...";
    if (editing) keyArea.placeholder = "留空则保持原有私钥不变";

    const passphrase = input();
    passphrase.type = "password";
    passphrase.autocomplete = "off";
    passphrase.placeholder = "私钥未加密时留空";

    // File picker: read the key from disk into the textarea.
    const picker = document.createElement("input");
    picker.type = "file";
    picker.style.display = "none";
    picker.addEventListener("change", async () => {
        const file = picker.files[0];
        if (!file) return;
        keyArea.value = await file.text();
        if (!name.value.trim()) name.value = file.name.replace(/\.[^.]+$/, "");
    });

    f("名称", name);
    f("备注", comment);

    const keyLabel = document.createElement("label");
    keyLabel.className = "field";
    const keyHead = document.createElement("div");
    keyHead.className = "field-row";
    const keyTitle = document.createElement("span");
    keyTitle.textContent = editing ? "私钥内容（留空保持不变）" : "私钥内容";
    keyHead.appendChild(keyTitle);
    keyHead.appendChild(button("btn small", "从文件读取…", () => picker.click()));
    keyHead.appendChild(picker);
    keyLabel.appendChild(keyHead);
    keyLabel.appendChild(keyArea);
    body.appendChild(keyLabel);

    f("口令（私钥已加密时填写）", passphrase);

    const hint = document.createElement("div");
    hint.className = "hint";
    hint.textContent =
        "私钥仅保存在本机（加密落盘），不会上传。复制公钥并追加到服务器的 ~/.ssh/authorized_keys 即可用该密钥登录。";
    body.appendChild(hint);

    const status = document.createElement("div");
    status.className = "status-msg";
    body.appendChild(status);

    const save = button("btn primary", "保存", async () => {
        status.classList.remove("err");
        const payload = {
            id: editing ? existing.id : "",
            name: name.value.trim(),
            comment: comment.value.trim(),
            privateKey: keyArea.value,
            passphrase: passphrase.value,
        };
        if (!payload.name) {
            status.classList.add("err");
            status.textContent = "请输入密钥名称";
            return;
        }
        if (!editing && !payload.privateKey.trim()) {
            status.classList.add("err");
            status.textContent = "请提交私钥内容";
            return;
        }
        try {
            await RPC.call("keys.save", payload);
            Modal.close();
            reRenderKeys();
        } catch (e) {
            status.classList.add("err");
            status.textContent = e.message;
        }
    });

    Modal.open(editing ? "编辑密钥" : "提交密钥", body, [
        button("btn", "取消", () => Modal.close()),
        save,
    ]);
}

async function deleteKey(k) {
    const body = document.createElement("div");
    body.innerHTML = `<p>删除密钥 <b>${esc(k.name)}</b>？</p><p class="muted" style="margin-top:6px">使用该密钥的连接将不再使用它，改回密码或其他方式认证。</p>`;
    const del = button("btn danger", "删除", async () => {
        try {
            await RPC.call("keys.delete", { id: k.id });
        } catch (e) {
            /* ignore */
        }
        Modal.close();
        reRenderKeys();
    });
    Modal.open("删除密钥", body, [button("btn", "取消", () => Modal.close()), del]);
}
