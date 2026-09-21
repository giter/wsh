import { createRoot } from "react-dom/client";
import "@xterm/xterm/css/xterm.css";
import "./styles.css";
import { AppProvider } from "./state/store.jsx";
import App from "./App.jsx";

// Note: no <StrictMode> on purpose. It double-invokes effects in development,
// which would open two RPC WebSockets and initialise xterm twice.
createRoot(document.getElementById("root")).render(
    <AppProvider>
        <App />
    </AppProvider>,
);
