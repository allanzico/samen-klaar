<script lang="ts">
  import { onMount, tick } from 'svelte';
  import AppBar, { type NavItem } from './features/shell/AppBar.svelte';
  import ProTabBar from './features/shell/ProTabBar.svelte';
  import Landing from './features/public/Landing.svelte';
  import OperatorGate from './features/public/OperatorGate.svelte';
  import OperatorDashboard from './features/operator/OperatorDashboard.svelte';
  import CustomerHome from './features/customer/CustomerHome.svelte';
  import CustomerRequests from './features/customer/CustomerRequests.svelte';
  import RequestPage from './features/request/RequestPage.svelte';
  import ProJobs from './features/professional/ProJobs.svelte';
  import FreeTimes from './features/professional/FreeTimes.svelte';
  import ProfileServices from './features/professional/ProfileServices.svelte';
  import ProOnboarding from './features/professional/ProOnboarding.svelte';
  import AgentSettings from './features/agent/AgentSettings.svelte';
  import AgentAttention from './features/agent/AgentAttention.svelte';
  import Banner from './ui/Banner.svelte';
  import Toast from './ui/Toast.svelte';
  import Icon from './ui/Icon.svelte';
  import { createAppActions } from './state/create-app-controller';
  import { createAppState } from './state/app-state.svelte';
  import { statusUi } from './lib/status-ui';
  import { refreshAgentEscalations } from './lib/agent.svelte';
  import { currentLocale, t } from './lib/i18n.svelte';

  const app = createAppState();
  const operatorRoute = window.location.pathname.replace(/\/$/, '') === '/operator';
  let proView = $state<'jobs' | 'times' | 'profile'>('jobs');

  const pendingPro = $derived(app.role === 'professional' && !!app.session?.memberships?.some((m) => m.role === 'professional' && m.status === 'pending'));
  const onRequestPage = $derived(!!app.serverState && !!app.workspaceId && ((app.role === 'customer' && app.customerPage === 'detail') || (app.role === 'professional' && proView === 'jobs' && !pendingPro)));
  // The job and request lists mark the asking card themselves, so they do not need the banner too.
  const onListPage = $derived((app.role === 'professional' && proView === 'jobs') || (app.role === 'customer' && app.customerPage === 'requests'));
  const proNeeds = $derived(app.role === 'professional' ? app.workspaces.filter((item) => statusUi(item.status, 'professional').pill === 'your-turn').length : 0);

  // The thread pane scrolls, never the page. Chat pages open at the newest message.
  async function scrollToCurrent(smooth = false, targetID?: string) {
    await tick();
    const pane = document.getElementById('request-thread');
    if (!pane) return;
    const target = targetID ? document.getElementById(targetID) : null;
    if (target) pane.scrollTo({ top: target.getBoundingClientRect().top - pane.getBoundingClientRect().top + pane.scrollTop - 16, behavior: smooth ? 'smooth' : 'auto' });
    else if (pane.dataset.chat === 'true') pane.scrollTo({ top: pane.scrollHeight, behavior: smooth ? 'smooth' : 'auto' });
    else pane.scrollTo({ top: 0 });
  }
  const actions = createAppActions(app, scrollToCurrent);

  function openRequest(id: string) { proView = 'jobs'; return actions.selectRequest(id); }
  function backToList() { if (app.role === 'professional') actions.showJobs(); else actions.showRequests(); }

  const customerNav = $derived<NavItem[]>([
    { label: t('Home'), icon: 'home', current: app.customerPage === 'home', onclick: actions.showHome },
    { label: t('Requests'), icon: 'list', current: app.customerPage !== 'home', onclick: actions.showRequests, phoneIcon: true }
  ]);
  const proNav = $derived<NavItem[]>([
    { label: t('Jobs'), icon: 'briefcase', current: proView === 'jobs', onclick: () => { proView = 'jobs'; actions.showJobs(); } },
    { label: t('Free times'), icon: 'calendar', current: proView === 'times', onclick: () => { proView = 'times'; } },
    { label: t('Profile & services'), icon: 'user', current: proView === 'profile', onclick: () => { proView = 'profile'; } }
  ]);

  onMount(() => {
    document.documentElement.lang = currentLocale();
    const onLocationChange = () => { void actions.restoreLocation(); };
    window.addEventListener('popstate', onLocationChange);
    void Promise.resolve(actions.initialize()).then(() => { if (app.session?.authenticated && (app.role === 'customer' || app.role === 'professional')) void refreshAgentEscalations(); });
    const timer = window.setInterval(() => {
      if (document.visibilityState !== 'visible') return;
      void actions.refresh();
      if (app.session?.authenticated && (app.role === 'customer' || app.role === 'professional') && !pendingPro) void refreshAgentEscalations();
    }, 5000);
    return () => { window.clearInterval(timer); window.removeEventListener('popstate', onLocationChange); };
  });
</script>

<svelte:head><title>{operatorRoute ? `${t('Operator dashboard')} — ${app.brandName}` : `${app.brandName} — ${t('Home repairs in Amsterdam')}`}</title></svelte:head>

<a class="skip-link" href="#main-content">{t('Skip to main content')}</a>

{#if operatorRoute && (!app.session || (!app.role && app.loading))}
  <OperatorGate {app} {actions} mode="checking"/>
{:else if operatorRoute && !app.session?.authenticated}
  <OperatorGate {app} {actions} mode="sign-in"/>
{:else if operatorRoute && app.role !== 'operator'}
  <OperatorGate {app} {actions} mode="no-access"/>
{:else if !app.session?.authenticated}
  <Landing {app}/>
{:else if app.role === 'operator'}
  <OperatorDashboard {app} {actions}/>
{:else}
  <div class="frame">
    <AppBar {app} {actions}
      nav={pendingPro ? [] : app.role === 'professional' ? proNav : customerNav}
      badge={app.role === 'professional' ? t('Pro') : undefined}
      onHome={() => { if (app.role === 'professional') { proView = 'jobs'; actions.showJobs(); } else actions.showHome(); }}
      onSelectNotification={pendingPro ? undefined : openRequest}
      hideOnPhone={onRequestPage}/>
    {#if !pendingPro && !onRequestPage && !onListPage}<AgentAttention onOpen={openRequest}/>{/if}
    <main id="main-content" class="main" class:request={onRequestPage}>
      {#if app.error && !onRequestPage && !app.loading}<div class="page-error"><Banner kind="error" text={t(app.error)} onDismiss={actions.dismissError}/></div>{/if}
      {#if app.loading && !onRequestPage}
        <p class="loading" role="status"><span class="spinner" aria-hidden="true"></span>{t('Loading…')}</p>
      {:else if pendingPro}
        <ProOnboarding {actions} testPayments={app.providerConfig?.testPayments ?? false}/>
      {:else if app.role === 'customer'}
        {#if app.customerPage === 'home'}<CustomerHome {app} {actions}/>
        {:else if app.customerPage === 'requests'}<CustomerRequests {app} {actions}/>
        {:else if app.serverState}{#key app.workspaceId}<RequestPage {app} {actions} workspace={app.serverState} onBack={backToList}/>{/key}
        {:else}
          <div class="failed">
            <div class="empty"><span class="icon-tile icon-tile-48" aria-hidden="true"><Icon name="alert"/></span><h1>{t('We couldn’t load this request')}</h1><p>{t('Check your connection and try again.')}</p>
              <div class="row"><button type="button" class="btn btn-primary" onclick={actions.loadState}>{t('Try again')}</button><button type="button" class="btn btn-outline" onclick={actions.showRequests}>{t('Back to your requests')}</button></div></div>
          </div>
        {/if}
      {:else if app.role === 'professional'}
        {#if proView === 'times'}<FreeTimes/>
        {:else if proView === 'profile'}<ProfileServices testPayments={app.providerConfig?.testPayments ?? false}/>
        {:else if app.serverState && app.workspaceId}{#key app.workspaceId}<RequestPage {app} {actions} workspace={app.serverState} onBack={backToList}/>{/key}
        {:else}<ProJobs {app} {actions} onSetTimes={() => { proView = 'times'; }} onProfile={() => { proView = 'profile'; }}/>{/if}
      {/if}
    </main>
    {#if !pendingPro}<AgentSettings {app}/>{/if}
    {#if app.role === 'professional' && !pendingPro && !onRequestPage}
      <ProTabBar items={proNav.map((item, i) => ({ ...item, badge: i === 0 ? proNeeds : undefined }))}/>
    {/if}
  </div>
{/if}
<Toast/>

<style>
  .frame { display: flex; flex-direction: column; min-height: 100svh; }
  .main { flex: 1; min-width: 0; }
  .main.request { overflow: hidden; }
  .page-error { max-width: 1040px; margin: 16px auto 0; padding: 0 16px; }
  .loading { display: flex; align-items: center; justify-content: center; gap: 10px; padding: 18vh 16px; color: var(--sk-muted); }
  .failed { max-width: 640px; margin: 0 auto; padding: 12vh 16px; }
  .failed h1 { font: 700 22px/1.2 var(--sk-fd); }
  .row { display: flex; flex-wrap: wrap; gap: 10px; }
</style>
