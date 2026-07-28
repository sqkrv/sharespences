import { defineConfig } from "vite";
import react from "@vitejs/plugin-react";
import tailwindcss from "@tailwindcss/vite";
import { VitePWA } from "vite-plugin-pwa";

// Build stamp for dev mode (web/src/dev/devmode.ts): __BUILD__ is compiled
// into the bundle, build.json ships the same value as a file. Dev mode
// compares them — they differ exactly when a service worker is still serving
// an old bundle. build.json is precache-free by construction (the workbox
// globPatterns below list no .json) and the Go static handler sends
// `no-cache` for everything outside assets/, so the fetch reaches the server.
const BUILD = new Date().toISOString();

// Build output goes to internal/web/dist so the Go binary can go:embed it
// (embed can't cross package directories). emptyOutDir stays false to keep
// the committed .placeholder; stale hashed assets are harmless and ignored.
export default defineConfig({
  define: { __BUILD__: JSON.stringify(BUILD) },
  plugins: [
    react(),
    tailwindcss(),
    {
      name: "build-stamp",
      generateBundle() {
        this.emitFile({ type: "asset", fileName: "build.json", source: JSON.stringify({ build: BUILD }) });
      },
    },
    // PWA per docs/specs/pwa.md (meta-repo): installable + offline READ.
    // Update flow is `prompt` (toast in the shell), never a silent swap.
    VitePWA({
      registerType: "prompt",
      includeAssets: ["favicon.png", "apple-touch-icon.png"],
      manifest: {
        name: "Sharespences",
        short_name: "Sharespences",
        lang: "ru",
        display: "standalone",
        start_url: "/",
        background_color: "#0f0b1c",
        theme_color: "#0f0b1c",
        icons: [
          { src: "/pwa-192.png", sizes: "192x192", type: "image/png" },
          { src: "/pwa-512.png", sizes: "512x512", type: "image/png" },
          { src: "/pwa-maskable-512.png", sizes: "512x512", type: "image/png", purpose: "maskable" },
        ],
      },
      workbox: {
        // Take control of already-open pages right after activation —
        // otherwise the FIRST visit is never SW-controlled and the offline
        // warm-up caches nothing (login → close tab → offline would fail).
        clientsClaim: true,
        // woff2 on top of the defaults: offline must keep Golos Text
        // (~190K for all subsets/weights — cheap). .woff legacy fallbacks
        // stay out of the precache.
        globPatterns: ["**/*.{js,css,html,ico,png,svg,webmanifest,woff2}"],
        // Navigations to the API/docs must never be answered with index.html.
        navigateFallbackDenylist: [/^\/api\//, /^\/docs/, /^\/openapi\.json/, /^\/schemas/],
        runtimeCaching: [
          {
            // Read endpoints only: runtime caching matches GET by default, so
            // POST login/register/logout never enter the cache. Attachments
            // are heavy and personal — excluded. The recognition poll is
            // excluded too: a job status answered by yesterday's cache would
            // spin the review screen forever. ignoreVary because scs adds
            // Vary: Cookie (see docs/specs/pwa.md trap list).
            urlPattern: ({ url, sameOrigin }) =>
              sameOrigin &&
              url.pathname.startsWith("/api/v1/") &&
              !url.pathname.startsWith("/api/v1/attachments/") &&
              !url.pathname.startsWith("/api/v1/cashback/recognitions"),
            handler: "NetworkFirst",
            options: {
              cacheName: "api-v1",
              networkTimeoutSeconds: 3, // bad signal at a checkout beats no signal
              matchOptions: { ignoreVary: true },
              expiration: { maxEntries: 200, maxAgeSeconds: 30 * 24 * 3600 },
            },
          },
        ],
      },
      devOptions: { enabled: false },
    }),
  ],
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
