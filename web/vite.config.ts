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
        // The legal pages must NOT be precached. Workbox registers
        // precacheAndRoute before the NavigationRoute and its PrecacheRoute
        // defaults to cleanURLs:true, so a precached privacy.html answers
        // /privacy cache-first — ahead of the denylist below, which would
        // never be consulted, and past the `no-cache` the Go handler sends.
        // Combined with registerType "prompt" (the refreshed copy waits for a
        // toast these plain-HTML pages cannot render), a user could read and
        // consent to a superseded policy while online. They are served
        // NetworkFirst instead: current when there is a network, still
        // readable offline.
        globIgnores: ["privacy.html", "terms.html", "legal.css"],
        // Navigations to the API/docs must never be answered with index.html.
        // /privacy and /terms are static documents served outside the SPA
        // (web/public/*.html, resolved without the extension by
        // internal/web/web.go) — without them here the navigation fallback
        // answers with the app shell and the legal pages become unreachable
        // in the installed PWA, exactly where they must stay readable.
        navigateFallbackDenylist: [
          /^\/api\//,
          /^\/docs/,
          /^\/openapi\.json/,
          /^\/schemas/,
          /^\/privacy$/,
          /^\/terms$/,
        ],
        runtimeCaching: [
          {
            // The legal pages (globIgnores above keeps them out of the
            // precache). NetworkFirst so a published revision is what the user
            // reads whenever there is a network — the one property a policy
            // page must have — while an offline visit still resolves. The
            // navigateFallbackDenylist entries are what let this route see the
            // navigation at all: NavigationRoute is registered first and would
            // otherwise answer /privacy with the app shell.
            urlPattern: ({ url, sameOrigin }) =>
              sameOrigin && /^\/(privacy|terms)(\.html)?$|^\/legal\.css$/.test(url.pathname),
            handler: "NetworkFirst",
            options: {
              cacheName: "legal",
              networkTimeoutSeconds: 5,
              expiration: { maxEntries: 8, maxAgeSeconds: 365 * 24 * 3600 },
            },
          },
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
