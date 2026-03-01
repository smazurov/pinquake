/// <reference types="vitest/config" />
import { defineConfig } from "vite";
import react from "@vitejs/plugin-react";
import tailwindcss from "@tailwindcss/vite";
import tsconfigPaths from "vite-tsconfig-paths";

export default defineConfig({
  plugins: [tailwindcss(), tsconfigPaths(), react()],
  test: {
    include: ["src/**/*.test.ts"],
  },
  build: {
    outDir: "dist",
    chunkSizeWarningLimit: 1000,
  },
  server: {
    host: "localhost",
    port: 5173,
    proxy: {
      "/api": "http://localhost:8091",
    },
  },
});
