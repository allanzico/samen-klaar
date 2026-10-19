<script lang="ts">
  import { onMount, tick } from 'svelte';
  import { api } from '../../lib/api';
  import type { AdminMembership, AdminUser, AdminUserAction, AdminUserDetail, AdminUserFilter, AdminUserPage } from '../../lib/operator-admin';
  import { t } from '../../lib/i18n.svelte';
  import { dayTime } from '../../lib/format';
  import Banner from '../../ui/Banner.svelte';
  import Pill from '../../ui/Pill.svelte';
  import { showToast } from '../../ui/toast-state.svelte';

  let { location = '', onReview }: { location?: string; onReview: (id: string) => void } = $props();
  let query = $state('');
  let role = $state('');
  let status = $state('');
  let area = $state('');
  let filter = $state<AdminUserFilter>({ q: '', role: '', status: '', location: '', offset: '0' });
  let page = $state<AdminUserPage>({ users: [], total: 0, offset: 0, hasMore: false });
  let detail = $state<AdminUserDetail | null>(null);
  let selected = $state('');
  let loading = $state(true);
  let loadingDetail = $state(false);
  let busy = $state(false);
  let error = $state('');
  let detailError = $state('');
  let action = $state<AdminUserAction | null>(null);
  let reason = $state('');
  let listRequest = 0;
  let detailRequest = 0;
  const roleLabel = (value: string) => value === 'customer' ? t('Customer') : value === 'professional' ? t('Professional') : t('Operator');
  const statusLabel = (value: string, suspended = false) => suspended ? t('Suspended') : value === 'active' ? t('Active') : value === 'pending' ? t('Pending review') : t('Disabled');
  const actionLabel = (value: string) => value === 'suspend-role' ? t('Suspend access') : value === 'restore-role' ? t('Restore access') : t('Sign out all sessions');
  const protectedAccount = $derived(detail?.user.memberships.some((m) => m.role === 'operator'));

  onMount(() => {
    area = location; role = location ? 'professional' : '';
    filter = { q: '', role, status: '', location: area, offset: '0' };
    void load();
  });

  async function load(keepSelection = false) {
    const request = ++listRequest;
    loading = true; error = '';
    try {
      const result = await api.operatorUsers(filter);
      if (request !== listRequest) return;
      page = result;
      if (!keepSelection || !result.users.some((u) => u.id === selected)) {
        detail = null; selected = ''; action = null; detailError = ''; loadingDetail = false; ++detailRequest;
      }
    } catch (e) { if (request === listRequest) error = e instanceof Error ? e.message.trim() : t('Could not load users. Try again.'); }
    finally { if (request === listRequest) loading = false; }
  }
  function search(event?: SubmitEvent) {
    event?.preventDefault();
    filter = { q: query.trim(), role, status, location: area, offset: '0' }; void load();
  }
  function paginate(offset: number) { filter = { ...filter, offset: String(offset) }; void load(); }
  async function select(user: AdminUser) {
    const request = ++detailRequest;
    selected = user.id; detail = null; action = null; reason = ''; detailError = ''; loadingDetail = true;
    try { const result = await api.operatorUser(user.id); if (request === detailRequest) detail = result; }
    catch (e) { if (request === detailRequest) detailError = e instanceof Error ? e.message.trim() : t('Could not load this account. Try again.'); }
    finally {
      if (request === detailRequest) {
        loadingDetail = false;
        await tick();
        if (window.matchMedia('(max-width: 1199px)').matches) document.getElementById('operator-account-detail')?.focus();
      }
    }
  }
  function prepare(membership?: AdminMembership) {
    reason = ''; detailError = '';
    action = membership ? { action: membership.suspended ? 'restore-role' : 'suspend-role', role: membership.role, expectedStatus: membership.status, reason: '' }
      : { action: 'revoke-sessions', role: '', expectedStatus: '', reason: '' };
  }
  async function submit(event: SubmitEvent) {
    event.preventDefault(); if (!detail || !action || busy || reason.trim().length < 10) return;
    const id = detail.user.id;
    busy = true; detailError = '';
    try {
      await api.administerUser(id, { ...action, reason: reason.trim() });
      action = null; reason = ''; showToast(t('Account action recorded'));
      // A successful mutation stays successful even if refreshing its projection fails.
      try { detail = await api.operatorUser(id); await load(true); }
      catch { detailError = t('Action saved. Refresh this account to see the latest access.'); }
    } catch (e) { detailError = e instanceof Error ? e.message.trim() : t('Could not update access. Refresh and try again.'); }
    finally { busy = false; }
  }
</script>

<section class="directory" aria-labelledby="users-title">
  <header class="heading"><div><h2 id="users-title" class="detail-title">{t('Users')}</h2><p class="secondary-text">{t('Find customers and professionals, inspect their access and record account actions.')}</p></div><button class="btn btn-outline btn-sm" disabled={loading || busy} onclick={() => load(true)}>{t('Refresh users')}</button></header>
  <form class="filters" onsubmit={search}>
    <div class="field search"><label for="user-search">{t('Search users')}</label><input id="user-search" class="input" bind:value={query} maxlength="160" placeholder={t('Name, email or account ID')}/></div>
    <div class="field"><label for="user-role">{t('Role')}</label><select id="user-role" class="input" bind:value={role}><option value="">{t('All roles')}</option><option value="customer">{t('Customers')}</option><option value="professional">{t('Professionals')}</option><option value="operator">{t('Operators')}</option></select></div>
    <div class="field"><label for="user-status">{t('Access status')}</label><select id="user-status" class="input" bind:value={status}><option value="">{t('All statuses')}</option><option value="active">{t('Active')}</option><option value="pending">{t('Pending review')}</option><option value="disabled">{t('Disabled or suspended')}</option></select></div>
    <button class="btn btn-primary" disabled={loading || busy}>{t('Search')}</button>
  </form>
  {#if area}<div class="area-filter"><span>{t('Service area: {area}', { area })}</span><button class="btn btn-quiet btn-sm" disabled={loading || busy} onclick={() => { area = ''; search(); }}>{t('Clear area filter')}</button></div>{/if}
  {#if error}<Banner kind="error" text={t(error)}/><button class="btn btn-outline" onclick={() => load()}>{t('Try again')}</button>{/if}
  <div class="directory-columns" aria-busy={loading}>
    <div class="results">
      <p class="meta" role="status">{loading ? t('Loading users…') : t('{count} matching accounts', { count: page.total })}</p>
      {#if !loading && !page.users.length}<div class="empty-result"><h3 class="small-title">{t('No accounts match')}</h3><p class="secondary-text">{t('Try another name or email, or choose a different role and status.')}</p></div>{/if}
      <ul class="user-list">
        {#each page.users as user (user.id)}
          <li><button type="button" class="user-row" disabled={loading || busy} aria-current={selected === user.id ? 'true' : undefined} onclick={() => select(user)}>
            <strong>{user.displayName || user.email || t('Unnamed account')}</strong><span class="meta">{user.email || user.id}</span>
            <span class="memberships">{#each user.memberships as membership (membership.role)}<span>{roleLabel(membership.role)} · {statusLabel(membership.status, membership.suspended)}</span>{/each}</span>
            {#if user.professional}<span class="meta">{user.professional.serviceName} · {user.professional.serviceArea}</span>{/if}
            {#if user.sameEmailCount > 1}<span class="duplicate">{t('Email shared by {count} accounts', { count: user.sameEmailCount })}</span>{/if}
          </button></li>
        {/each}
      </ul>
      <nav class="pager" aria-label={t('User pages')}><button class="btn btn-outline btn-sm" disabled={loading || busy || page.offset === 0} onclick={() => paginate(Math.max(0, page.offset - 25))}>{t('Previous')}</button><span class="meta">{page.total ? t('{start}–{end} of {total}', { start: page.offset + 1, end: page.offset + page.users.length, total: page.total }) : t('No results')}</span><button class="btn btn-outline btn-sm" disabled={loading || busy || !page.hasMore} onclick={() => paginate(page.offset + 25)}>{t('Next')}</button></nav>
    </div>
    <section id="operator-account-detail" class="account" tabindex="-1" aria-label={t('Account details')} aria-busy={loadingDetail}>
      {#if loadingDetail}<p role="status">{t('Loading account details…')}</p>
      {:else if detail}
        {@const user = detail.user}
        <header><h3 class="account-title">{user.displayName || user.email || t('Unnamed account')}</h3><p class="secondary-text">{user.email || t('No email recorded')}</p></header>
        {#if user.sameEmailCount > 1}<Banner kind="warn" text={t('This email belongs to multiple identities. Compare the issuer and identity ID before changing access.')}/>{/if}
        <dl class="facts"><div><dt>{t('Account ID')}</dt><dd>{user.id}</dd></div><div><dt>{t('Created')}</dt><dd>{dayTime(user.createdAt)}</dd></div><div><dt>{t('Last recorded activity')}</dt><dd>{dayTime(user.lastSeenAt)}</dd></div><div><dt>{t('Language')}</dt><dd>{user.locale.toUpperCase()}</dd></div><div><dt>{t('Customer jobs')}</dt><dd>{user.customerJobs}</dd></div><div><dt>{t('Assigned jobs')}</dt><dd>{user.professionalJobs}</dd></div><div><dt>{t('Active sessions')}</dt><dd>{user.activeSessions}</dd></div></dl>
        <details class="disclosure"><summary>{t('Sign-in identity')}</summary><dl class="facts identity"><div><dt>{t('Issuer')}</dt><dd>{user.issuer}</dd></div><div><dt>{t('Identity ID')}</dt><dd>{user.subject}</dd></div></dl></details>
        {#if user.professional}
          <section class="service"><h4 class="small-title">{t('Professional service')}</h4><p><b>{user.professional.serviceName}</b> · {t(user.professional.category)}</p><p class="secondary-text">{user.professional.serviceArea}{user.professional.travelRadiusKm ? ` · ${t('Travel radius: {radius} km', { radius: user.professional.travelRadiusKm })}` : ''}</p><p class="meta">{t('Profile status: {status}', { status: t(user.professional.status === 'active' ? 'Active' : user.professional.status === 'pending' ? 'Pending review' : user.professional.status === 'rejected' ? 'Declined' : 'Disabled') })}</p>{#if user.professional.status === 'pending'}<button class="btn btn-outline btn-sm" onclick={() => onReview(user.professional!.id)}>{t('Review application')}</button>{/if}</section>
        {/if}
        <section class="access"><h4 class="small-title">{t('Workspace access')}</h4>
          {#if protectedAccount}<p class="secondary-text">{t('Operator accounts are managed by the service owner.')}</p>{/if}
          <ul class="rows">{#each user.memberships as membership (membership.role)}<li><div><b>{roleLabel(membership.role)}</b><Pill variant={membership.status === 'active' ? 'done' : membership.suspended ? 'error' : 'neutral'} label={statusLabel(membership.status, membership.suspended)} small/></div>{#if !protectedAccount && membership.role !== 'operator' && (membership.status !== 'disabled' || membership.suspended)}<button class="btn btn-outline btn-sm" disabled={busy} onclick={() => prepare(membership)}>{membership.suspended ? t('Restore access') : t('Suspend access')}</button>{/if}</li>{/each}</ul>
          {#if !protectedAccount}<button class="btn btn-outline btn-sm" disabled={busy || !user.activeSessions} onclick={() => prepare()}>{t('Sign out all sessions')}</button>{/if}
        </section>
        {#if action}<form class="action-form" onsubmit={submit}><h4 class="small-title">{actionLabel(action.action)}{action.role ? ` · ${roleLabel(action.role)}` : ''}</h4><p class="secondary-text">{action.action === 'suspend-role' ? t('This blocks access to this workspace and ends its sessions. Review any booked jobs separately.') : action.action === 'restore-role' ? t('Restore the access this account had before suspension. Professional approval still follows the review process.') : t('End every app session for this account. The user will need to sign in again.')}</p><div class="field"><label for="admin-reason">{t('Reason')}</label><textarea id="admin-reason" class="textarea" rows="3" maxlength="1000" bind:value={reason} disabled={busy} aria-describedby="admin-reason-help"></textarea><p id="admin-reason-help" class="field-help">{t('Write at least 10 characters. Your name and reason are saved in the account history.')}</p></div><div class="action-buttons"><button class="btn {action.action === 'suspend-role' ? 'btn-danger' : 'btn-primary'}" disabled={busy || reason.trim().length < 10}>{busy ? t('Saving…') : t('Confirm account action')}</button><button class="btn btn-outline" type="button" disabled={busy} onclick={() => { action = null; }}>{t('Cancel')}</button></div></form>{/if}
        <section class="history"><h4 class="small-title">{t('Account action history')}</h4>{#if !detail.events.length}<p class="secondary-text">{t('No admin actions recorded for this account.')}</p>{:else}<p class="meta">{t('Most recent 50 actions')}</p><ol class="rows">{#each detail.events as event (event.id)}<li><b>{actionLabel(event.action)}{event.role ? ` · ${roleLabel(event.role)}` : ''}</b><p>{event.reason}</p><p class="meta">{event.actorName || event.actorId || t('Former operator')} · {dayTime(event.createdAt)}</p>{#if event.nextStatus}<p class="meta">{statusLabel(event.previousStatus)} → {statusLabel(event.nextStatus, event.action === 'suspend-role')}</p>{/if}</li>{/each}</ol>{/if}</section>
      {:else if !detailError}<div class="empty-result"><h3 class="small-title">{t('Choose an account')}</h3><p class="secondary-text">{t('Select a user to inspect their roles, service and account history.')}</p></div>{/if}
      {#if detailError}<Banner kind="error" text={t(detailError)}/><button class="btn btn-outline btn-sm" disabled={busy} onclick={() => { const user = page.users.find((u) => u.id === selected); if (user) void select(user); }}>{t('Refresh account')}</button>{/if}
    </section>
  </div>
</section>

<style>
  .directory { display: grid; gap: 22px; min-width: 0; }
  .heading { display: flex; flex-wrap: wrap; justify-content: space-between; align-items: start; gap: 16px; }
  .heading > div { display: grid; gap: 8px; max-width: 65ch; }
  .detail-title { font: 800 26px/1.1 var(--sk-fd); }
  .filters { display: grid; grid-template-columns: minmax(180px,1fr) minmax(120px,.6fr) minmax(150px,.7fr) auto; align-items: end; gap: 12px; }
  .directory-columns { display: grid; grid-template-columns: minmax(260px,.85fr) minmax(340px,1.15fr); gap: 28px; align-items: start; }
  .results, .account { display: grid; gap: 16px; min-width: 0; }
  .account { padding-left: 28px; border-left: 1px solid var(--sk-line); }
  .account > header { display: grid; gap: 6px; }
  .account-title { font: 700 21px/1.2 var(--sk-fd); overflow-wrap: anywhere; }
  .user-list { list-style: none; padding: 0; margin: 0; border-top: 1px solid var(--sk-line); }
  .user-row { display: grid; gap: 6px; width: 100%; padding: 16px 12px; text-align: left; color: var(--sk-ink); background: transparent; border: 0; border-bottom: 1px solid var(--sk-line); border-radius: 0; overflow-wrap: anywhere; }
  .user-row:hover, .user-row[aria-current] { background: var(--sk-soft); }
  .user-row[aria-current] { outline: 1px solid var(--sk-ink); outline-offset: -1px; }
  .memberships { display: flex; flex-wrap: wrap; gap: 6px 12px; font-size: 12px; }
  .duplicate { font-size: 12px; font-weight: 600; color: var(--sk-ink); }
  .pager, .area-filter, .action-buttons { display: flex; flex-wrap: wrap; align-items: center; justify-content: space-between; gap: 10px; }
  .facts { display: grid; grid-template-columns: repeat(2,minmax(0,1fr)); gap: 14px 20px; }
  .facts dt { color: var(--sk-muted); font-size: 12px; }
  .facts dd { overflow-wrap: anywhere; }
  .identity { grid-template-columns: 1fr; margin-top: 12px; }
  .service, .access, .history, .action-form { display: grid; gap: 12px; padding-top: 20px; border-top: 1px solid var(--sk-line); }
  .service > .btn, .access > .btn { justify-self: start; }
  .access li, .access li > div { display: flex; flex-wrap: wrap; align-items: center; gap: 10px; }
  .access li { justify-content: space-between; padding: 12px 0; }
  .history li { display: grid; gap: 6px; padding: 14px 0; overflow-wrap: anywhere; }
  .action-buttons { justify-content: start; }
  .empty-result { display: grid; gap: 8px; padding: 24px 0; }
  @media (max-width: 1199px) { .directory-columns { grid-template-columns: 1fr; } .account { border-left: 0; border-top: 1px solid var(--sk-line); padding: 24px 0 0; } }
  @media (max-width: 767px) { .filters { grid-template-columns: repeat(2,minmax(0,1fr)); } .search { grid-column: 1 / -1; } .filters > .btn { grid-column: 1 / -1; justify-self: start; } }
  @media (max-width: 639px) { .detail-title { font-size: 20px; } .facts { grid-template-columns: 1fr; } }
</style>
