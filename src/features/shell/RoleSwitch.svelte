<script lang="ts">
  import type { AppActions, AppState, WorkspaceRole } from '../../lib/types';
  import { t } from '../../lib/i18n.svelte';

  // Customer / Professional switch over the account's own memberships. Operators never appear here.
  let { app, actions, tone = 'navy' }: { app: AppState; actions: AppActions; tone?: 'navy' | 'light' } = $props();
  const roles = $derived((app.session?.memberships ?? []).filter((item) => item.status !== 'disabled' && item.role !== 'operator').map((item) => item.role));
  const label = (role: WorkspaceRole) => t(role === 'professional' ? 'Professional' : 'Customer');
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
