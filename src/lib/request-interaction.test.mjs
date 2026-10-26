import assert from 'node:assert/strict';
import { test } from 'node:test';
import { build } from 'esbuild';

const bundle = await build({ entryPoints: ['src/lib/request-interaction.ts'], bundle: true, platform: 'node', format: 'cjs', write: false });
const module = { exports: {} };
new Function('module', 'exports', bundle.outputFiles[0].text)(module, module.exports);
const { requestInteraction, requestReplyKey } = module.exports;
const question = { seq: 3, role: 'agent', audience: 'customer', text: 'Your address?' };
const workspace = () => ({ job: {id:'selected-job'}, agentDialog: { awaiting: true, stage: 'address', asked: 3 }, intake: { messages: [question, { seq: 4, role: 'agent', audience: 'customer', text: 'Unrelated update' }] } });

test('old assistant questions cannot interrupt professional review without an assigned job', () => {
  const w = workspace();
  w.job.id = '';
  assert.equal(requestInteraction(w,'customer'),undefined);
  w.agentDialog.awaiting = false;
  w.intake.messages.push({seq:5,role:'customer',audience:'customer',text:'An earlier reply'});
  assert.equal(requestReplyKey(w,'customer'),'');
});

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

const autonomyBundle = await build({ entryPoints: ['src/lib/booking-autonomy.ts'], bundle: true, platform: 'node', format: 'cjs', write: false });
const autonomyModule = { exports: {} };
new Function('module', 'exports', autonomyBundle.outputFiles[0].text)(autonomyModule, autonomyModule.exports);
const { autonomyChoiceKey, automaticReleaseOn, needsAutonomyChoice } = autonomyModule.exports;

test('new bookings require an explicit current agreement permission; legacy bookings stay manual', () => {
  const job = {id:'job',scopeVersion:2,professionalAccountId:'pro',professionalProfileId:'service',quote:{totalMinor:12500,currency:'EUR'}};
  assert.equal(needsAutonomyChoice(job), false);
  job.bookingAutonomyRequired = true;
  assert.equal(needsAutonomyChoice(job), true);
  assert.equal(autonomyChoiceKey(job), 'job|2|12500|EUR|service');
  job.bookingAutonomy = {mode:'automatic',version:1,scopeVersion:2,amountMinor:12500,currency:'EUR',professionalAccountId:'pro',professionalProfileId:'service',decidedAt:'2026-10-25T12:00:00Z'};
  assert.equal(automaticReleaseOn(job), true);
  assert.equal(needsAutonomyChoice(job), false);
  for (const [key,value] of Object.entries({scopeVersion:3,amountMinor:13000,currency:'USD',professionalAccountId:'other',professionalProfileId:'other',decidedAt:''})) {
    const changed = {...job,bookingAutonomy:{...job.bookingAutonomy,[key]:value}};
    assert.equal(automaticReleaseOn(changed),false,key);
    assert.equal(needsAutonomyChoice(changed),true,key);
  }
  job.bookingAutonomy.mode = 'manual';
  job.quote.totalMinor = 14000;
  assert.equal(needsAutonomyChoice(job),false);
  assert.equal(automaticReleaseOn(job),false);
});

const navigationBundle = await build({entryPoints:['src/lib/prebooking.ts'],bundle:true,platform:'node',format:'cjs',write:false});
const navigationModule = {exports:{}};
new Function('module','exports',navigationBundle.outputFiles[0].text)(navigationModule,navigationModule.exports);
const {canReviseUnbookedRequest,previousIntakeStep}=navigationModule.exports;
test('an agreed price can go back, while a reserved visit cannot rewind',()=>{
 const w={job:{id:'job',quoteApproved:true,evidence:[]}};
 assert.equal(canReviseUnbookedRequest(w),true);
 for(const change of [{appointment:{slotId:'reserved'}},{paymentAuthorizedVersion:1},{workStarted:true},{paid:true}]) {
  assert.equal(canReviseUnbookedRequest({...w,job:{...w.job,...change}}),false);
 }
 assert.equal(previousIntakeStep('urgency'),'detail');
 assert.equal(previousIntakeStep('location'),'budget');
 assert.equal(previousIntakeStep('matches'),'location');
});
test('stale assistant questions cannot cover the professional review step',()=>{
 const w=workspace();w.intake.step='professional_review';
 assert.equal(requestInteraction(w,'customer'),undefined);
 assert.equal(requestReplyKey(w,'customer'),'');
});

test('price and autonomy review stays manual until Continue, without hiding later booked dialogs',()=>{
 const w=workspace();w.intake.step='plan_review';w.job.evidence=[];
 assert.equal(requestInteraction(w,'customer'),undefined);
 w.job.appointment={slotId:'booked'};
 assert.equal(requestInteraction(w,'customer').address,true);
});
