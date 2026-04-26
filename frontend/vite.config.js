import { defineConfig } from 'vite'
import react from '@vitejs/plugin-react'
import tailwindcss from '@tailwindcss/vite'

// https://vite.dev/config/
export default defineConfig({
  plugins: [react(), tailwindcss()],
  build: {
    outDir: '../internal/api/dist', // Output to the Go server's static directory
    emptyOutDir: true, // Clean the output directory before building
  }
})
