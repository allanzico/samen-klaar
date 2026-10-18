<script lang="ts">
  import type { Snippet } from 'svelte';
  import Icon from './Icon.svelte';
  import { t } from '../lib/i18n.svelte';

  // A centred dialog on web and a bottom sheet on phones. The native dialog provides
  // the focus trap and Escape; focus returns to the element that opened it.
  let { open = $bindable(false), title, wide = false, children, footer }: { open: boolean; title: string; wide?: boolean; children: Snippet; footer?: Snippet } = $props();
  let dialog = $state<HTMLDialogElement>();
  let opener: HTMLElement | null = null;
  const id = `modal-${Math.random().toString(36).slice(2, 8)}`;

  $effect(() => {
    if (!dialog) return;
    if (open && !dialog.open) { opener = document.activeElement as HTMLElement | null; dialog.showModal(); }
    else if (!open && dialog.open) dialog.close();
  });
  function closed() { open = false; opener?.focus?.(); opener = null; }
</script>

<dialog bind:this={dialog} class="modal" class:wide aria-labelledby={id} onclose={closed} onclick={(event) => { if (event.target === dialog) open = false; }}>
  <div class="modal-body">
    <span class="handle" aria-hidden="true"></span>
    <div class="modal-head">
      <h2 {id}>{title}</h2>
      <button type="button" class="icon-btn" aria-label={t('Close')} onclick={() => { open = false; }}><Icon name="close"/></button>
    </div>
    {@render children()}
    {#if footer}<div class="modal-actions">{@render footer()}</div>{/if}
  </div>
</dialog>

<style>
  .modal { width: min(520px, calc(100vw - 32px)); max-height: calc(100svh - 48px); margin: auto; padding: 0; border: 0; border-radius: var(--sk-r-box); background: var(--sk-surface); color: var(--sk-ink); box-shadow: var(--sk-shadow-dialog); overflow: auto; }
  .modal.wide { width: min(860px, calc(100vw - 32px)); }
  .modal::backdrop { background: var(--sk-scrim); }
  .modal-body { display: grid; gap: 18px; padding: 28px; }
  .modal-head { display: flex; align-items: flex-start; justify-content: space-between; gap: 12px; }
  .modal-head h2 { font: 700 22px/1.15 var(--sk-fd); letter-spacing: -0.015em; padding-top: 4px; }
  .modal-head .icon-btn { margin: -6px -10px 0 0; }
  .modal-actions { display: flex; flex-wrap: wrap; gap: 12px; }
  .handle { display: none; }
  @media (max-width: 639px) {
    .modal, .modal.wide { width: 100%; max-width: 100%; max-height: 92svh; margin: auto 0 0; border-radius: var(--sk-r-sheet) var(--sk-r-sheet) 0 0; box-shadow: var(--sk-shadow-sheet); }
    .modal-body { padding: 10px 16px calc(28px + env(safe-area-inset-bottom)); gap: 16px; }
    .handle { display: block; justify-self: center; width: 40px; height: 4px; border-radius: 2px; background: var(--sk-line); }
    .modal-head h2 { font-size: 19px; }
    .modal-actions { flex-direction: column; }
    .modal-actions :global(.btn) { width: 100%; }
  }
</style>
