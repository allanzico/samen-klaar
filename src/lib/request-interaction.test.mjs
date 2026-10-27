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

globalThis.$state = (value) => value;
const stepBundle = await build({entryPoints:['src/lib/next-steps.ts'],bundle:true,platform:'node',format:'cjs',write:false});
const stepModule = {exports:{}};
new Function('module','exports',stepBundle.outputFiles[0].text)(stepModule,stepModule.exports);
const {customerStep,proStep} = stepModule.exports;
const completionContext = () => ({
  app:{paypalStatus:'authorized',providerConfig:{paymentsAvailable:true}}, actions:{}, code:'review',
  workspace:{workspaceId:'fixture',completionAutomation:'checking',availableSlots:[],job:{
    id:'job',professional:'Test professional',professionalAccountId:'pro',professionalProfileId:'service',
    scopeVersion:1,detailAdded:true,quote:{totalMinor:12500,currency:'EUR'},
    bookingAutonomy:{mode:'automatic',decidedAt:'2026-10-26T10:00:00Z',scopeVersion:1,amountMinor:12500,currency:'EUR',professionalAccountId:'pro',professionalProfileId:'service'}
  }}
});
test('automatic completion checking does not ask the customer to approve the work',()=>{
  const view=customerStep(completionContext());
  assert.equal(view.kind,'waiting');
  assert.match(view.title,/assistant.*checking/i);
  assert.equal(view.primary,undefined);
});
test('unclear automatic review puts corrections with the professional instead of offering invalid approval',()=>{
  const ctx=completionContext();ctx.workspace.completionAutomation='needs_review';
  const view=customerStep(ctx);
  assert.match(view.title,/needs to improve/i);
  assert.match(view.text,/professional to fix/i);
  assert.equal(view.primary,undefined);
  assert.match(view.secondary.label,/clearer photos/i);
});
test('a failed automated check exposes a manual fallback instead of waiting forever',()=>{
  const ctx=completionContext();ctx.automationEscalation='agent_blocked';
  const view=customerStep(ctx);
  assert.equal(view.kind,'your-turn');
  assert.match(view.text,/couldn.t finish/i);
});
test('ordinary manual completion keeps the existing approval action',()=>{
  const ctx=completionContext();ctx.workspace.completionAutomation='';ctx.workspace.job.bookingAutonomy.mode='manual';
  assert.equal(customerStep(ctx).primary.label,'Approve the work');
});
test('personal fallback revokes automatic payment authority before approving completion',async()=>{
  const ctx=completionContext();ctx.automationEscalation='agent_blocked';
  const calls=[];ctx.actions.act=async(...args)=>{calls.push(args);return true};
  await customerStep(ctx).primary.run();
  assert.deepEqual(calls,[['set-booking-autonomy','manual'],['approve-completion']]);
  calls.length=0;ctx.actions.act=async(...args)=>{calls.push(args);return false};
  await customerStep(ctx).primary.run();
  assert.deepEqual(calls,[['set-booking-autonomy','manual']]);
});
test('documented automatic completion waits for payment release without a second payment button',()=>{
  const ctx=completionContext();ctx.code='payment';ctx.workspace.completionAutomation='releasing';ctx.workspace.job.completionApprovedByAgent=true;
  assert.equal(customerStep(ctx).kind,'waiting');
  assert.equal(customerStep(ctx).primary,undefined);
  ctx.app.paypalStatus='expired';
  assert.equal(customerStep(ctx).kind,'your-turn');
  assert.match(customerStep(ctx).banner.title,/expired/i);
});
test('a below-limit offer says when it is waiting for the professional personally',()=>{
  const ctx=completionContext();ctx.code='waiting_price';
  ctx.workspace.job.priceProposal={id:'offer',status:'pending',actor:'customer',amountMinor:9500,professionalReviewRequired:true};
  const view=customerStep(ctx);
  assert.match(view.text,/personally/i);
  assert.match(view.title,/review your offer/i);
});

test('professional submission saves new note details before checking and never skips a failed save',async()=>{
 const ctx=completionContext();ctx.code='in_progress';ctx.noteReady=true;ctx.noteEditing=true;ctx.workspace.job.customer='Test customer';ctx.workspace.job.evidence=[{assetId:'photo'}];
 const calls=[];ctx.saveNote=async()=>{calls.push('save-note');return true};ctx.actions.act=async(action)=>calls.push(action);
 const view=proStep(ctx);assert.match(view.primary.label,/Check and send/);await view.primary.run();assert.deepEqual(calls,['save-note','submit-evidence']);
 calls.length=0;ctx.saveNote=async()=>{calls.push('save-note');return false};await proStep(ctx).primary.run();assert.deepEqual(calls,['save-note']);
 ctx.noteReady=false;assert.equal(proStep(ctx).primary.disabled,true);
});
test('professional correction keeps the customer waiting without an approval task',()=>{
 const ctx=completionContext();ctx.code='in_progress';ctx.workspace.job.completionRevision=true;
 const view=customerStep(ctx);assert.equal(view.kind,'waiting');assert.equal(view.primary,undefined);assert.match(view.text,/nothing for you to fix/i);
});

test('professional completion steps name the delegated reviewer only for a current grant',()=>{
 const ctx=completionContext();ctx.workspace.job.customer='Alex';ctx.workspace.job.evidence=[{assetId:'photo'}];
 ctx.code='in_progress';assert.match(proStep(ctx).helper,/customer’s assistant/i);
 ctx.code='review';assert.equal(proStep(ctx).tag.label,'Automatic review');assert.match(proStep(ctx).title,/assistant/i);
 ctx.code='payment';ctx.workspace.completionAutomation='releasing';ctx.workspace.job.completionApprovedByAgent=true;
 assert.match(proStep(ctx).title,/assistant.*releasing/i);
 for(const edit of [job=>{job.bookingAutonomy.mode='manual'},job=>{job.bookingAutonomy.amountMinor++}]){
  const manual=completionContext();manual.workspace.job.customer='Alex';edit(manual.workspace.job);
  assert.equal(proStep(manual).tag.label,'Customer checking');assert.match(proStep(manual).text,/Alex checks/);
 }
});
