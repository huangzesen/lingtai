import { defineConfig } from "vite";
import react from "@vitejs/plugin-react";
import tailwindcss from "@tailwindcss/vite";

export default defineConfig({
  plugins: [react(), tailwindcss()],
  resolve: {
    dedupe: ["three"],
  },
  optimizeDeps: {
    include: ["three", "three-spritetext", "3d-force-graph"],
  },
  server: {
    proxy: {
      "/api": "http://localhost:8080",
    },
  },
});
