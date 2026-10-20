import assert from 'node:assert/strict';
import { test } from 'node:test';
import { readFileSync } from 'node:fs';
import ts from 'typescript';
import { build } from 'esbuild';

// Run the component's actual event logic with browser/provider boundaries replaced.
// This recovery contract does not verify rendering or reactive DOM behavior.
const component = readFileSync('src/features/professional/PayPalPayouts.svelte', 'utf8');
const script = component.split('<script lang="ts">')[1].split('</script>')[0].replace(/^\s*import [^;]+;\s*$/gm, '');
const compiled = ts.transpileModule(script, { compilerOptions: { target: ts.ScriptTarget.ES2022, module: ts.ModuleKind.ESNext } }).outputText;

function setup(api, href = 'https://example.test/professional?paypal_onboarding=1&merchantIdInPayPal=QA_MERCHANT&keep=1') {
  const mounts = [];
  let url = new URL(href);
  const window = { location: { get href() { return url.href; }, assign(value) { url = new URL(value); } }, history: { replaceState(_state, _title, value) { url = new URL(value, url); } } };
  const derived = Object.assign((value) => value, { by: (fn) => fn() });
  const controller = new Function('onMount', 'api', 'window', '$props', '$state', '$derived', 't', compiled + ';return {refresh,connect,getStatus:()=>status,getError:()=>error};')(
    (fn) => mounts.push(fn), api, window, () => ({}), (value) => value, derived, (value) => value
  );
  return { controller, mounts, url: () => url };
}

test('a failed PayPal return keeps the merchant reference for Check status and clears it only after verification', async () => {
  const calls = [];
  const instance = setup({ async paypalSellerStatus(id) {
    calls.push(id);
    if (calls.length === 1) throw new Error('temporary verification failure');
    return { status: 'verified', connected: true };
  } });
  instance.mounts[0]();
  await new Promise(setImmediate);
  assert.equal(instance.url().searchParams.get('merchantIdInPayPal'), 'QA_MERCHANT');
  await instance.controller.refresh();
  assert.deepEqual(calls, ['QA_MERCHANT', 'QA_MERCHANT']);
  assert.equal(instance.controller.getStatus().connected, true);
  assert.equal(instance.url().searchParams.has('merchantIdInPayPal'), false);
  assert.equal(instance.url().searchParams.has('paypal_onboarding'), false);
  assert.equal(instance.url().searchParams.get('keep'), '1');
});

test('a connection-start failure shows the actionable API error', async () => {
  const message = 'PayPal payouts are not available yet. Please contact our team.';
  const instance = setup({ async beginPayPalSellerOnboarding() { throw new Error(message); } }, 'https://example.test/professional');
  await instance.controller.connect();
  assert.equal(instance.controller.getError(), message);
});

const bundle = await build({ entryPoints: ['src/lib/api.ts'], bundle: true, platform: 'node', format: 'cjs', write: false });
const module = { exports: {} };
new Function('module', 'exports', bundle.outputFiles[0].text)(module, module.exports);
const { api } = module.exports;

test('PayPal API errors use known public codes while hiding arbitrary provider bodies', async (t) => {
  const original = globalThis.fetch;
  t.after(() => { globalThis.fetch = original; });
  globalThis.fetch = async () => new Response(JSON.stringify({ code: 'PAYPAL_CONNECTION_VERIFY_FAILED', error: 'private-provider-detail' }), { status: 502, headers: { 'Content-Type': 'application/json' } });
  await assert.rejects(api.paypalSellerStatus('QA_MERCHANT'), /couldn’t verify your PayPal connection/);
  globalThis.fetch = async () => new Response(JSON.stringify({ code: 'UNKNOWN', error: 'private-provider-detail' }), { status: 503, headers: { 'Content-Type': 'application/json' } });
  await assert.rejects(api.paypalSellerStatus(), (err) => !err.message.includes('private-provider-detail') && err.message.includes('temporarily unavailable'));
});

test('request timeouts offer recovery and requests have a finite default deadline', async (t) => {
  const original = globalThis.fetch;
  t.after(() => { globalThis.fetch = original; });
  globalThis.fetch = async (_url, init) => {
    assert.ok(init.signal instanceof AbortSignal);
    throw new DOMException('internal timeout details', 'TimeoutError');
  };
  await assert.rejects(api.state('fixture'), /The request took too long. Please try again./);
  await assert.rejects(api.paypalSellerStatus(), /The request took too long. Please try again./);
});
