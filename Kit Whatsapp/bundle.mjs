import ncc from '@vercel/ncc';
import { mkdir, writeFile } from 'node:fs/promises';
import { join, dirname } from 'node:path';
import { fileURLToPath } from 'node:url';

const __dirname = dirname(fileURLToPath(import.meta.url));

// ---- Modos de empaquetado ----
//  --asset   : assetBuilds=true  -> genera carpeta con muchos *.js (ideal Tauri)
//  --single  : assetBuilds=false -> genera UN solo index.js autocontenido (ideal Flutter)
const single = process.argv.includes('--single') || process.argv.includes('-s');
const outDir = single
    ? join(__dirname, 'single')          // un solo index.js
    : join(__dirname, 'whatsapp-engine-bundle'); // carpeta con assets (Tauri)

const { code, assets } = await ncc(join(__dirname, 'engine.js'), {
  minify: true,
  sourceMap: false,
  assetBuilds: !single,
});

await mkdir(outDir, { recursive: true });

const indexFile = join(outDir, 'index.js');
await writeFile(indexFile, code, 'utf-8');

if (!single) {
  for (const [name, { source }] of Object.entries(assets)) {
    const assetPath = join(outDir, name);
    await mkdir(dirname(assetPath), { recursive: true });
    await writeFile(assetPath, source);
  }
}

console.log(`✅ WhatsApp engine bundled (${single ? 'single-file' : 'asset-build'}) → ${outDir}`);