<script lang="ts">
  import Modal from '../../ui/Modal.svelte';
  import type { AppActions, AppState } from '../../lib/types';
  import { t } from '../../lib/i18n.svelte';

  // Confirms deleting a request that was never approved. A customer deletes it for good; a
  // professional declines it (if it is still active) and takes it off their list.
  let { app, actions, open = $bindable(false), workspaceId, title, counterpart = '', cancelled = false, onDeleted }: {
    app: AppState; actions: AppActions; open: boolean; workspaceId: string; title: string; counterpart?: string; cancelled?: boolean; onDeleted?: () => void;
  } = $props();
  const pro = $derived(app.role === 'professional');
  const name = $derived(counterpart || (pro ? t('the customer') : t('the professional')));
  const shown = $derived(title === 'New request' ? t('New request') : title);

  async function confirm() {
    if (await actions.deleteRequest(workspaceId)) { open = false; onDeleted?.(); }
  }
</script>

<Modal bind:open title={pro ? t('Remove this job?') : t('Delete this request?')}>
  <p><b>{shown}</b></p>
  {#if pro}
    <p>{cancelled ? t('This removes the job from your list. Nothing changes for {name}.', { name }) : t('This declines the job and removes it from your list. {name} keeps their request and can choose another professional.', { name })}</p>
  {:else}
    <p>{t('This deletes the request and its conversation for good. You can’t undo this.')}</p>
    {#if counterpart && !cancelled}<p class="secondary-text">{t('{name} will no longer see it.', { name })}</p>{/if}
  {/if}
  {#if app.error && open}<p class="reason" role="alert">{t(app.error)}</p>{/if}
  {#snippet footer()}
    <button type="button" class="btn btn-danger" disabled={app.pending} aria-busy={app.pending ? 'true' : undefined} onclick={confirm}>{pro ? t('Remove job') : t('Delete request')}</button>
    <button type="button" class="btn btn-outline" onclick={() => { open = false; }}>{pro ? t('Keep job') : t('Keep request')}</button>
  {/snippet}
</Modal>
