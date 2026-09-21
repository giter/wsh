import { defineConfig } from "vite";
import react from "@vitejs/plugin-react";

// Development: `bun run dev` serves the React app with HMR and proxies the RPC
// WebSocket to a locally running Go backend, for example:
//
//   go run . -no-open -port 17777
//
// Production: `bun run build` emits into ../web, which the Go binary embeds and
// serves (assets live under /static/).
export default defineConfig(({ command }) => ({
  plugins: [react()],
  base: command === "build" ? "/static/" : "/",
  build: {
    outDir: "../web",
    emptyOutDir: true,
  },
  server: {
    port: 5173,
    proxy: {
      "/ws": { target: "http://127.0.0.1:17777", ws: true },
    },
  },
}));
