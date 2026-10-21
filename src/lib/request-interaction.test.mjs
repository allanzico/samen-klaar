import assert from 'node:assert/strict';
import { test } from 'node:test';
import { build } from 'esbuild';

const bundle = await build({ entryPoints: ['src/lib/request-interaction.ts'], bundle: true, platform: 'node', format: 'cjs', write: false });
const module = { exports: {} };
new Function('module', 'exports', bundle.outputFiles[0].text)(module, module.exports);
const { requestInteraction, requestReplyKey } = module.exports;
const question = { seq: 3, role: 'agent', audience: 'customer', text: 'Your address?' };
const workspace = () => ({ job: {}, agentDialog: { awaiting: true, stage: 'address', asked: 3 }, intake: { messages: [question, { seq: 4, role: 'agent', audience: 'customer', text: 'Unrelated update' }] } });

test('the active question is named by the saved dialog, not the newest bot message', () => {
  assert.equal(requestInteraction(workspace(), 'customer').question, question);
  assert.equal(requestInteraction(workspace(), 'customer').address, true);
  assert.equal(requestInteraction(workspace(), 'professional'), undefined);
});

test('saved replies keep a single waiting interaction until the assistant responds', () => {
  const w = workspace();
  w.workspaceId = 'request';
  w.job.serviceAddress = 'Confirmed address';
  assert.ok(requestReplyKey(w, 'customer'));
  w.agentDialog = { awaiting: false, stage: 'confirm' };
  w.intake.messages.push({seq:5,role:'customer',audience:'customer',text:'Yes'});
  assert.ok(requestReplyKey(w, 'customer'));
  w.intake.messages.push({seq:6,role:'agent',audience:'customer',text:'Next question'});
  assert.equal(requestReplyKey(w, 'customer'), '');
});

test('completed and stale dialogs cannot hide the real next action', () => {
  for (const edit of [
    (w) => { w.cancelled = true; },
    (w) => { w.job.paid = true; },
    (w) => { w.cancellationReview = { status: 'pending' }; },
    (w) => { w.job.dispute = { status: 'open' }; },
    (w) => { w.job.appointmentConfirmation = { status: 'pending' }; w.job.paymentAuthorizationTiming = 'at_booking'; w.job.scopeVersion = 1; },
    (w) => { w.job.serviceAddress = 'Confirmed address'; },
    (w) => { w.agentDialog.awaiting = false; },
    (w) => { w.agentDialog.asked = 99; },
    (w) => { w.agentDialog = { awaiting: true, pending: 'accept-price', pendingId: 'old' }; w.job.priceProposal = { id: 'new', status: 'pending' }; }
  ]) {
    const w = workspace(); edit(w);
    assert.equal(requestInteraction(w, 'customer'), undefined);
  }
});
