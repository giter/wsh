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
