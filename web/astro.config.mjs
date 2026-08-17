import { defineConfig } from "astro/config";

// A static marketing site: no server, no framework runtime, no hydration. Astro
// ships zero JavaScript unless a page asks for it, and the only scripts here are
// two hand-written inline ones, so the built page is HTML plus a stylesheet.
export default defineConfig({
  site: "https://onecatch.app",
  build: {
    // Inlining keeps every built page a single self-contained file, which is
    // what makes it hostable anywhere and previewable straight off disk.
    inlineStylesheets: "always",
  },
  devToolbar: { enabled: false },
  server: { port: 9246, host: "127.0.0.1" },
});
