"use strict";

let settingsCache = {};

// applySettings pushes global options into the live UI: font size (via the
// html font-size so all rem/em-based text follows) and theme (data-theme).
function applySettings(st) {
    settingsCache = st || {};
    const fs = parseInt(settingsCache.fontSize, 10);
    document.documentElement.style.fontSize = fs >= 8 && fs <= 32 ? fs + "px" : "";
    document.documentElement.dataset.theme = settingsCache.theme === "light" ? "light" : "dark";
}

async function loadSettings() {
    try {
        applySettings(await RPC.call("settings.get"));
    } catch (e) {
        applySettings({});
    }
}

// persistSettings writes the current settings back with a debounce so rapid
// font zoom keystrokes do not hammer the disk.
let zoomSaveTimer = null;
function persistSettings() {
    clearTimeout(zoomSaveTimer);
    zoomSaveTimer = setTimeout(() => {
        RPC.call("settings.save", {
            fontSize: settingsCache.fontSize || 0,
            theme: settingsCache.theme || "dark",
            defaultPort: settingsCache.defaultPort || 0,
            defaultUser: settingsCache.defaultUser || "",
            tunnelLocalPort: settingsCache.tunnelLocalPort || 0,
            tunnelRemotePort: settingsCache.tunnelRemotePort || 0,
        }).catch(() => {});
    }, 400);
}

// zoomFont changes the global font size by delta px and remembers it.
function zoomFont(delta) {
    const base = parseInt(settingsCache.fontSize, 10) >= 8 ? settingsCache.fontSize : 13;
    const cur = parseInt(document.documentElement.style.fontSize, 10) || base;
    const next = Math.min(32, Math.max(8, cur + delta));
    document.documentElement.style.fontSize = next + "px";
    settingsCache.fontSize = next;
    persistSettings();
}

// renderSettingsPage builds the global options page for the dedicated settings
// window (opened from the native menu, "#settings"). Font size and theme
// preview live and can be reverted with「恢复」.
function renderSettingsPage(container) {
    const page = document.createElement("div");
    page.className = "config-page";

    // Header
    const head = document.createElement("header");
    head.className = "config-head";
    head.innerHTML = `
      <div class="config-head-icon">⚙</div>
      <div class="config-head-text">
        <h2>选项</h2>
        <p>调整界面外观与新连接、隧道的默认值</p>
      </div>`;
    page.appendChild(head);

    // Body
    const body = document.createElement("div");
    body.className = "config-body";
    page.appendChild(body);

    const field = (label, input) => {
        const l = document.createElement("label");
        l.className = "field";
        const s = document.createElement("span");
        s.textContent = label;
        l.appendChild(s);
        l.appendChild(input);
        return l;
    };
    const card = (title, ...fields) => {
        const sec = document.createElement("section");
        sec.className = "card";
        const h = document.createElement("h3");
        h.textContent = title;
        sec.appendChild(h);
        const grid = document.createElement("div");
        grid.className = "grid";
        fields.forEach((f) => grid.appendChild(f));
        sec.appendChild(grid);
        return sec;
    };

    const fontSize = input();
    fontSize.type = "number";
    fontSize.min = "8";
    fontSize.max = "32";
    fontSize.value = settingsCache.fontSize || 13;

    const theme = document.createElement("select");
    theme.innerHTML = `<option value="dark">暗色</option><option value="light">亮色</option>`;
    theme.value = settingsCache.theme === "light" ? "light" : "dark";

    const defaultPort = input();
    defaultPort.type = "number";
    defaultPort.min = "1";
    defaultPort.max = "65535";
    defaultPort.value = settingsCache.defaultPort || 22;

    const defaultUser = input("root");
    defaultUser.value = settingsCache.defaultUser || "";

    const tunLocal = input();
    tunLocal.type = "number";
    tunLocal.min = "1";
    tunLocal.max = "65535";
    tunLocal.value = settingsCache.tunnelLocalPort || 8080;

    const tunRemote = input();
    tunRemote.type = "number";
    tunRemote.min = "1";
    tunRemote.max = "65535";
    tunRemote.value = settingsCache.tunnelRemotePort || 80;

    body.appendChild(card("外观", field("字体大小 (px)", fontSize), field("主题", theme)));
    body.appendChild(card("新连接默认值", field("默认端口", defaultPort), field("默认用户", defaultUser)));
    body.appendChild(card("隧道默认值", field("本地端口", tunLocal), field("远程端口", tunRemote)));

    // Footer
    const foot = document.createElement("footer");
    foot.className = "config-foot";
    const status = document.createElement("div");
    status.className = "status-msg";
    const actions = document.createElement("div");
    actions.className = "foot-actions";
    foot.appendChild(status);
    foot.appendChild(actions);
    page.appendChild(foot);

    // Live preview while editing (「恢复」restores the stored values).
    fontSize.addEventListener("input", () => {
        const v = parseInt(fontSize.value, 10);
        document.documentElement.style.fontSize = v >= 8 && v <= 32 ? v + "px" : "";
    });
    theme.addEventListener("change", () => {
        document.documentElement.dataset.theme = theme.value;
    });

    const save = button("btn primary", "保存", async () => {
        const payload = {
            fontSize: parseInt(fontSize.value, 10) || 0,
            theme: theme.value,
            defaultPort: parseInt(defaultPort.value, 10) || 0,
            defaultUser: defaultUser.value.trim(),
            tunnelLocalPort: parseInt(tunLocal.value, 10) || 0,
            tunnelRemotePort: parseInt(tunRemote.value, 10) || 0,
        };
        status.classList.remove("err");
        try {
            settingsCache = await RPC.call("settings.save", payload);
            applySettings(settingsCache);
            status.textContent = "已保存";
        } catch (e) {
            status.classList.add("err");
            status.textContent = e.message;
        }
    });
    const reset = button("btn", "恢复", () => {
        applySettings(settingsCache);
        container.innerHTML = "";
        renderSettingsPage(container);
    });
    actions.appendChild(reset);
    actions.appendChild(save);

    container.appendChild(page);
}

/* ============================================================
 * Global keyboard shortcuts & context menu
 * ============================================================ */
// Ctrl/Cmd + =/+/, zooms in, Ctrl/Cmd + - zooms out, Ctrl/Cmd + 0 resets.
document.addEventListener("keydown", (e) => {
    if (!(e.ctrlKey || e.metaKey)) return;
    const k = e.key;
    if (k === "=" || k === "+") {
        e.preventDefault();
        zoomFont(1);
    } else if (k === "," && !e.shiftKey) {
        // Ctrl/Cmd+, zooms; Ctrl/Cmd+Shift+, opens the options window (chrome.js).
        e.preventDefault();
        zoomFont(1);
    } else if (k === "-" || k === "_") {
        e.preventDefault();
        zoomFont(-1);
    } else if (k === "0") {
        e.preventDefault();
        const base = parseInt(settingsCache.fontSize, 10) >= 8 ? settingsCache.fontSize : 13;
        document.documentElement.style.fontSize = base + "px";
    }
});

// Suppress the webview's default right-click context menu.
document.addEventListener("contextmenu", (e) => e.preventDefault());
