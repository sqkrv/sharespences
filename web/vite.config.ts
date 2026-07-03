import { defineConfig } from "vite";
import react from "@vitejs/plugin-react";
import tailwindcss from "@tailwindcss/vite";

// Build output goes to internal/web/dist so the Go binary can go:embed it
// (embed can't cross package directories). emptyOutDir stays false to keep
// the committed .placeholder; stale hashed assets are harmless and ignored.
export default defineConfig({
  plugins: [react(), tailwindcss()],
  build: { outDir: "../internal/web/dist", emptyOutDir: false },
  server: {
    proxy: {
      "/api": "http://localhost:8080",
      "/openapi.json": "http://localhost:8080",
      "/docs": "http://localhost:8080",
      "/schemas": "http://localhost:8080",
    },
  },
});
