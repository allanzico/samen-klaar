<script lang="ts">
  import ProJobs from '../../src/features/professional/ProJobs.svelte';
  import { createAppState } from '../../src/state/app-state.svelte';
  import { setLocale } from '../../src/lib/i18n.svelte';
  import type { AppActions } from '../../src/lib/types';
  const frame = new URLSearchParams(window.location.search).has('frame');
  const mobile = new URLSearchParams(window.location.search).has('mobile');
  let scenario = $state('normal');
  let fail = false;
  const app = createAppState();
  app.role = 'professional';
  const rows = [{id:'fixture-visit',jobId:'fixture-job',title:'Repair the kitchen sink',counterpart:'Test customer',role:'professional' as const,updatedAt:new Date().toISOString(),status:'booked',totalMinor:12500,currency:'EUR',visitStartUtc:new Date(Date.now()+86400000).toISOString(),visitEndUtc:new Date(Date.now()+90000000).toISOString(),visitTimezone:'Europe/Amsterdam'}];
  app.workspaces = rows;
  window.fetch = async () => {
    if(fail) {fail=false;return new Response('{}',{status:503});}
    const empty=scenario==='empty';
    return new Response(JSON.stringify({balances:empty?[]:[{currency:'EUR',receivedMinor:142500,awaitingPaymentMinor:25000},{currency:'USD',receivedMinor:5000,awaitingPaymentMinor:0}],upcomingVisits:empty?0:4,completedJobs:empty?0:12,awaitingConfirmation:empty?0:2,asOf:new Date().toISOString()}),{headers:{'Content-Type':'application/json'}});
  };
  const actions = {refresh:async()=>{},selectRequest:async()=>{}} as unknown as AppActions;
  function choose(value:string) {scenario=value;fail=value==='error';app.workspaces=value==='empty'?[]:[...rows];}
  setLocale(mobile ? 'nl' : 'en', false);
</script>
{#if !frame}
  <main class="inspection"><h1>Professional dashboard fixtures</h1><p>Synthetic data only. Desktop and 390px Dutch layout.</p><div class="frames"><iframe title="Desktop professional dashboard" src="?frame" width="880" height="850"></iframe><iframe title="Mobile professional dashboard" src="?frame&mobile" width="390" height="850"></iframe></div></main>
{:else}
  <nav class="fixture-controls" aria-label="Fixture scenarios">{#each ['normal','empty','error'] as value}<button class="btn btn-quiet btn-sm" onclick={()=>choose(value)}>{value}</button>{/each}</nav>
  <ProJobs {app} {actions} onSetTimes={()=>{}} onProfile={()=>{}}/>
{/if}
<style>
  .inspection { padding: 20px; }
  .frames { display: flex; gap: 20px; margin-top: 20px; align-items: start; }
  iframe { flex: 0 0 auto; border: 1px solid var(--sk-line); }
  .fixture-controls { display:flex; gap: 12px; padding: 12px 16px; }
</style>
