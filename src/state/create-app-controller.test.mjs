import assert from 'node:assert/strict';
import { test } from 'node:test';
import { build } from 'esbuild';

// Svelte runes are compiled away in the app; the controller only needs them to be plain values here.
globalThis.$state = (value) => value;

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
    location: { href: 'https://example.test/?workspaceId=A', pathname: '/' },
    history: {
      replaceState: (_, __, path) => { window.location.href = 'https://example.test' + path; window.location.pathname = new URL(window.location.href).pathname; },
      pushState: (_, __, path) => { window.location.href = 'https://example.test' + path; window.location.pathname = new URL(window.location.href).pathname; }
    },
    scrollTo: () => {}
  };
  globalThis.fetch = fetcher;
  const app = {
    session: { account: { id: 'customer' } }, role: 'customer', customerPage: 'detail', workspaceId: 'A',
    workspaces: [{ id: 'A' }, { id: 'B' }, { id: 'C' }], serverState: { job: { id: 'job-A' } },
    workspaceCursor: null,
    providerConfig: { paymentsAvailable: false }, pending: false, loading: false, error: '',
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

test('customer home clears the selected request and preserves it in history', () => {
  const { app, actions } = setup(async () => { throw new Error('unexpected request'); });
  actions.showHome();
  assert.equal(app.customerPage, 'home');
  assert.equal(app.workspaceId, null);
  assert.equal(app.serverState, null);
  assert.equal(window.location.href, 'https://example.test/');
  actions.showRequests();
  assert.equal(app.customerPage, 'requests');
  assert.equal(window.location.href, 'https://example.test/requests');
});

test('opening a request gives the customer a direct detail URL', async () => {
  const { app, actions } = setup(async (url) => {
    if (url === '/api/state?workspaceId=B') return response({ job: { id: 'job-B' } });
    throw new Error(`unexpected ${url}`);
  });
  actions.showRequests();
  await actions.selectRequest('B');
  assert.equal(app.customerPage, 'detail');
  assert.equal(app.workspaceId, 'B');
  assert.equal(window.location.href, 'https://example.test/requests/B');
});

test('describing a job creates a request and submits its first answer', async () => {
  const calls = [];
  const { app, actions } = setup(async (url, init) => {
    calls.push(url);
    if (url === '/api/workspaces' && init?.method === 'POST') return response({ workspace: { workspaceId: 'new-id', job: {} } });
    if (url === '/api/workspaces') return response({ workspaces: [{ id: 'new-id' }] });
    if (url === '/api/actions') {
      assert.deepEqual(JSON.parse(init.body), { workspaceId: 'new-id', action: 'start-intake', value: 'A leaking kitchen sink' });
      return response({ workspaceId: 'new-id', intake: { step: 'detail' }, job: {} });
    }
    throw new Error(`unexpected ${url}`);
  });
  actions.showHome();
  app.draft = 'A leaking kitchen sink';
  await actions.startRequest(app.draft);
  assert.equal(app.customerPage, 'detail');
  assert.equal(window.location.href, 'https://example.test/requests/new-id');
  assert.equal(app.serverState.intake.step, 'detail');
  assert.equal(app.draft, '');
  assert.ok(calls.includes('/api/actions'));
});

test('signed-in customer lands on the description form without creating a request', async () => {
  const calls = [];
  const { app, actions } = setup(async (url, init) => {
    calls.push([url, init?.method]);
    if (url === '/api/config') return response({ appName: 'SamenKlaar', paymentsAvailable: false });
    if (url === '/api/auth/session') return response({ authenticated: true, activeRole: 'customer', account: { id: 'customer' }, memberships: [{ role: 'customer', status: 'active' }] });
    if (url === '/api/workspaces') return response({ workspaces: [{ id: 'A', title: 'Sink leak', status: 'quote' }] });
    throw new Error(`unexpected ${url}`);
  });
  window.location.href = 'https://example.test/';
  await actions.initialize();
  assert.equal(app.customerPage, 'home');
  assert.equal(app.workspaceId, null);
  assert.equal(app.serverState, null);
  assert.ok(!calls.some(([url, method]) => url === '/api/workspaces' && method === 'POST'));
});

test('direct operator URL switches only an authorized membership before loading requests', async () => {
  const calls = [];
  const { actions } = setup(async (url) => {
    calls.push(url);
    if (url === '/api/config') return response({ appName: 'SamenKlaar', paymentsAvailable: false });
    if (url === '/api/auth/session') return response({ authenticated: true, activeRole: 'customer', memberships: [{ role: 'customer', status: 'active' }, { role: 'operator', status: 'active' }] });
    if (url === '/api/auth/role') return response({ path: '/operator' });
    throw new Error(`unexpected ${url}`);
  });
  window.location.pathname = '/operator';
  let destination = '';
  window.location.replace = (path) => { destination = path; };
  await actions.initialize();
  assert.equal(destination, '/operator');
  assert.deepEqual(calls.sort(), ['/api/auth/role', '/api/auth/session', '/api/config']);
});

test('direct operator URL never activates an absent membership', async () => {
  const calls = [];
  const { app, actions } = setup(async (url) => {
    calls.push(url);
    if (url === '/api/config') return response({ appName: 'SamenKlaar', paymentsAvailable: false });
    if (url === '/api/auth/session') return response({ authenticated: true, activeRole: 'customer', memberships: [{ role: 'customer', status: 'active' }] });
    throw new Error(`unexpected ${url}`);
  });
  window.location.pathname = '/operator';
  await actions.initialize();
  assert.equal(app.role, 'customer');
  assert.equal(app.loading, false);
  assert.deepEqual(calls.sort(), ['/api/auth/session', '/api/config']);
});

test('deleting the open request returns to the list and drops it', async () => {
  const calls = [];
  const { app, actions } = setup(async (url, init) => {
    calls.push([url, init?.method]);
    if (url === '/api/workspaces/A' && init?.method === 'DELETE') return response({ status: 'deleted' });
    if (url === '/api/workspaces') return response({ workspaces: [{ id: 'B' }, { id: 'C' }], openRequests: 1, openRequestLimit: 3 });
    throw new Error(`unexpected ${url}`);
  });
  assert.equal(await actions.deleteRequest('A'), true);
  assert.deepEqual(app.workspaces.map((item) => item.id), ['B', 'C']);
  assert.equal(app.workspaceId, null);
  assert.equal(app.customerPage, 'requests');
  assert.equal(window.location.href, 'https://example.test/requests');
  assert.equal(app.openRequests, 1);
  assert.ok(calls.some(([url, method]) => url === '/api/workspaces/A' && method === 'DELETE'));
});

test('a refused delete keeps the request and shows why', async () => {
  const { app, actions } = setup(async (url, init) => {
    if (url === '/api/workspaces/A' && init?.method === 'DELETE') return response('Only requests without an approved plan can be deleted. Cancel it instead, or message the other person.', false);
    throw new Error(`unexpected ${url}`);
  });
  assert.equal(await actions.deleteRequest('A'), false);
  assert.equal(app.workspaceId, 'A');
  assert.equal(app.workspaces.length, 3);
  assert.match(app.error, /approved plan/);
});

test('at the open-request limit a new request is not created', async () => {
  const calls = [];
  const { app, actions } = setup(async (url, init) => {
    calls.push([url, init?.method]);
    throw new Error(`unexpected ${url}`);
  });
  app.openRequests = 3;
  app.openRequestLimit = 3;
  await actions.createRequest();
  assert.equal(calls.length, 0);
  assert.equal(app.workspaceId, 'A');
  assert.match(app.error, /3 open requests/);
});

test('the server limit message reaches the customer when starting a request', async () => {
  const { app, actions } = setup(async (url, init) => {
    if (url === '/api/workspaces' && init?.method === 'POST') return response('You have 3 open requests. Finish, cancel or delete one before starting another.', false);
    throw new Error(`unexpected ${url}`);
  });
  app.openRequests = null;
  actions.showHome();
  app.draft = 'A leaking kitchen sink';
  await actions.startRequest(app.draft);
  assert.equal(app.workspaceId, null);
  assert.match(app.error, /3 open requests/);
  assert.equal(app.draft, 'A leaking kitchen sink');
});
