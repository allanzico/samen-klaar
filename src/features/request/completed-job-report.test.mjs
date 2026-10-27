import assert from 'node:assert/strict';
import { after, test } from 'node:test';
import { createServer } from 'vite';
import { svelte } from '@sveltejs/vite-plugin-svelte';

// Render the real stage components, so losing the report from either role fails here.
const server = await createServer({ configFile: false, plugins: [svelte()], logLevel: 'error', server: { middlewareMode: true, ws: false }, appType: 'custom', optimizeDeps: { noDiscovery: true, include: [] } });
after(() => server.close());
const { render } = await server.ssrLoadModule('svelte/server');
const { setLocale } = await server.ssrLoadModule('/src/lib/i18n.svelte.ts');
const { emptyWorkspace } = await server.ssrLoadModule('/tests/browser/fixtures.ts');
const { default: CustomerStage } = await server.ssrLoadModule('/src/features/request/CustomerStage.svelte');
const { default: ProStage } = await server.ssrLoadModule('/src/features/request/ProStage.svelte');

function fixture() {
  const workspace = emptyWorkspace('report-fixture');
  Object.assign(workspace.job, {
    id: 'report-job', title: 'Sink connection repair', customer: 'Test customer', professional: 'Test professional',
    serviceDescription: 'Repair the accessible sink connection', scopeVersion: 2, completionRound: 2,
    paid: true, completionApproved: true, completionApprovedByAgent: true, paymentApprovedByAgent: true,
    completionApprovedAt: '2026-10-26T10:15:00Z', paymentApprovedAt: '2026-10-26T10:16:00Z',
    paymentApprovedVersion: 2, paymentTransactionId: 'SYNTHETIC-CAPTURE',
    evidence: [
      { kind: 'note', label: 'Professional work note', description: 'Reseated the washer and checked the joint under running water.', scopeVersion: 2, completionRound: 2 },
      { kind: 'document', assetId: 'synthetic-file', contentType: 'application/pdf', scopeVersion: 2, completionRound: 2, label: 'Test report', description: 'Synthetic document' },
      { kind: 'note', label: 'Earlier note', description: 'Previous attempt needs more documentation.', scopeVersion: 2, completionRound: 1 }
    ]
  });
  workspace.job.quote = { ...workspace.job.quote, totalMinor: 11000, materialsPolicy: 'No additional materials charged', exclusions: ['Concealed pipework'], lines: [{ label: 'Service', amountMinor: 10000, basis: 'Agreed price' }, { label: 'Call-out', amountMinor: 1000, basis: 'Agreed price' }] };
  return workspace;
}
function stage(role, workspace, code = 'paid') {
  return render(role === 'customer' ? CustomerStage : ProStage, { props: { app: { role, workspaceId: workspace.workspaceId }, actions: {}, workspace, code, work: { note: '' } } }).body;
}

test('both finished stages show factual scope, work notes, approval and payment reference', () => {
  setLocale('en', false);
  for (const role of ['customer', 'professional']) {
    const html = stage(role, fixture());
    for (const text of ['Job completion report', 'Repair the accessible sink connection', 'Reseated the washer', 'No additional materials charged', 'Concealed pipework', 'Customer’s assistant', 'SYNTHETIC-CAPTURE', '1 document', '26 Oct 2026']) assert.ok(html.includes(text), `${role}: missing ${text}`);
    assert.ok(html.indexOf('Reseated the washer') < html.indexOf('<details'), 'the accepted work note should be visible without expanding history');
    assert.match(html, /<details[\s\S]*Previous attempt needs more documentation/);
    assert.doesNotMatch(html.slice(html.indexOf('<details')), /Completion approved:/, 'earlier documentation must not inherit the accepted attempt’s approval timestamp');
    assert.doesNotMatch(html, /NaN|Invalid Date|0 photos/);
  }
});

test('refund reports keep payment and refund references distinct and do not claim approval by an assistant', () => {
  setLocale('en', false);
  const workspace = fixture();
  workspace.job.completionApprovedByAgent = false;
  workspace.job.paymentApprovedByAgent = false;
  workspace.job.refundTransactionId = 'SYNTHETIC-REFUND';
  for (const role of ['customer', 'professional']) {
    assert.match(stage(role, workspace, 'refund_pending'), /Refund pending/);
    workspace.job.paymentRefunded = true;
    const html = stage(role, workspace, 'refunded');
    assert.match(html, /Refunded/);
    assert.ok(html.includes('SYNTHETIC-CAPTURE') && html.includes('SYNTHETIC-REFUND'));
    assert.doesNotMatch(html, /Customer’s assistant/);
    workspace.job.paymentRefunded = false;
  }
});

test('legacy records omit missing timestamps and references and localize the report', () => {
  const workspace = fixture();
  workspace.job.completionApprovedAt = 'invalid';
  workspace.job.paymentApprovedAt = '';
  workspace.job.paymentTransactionId = '';
  workspace.job.evidence = [];
  setLocale('nl', false);
  const html = stage('professional', workspace);
  assert.match(html, /Opleverrapport/);
  assert.match(html, /geen opleverdocumentatie/);
  assert.doesNotMatch(html, /Invalid Date|PayPal-betalingskenmerk|SYNTHETIC-CAPTURE/);
  setLocale('en', false);
});

test('professional report shows the platform deduction while customer total stays unchanged',()=>{
 setLocale('en',false);
 const workspace=fixture();workspace.job.platformFeeBasisPoints=500;
 const professional=stage('professional',workspace);const customer=stage('customer',workspace);
 assert.match(professional,/SamenKlaar fee \(5%\)/);assert.match(professional,/104,50/);assert.match(professional,/5,50/);
 assert.doesNotMatch(customer,/SamenKlaar fee|Your payout/);assert.match(customer,/110,00/);
 workspace.job.paymentRefunded=true;const refunded=stage('professional',workspace,'refunded');
 assert.match(refunded,/Full refunds include the platform fee/);assert.doesNotMatch(refunded,/You receive before PayPal fees/);
});
