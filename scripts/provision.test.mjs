import assert from 'node:assert/strict';
import { test } from 'node:test';

import { reemplazarEnBloque, valorEnBloque } from './provision.mjs';

for (const salto of ['\n', '\r\n']) {
  test(`reemplaza database_id conservando saltos ${JSON.stringify(salto)}`, () => {
    const toml = [
      'name = "school-tracker-flutter"',
      '',
      '[[d1_databases]]',
      'binding = "DB"',
      'database_name = "school-tracker-flutter"',
      'database_id = "00000000-0000-0000-0000-000000000000"',
      '',
    ].join(salto);

    const actualizado = reemplazarEnBloque(
      toml,
      'd1_databases',
      'database_id',
      '11111111-2222-3333-4444-555555555555',
    );

    assert.equal(
      valorEnBloque(actualizado, 'd1_databases', 'database_id'),
      '11111111-2222-3333-4444-555555555555',
    );
    assert.equal(actualizado.includes(`\r\n`), salto === '\r\n');
  });
}
