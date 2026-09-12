"use strict";

function button(cls, text, onClick) {
    const b = document.createElement("button");
    b.className = cls;
    b.textContent = text;
    b.addEventListener("click", onClick);
    return b;
}
function input(placeholder) {
    const i = document.createElement("input");
    if (placeholder !== undefined) i.placeholder = placeholder;
    return i;
}
function containerHBox() {
    const d = document.createElement("div");
    d.style.cssText = "display:flex;gap:8px;justify-content:flex-end;margin-top:16px;";
    for (const child of arguments) d.appendChild(child);
    return d;
}
function esc(s) {
    return String(s).replace(
        /[&<>"']/g,
        (c) =>
            ({
                "&": "&amp;",
                "<": "&lt;",
                ">": "&gt;",
                '"': "&quot;",
                "'": "&#39;",
            })[c],
    );
}
function fmtSize(n) {
    if (n < 1024) return n + " B";
    const u = ["KB", "MB", "GB", "TB"];
    let v = n / 1024,
        i = 0;
    while (v >= 1024 && i < u.length - 1) {
        v /= 1024;
        i++;
    }
    return v.toFixed(1) + " " + u[i];
}

// Local path helpers. The server returns absolute paths that may use either
// separator; we handle both.
function dirOf(path) {
    if (!path) return path;
    const sep = path.includes("\\") ? "\\" : "/";
    const idx = path.lastIndexOf(sep);
    if (idx <= 0) return path.slice(0, idx + 1); // keep drive root / unix root
    return path.slice(0, idx);
}
function joinLocal(dir, name) {
    const sep = dir.includes("\\") ? "\\" : "/";
    if (dir.endsWith("/") || dir.endsWith("\\")) return dir + name;
    return dir + sep + name;
}
function base64Encode(buf) {
    const bytes = new Uint8Array(buf);
    let bin = "";
    const chunk = 0x8000;
    for (let i = 0; i < bytes.length; i += chunk) {
        bin += String.fromCharCode.apply(null, bytes.subarray(i, i + chunk));
    }
    return btoa(bin);
}
function base64Decode(b64) {
    const bin = atob(b64);
    const bytes = new Uint8Array(bin.length);
    for (let i = 0; i < bin.length; i++) bytes[i] = bin.charCodeAt(i);
    return bytes;
}
function triggerDownload(name, bytes) {
    console.log("[zmodem] triggerDownload:", name, bytes && bytes.length);
    const blob = new Blob([bytes]);
    const url = URL.createObjectURL(blob);
    const a = document.createElement("a");
    a.href = url;
    a.download = name;
    document.body.appendChild(a);
    a.click();
    document.body.removeChild(a);
    URL.revokeObjectURL(url);
    console.log("[zmodem] 下载已触发（a.click 完成）");
}
