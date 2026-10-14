<script lang="ts">
  import { t } from '../../lib/i18n.svelte';

  let { open = $bindable(false), customer, currentTotalMinor, money, pending, description = $bindable(''), price = $bindable(''), onsubmit }: {
    open: boolean; customer: string; currentTotalMinor: number; money: (minor: number) => string; pending: boolean;
    description: string; price: string | number; onsubmit: (event: SubmitEvent) => void | Promise<void>;
  } = $props();

  let dialog = $state<HTMLDialogElement>();
  const extraMinor = $derived(Math.max(0, Math.round(Number(price) * 100) || 0));
  const reason = $derived(description.trim().length < 5 || extraMinor <= 0 ? t('Describe the extra work and add a cost to continue') : '');

  // A native dialog gives the sheet its focus trap and Escape handling.
  $effect(() => {
    if (!dialog) return;
    if (open && !dialog.open) dialog.showModal();
    else if (!open && dialog.open) dialog.close();
  });
</script>

<dialog bind:this={dialog} class="sheet" aria-labelledby="extra-work-title" onclose={() => { open = false; }}>
  <form class="sheet-form" onsubmit={onsubmit}>
    <span class="sheet-handle" aria-hidden="true"></span>
    <div class="sheet-head">
      <h2 id="extra-work-title">{t('Propose extra work')}</h2>
      <button type="button" class="icon-button" aria-label={t('Close')} onclick={() => { open = false; }}><svg class="icon" viewBox="0 0 24 24" aria-hidden="true"><path d="M18 6 6 18M6 6l12 12"/></svg></button>
    </div>
    <label>{t('Additional work')}<textarea bind:value={description} required minlength="5" maxlength="300" rows="3" placeholder={t('Describe the additional work. The customer must approve it before it is added.')}></textarea></label>
    <label>{t('Extra cost')}<input type="number" bind:value={price} required min="0.01" step="0.01" inputmode="decimal"/></label>
    <dl class="ledger sheet-ledger"><div class="total-row"><dt>{t('New total for {name}', { name: customer })}</dt><dd>{money(currentTotalMinor + extraMinor)}</dd></div></dl>
    <button class="primary" disabled={pending || !!reason}>{t('Send to {name} for approval', { name: customer })}</button>
    {#if reason}<p class="reason">{reason}</p>{/if}
    <p class="helper">{t('Wait for {name}’s approval before doing this work. If they decline, finish the original job.', { name: customer })}</p>
  </form>
</dialog>
