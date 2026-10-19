<script lang="ts">
  import type { AppActions, AppState, WorkspaceRole } from '../../lib/types';
  import { t } from '../../lib/i18n.svelte';

  // Switch over the account's own memberships, in a fixed order. A pending professional can open
  // onboarding; operator access shows only once it is active.
  let { app, actions, tone = 'navy' }: { app: AppState; actions: AppActions; tone?: 'navy' | 'light' } = $props();
  const order: WorkspaceRole[] = ['customer', 'professional', 'operator'];
  const usable = (role: WorkspaceRole, status: string) => status === 'active' || (role === 'professional' && status === 'pending');
  const roles = $derived(order.filter((role) => (app.session?.memberships ?? []).some((item) => item.role === role && usable(item.role, item.status))));
  const labels: Record<WorkspaceRole, string> = { customer: 'Customer', professional: 'Professional', operator: 'Operator' };
  const label = (role: WorkspaceRole) => t(labels[role]);
</script>

{#if roles.length > 1}
  <div class="switch {tone}" role="tablist" aria-label={t('Account workspace')}>
    {#each roles as role (role)}
      <button type="button" role="tab" aria-selected={role === app.role} disabled={app.pending || app.loading} onclick={() => { if (role !== app.role) void actions.switchWorkspace(role); }}>{label(role)}</button>
    {/each}
  </div>
{/if}

<style>
  .switch { display: inline-flex; gap: 2px; padding: 4px; border-radius: var(--sk-r-pill); }
  .switch button { min-height: 36px; padding: 0 14px; border: 0; border-radius: var(--sk-r-pill); background: transparent; font: 500 13px/1 var(--sk-ft); white-space: nowrap; }
  .navy { background: var(--sk-brand-2); }
  .navy button { color: var(--sk-on-brand-2); }
  .navy button:focus-visible { outline-color: var(--sk-on-brand); }
  .navy button[aria-selected="true"] { background: var(--sk-ground); color: var(--sk-ink); font-weight: 600; }
  .light { background: var(--sk-surface-2); }
  .light button { color: var(--sk-muted); }
  .light button[aria-selected="true"] { background: var(--sk-surface); color: var(--sk-ink); font-weight: 600; box-shadow: var(--sk-shadow-raised); }
</style>
