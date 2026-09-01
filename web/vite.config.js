import { defineConfig } from "vite";
import react from "@vitejs/plugin-react";

const apiTarget = process.env.VITE_API_TARGET || "http://127.0.0.1:9000";
const webPort = Number(process.env.WEB_PORT || 5173);

export default defineConfig({
    plugins: [react()],
    server: {
        port: webPort,
        strictPort: true,
        proxy: {
            "/api": {
                target: apiTarget,
                changeOrigin: true,
            },
            "/uploads": {
                target: apiTarget,
                changeOrigin: true,
            },
        },
    },
});
