import { defineConfig } from "vite";
import { svelte } from "@sveltejs/vite-plugin-svelte";
import tailwindcss from "@tailwindcss/vite";
import wails from "@wailsio/runtime/plugins/vite";
import { readFileSync } from "node:fs";

const pkg = JSON.parse(readFileSync(new URL("./package.json", import.meta.url), "utf8"));

// https://vitejs.dev/config/
export default defineConfig({
  server: {
    host: "127.0.0.1",
    port: Number(process.env.WAILS_VITE_PORT) || 9245,
    strictPort: true,
  },
  // The bundle is embedded in the binary and loaded from disk, so size warnings don't apply.
  build: { chunkSizeWarningLimit: 1000 },
  // The version shown in About and compared by the update check; bumped at each release.
  define: { __APP_VERSION__: JSON.stringify(pkg.version) },
  plugins: [tailwindcss(), svelte(), wails("./bindings")],
});
