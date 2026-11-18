import { defineConfig } from 'vite'
import { svelte } from '@sveltejs/vite-plugin-svelte'

export default defineConfig({
  plugins: [svelte()],
  // emptyOutDir:false conserva el .gitkeep que mantiene la carpeta para
  // //go:embed en un clon limpio. dist.bat limpia assets/ e index.html antes
  // de compilar, asi que no se acumulan bundles viejos.
  build: { outDir: '../cmd/compass-web/frontend/dist', emptyOutDir: false },
})
