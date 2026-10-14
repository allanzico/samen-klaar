import assert from 'node:assert/strict';
import { test } from 'node:test';
import { build } from 'esbuild';

const bundle = await build({ entryPoints: ['src/state/create-app-controller.ts'], bundle: true, platform: 'node', format: 'cjs', write: false });
const module = { exports: {} };
new Function('module', 'exports', bundle.outputFiles[0].text)(module, module.exports);
const { createAppActions } = module.exports;

function setup(fetcher) {
  const storage = new Map();
  globalThis.sessionStorage = {
    getItem: (key) => storage.get(key) ?? null,
    setItem: (key, value) => storage.set(key, value),
    removeItem: (key) => storage.delete(key)
  };
  globalThis.window = {
    location: { href: 'https://example.test/?workspaceId=A' },
    history: { replaceState: (_, __, path) => { window.location.href = 'https://example.test' + path; } }
  };
  globalThis.fetch = fetcher;
  const app = {
    session: { account: { id: 'customer' } }, role: 'customer', workspaceId: 'A',
    workspaces: [{ id: 'A' }, { id: 'B' }, { id: 'C' }], serverState: { job: { id: 'job-A' } },
    workspaceCursor: null,
    providerConfig: { paymentProvider: '' }, pending: false, loading: false, error: '',
    draft: '', draftRestored: false, draftSaved: true, paypalStatus: 'not_started'
  };
  return { app, actions: createAppActions(app, async () => {}) };
}

function response(value, ok = true) {
  return { ok, json: async () => value, text: async () => String(value) };
}

test('failed selection keeps displayed state and action target together', async () => {
  const sent = [];
  const { app, actions } = setup(async (url, init) => {
    if (url.includes('workspaceId=B')) return response('state unavailable', false);
    if (url === '/api/actions') { sent.push(JSON.parse(init.body)); return response({ job: { id: 'job-A' } }); }
    if (url === '/api/workspaces') return response({ workspaces: app.workspaces });
    throw new Error(`unexpected ${url}`);
  });
  await actions.selectRequest('B');
  assert.equal(app.workspaceId, 'A');
  assert.equal(app.serverState.job.id, 'job-A');
  await actions.act('send-message', 'Hello');
  assert.equal(sent[0].workspaceId, 'A');
});

test('last request selection wins when responses resolve out of order', async () => {
  const pending = new Map();
  const { app, actions } = setup((url) => new Promise((resolve) => pending.set(url, resolve)));
  const first = actions.selectRequest('B');
  const second = actions.selectRequest('C');
  pending.get('/api/state?workspaceId=C')(response({ job: { id: 'job-C' } }));
  await second;
  pending.get('/api/state?workspaceId=B')(response({ job: { id: 'job-B' } }));
  await first;
  assert.equal(app.workspaceId, 'C');
  assert.equal(app.serverState.job.id, 'job-C');
});

test('saved message clears its draft despite a failed list refresh', async () => {
  const sent = [];
  const { app, actions } = setup(async (url, init) => {
    if (url === '/api/actions') {
      sent.push(JSON.parse(init.body));
      return response({ job: { id: 'job-A' }, intake: { messages: [{ text: 'Saved message' }] } });
    }
    if (url === '/api/workspaces') return response('list unavailable', false);
    throw new Error(`unexpected ${url}`);
  });
  app.draft = 'Saved message';
  await actions.sendMessage();
  assert.equal(sent.length, 1);
  assert.equal(app.serverState.intake.messages[0].text, 'Saved message');
  assert.equal(app.draft, '');
  assert.match(app.error, /saved/);
});

test('older request pages append without losing current selection', async () => {
  const { app, actions } = setup(async (url) => {
    if (url === '/api/workspaces?cursor=older') return response({ workspaces: [{ id: 'C' }, { id: 'D' }], nextCursor: '' });
    throw new Error(`unexpected ${url}`);
  });
  app.workspaceCursor = 'older';
  await actions.loadMoreWorkspaces();
  assert.deepEqual(app.workspaces.map((item) => item.id), ['A', 'B', 'C', 'D']);
  assert.equal(app.workspaceId, 'A');
  assert.equal(app.workspaceCursor, null);
});
