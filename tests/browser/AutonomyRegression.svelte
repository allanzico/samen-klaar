<script lang="ts">
  import RequestPage from '../../src/features/request/RequestPage.svelte';
  import { createAppState } from '../../src/state/app-state.svelte';
  import { setLocale } from '../../src/lib/i18n.svelte';
  import { emptyWorkspace } from './fixtures';
  import type { AppActions, WorkspaceState } from '../../src/lib/types';
  const params = new URLSearchParams(location.search);
  const app = createAppState();
  app.role = 'customer';
  app.workspaceId = 'autonomy-fixture';
  app.providerConfig = {paymentsAvailable:true,automaticReleaseAvailable:!params.has('unavailable')} as typeof app.providerConfig;
  const initial = emptyWorkspace('autonomy-fixture');
  Object.assign(initial.job, {id:'fixture-job',title:'Repair the kitchen sink',professional:'Test professional',customer:'Test customer',professionalAccountId:'fixture-professional',professionalProfileId:'fixture-service',serviceAddress:'Public test address',bookingAutonomyRequired:true});
  initial.job.quote.totalMinor = 12500;
  initial.job.quote.lines = [{label:'Repair the sink connection',amountMinor:12500,basis:'Agreed scope'}];
  const visitStart = new Date(Date.now()+7*86400000);
  initial.availableSlots = [{slotId:'fixture-slot',startsAt:visitStart.toLocaleString(),startUtc:visitStart.toISOString(),endUtc:new Date(+visitStart+3600000).toISOString(),timezone:'Europe/Amsterdam'}];
  let workspace = $state<WorkspaceState>(JSON.parse(sessionStorage.getItem('autonomy-fixture') || 'null') || initial);
  $effect(() => { app.serverState = workspace; });
  function save() { sessionStorage.setItem('autonomy-fixture', JSON.stringify(workspace)); app.serverState = workspace; }
  function reset() { workspace = structuredClone(initial); save(); }
  const actions = {act:async(action:string,value:string,_location:unknown,_extra:unknown,_proposal:unknown,_files:unknown,timing: 'at_booking'|'after_completion')=>{
    if(action==='set-booking-autonomy') {
      workspace.job.bookingAutonomy = {mode:value.startsWith('automatic|')?'automatic':'manual',version:(workspace.job.bookingAutonomy?.version||0)+1,scopeVersion:workspace.job.scopeVersion,amountMinor:workspace.job.quote.totalMinor,currency:workspace.job.quote.currency,professionalAccountId:'fixture-professional',professionalProfileId:'fixture-service',decidedAt:new Date().toISOString()};
      if(value.startsWith('automatic|')) workspace.job.paymentAuthorizationTiming='at_booking';
    } else if(action==='approve-and-book') { workspace.job.quoteApproved=true; workspace.job.appointment=workspace.availableSlots[0]; workspace.job.appointmentConfirmation={appointment:workspace.availableSlots[0],status:'pending',dueAt:new Date(Date.now()+7200000).toISOString()}; }
    else if(action==='set-payment-timing') workspace.job.paymentAuthorizationTiming=timing;
    save(); return true;
  },beginPayPalCheckout:async()=>{},refresh:async()=>{},dismissError:()=>{},focusComposer:()=>{}} as unknown as AppActions;
  setLocale(params.has('nl')?'nl':'en',false);
</script>
<nav class="fixture-controls"><span>Synthetic booking UI</span><button class="btn btn-quiet btn-sm" onclick={reset}>Reset fixture</button><a href="?nl">Nederlands</a><a href="?unavailable">Provider unavailable</a></nav>
<RequestPage {app} {actions} {workspace} onBack={()=>{}}/>
<style>.fixture-controls {display:flex;flex-wrap:wrap;align-items:center;gap:16px;padding:12px 20px;border-bottom:1px solid var(--sk-line)} :global(body) {background:var(--sk-surface)}</style>
