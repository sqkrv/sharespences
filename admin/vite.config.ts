import { defineConfig } from "vite";
import react from "@vitejs/plugin-react";
import tailwindcss from "@tailwindcss/vite";

// Build output goes to internal/adminweb/dist so the sidecar binary can
// go:embed it (ADR-0008; internal/web precedent). emptyOutDir stays false to
// keep the committed .placeholder. No PWA — the admin UI is an operator tool
// behind an SSH tunnel; offline support would only mask a dead tunnel.
export default defineConfig({
  plugins: [react(), tailwindcss()],
  build: {
    outDir: "../internal/adminweb/dist",
    emptyOutDir: false,
  },
  server: {
    proxy: {
      "/api": "http://127.0.0.1:8081",
    },
  },
});
