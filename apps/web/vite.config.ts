import babel from "@rolldown/plugin-babel";
import tailwindcss from "@tailwindcss/vite";
import react, { reactCompilerPreset } from "@vitejs/plugin-react";
import path from "node:path";
import { type Connect, type Plugin } from "vite";
import { defineConfig } from "vitest/config";

const healthPlugin = (): Plugin => {
  const healthMiddleware: Connect.NextHandleFunction = (req, res, next) => {
    if (req.url === "/health/live" || req.url === "/health/ready") {
      const status = req.url === "/health/live" ? "alive" : "ready";

      res.statusCode = 200;
      res.setHeader("Content-Type", "application/json");
      res.end(
        JSON.stringify({
          status,
          timestamp: new Date().toISOString(),
        }),
      );
      return;
    }
    next();
  };

  return {
    name: "vite-plugin-health-probes",
    configureServer(server) {
      server.middlewares.use(healthMiddleware);
    },
    configurePreviewServer(server) {
      server.middlewares.use(healthMiddleware);
    },
  };
};

// Proxy instead of cross-origin calls, so the browser stays on one origin and
// CORS stays out of the dev loop.
const serverOrigin = process.env.VITE_SERVER_ORIGIN ?? "http://localhost:8080";

export default defineConfig({
  resolve: {
    alias: { "@": path.resolve(import.meta.dirname, "./src") },
  },
  server: {
    allowedHosts: true,
    proxy: {
      "/api": { target: serverOrigin, changeOrigin: true },
      "/mock": { target: serverOrigin, changeOrigin: true },
    },
  },
  plugins: [react(), babel({ presets: [reactCompilerPreset()] }), tailwindcss(), healthPlugin()],
  test: {
    environment: "jsdom",
    globals: true,
    include: ["src/**/*.test.{ts,tsx}"],
    css: false,
  },
});
