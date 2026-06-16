import { defineConfig } from "vite";
import react from "@vitejs/plugin-react";

// The admin UI is served under /admin by the all-in-one server, so it is built
// with that base path. In dev, /admin/api is proxied to the Go server so the
// browser talks to the API same-origin (no CORS needed).
export default defineConfig({
  base: "/admin/",
  plugins: [react()],
  server: {
    proxy: {
      "/admin/api": "http://localhost:8080",
    },
  },
});
