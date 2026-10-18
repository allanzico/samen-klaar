<script lang="ts" module>
  import type { IconName } from '../../ui/icons';
  export type NavItem = { label: string; icon: IconName; current: boolean; onclick: () => void; phoneIcon?: boolean };
</script>

<script lang="ts">
  import type { Snippet } from 'svelte';
  import Icon from '../../ui/Icon.svelte';
  import Logo from '../../ui/Logo.svelte';
  import LanguageToggle from './LanguageToggle.svelte';
  import RoleSwitch from './RoleSwitch.svelte';
  import AccountMenu from './AccountMenu.svelte';
  import NotificationCenter from './NotificationCenter.svelte';
  import type { AppActions, AppState } from '../../lib/types';
  import { t } from '../../lib/i18n.svelte';

  // Navy app bars. On phones the nav collapses: customers keep a requests icon,
  // professionals get the bottom tab bar, and language and role move into the account sheet.
  let { app, actions, nav = [], badge, extra, onHome, onSelectNotification, hideOnPhone = false }: {
    app: AppState; actions: AppActions; nav?: NavItem[]; badge?: string; extra?: Snippet; onHome: () => void;
    onSelectNotification?: (id: string) => Promise<void>; hideOnPhone?: boolean;
  } = $props();
</script>

<header class="appbar on-navy" class:hide-on-phone={hideOnPhone}>
  <a class="home" href="/" aria-label={t('{brand} home', { brand: app.brandName })} onclick={(event) => { event.preventDefault(); onHome(); }}>
    <Logo onNavy name={app.brandName}/>
  </a>
  {#if badge}<span class="role-badge">{badge}</span>{/if}
  {#if nav.length}
    <nav class="nav" aria-label={t('Main navigation')}>
      {#each nav as item (item.label)}
        <button type="button" class="nav-link" aria-current={item.current ? 'page' : undefined} onclick={item.onclick}><Icon name={item.icon}/>{item.label}</button>
      {/each}
    </nav>
  {/if}
  <div class="end">
    {@render extra?.()}
    <span class="wide-only"><RoleSwitch {app} {actions}/></span>
    <span class="wide-only"><LanguageToggle/></span>
    {#each nav.filter((item) => item.phoneIcon) as item (item.label)}
      <button type="button" class="icon-btn phone-only" aria-label={item.label} aria-current={item.current ? 'page' : undefined} onclick={item.onclick}><Icon name={item.icon}/></button>
    {/each}
    {#if onSelectNotification}<NotificationCenter onSelect={onSelectNotification} workspaces={app.workspaces}/>{/if}
    <AccountMenu {app} {actions}/>
  </div>
</header>

<style>
  .appbar { position: sticky; top: 0; z-index: 30; display: flex; align-items: center; gap: 14px; min-height: var(--sk-topbar-h); padding: 10px 20px 10px 24px; background: var(--sk-brand); color: var(--sk-on-brand); }
  .home { display: inline-flex; align-items: center; min-height: 44px; border-radius: 10px; text-decoration: none; color: inherit; }
  .nav { display: flex; gap: 4px; margin-left: 10px; }
  .nav-link { display: inline-flex; align-items: center; gap: 8px; min-height: 40px; padding: 0 14px; border: 0; border-radius: var(--sk-r-card); background: transparent; color: var(--sk-on-brand-2); font: 500 14px/1 var(--sk-ft); white-space: nowrap; }
  .nav-link:hover { color: var(--sk-on-brand); }
  .nav-link[aria-current="page"] { background: var(--sk-brand-2); color: var(--sk-on-brand); font-weight: 600; }
  .end { display: flex; align-items: center; gap: 10px; margin-left: auto; }
  .phone-only { display: none; }
  @media (max-width: 1023px) { .nav-link { padding: 0 10px; } }
  @media (max-width: 879px) { .nav { display: none; } .phone-only { display: inline-grid; } }
  @media (max-width: 639px) {
    .appbar { gap: 8px; padding: 8px 8px 8px 16px; }
    .wide-only { display: none; }
    .end { gap: 2px; }
    .hide-on-phone { display: none; }
  }
</style>
