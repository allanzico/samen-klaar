import assert from 'node:assert/strict';
import { test } from 'node:test';
import { readFileSync, readdirSync } from 'node:fs';
import { join } from 'node:path';

// Handoff section 16.7: every operator string must have a Dutch entry. Strings people typed
// (reports, names, areas) never go through t(), so only literals inside t('…') are checked.
const catalogue = readFileSync('src/lib/i18n.svelte.ts', 'utf8') + readFileSync('src/lib/i18n-redesign.ts', 'utf8') + readFileSync('src/lib/operator-copy.ts', 'utf8');
const files = [...readdirSync('src/features/operator').map((f) => join('src/features/operator', f)), 'src/lib/operator-copy.ts'];

test('operator screens have a Dutch entry for every translated string', () => {
  const missing = new Set();
  for (const file of files) {
    const source = readFileSync(file, 'utf8');
    for (const match of source.matchAll(/\bt\(\s*(['"])((?:\\.|(?!\1).)*)\1/g)) {
      const key = match[2].replace(/\\'/g, "'");
      const quoted = JSON.stringify(key).slice(1, -1);
      if (!catalogue.includes(`'${key}'`) && !catalogue.includes(`"${quoted}"`) && !catalogue.includes(`"${key}"`)) missing.add(key);
    }
  }
  assert.deepEqual([...missing], []);
});
