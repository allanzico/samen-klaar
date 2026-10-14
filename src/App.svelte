<script lang="ts">
  import { requestStatus } from './lib/request-status';
  import { onMount, tick } from 'svelte';
  import CustomerWorkspace from './features/customer/CustomerWorkspace.svelte';
  import OperatorQueue from './features/operator/OperatorQueue.svelte';
  import ProfessionalPending from './features/auth/ProfessionalPending.svelte';
  import SignIn from './features/auth/SignIn.svelte';
  import ProfessionalAvailability from './features/professional/ProfessionalAvailability.svelte';
  import ProfessionalWorkspace from './features/professional/ProfessionalWorkspace.svelte';
  import WorkspaceNavigation from './features/workspaces/WorkspaceNavigation.svelte';
  import NotificationCenter from './features/notifications/NotificationCenter.svelte';
  import PayPalPayoutSetup from './features/professional/PayPalPayoutSetup.svelte';
  import type { WorkspaceRole } from './lib/types';
  import { createAppActions } from './state/create-app-controller';
  import { createAppState } from './state/app-state.svelte';
  import { currentLocale, setLocale, t } from './lib/i18n.svelte';

  const app = createAppState();
  let job = $derived(app.serverState?.job);
  let intake = $derived(app.newConversation ? undefined : app.serverState?.intake);
  let activeJob = $derived(!app.newConversation && !!job?.id && !!job?.description);
  let composing = $derived(app.role === 'customer');
  let missingDocs = $derived(!!job?.evidenceSubmitted && !job?.detailAdded);
  let canBecomeProfessional = $derived(app.role === 'customer' && !app.session?.memberships?.some((membership) => membership.role === 'professional'));
  let statusLabel = $derived(requestStatus(app.serverState, app.role).label);
  let workspaceLayout = $derived(!!app.session?.authenticated && !!app.serverState && (app.role === 'customer' || app.role === 'professional') && !app.session.memberships?.some((m) => m.role === app.role && m.status === 'pending'));
  let viewportHeight = $state(0);
  let keyboardOpen = $state(false);
  let requestsOpen = $state(false);
  const proTabs = [
    { view: 'jobs' as const, label: 'Jobs', icon: '<rect x="3" y="7" width="18" height="13" rx="2"/><path d="M8 7V5a2 2 0 0 1 2-2h4a2 2 0 0 1 2 2v2M3 13h18"/>' },
    { view: 'times' as const, label: 'Free times', icon: '<rect x="3" y="5" width="18" height="16" rx="2"/><path d="M8 3v4M16 3v4M3 11h18"/>' },
    { view: 'profile' as const, label: 'Profile & services', icon: '<circle cx="12" cy="8" r="4"/><path d="M4 21a8 8 0 0 1 16 0"/>' }
  ];
  let proView = $state<'jobs' | 'times' | 'profile'>('jobs');
  let requestsButton = $state<HTMLButtonElement>();
  let requestPane = $state<HTMLElement>();
  let accountName = $derived(app.session?.account?.displayName || app.session?.account?.email || t('Your account'));
  let initials = $derived.by(() => {
    const words = app.session?.account?.displayName?.trim().split(/\s+/) ?? [];
    const letters = words.length > 1 ? [words[0], words[words.length - 1]] : [words[0] || app.session?.account?.email || ''];
    return letters.map((word) => word.charAt(0).toUpperCase()).join('') || '?';
  });

  $effect(() => {
    const j = app.serverState?.job;
    if (app.composeMode !== 'message' && (!j?.id || app.serverState?.cancelled || j.workStarted || j.paid || (app.composeMode === 'price' && j.quoteApproved) || (app.composeMode === 'date' && !j.quoteApproved))) app.composeMode = 'message';
  });

  async function scrollToCurrent(smooth = false, targetID?: string) {
    await tick();
    const pane = document.getElementById('request-thread');
    if (pane && targetID) {
      const target = document.getElementById(targetID);
      if (target) pane.scrollTo({top: target.getBoundingClientRect().top - pane.getBoundingClientRect().top + pane.scrollTop - 12, behavior: smooth ? 'smooth' : 'auto'});
    } else if (pane && smooth) pane.scrollTo({top:pane.scrollHeight,behavior:'smooth'});
    else if (!pane) document.getElementById('message')?.scrollIntoView({block:'nearest'});
  }

  const actions = createAppActions(app, scrollToCurrent);

  // The request list is a drawer below 1024px. It traps focus while open and closes on Escape.
  async function openRequests() {
    requestsOpen = true;
    await tick();
    (requestPane?.querySelector<HTMLElement>('[aria-current="page"]:not(:disabled)') ?? requestPane?.querySelector<HTMLElement>('button:not(:disabled)'))?.focus();
  }
  function closeRequests(restoreFocus = true) {
    if (!requestsOpen) return;
    requestsOpen = false;
    if (restoreFocus) requestsButton?.focus();
  }
  function trapRequests(event: KeyboardEvent) {
    if (!requestsOpen) return;
    if (event.key === 'Escape') { event.preventDefault(); closeRequests(); return; }
    if (event.key !== 'Tab' || !requestPane) return;
    const focusable = [...requestPane.querySelectorAll<HTMLElement>('button:not(:disabled), a[href]')];
    if (!focusable.length) { event.preventDefault(); return; }
    const first = focusable[0], last = focusable[focusable.length - 1];
    const active = document.activeElement;
    if (event.shiftKey && (active === first || !requestPane.contains(active))) { event.preventDefault(); last.focus(); }
    else if (!event.shiftKey && (active === last || !requestPane.contains(active))) { event.preventDefault(); first.focus(); }
  }
  const selectRequest = (id: string) => { closeRequests(false); proView = 'jobs'; return actions.selectRequest(id); };
  const createRequest = () => { closeRequests(false); return actions.createRequest(); };
  onMount(() => {
    document.documentElement.lang = currentLocale();
    function sizeWorkspace() {
      viewportHeight = Math.round(window.visualViewport?.height ?? window.innerHeight);
      keyboardOpen = window.innerWidth <= 600 && viewportHeight < 550 && !!document.activeElement?.matches('input, textarea');
    }
    // Focus can change during a Svelte render when a composer mode unmounts.
    // Apply viewport state after that render, never inside its focus event.
    function queueSizing() { queueMicrotask(sizeWorkspace); }
    sizeWorkspace();
    window.visualViewport?.addEventListener('resize', queueSizing);
    window.addEventListener('resize', queueSizing);
    document.addEventListener('focusin', queueSizing);
    document.addEventListener('focusout', queueSizing);
    const wide = window.matchMedia('(min-width: 1024px)');
    const closeOnWide = () => { if (wide.matches) requestsOpen = false; };
    wide.addEventListener('change', closeOnWide);
    void actions.initialize();
    const timer = window.setInterval(() => { if (document.visibilityState === 'visible') void actions.refresh(); }, 5000);
    return () => {
      window.clearInterval(timer);
      window.visualViewport?.removeEventListener('resize', queueSizing);
      window.removeEventListener('resize', queueSizing);
      document.removeEventListener('focusin', queueSizing);
      document.removeEventListener('focusout', queueSizing);
      wide.removeEventListener('change', closeOnWide);
    };
  });

  function money(minor: number) {
    return new Intl.NumberFormat(currentLocale() === 'nl' ? 'nl-NL' : 'en-GB', {
      style: 'currency',
      currency: app.serverState?.job.quote.currency ?? 'USD'
    }).format(minor / 100);
  }

  function roleLabel(role: WorkspaceRole) {
    return t(role === 'professional' ? 'Professional' : role === 'operator' ? 'Operator' : 'Customer');
  }
</script>

<svelte:window onkeydown={trapRequests}/>

{#snippet requestPaneContent(canCreate: boolean)}
  <button type="button" class="drawer-scrim" class:open={requestsOpen} tabindex="-1" aria-hidden="true" onclick={() => closeRequests()}></button>
  <div id="request-list" class="request-list-pane" class:open={requestsOpen} bind:this={requestPane} role={requestsOpen ? 'dialog' : undefined} aria-modal={requestsOpen ? 'true' : undefined} aria-label={t('Your requests')}>
    <div class="drawer-head">
      <h2>{t('Your requests')}</h2>
      <button type="button" class="icon-button" aria-label={t('Close')} onclick={() => closeRequests()}><svg class="icon" viewBox="0 0 24 24" aria-hidden="true"><path d="M18 6 6 18M6 6l12 12"/></svg></button>
    </div>
    <WorkspaceNavigation items={app.workspaces} currentID={app.workspaceId} {canCreate} canLoadMore={!!app.workspaceCursor} busy={app.pending || app.loading} onSelect={selectRequest} onCreate={createRequest} onLoadMore={actions.loadMoreWorkspaces}/>
  </div>
{/snippet}

<svelte:head><title>{app.brandName} — {t('Home services, handled faster.')}</title></svelte:head>

<a class="skip-link" href="#main-content">{t('Skip to main content')}</a>

{#if !app.session?.authenticated}
  <div class="app-shell landing-shell">
    <header class="topbar landing-topbar">
      <a class="brand" href="/" aria-label="{app.brandName} home"><svg class="brand-mark" viewBox="0 0 48 48" aria-hidden="true"><rect width="48" height="48" rx="12"/><g fill="none" stroke-width="3" stroke-linecap="round"><path d="M24 11v26M11 24h26M15 15l18 18M15 33l18-18"/></g></svg>{app.brandName}</a>
      <nav class="landing-navigation" aria-label={t('Main navigation')}>
        <a href="#how-it-works">{t('How it works')}</a>
      </nav>
      <div class="landing-account-links">
        <button class="language-button" aria-label="English" aria-pressed={currentLocale() === 'en'} onclick={() => setLocale('en')}>EN</button>
        <button class="language-button" aria-label="Nederlands" aria-pressed={currentLocale() === 'nl'} onclick={() => setLocale('nl')}>NL</button>
        <a class="login-link" href="/api/auth/login">{t('Log in')}</a>
        <a class="nav-cta" href="/api/auth/login">{t('Get started')} <svg class="icon" viewBox="0 0 24 24" aria-hidden="true"><path d="M7 7h10v10M7 17 17 7"/></svg></a>
      </div>
    </header>
    <main id="main-content" class="landing-main"><SignIn {app}/></main>
  </div>
{:else}
  <div class="app-shell" class:workspace-shell={workspaceLayout} class:keyboard-open={workspaceLayout && keyboardOpen} style:height={workspaceLayout && viewportHeight ? `${viewportHeight}px` : undefined}>
    <header class="topbar">
      <a class="brand" href="/" aria-label="{app.brandName} home"><svg class="brand-mark" viewBox="0 0 48 48" aria-hidden="true"><rect width="48" height="48" rx="12"/><g fill="none" stroke-width="3" stroke-linecap="round"><path d="M24 11v26M11 24h26M15 15l18 18M15 33l18-18"/></g></svg>{app.brandName}</a>
      {#if app.role === 'professional' || app.role === 'operator'}<span class="role-tag">{t(app.role === 'operator' ? 'Operator' : 'Pro')}</span>{/if}
      {#if workspaceLayout && app.role === 'professional'}
      <nav class="pro-nav" aria-label={t('Professional navigation')}>
        {#each proTabs as tab (tab.view)}<button type="button" class:current={proView === tab.view} aria-current={proView === tab.view ? 'page' : undefined} onclick={() => { proView = tab.view; }}>{t(tab.label)}</button>{/each}
      </nav>
      {/if}
      <div class="topbar-actions">
      <div class="language-switch language-switch-wide" role="group" aria-label={t('Language')}>
        <button class:active={currentLocale() === 'en'} aria-pressed={currentLocale() === 'en'} onclick={() => setLocale('en')}>EN</button>
        <button class:active={currentLocale() === 'nl'} aria-pressed={currentLocale() === 'nl'} onclick={() => setLocale('nl')}>NL</button>
      </div>
      {#if (app.session.memberships?.filter((item) => item.status !== 'disabled').length ?? 0) > 1}
      <nav class="role-switch role-switch-wide" aria-label={t('Account workspace')}>
        {#each app.session.memberships ?? [] as membership (membership.role)}
          {#if membership.status !== 'disabled'}
            <button class:active={membership.role === app.role} disabled={app.pending || app.loading} aria-pressed={membership.role === app.role} onclick={() => { if (membership.role !== app.role) void actions.switchWorkspace(membership.role); }}>{roleLabel(membership.role)}</button>
          {/if}
        {/each}
      </nav>
      {/if}
      {#if workspaceLayout && (app.role !== 'professional' || proView === 'jobs')}<button type="button" class="requests-button" bind:this={requestsButton} aria-controls="request-list" aria-expanded={requestsOpen} onclick={openRequests}><svg class="icon" viewBox="0 0 24 24" aria-hidden="true"><path d="M8 6h13M8 12h13M8 18h13M3 6h.01M3 12h.01M3 18h.01"/></svg><span class="requests-label">{t('Requests')}</span></button>{/if}
      {#if app.role === 'customer' || app.role === 'professional'}<NotificationCenter onSelect={selectRequest}/>{/if}
      <details class="account-menu">
        <summary aria-label={accountName}><span aria-hidden="true">{initials}</span></summary>
        <div class="account-menu-panel">
          <p class="account-panel-name">{accountName}</p>
          {#if (app.session.memberships?.filter((m) => m.status !== 'disabled').length ?? 0) > 1}
          <nav class="role-switch role-switch-mobile" aria-label="Account workspace">
            {#each app.session.memberships ?? [] as membership (membership.role)}
              {#if membership.status !== 'disabled'}<button class:active={membership.role === app.role} disabled={app.pending || app.loading} aria-pressed={membership.role === app.role} onclick={() => { if (membership.role !== app.role) void actions.switchWorkspace(membership.role); }}>{roleLabel(membership.role)}</button>{/if}
            {/each}
          </nav>
          {/if}
          
          <div class="language-switch language-switch-mobile" role="group" aria-label={t('Language')}>
            <button class:active={currentLocale() === 'en'} aria-pressed={currentLocale() === 'en'} onclick={() => setLocale('en')}>EN</button>
            <button class:active={currentLocale() === 'nl'} aria-pressed={currentLocale() === 'nl'} onclick={() => setLocale('nl')}>NL</button>
          </div>
          {#if app.session.account?.displayName}<small>{app.session.account?.email}</small>{/if}
          {#if canBecomeProfessional}<button class="workspace-link" disabled={app.pending} onclick={actions.beginProfessional}>Become a professional</button>{/if}
          <button class="sign-out-button" onclick={actions.logout}>Sign out</button>
        </div>
      </details>
      </div>
    </header>

    <main id="main-content" class:workspace-main={workspaceLayout}>
      {#if app.error}<div class="error" role="alert">{app.error}<button class="text-button" onclick={actions.dismissError} aria-label="Dismiss error">×</button></div>{/if}
      {#if app.loading}
        <div class="card loading" role="status">{t('Loading your requests…')}</div>
      {:else if app.role === 'professional' && app.session.memberships?.some((item) => item.role === 'professional' && item.status === 'pending')}
        <ProfessionalPending {actions}/>
      {:else if app.role === 'operator'}
        <OperatorQueue {actions}/>
      {:else if app.role === 'professional' && !app.serverState}
        
        <WorkspaceNavigation items={app.workspaces} currentID={app.workspaceId} canCreate={false} canLoadMore={!!app.workspaceCursor} busy={app.pending || app.loading} onSelect={actions.selectRequest} onCreate={actions.createRequest} onLoadMore={actions.loadMoreWorkspaces}/>
        <details class="service-settings"><summary>Manage your service and availability</summary><ProfessionalAvailability/></details>
        <div class="card empty-state"><span>✓</span><h2>No assigned requests yet.</h2><p>Your customer bookings will appear here. You can keep your service profile and availability up to date.</p></div>
      {:else if !app.serverState}
        <div class="card"><h2>Workspace unavailable</h2><p>Your account is signed in, but its workspace data could not be loaded.</p><button onclick={actions.loadState}>Try again</button></div>
      {:else if app.role === 'customer'}
        <div class="request-shell">
        {@render requestPaneContent(true)}
        <div class="request-main" inert={requestsOpen}>
        {#key app.workspaceId}
        <CustomerWorkspace
          {app}
          {actions}
          state={app.serverState}
          {job}
          {intake}
          {activeJob}
          {composing}
          {missingDocs}
          {statusLabel}
          {money}
        />
        {/key}
        </div>
        </div>
      {:else if app.role === 'professional' && proView !== 'jobs'}
        <section class="pro-page" aria-labelledby="pro-page-title">
          <h1 id="pro-page-title">{t(proView === 'times' ? 'Free times' : 'Profile & services')}</h1>
          {#if proView === 'times'}
            <p class="lead">{t('Customers can book the times you add here. Add future times so they can pick one.')}</p>
            <ProfessionalAvailability show="times"/>
          {:else}
            <ProfessionalAvailability show="profile"/>
            <PayPalPayoutSetup/>
          {/if}
        </section>
      {:else if app.role === 'professional'}
        <div class="request-shell">
        {@render requestPaneContent(false)}
        <div class="request-main" inert={requestsOpen}>
        {#key app.workspaceId}<ProfessionalWorkspace {app} {actions} {job} {intake} {activeJob} {statusLabel} {money}/>{/key}
        </div>
        </div>
      {/if}
      {#if !workspaceLayout}<footer><span>{t('Made for a little less back-and-forth.')}</span><span>{app.session.account?.email ?? t('Your account workspace')}</span></footer>{/if}
    </main>
    {#if workspaceLayout && app.role === 'professional'}
    <nav class="pro-tabs" aria-label={t('Professional navigation')}>
      {#each proTabs as tab (tab.view)}<button type="button" class:current={proView === tab.view} aria-current={proView === tab.view ? 'page' : undefined} onclick={() => { proView = tab.view; }}><svg class="icon" viewBox="0 0 24 24" aria-hidden="true">{@html tab.icon}</svg><span>{t(tab.label)}</span></button>{/each}
    </nav>
    {/if}
  </div>
{/if}
