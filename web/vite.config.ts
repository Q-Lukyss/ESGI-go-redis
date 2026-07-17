import { defineConfig } from 'vite'
import react from '@vitejs/plugin-react'

// Proxy vers cmd/server en dev : les composants React n'ont besoin que de
// chemins relatifs (/keys, /browse, /stats, /ws), pas de gérer CORS ou une
// URL absolue différente entre dev et prod.
export default defineConfig({
  plugins: [react()],
  server: {
    proxy: {
      '/keys': 'http://localhost:8080',
      '/browse': 'http://localhost:8080',
      '/query': 'http://localhost:8080',
      '/stats': 'http://localhost:8080',
      '/ws': {
        target: 'ws://localhost:8080',
        ws: true,
      },
    },
  },
})
