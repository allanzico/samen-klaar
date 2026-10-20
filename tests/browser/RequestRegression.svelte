<script lang="ts">
  import { onMount, tick } from 'svelte';
  import PlanStage from '../../src/features/request/PlanStage.svelte';
  import AgreedLedger from '../../src/features/request/AgreedLedger.svelte';
  import { emptyWorkspace } from './fixtures';
  import type { AppActions, AppState, WorkspaceState } from '../../src/lib/types';
  let result = $state('Checking negotiated quote…');
  let failure = $state(false);
  const workspace = {
    ...emptyWorkspace('render-regression'),
    job: { ...emptyWorkspace('render-regression').job, id: 'job', title: 'Plumbing', professional: 'Test professional', customer: 'Test customer',
      serviceAddress: 'Public test address', scopeVersion: 3,
      quote: { currency: 'EUR', totalMinor: 8000, durationMinutes: 60, expires: '', materialsPolicy: 'Materials extra', exclusions: [], lines: [
        { label: 'Service', amountMinor: 10000, basis: 'Published price' },
        { label: 'Price adjustment', amountMinor: -1000, basis: 'First agreement' },
        { label: 'Price adjustment', amountMinor: -1000, basis: 'Second agreement' }
      ] }
    }
  } as WorkspaceState;
  const app = { pending: false, role: 'customer' } as AppState;
  const actions = {} as AppActions;
  onMount(async () => { await tick(); if (!failure) result = 'PASS: both negotiated quote views render every price adjustment'; });
</script>
<main style="max-width:900px;margin:40px auto;padding:20px">
  <h1>Negotiated quote regression</h1><p role="status">{result}</p>
  <svelte:boundary onerror={(error) => { failure = true; result = `FAIL: ${String(error)}`; }}>
    <PlanStage {app} {actions} {workspace} code="quote"/>
    <AgreedLedger job={workspace.job}/>
    {#snippet failed()}<p role="alert">Quote rendering failed.</p>{/snippet}
  </svelte:boundary>
</main>
