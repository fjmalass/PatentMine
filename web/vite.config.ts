import { defineConfig } from "vite";
import react from "@vitejs/plugin-react";

export default defineConfig({
  plugins: [react()],
  base: "/ui/",
  server: {
    proxy: {
      "/healthz": "http://127.0.0.1:18080",
      "/patents": "http://127.0.0.1:18080",
      "/projects": "http://127.0.0.1:18080",
      "/events": "http://127.0.0.1:18080",
      "/session": "http://127.0.0.1:18080",
      "/commands": "http://127.0.0.1:18080",
    },
  },
  build: {
    outDir: "dist",
    emptyOutDir: true,
  },
});
