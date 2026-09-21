// Small formatting / path / encoding helpers shared across the UI.

export function fmtSize(n) {
    if (n < 1024) return n + " B";
    const units = ["KB", "MB", "GB", "TB"];
    let v = n / 1024;
    let i = 0;
    while (v >= 1024 && i < units.length - 1) {
        v /= 1024;
        i++;
    }
    return v.toFixed(1) + " " + units[i];
}

// Local path helpers. The server returns absolute paths that may use either
// separator; both are handled.
export function dirOf(path) {
    if (!path) return path;
    const sep = path.includes("\\") ? "\\" : "/";
    const idx = path.lastIndexOf(sep);
    if (idx <= 0) return path.slice(0, idx + 1); // keep drive root / unix root
    return path.slice(0, idx);
}

export function joinLocal(dir, name) {
    const sep = dir.includes("\\") ? "\\" : "/";
    if (dir.endsWith("/") || dir.endsWith("\\")) return dir + name;
    return dir + sep + name;
}

export function base64Encode(buf) {
    const bytes = new Uint8Array(buf);
    let bin = "";
    const chunk = 0x8000;
    for (let i = 0; i < bytes.length; i += chunk) {
        bin += String.fromCharCode.apply(null, bytes.subarray(i, i + chunk));
    }
    return btoa(bin);
}

export function base64Decode(b64) {
    const bin = atob(b64);
    const bytes = new Uint8Array(bin.length);
    for (let i = 0; i < bin.length; i++) bytes[i] = bin.charCodeAt(i);
    return bytes;
}

export function triggerDownload(name, bytes) {
    const blob = new Blob([bytes]);
    const url = URL.createObjectURL(blob);
    const a = document.createElement("a");
    a.href = url;
    a.download = name;
    document.body.appendChild(a);
    a.click();
    document.body.removeChild(a);
    URL.revokeObjectURL(url);
}

// defaultConnName builds the pre-filled name for an address, e.g.
// "root@10.0.0.1", or "root@10.0.0.1:2222" for a non-default port.
export function defaultConnName(user, host, port) {
    const h = port && port !== 22 ? `${host}:${port}` : host;
    return `${user}@${h}`;
}

// parseQuickConnect parses an Xshell-style quick connect address. Accepted:
//   ssh://user@host:port, user@host:port, host:port, user@host, host
// Returns {user, host, port} (port defaults to 22), or null when no host is
// present. The user falls back to defaultUser when the address omits it.
export function parseQuickConnect(input, defaultUser = "") {
    let s = String(input || "").trim();
    if (!s) return null;
    s = s.replace(/^ssh:\/\//i, "").replace(/\/+$/, "");

    let user = "";
    const at = s.lastIndexOf("@");
    if (at >= 0) {
        user = s.slice(0, at);
        s = s.slice(at + 1);
    }

    let host = s;
    let port = 0;
    const bracketed = s.match(/^\[([^\]]+)\](?::(\d+))?$/); // [::1]:22
    if (bracketed) {
        host = bracketed[1];
        port = parseInt(bracketed[2] || "0", 10) || 0;
    } else {
        const colon = s.lastIndexOf(":");
        if (colon > 0) {
            host = s.slice(0, colon);
            port = parseInt(s.slice(colon + 1), 10) || 0;
        }
    }

    host = host.trim();
    if (!host) return null;
    if (!user) user = defaultUser || "";
    return {
        user: user.trim(),
        host,
        port: port > 0 && port <= 65535 ? port : 22,
    };
}
