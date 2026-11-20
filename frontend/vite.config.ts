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
  //
  // host: '127.0.0.1' es obligatorio: por defecto Vite 6 escucha solo en ::1
  // (IPv6), y tanto el proxy de Wails como el sondeo de dev.bat van por IPv4
  // (127.0.0.1). Sin esto el puerto parece cerrado y la app queda en blanco.
  server: {
    host: '127.0.0.1',
    port: 5173,
    strictPort: true,
    hmr: { protocol: 'ws', host: '127.0.0.1', port: 5173 },
  },
})
