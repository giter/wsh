import { useState } from "react";
import { useApp } from "../state/store.jsx";
import { defaultConnName, parseQuickConnect } from "../lib/format.js";

// QuickConnectBar opens a one-off session straight from an address, without
// saving a connection first (Xshell's quick connect). Saving is opt-in: the
// address can be written to the connection list instead of connecting, and an
// open quick session can be saved later from its tab.
export default function QuickConnectBar() {
    const app = useApp();
    const [value, setValue] = useState("");

    const notice = (message) => app.openDialog({ type: "notice", title: "快速连接", message });

    // read parses the address, reporting why it is unusable through a notice.
    const read = () => {
        const spec = parseQuickConnect(value, app.settings.defaultUser);
        if (!spec) {
            notice("请输入地址，例如 ssh://root@10.0.0.1:22");
            return null;
        }
        if (!spec.user) {
            notice("地址里需要包含用户名，例如 ssh://root@10.0.0.1");
            return null;
        }
        return spec;
    };

    const submit = () => {
        const spec = read();
        if (!spec) return;
        app.openQuickTerminal(spec);
        setValue("");
    };

    // save writes the address into the connection list instead of connecting,
    // by opening the connection editor pre-filled with it. The input is kept so
    // cancelling the dialog does not lose what was typed.
    const save = () => {
        const spec = read();
        if (!spec) return;
        app.openDialog({
            type: "connection",
            conn: null,
            draft: { name: defaultConnName(spec.user, spec.host, spec.port), host: spec.host, port: spec.port, user: spec.user },
        });
    };

    return (
        <div id="quick-connect">
            <span className="qc-label">快速连接</span>
            <input
                value={value}
                spellCheck={false}
                placeholder="ssh://user@host:22（回车直接连接，「保存」写入连接列表）"
                title="回车直接连接；点「保存」将其保存为连接"
                onChange={(e) => setValue(e.target.value)}
                onKeyDown={(e) => {
                    if (e.key === "Enter") {
                        e.preventDefault();
                        submit();
                    }
                }}
            />
            <button className="btn small" onClick={submit}>
                连接
            </button>
            <button className="btn small" onClick={save} title="保存为连接（不立即连接）">
                保存
            </button>
        </div>
    );
}
