import assert from 'node:assert/strict';
import { test } from 'node:test';
import { build } from 'esbuild';

const bundle = await build({ entryPoints: ['src/lib/agent-names.ts'], bundle: true, platform: 'node', format: 'cjs', write: false });
const module = { exports: {} };
new Function('module', 'exports', bundle.outputFiles[0].text)(module, module.exports);
const { assistantName } = module.exports;

// Expected values come from the Go implementation (internal/workflow/agent_names.go); both must agree.
test('assistant names match the server, so the name is the same before and after a match', () => {
  const expected = { 'Alex Example': 'Luca', 'SamenKlaar Local Operator': 'Finn', 'customer@example.test': 'Sky', 'Marieke de Vries': 'Remy', 'Professional 3718': 'Teo', 'Jane Price': 'Rio', '  JANE price ': 'Rio' };
  for (const [seed, name] of Object.entries(expected)) assert.equal(assistantName(seed), name, seed);
});
