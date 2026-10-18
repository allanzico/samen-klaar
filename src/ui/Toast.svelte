<script lang="ts">
  import Icon from './Icon.svelte';
  import { clearToast, toastState } from './toast-state.svelte';

  let paused = $state(false);
  $effect(() => {
    const id = toastState.id;
    if (!toastState.message || paused) return;
    const timer = window.setTimeout(() => { if (toastState.id === id) clearToast(); }, 5000);
    return () => window.clearTimeout(timer);
  });
</script>

<div class="toast-region" role="status" aria-live="polite">
  {#if toastState.message}
    <!-- svelte-ignore a11y_no_static_element_interactions -->
    <div class="toast" onmouseenter={() => { paused = true; }} onmouseleave={() => { paused = false; }} onfocusin={() => { paused = true; }} onfocusout={() => { paused = false; }}>
      <Icon name="check-circle"/><span>{toastState.message}</span>
    </div>
  {/if}
</div>

<style>
  .toast-region { position: fixed; left: 50%; bottom: calc(24px + env(safe-area-inset-bottom)); z-index: 60; transform: translateX(-50%); pointer-events: none; }
  .toast { display: flex; align-items: center; gap: 10px; max-width: min(520px, calc(100vw - 32px)); padding: 12px 16px; border-radius: var(--sk-r-card); background: var(--sk-ink); color: var(--sk-surface); box-shadow: var(--sk-shadow-toast); font-size: 14px; pointer-events: auto; }
</style>
