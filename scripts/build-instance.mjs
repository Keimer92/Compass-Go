#!/usr/bin/env node
/**
 * build-instance.mjs — assemble a project's worker directory from the kit
 * template: `template/src` + the instance's manifest/config/legacy glue.
 *
 *   node scripts/build-instance.mjs school-tracker
 *
 * Result (ready for `npx wrangler dev/deploy` from inside it):
 *   worker/instances/<p>/src/**      (copied from template/src)
 *   worker/instances/<p>/sync.schema.json   must exist (go run …/gen-manifest)
 *   worker/instances/<p>/sync.config.json   optional
 *   worker/instances/<p>/instance.ts        optional (legacy migrations)
 */
import { cpSync, existsSync, mkdirSync, rmSync, writeFileSync, readFileSync } from 'node:fs';
import { dirname, join, resolve } from 'node:path';
import { fileURLToPath } from 'node:url';

const KIT = resolve(dirname(fileURLToPath(import.meta.url)), '..');
const project = process.argv[2];
if (!project) {
  console.error('usage: node scripts/build-instance.mjs <project-instance>');
  process.exit(1);
}

const instDir = join(KIT, 'worker', 'instances', project);
if (!existsSync(instDir)) {
  console.error(`no instance directory: ${instDir}`);
  process.exit(1);
}

const schemaPath = join(instDir, 'sync.schema.json');
if (!existsSync(schemaPath)) {
  console.error(`missing ${schemaPath}\n  generate it from the Go registry (cmd/gen-manifest).`);
  process.exit(1);
}
const schema = JSON.parse(readFileSync(schemaPath, 'utf8'));
if (!schema.version || !Array.isArray(schema.tables) || schema.tables.length === 0) {
  console.error('sync.schema.json is not a v1 manifest with tables');
  process.exit(1);
}

// Optional files get template no-op stand-ins so src/ imports always resolve.
const configPath = join(instDir, 'sync.config.json');
if (!existsSync(configPath)) writeFileSync(configPath, '{}\n', 'utf8');

const instancePath = join(instDir, 'instance.ts');
if (!existsSync(instancePath)) {
  writeFileSync(instancePath,
    'export async function legacyMigrations(_env: any): Promise<void> { /* no legacy shapes */ }\n',
    'utf8');
}

const srcOut = join(instDir, 'src');
rmSync(srcOut, { recursive: true, force: true });
mkdirSync(instDir, { recursive: true });
cpSync(join(KIT, 'worker', 'template', 'src'), srcOut, { recursive: true });

// TypeScript/wrangler setup for the assembled instance.
writeFileSync(join(instDir, 'tsconfig.json'), JSON.stringify({
  compilerOptions: {
    target: 'ES2022',
    module: 'ES2022',
    moduleResolution: 'bundler',
    lib: ['ES2022'],
    types: ['@cloudflare/workers-types'],
    strict: true,
    noEmit: true,
    resolveJsonModule: true,
    skipLibCheck: true,
  },
  include: ['src/**/*', 'instance.ts'],
}, null, 2) + '\n', 'utf8');

console.log(`assembled ${instDir}`);
console.log('next: cd there, install devDeps, `npx tsc --noEmit`, `npx wrangler dev`.');
