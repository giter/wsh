import { useState } from "react";
import { useApp } from "../state/store.jsx";
import { parseQuickConnect } from "../lib/format.js";

// QuickConnectBar opens a one-off session straight from an address, without
// saving a connection first (Xshell's quick connect). The session is ad-hoc:
// nothing is written to the config.
export default function QuickConnectBar() {
    const app = useApp();
    const [value, setValue] = useState("");

    const notice = (message) => app.openDialog({ type: "notice", title: "快速连接", message });

    const submit = () => {
        const spec = parseQuickConnect(value, app.settings.defaultUser);
        if (!spec) {
            notice("请输入地址，例如 ssh://root@10.0.0.1:22");
            return;
        }
        if (!spec.user) {
            notice("地址里需要包含用户名，例如 ssh://root@10.0.0.1");
            return;
        }
        app.openQuickTerminal(spec);
        setValue("");
    };

    return (
        <div id="quick-connect">
            <span className="qc-label">快速连接</span>
            <input
                value={value}
                spellCheck={false}
                placeholder="ssh://user@host:22（回车连接，不保存）"
                title="临时连接，不会保存到连接列表"
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
        </div>
    );
}
