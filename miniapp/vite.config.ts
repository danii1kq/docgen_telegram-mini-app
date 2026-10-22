import { defineConfig } from 'vite';
import react from '@vitejs/plugin-react-swc';
import tailwindcss from '@tailwindcss/vite';

// The Mini App is served by the Go API on the same origin in production
// (MINIAPP_DIR), so the build uses relative asset URLs and the dev server
// proxies /api to the local backend.
export default defineConfig({
  base: './',
  plugins: [react(), tailwindcss()],
  resolve: {
    // Vite resolves the "@/*" alias from tsconfig.json natively.
    tsconfigPaths: true,
  },
  server: {
    host: true,
    port: 5174,
    proxy: {
      '/api': {
        target: 'http://127.0.0.1:8080',
        changeOrigin: true,
      },
    },
  },
  build: {
    outDir: 'dist',
    sourcemap: false,
  },
});
