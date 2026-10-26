<script lang="ts">
  import RequestPage from '../../src/features/request/RequestPage.svelte';
  import { createAppState } from '../../src/state/app-state.svelte';
  import { setLocale } from '../../src/lib/i18n.svelte';
  import { emptyWorkspace } from './fixtures';
  import type { AppActions, WorkspaceState } from '../../src/lib/types';
  const params = new URLSearchParams(location.search);
  const mobileCheck = params.has('mobile-check');
  const scenario = params.get('scenario') || 'planning';
  const storageKey = `planning-fixture-${scenario}`;
  const app = createAppState();
  app.role = 'customer';
  app.workspaceId = 'autonomy-fixture';
  app.providerConfig = {paymentsAvailable:true,automaticReleaseAvailable:!params.has('unavailable')} as typeof app.providerConfig;
  const initial = emptyWorkspace('autonomy-fixture');
  Object.assign(initial.job, {id:'fixture-job',title:'Repair the kitchen sink',professional:'Test professional',customer:'Test customer',professionalAccountId:'fixture-professional',professionalProfileId:'fixture-service',serviceAddress:'Public test address',bookingAutonomyRequired:true});
  initial.intake = {step:'plan_review',description:'Repair the kitchen sink',detail:'The sink connection leaks',category:'Plumbing',route:'catalog',question:'Review the price and payment options',options:[],messages:[],urgency:'Flexible',budget:'Show the price',matches:[],location:null,needsReview:false};
  initial.job.quote.totalMinor = 12500;
  initial.job.quote.lines = [{label:'Repair the sink connection',amountMinor:12500,basis:'Agreed scope'}];
  const visitStart = new Date(Date.now()+7*86400000);
  initial.availableSlots = [{slotId:'fixture-slot',startsAt:visitStart.toLocaleString(),startUtc:visitStart.toISOString(),endUtc:new Date(+visitStart+3600000).toISOString(),timezone:'Europe/Amsterdam'}];
  if (scenario === 'professional') initial.intake.step='professional_review';
  if (scenario === 'offer') {
    initial.job.priceProposal={id:'fixture-offer',status:'pending',actor:'customer',amountMinor:9500,note:'Could you accept this total?',scopeVersion:1,professionalReviewRequired:true};
  }
  if (scenario === 'checking' || scenario === 'unclear') {
    initial.intake.step='selected';
    Object.assign(initial.job,{quoteApproved:true,workStarted:true,evidenceSubmitted:true,detailAdded:true,appointment:initial.availableSlots[0]});
    initial.job.bookingAutonomy={mode:'automatic',version:1,scopeVersion:1,amountMinor:12500,currency:'EUR',professionalAccountId:'fixture-professional',professionalProfileId:'fixture-service',decidedAt:new Date().toISOString()};
    initial.job.evidence=[{kind:'note',label:'Work note',description:'The work is complete.'}];
    initial.completionAutomation=scenario==='checking'?'checking':'needs_review';
    if(scenario==='unclear')initial.job.evidenceReview={summary:'The note does not describe the agreed repair, and no repair photo is available.',coverage:[{scopeItem:'Repair the sink connection',coverage:'unclear',observation:'The completed repair is not documented.',evidenceIds:[]}],limitations:'Photos cannot confirm workmanship or safety.',reviewedAt:new Date().toISOString()};
  }
  let workspace = $state<WorkspaceState>(JSON.parse(sessionStorage.getItem(storageKey) || 'null') || initial);
  $effect(() => { app.serverState = workspace; });
  function save() { sessionStorage.setItem(storageKey, JSON.stringify(workspace)); app.serverState = workspace; }
  function reset() { workspace = structuredClone(initial); save(); }
  const actions = {act:async(action:string,value:string,_location:unknown,_extra:unknown,_proposal:unknown,_files:unknown,timing: 'at_booking'|'after_completion')=>{
    if(action==='start-planning') { workspace.intake!.step='selected'; } else if(action==='set-booking-autonomy') {
      workspace.job.bookingAutonomy = {mode:value.startsWith('automatic|')?'automatic':'manual',version:(workspace.job.bookingAutonomy?.version||0)+1,scopeVersion:workspace.job.scopeVersion,amountMinor:workspace.job.quote.totalMinor,currency:workspace.job.quote.currency,professionalAccountId:'fixture-professional',professionalProfileId:'fixture-service',decidedAt:new Date().toISOString()};
      if(value.startsWith('automatic|')) workspace.job.paymentAuthorizationTiming='at_booking';
      else workspace.completionAutomation=undefined;
    } else if(action==='approve-and-book') { workspace.job.quoteApproved=true; workspace.job.appointment=workspace.availableSlots[0]; workspace.job.appointmentConfirmation={appointment:workspace.availableSlots[0],status:'pending',dueAt:new Date(Date.now()+7200000).toISOString()}; }
    else if(action==='set-payment-timing') workspace.job.paymentAuthorizationTiming=timing;
    else if(action==='approve-completion')workspace.job.completionApproved=true;
    save(); return true;
  },beginPayPalCheckout:async()=>{},refresh:async()=>{},dismissError:()=>{},focusComposer:()=>{},openProposal:()=>{app.composeMode='price'},closeProposal:()=>{app.composeMode='message'}} as unknown as AppActions;
  setLocale(params.has('nl')?'nl':'en',false);
</script>
{#if mobileCheck}<main class="mobile-check"><h1>390px booking check</h1><iframe title="Mobile request" width="390" height="690" src="?frame&nl&scenario={scenario}"></iframe></main>{:else}
<nav class="fixture-controls"><span>Synthetic booking UI</span><button class="btn btn-quiet btn-sm" onclick={reset}>Reset fixture</button><a href="?nl&scenario={scenario}">Nederlands</a><a href="?unavailable">Provider unavailable</a><a href="?scenario=professional">Professional review</a><a href="?scenario=checking">Checking</a><a href="?scenario=unclear">Unclear</a><a href="?scenario=offer">Offer</a></nav>
<RequestPage {app} {actions} {workspace} onBack={()=>{}}/>
{/if}
<style>.mobile-check {padding:16px}.mobile-check iframe {border:1px solid var(--sk-line);margin-top:12px}.fixture-controls {white-space:nowrap;overflow:hidden;background:var(--sk-ground);position:sticky;top:0;z-index:30;height:var(--sk-topbar-h);box-sizing:border-box;display:flex;flex-wrap:wrap;align-items:center;gap:16px;padding:12px 20px;border-bottom:1px solid var(--sk-line)} :global(body) {background:var(--sk-surface)}</style>
