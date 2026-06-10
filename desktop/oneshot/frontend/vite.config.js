import { defineConfig } from "vite";
import react from "@vitejs/plugin-react";
import wails from "@wailsio/runtime/plugins/vite";
import { writeFileSync } from "node:fs";

function keepDistDirectory() {
  return {
    name: "keep-dist-directory",
    closeBundle() {
      writeFileSync("dist/.gitkeep", "\n");
    },
  };
}

// https://vitejs.dev/config/
export default defineConfig({
  server: {
    host: "127.0.0.1",
    port: Number(process.env.WAILS_VITE_PORT) || 9245,
    strictPort: true,
  },
  build: {
    emptyOutDir: true,
  },
  plugins: [react(), wails("./bindings"), keepDistDirectory()],
});
