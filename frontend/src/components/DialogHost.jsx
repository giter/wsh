import { useApp } from "../state/store.jsx";
import ConnectionDialog from "./ConnectionDialog.jsx";
import KeyDialog from "./KeyDialog.jsx";
import TunnelDialog from "./TunnelDialog.jsx";
import { AboutDialog, ConfirmDialog, NoticeDialog, PassphraseDialog, PasswordDialog, PromptDialog } from "./SimpleDialogs.jsx";

// DialogHost renders the single active dialog requested through the store's
// openDialog({type, ...}) API.
export default function DialogHost() {
    const { dialog, closeDialog } = useApp();
    if (!dialog) return null;

    switch (dialog.type) {
        case "connection":
            return <ConnectionDialog conn={dialog.conn} draft={dialog.draft} onSaved={dialog.onSaved} onClose={closeDialog} />;
        case "key":
            return <KeyDialog item={dialog.key} onSaved={dialog.onSaved} onClose={closeDialog} />;
        case "tunnel":
            return <TunnelDialog tunnel={dialog.tunnel} onSaved={dialog.onSaved} onClose={closeDialog} />;
        case "confirm":
            return (
                <ConfirmDialog
                    title={dialog.title}
                    body={dialog.body}
                    confirmLabel={dialog.confirmLabel}
                    onConfirm={dialog.onConfirm}
                    onClose={closeDialog}
                />
            );
        case "prompt":
            return (
                <PromptDialog
                    title={dialog.title}
                    label={dialog.label}
                    initial={dialog.initial}
                    onSubmit={dialog.onSubmit}
                    onClose={closeDialog}
                />
            );
        case "password":
            return <PasswordDialog message={dialog.message} onSubmit={dialog.onSubmit} onClose={closeDialog} />;
        case "passphrase":
            return <PassphraseDialog message={dialog.message} onSubmit={dialog.onSubmit} onClose={closeDialog} />;
        case "notice":
            return <NoticeDialog title={dialog.title} message={dialog.message} onClose={closeDialog} />;
        case "about":
            return <AboutDialog onClose={closeDialog} />;
        default:
            return null;
    }
}
