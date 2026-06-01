import { defineConfig } from 'vite'
import react from '@vitejs/plugin-react'
import { resolve } from 'path'

export default defineConfig({
  plugins: [react()],
  base: '/app/',
  build: {
    outDir: resolve(__dirname, '../cp/internal/embeddedui/dist'),
    emptyOutDir: true,
  },
  server: {
    proxy: {
      '/auth': 'http://localhost:8080',
      '/hosts': 'http://localhost:8080',
      '/policies': 'http://localhost:8080',
      '/objectgroups': 'http://localhost:8080',
      '/tokens': 'http://localhost:8080',
      '/default-policy': 'http://localhost:8080',
      '/audit-log': 'http://localhost:8080',
      '/users': 'http://localhost:8080',
      '/api-tokens': 'http://localhost:8080',
      '/healthz': 'http://localhost:8080',
    },
  },
})
