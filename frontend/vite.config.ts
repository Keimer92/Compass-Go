import { defineConfig } from 'vite'
import { svelte } from '@sveltejs/vite-plugin-svelte'

export default defineConfig({
  plugins: [svelte()],
  // emptyOutDir:false conserva el .gitkeep que mantiene la carpeta para
  // //go:embed en un clon limpio. dist.bat limpia assets/ e index.html antes
  // de compilar, asi que no se acumulan bundles viejos.
  build: { outDir: '../cmd/compass-web/frontend/dist', emptyOutDir: false },
  // Modo desarrollo (dev.bat): Wails sirve la UI a traves de su propio esquema
  // (wails://wails:5173), asi que el cliente HMR no puede deducir el host del
  // servidor. Se fija a localhost para que el WebSocket de Vite conecte directo.
  server: {
    port: 5173,
    strictPort: true,
    hmr: { protocol: 'ws', host: 'localhost', port: 5173 },
  },
})
