import { defineConfig } from "vite";
import vue from "@vitejs/plugin-vue";
import tailwindcss from "@tailwindcss/vite";

// https://vite.dev/config/
export default defineConfig({
  base: "/app/",
  plugins: [vue(), tailwindcss()],
  server: {
    proxy: {
      "/podcasts": { target: "http://localhost:8080", changeOrigin: true },
      "/podcastitems": { target: "http://localhost:8080", changeOrigin: true },
      "/tags": { target: "http://localhost:8080", changeOrigin: true },
      "/search": { target: "http://localhost:8080", changeOrigin: true },
      "/settings": { target: "http://localhost:8080", changeOrigin: true },
      "/opml": { target: "http://localhost:8080", changeOrigin: true },
      "/rss": { target: "http://localhost:8080", changeOrigin: true },
      "/player": { target: "http://localhost:8080", changeOrigin: true },
      "/assets": { target: "http://localhost:8080", changeOrigin: true },
      "/webassets": { target: "http://localhost:8080", changeOrigin: true },
      "/ws": { target: "ws://localhost:8080", ws: true, changeOrigin: true },
    },
  },
});
