<script lang="ts">
  import Icon from '../../ui/Icon.svelte';
  import Modal from '../../ui/Modal.svelte';
  import LanguageToggle from './LanguageToggle.svelte';
  import RoleSwitch from './RoleSwitch.svelte';
  import type { AppActions, AppState } from '../../lib/types';
  import { initials } from '../../lib/trade';
  import { media } from '../../lib/media.svelte';
  import { agentUi } from '../../lib/agent.svelte';
  import { t } from '../../lib/i18n.svelte';

  // A06: a popover under the avatar on web, a bottom sheet with language and role on phones.
  let { app, actions }: { app: AppState; actions: AppActions } = $props();
  let open = $state(false);
  let sheetOpen = $state(false);
  let root = $state<HTMLElement>();
  let trigger = $state<HTMLButtonElement>();

  const name = $derived(app.session?.account?.displayName || app.session?.account?.email || t('Your account'));
  const email = $derived(app.session?.account?.displayName ? app.session?.account?.email : '');
  const canBecomePro = $derived(app.role === 'customer' && !app.session?.memberships?.some((m) => m.role === 'professional'));

  function toggle() {
    if (media.phone) { sheetOpen = true; return; }
    open = !open;
    if (open) queueMicrotask(() => root?.querySelector<HTMLElement>('.menu-row')?.focus());
  }
  function close(focus = true) { open = false; if (focus) trigger?.focus(); }
  function onWindow(event: Event) {
    if (!open) return;
    if (event instanceof KeyboardEvent) { if (event.key === 'Escape') close(); }
    else if (!root?.contains(event.target as Node)) close(false);
  }
</script>

<svelte:window onkeydown={onWindow} onclick={onWindow}/>

{#snippet menuBody()}
  <div class="who">
    <span class="avatar avatar-44" aria-hidden="true">{initials(name)}</span>
    <span><b>{name}</b>{#if email}<small>{email}</small>{/if}</span>
  </div>
  {#if media.phone}
    <div class="sheet-group"><span class="group-label" id="sheet-language">{t('Language')}</span><LanguageToggle tone="light"/></div>
    <RoleSwitch {app} {actions} tone="light"/>
  {/if}
  <div class="rows-wrap">
    {#if app.role === 'customer'}<button type="button" class="menu-row" onclick={() => { close(false); sheetOpen = false; actions.showProfile(); }}><Icon name="user"/>{t('Your profile')}</button>{/if}
    {#if app.role === 'customer' || app.role === 'professional'}<button type="button" class="menu-row" onclick={() => { close(false); sheetOpen = false; agentUi.settingsOpen = true; }}><Icon name="bot"/>{t('Assistant preferences')}</button>{/if}
    {#if canBecomePro}<button type="button" class="menu-row" disabled={app.pending} onclick={actions.beginProfessional}><Icon name="briefcase"/>{t('Become a professional')}</button>{/if}
    <button type="button" class="menu-row" onclick={actions.logout}><Icon name="sign-out"/>{t('Sign out')}</button>
  </div>
{/snippet}

<div class="account" bind:this={root}>
  <button type="button" class="avatar-btn" bind:this={trigger} aria-haspopup="true" aria-expanded={open || sheetOpen} aria-label={t('Account menu for {name}', { name })} onclick={toggle}>
    <span class="avatar-face" aria-hidden="true">{initials(name)}</span>
  </button>
  {#if open && !media.phone}
    <div class="popover">{@render menuBody()}</div>
  {/if}
</div>
{#if media.phone}
  <Modal bind:open={sheetOpen} title={t('Account')}>{@render menuBody()}</Modal>
{/if}

<style>
  .account { position: relative; }
  .avatar-btn { display: grid; place-items: center; width: 44px; height: 44px; padding: 0; border: 0; border-radius: 50%; background: transparent; }
  .avatar-btn:focus-visible { outline-color: var(--sk-on-brand); }
  .avatar-face { display: grid; place-items: center; width: 36px; height: 36px; border-radius: 50%; background: var(--sk-ground); color: var(--sk-ink); font: 700 12px/1 var(--sk-fd); }
  .popover { position: absolute; top: calc(100% + 10px); right: 0; z-index: 40; display: grid; gap: 12px; width: 340px; padding: 18px; border: 1px solid var(--sk-line); border-radius: var(--sk-r-box); background: var(--sk-surface); color: var(--sk-ink); box-shadow: var(--sk-shadow-pop); }
  .who { display: flex; align-items: center; gap: 12px; padding-bottom: 12px; border-bottom: 1px solid var(--sk-line-2); }
  .who b { display: block; font-size: 14px; font-weight: 600; }
  .who small { display: block; font-size: 13px; color: var(--sk-muted); overflow-wrap: anywhere; }
  .rows-wrap { display: grid; }
  .menu-row { display: flex; align-items: center; gap: 12px; min-height: 40px; padding: 0 12px; border: 0; border-radius: var(--sk-r-card); background: transparent; color: var(--sk-ink); font-size: 14px; font-weight: 500; text-align: left; }
  .menu-row:hover:not(:disabled) { background: var(--sk-ground); }
  .menu-row :global(.icon) { width: 20px; height: 20px; }
  .sheet-group { display: flex; align-items: center; justify-content: space-between; gap: 12px; }
  .group-label { font-weight: 600; }
</style>
