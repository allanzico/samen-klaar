import assert from 'node:assert/strict';
import { test } from 'node:test';
import { build } from 'esbuild';

globalThis.$state = (value) => value;
const bundle = await build({ stdin: { contents: "export * from './src/lib/format'; export { noteText } from './src/lib/agent-copy';", resolveDir: process.cwd() }, bundle: true, platform: 'node', format: 'cjs', write: false });
const module = { exports: {} };
new Function('module', 'exports', bundle.outputFiles[0].text)(module, module.exports);
const { money, decimalMoney, noteText, untilShort } = module.exports;

test('shared money formatting never displays non-finite or missing amounts as prices', () => {
  for (const value of [NaN, Infinity, -Infinity, undefined, null, '', 'NaN', {}]) {
    assert.equal(money(value), '—', String(value));
  }
  assert.equal(money(9900), '€\u00a099,00');
  assert.equal(money(0), '€\u00a00,00');
});

test('decimal amounts support saved Dutch and English prices and reject invalid input', () => {
  for (const value of ['1250.50', '€ 1.250,50', 'EUR 1,250.50']) assert.equal(decimalMoney(value), '€\u00a01.250,50');
  for (const value of ['unknown', '€ NaN', '1,2,3', Infinity, Number.MAX_VALUE]) assert.equal(decimalMoney(value), '—');
  assert.equal(decimalMoney('-10.00'), '€\u00a0-10,00');
});

test('an accepted price with historical formatted parameters renders its real amount', () => {
  for (const amount of ['99.00', '€99.00', '€ 99,00']) {
    assert.equal(noteText({code: 'customer:accept-price', text: 'Agreed on €99.00.', params: {amount}}), 'Agreed on €\u00a099,00.');
  }
});

test('an invalid deadline cannot produce NaN in a countdown', () => {
  assert.equal(untilShort('invalid'), '—');
});
