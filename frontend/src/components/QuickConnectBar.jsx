import { useState } from "react";
import { useApp } from "../state/store.jsx";
import { defaultConnName, parseQuickConnect } from "../lib/format.js";
import { useT } from "../lib/i18n.js";

// QuickConnectBar opens a one-off session straight from an address, without
// saving a connection first (Xshell's quick connect). Saving is opt-in: the
// address can be written to the connection list instead of connecting, and an
// open quick session can be saved later from its tab.
export default function QuickConnectBar() {
    const app = useApp();
    const t = useT();
    const [value, setValue] = useState("");

    const notice = (message) => app.openDialog({ type: "notice", title: t("quick.title"), message });

    // read parses the address, reporting why it is unusable through a notice.
    const read = () => {
        const spec = parseQuickConnect(value, app.settings.defaultUser);
        if (!spec) {
            notice(t("quick.badAddress"));
            return null;
        }
        if (!spec.user) {
            notice(t("quick.needUser"));
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
            <span className="qc-label">{t("quick.title")}</span>
            <input
                value={value}
                spellCheck={false}
                placeholder={t("quick.placeholder")}
                title={t("quick.inputTitle")}
                onChange={(e) => setValue(e.target.value)}
                onKeyDown={(e) => {
                    if (e.key === "Enter") {
                        e.preventDefault();
                        submit();
                    }
                }}
            />
            <button className="btn small" onClick={submit}>
                {t("common.connect")}
            </button>
            <button className="btn small" onClick={save} title={t("quick.saveTitle")}>
                {t("common.save")}
            </button>
        </div>
    );
}
