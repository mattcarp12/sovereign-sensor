import { defineConfig } from 'vite'
import react from '@vitejs/plugin-react'
import tailwindcss from '@tailwindcss/vite'

// https://vite.dev/config/
export default defineConfig({
  plugins: [react(), tailwindcss()],
  build: {
    outDir: '../internal/api/dist', // Output to the Go server's static directory
    emptyOutDir: true, // Clean the output directory before building
  },
  server: {
    proxy: {
      // Any request starting with /api will be forwarded to our port-forwarded Go server
      '/api': {
        target: 'http://127.0.0.1:8080',
        changeOrigin: true,
      }
    }
  }
})
