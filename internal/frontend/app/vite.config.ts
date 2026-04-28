import {defineConfig} from "vite";
import react from "@vitejs/plugin-react";
import eslint from 'vite-plugin-eslint2';
import pkg from './package.json';

const eslintOptions = {
    dev: true
};

export default defineConfig({
    plugins: [react(), eslint(eslintOptions)],
    base: '/',
    build: {
        outDir: 'dist'
    },
    server: {
        port: 5173,
        proxy: {
            "/api": {
                target: "http://localhost:8080",
                changeOrigin: true,
            },
        },
    },
    define: {
        '__APP_VERSION__': JSON.stringify(pkg.version),
    },
});
