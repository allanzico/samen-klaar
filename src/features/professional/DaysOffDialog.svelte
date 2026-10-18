<script lang="ts">
  import Modal from '../../ui/Modal.svelte';
  import Banner from '../../ui/Banner.svelte';
  import { t } from '../../lib/i18n.svelte';
  import { visitRange } from '../../lib/format';
  import { showToast } from '../../ui/toast-state.svelte';
  import { scheduleApi, type DaysOffPreview } from './professional-schedule-api';

  let { open = $bindable(false), timezone = 'Europe/Amsterdam', onSaved }: { open: boolean; timezone?: string; onSaved: () => void } = $props();
  let firstDay = $state('');
  let lastDay = $state('');
  let note = $state('');
  let preview = $state<DaysOffPreview | null>(null);
  let busy = $state(false);
  let error = $state('');
  $effect(() => {
    const first = firstDay, last = lastDay, visible = open;
    let cancelled = false;
    preview = null;
    if (!visible || !first || !last) return;
    const timer = setTimeout(() => { void scheduleApi.previewDaysOff(first, last).then((result) => { if (!cancelled) { preview = result; error = ''; } }).catch((e) => { if (!cancelled) error = e instanceof Error ? e.message : t('Could not preview days off.'); }); }, 300);
    return () => { cancelled = true; clearTimeout(timer); };
  });
  async function save(event: SubmitEvent) {
    event.preventDefault(); if (busy || !preview) return;
    busy = true; error = '';
    try { await scheduleApi.addDaysOff(firstDay, lastDay, note); open = false; firstDay = ''; lastDay = ''; note = ''; showToast(t('Days off added')); onSaved(); }
    catch (e) { error = e instanceof Error ? e.message : t('Could not save days off.'); }
    finally { busy = false; }
  }
</script>

<Modal bind:open title={t('Add days off')}>
  <form onsubmit={save}>
    <p class="secondary-text">{t('Customers can’t book you on these days.')}</p>
    <div class="pair"><div class="field"><label for="off-first">{t('First day off')}</label><input id="off-first" class="input" type="date" bind:value={firstDay} required disabled={busy}/></div><div class="field"><label for="off-last">{t('Last day off')}</label><input id="off-last" class="input" type="date" min={firstDay} bind:value={lastDay} required disabled={busy}/></div></div>
    <div class="field"><label for="off-note">{t('Note for yourself (optional)')}</label><input id="off-note" class="input" bind:value={note} maxlength="80" placeholder={t('For example, holiday')} disabled={busy}/></div>
    {#if error}<Banner kind="error" text={t(error)}/>{/if}
    {#if preview}<p aria-live="polite">{t('{count} free times will be removed.', { count: preview.removedCount })}</p>{#if preview.bookedVisits.length}<Banner kind="warn" text={t('Booked visits stay booked. If you cannot go, open the job.')}/><ul class="booked">{#each preview.bookedVisits as visit (visit.jobId)}<li><a href={`/?workspaceId=${encodeURIComponent(visit.workspaceId)}`}>{visit.title}</a><small>{visitRange(visit.startUtc, visit.endUtc, timezone)}</small></li>{/each}</ul>{/if}{/if}
    <div class="actions"><button class="btn btn-primary" disabled={busy || !preview}>{busy ? t('Saving…') : t('Add days off')}</button><button class="btn btn-outline" type="button" disabled={busy} onclick={() => { open = false; }}>{t('Cancel')}</button></div>
  </form>
</Modal>

<style>
  form { display: grid; gap: 18px; }
  .pair { display: grid; grid-template-columns: repeat(2,minmax(0,1fr)); gap: 12px; }
  .actions { display: flex; flex-wrap: wrap; gap: 12px; }
  .booked { display: grid; gap: 12px; }
  .booked li { display: grid; gap: 4px; overflow-wrap: anywhere; }
  .booked small { color: var(--sk-muted); }
  @media(max-width: 639px) { .pair { grid-template-columns: 1fr; } .actions { flex-direction: column; } }
</style>
