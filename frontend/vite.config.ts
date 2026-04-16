import { defineConfig } from 'vite'
import react from '@vitejs/plugin-react'

export default defineConfig({
  plugins: [react()],
  // Use /tmp for Vite's dep-prebundling cache to avoid atomic rename failures
  // on Docker volume mounts (devcontainer, Codespaces, etc.)
  cacheDir: '/tmp/vite-bso',
  build: {
    outDir: 'dist',
    emptyOutDir: true,
  },
  server: {
    // Development proxy: /api/* requests forwarded to Go backend
    proxy: {
      '/api': {
        target: 'http://localhost:8090',
        changeOrigin: true,
      },
    },
  },
})
