import { setLocale } from '../lib/i18n.svelte';
import { draftKey, readDraft, saveDraft, clearAccountDrafts } from '../lib/drafts';
import { api } from '../lib/api';
import type { AppActions, AppState, Location, NegotiationInput } from '../lib/types';

type ScrollToCurrent = (smooth?: boolean, targetID?: string) => Promise<void>;

export function createAppActions(app: AppState, scrollToCurrent: ScrollToCurrent): AppActions {
  let stateGeneration = 0;
  let bookingCheckoutWorkspace: string | null = null;
  function currentDraftKey() { return draftKey(app.session?.account?.id, app.role, app.workspaceId); }
  function persistDraft() { app.draftRestored = false; app.draftSaved = saveDraft(currentDraftKey(), app.draft); }
  function requestedWorkspace(url: URL) {
    const route = url.pathname.match(/^\/requests\/([^/]+)\/?$/);
    if (route) {
      try { return decodeURIComponent(route[1]); } catch { return null; }
    }
    return url.searchParams.get('workspaceId');
  }
  function rememberRequest(replace = false) {
    const url = new URL(window.location.href);
    if (app.role === 'customer' && app.workspaceId) {
      url.pathname = `/requests/${encodeURIComponent(app.workspaceId)}`;
      url.searchParams.delete('workspaceId');
      app.customerPage = 'detail';
    } else if (app.workspaceId) url.searchParams.set('workspaceId', app.workspaceId);
    if (replace) window.history.replaceState({}, '', url.pathname + url.search + url.hash);
    else window.history.pushState({}, '', url.pathname + url.search + url.hash);
  }
  // Professionals land on their jobs list (F01); a job opens with ?workspaceId=.
  function showJobs(navigate = true) {
    if (app.role !== 'professional' || app.pending) return;
    stateGeneration++;
    persistDraft();
    app.workspaceId = null;
    app.serverState = null;
    app.composeMode = 'message';
    app.loading = false;
    app.error = '';
    if (navigate) {
      const url = new URL(window.location.href);
      url.searchParams.delete('workspaceId');
      window.history.pushState({}, '', url.pathname + url.search + url.hash);
    }
    window.scrollTo?.(0, 0);
  }
  function restoreDraft() { app.draft = readDraft(currentDraftKey()); app.draftRestored = !!app.draft; app.draftSaved = true; }
  function setCustomerPage(page: 'home' | 'requests' | 'profile', navigate = true) {
    if (app.role !== 'customer') return;
    stateGeneration++;
    persistDraft();
    app.workspaceId = null;
    app.serverState = null;
    app.newConversation = false;
    app.paypalStatus = 'not_started';
    app.customerPage = page;
    app.loading = false;
    app.error = '';
    restoreDraft();
    if (navigate) window.history.pushState({}, '', page === 'home' ? '/' : page === 'profile' ? '/profile' : '/requests');
    window.scrollTo?.(0, 0);
  }
  function showHome() { if (!app.pending) setCustomerPage('home'); }
  function showRequests() { if (!app.pending) setCustomerPage('requests'); }
  function showProfile() { if (!app.pending) setCustomerPage('profile'); }
  async function restoreLocation() {
    if (app.role === 'professional') {
      const id = new URL(window.location.href).searchParams.get('workspaceId');
      if (id && id !== app.workspaceId) await selectRequest(id, true);
      else if (!id) showJobs(false);
      return;
    }
    if (app.role !== 'customer') return;
    const url = new URL(window.location.href);
    const id = requestedWorkspace(url);
    if (id && (id !== app.workspaceId || app.customerPage !== 'detail')) await selectRequest(id, true);
    else if (!id) setCustomerPage(url.pathname === '/requests' ? 'requests' : url.pathname === '/profile' ? 'profile' : 'home', false);
  }
  async function initialize() {
    await Promise.all([
      loadConfig(),
      api.session().then((session) => { app.session = session; app.loading = session.authenticated; if (session.account?.locale) setLocale(session.account.locale,false); }).catch(() => {
        app.error = 'We couldn’t check your sign-in. You can try signing in again.';
      })
    ]);
    if (!app.session?.authenticated || !app.session.activeRole) {
      app.loading = false;
      return;
    }
    if (window.location.pathname.replace(/\/$/, '') === '/operator' && app.session.activeRole !== 'operator') {
      if (app.session.memberships?.some((item) => item.role === 'operator' && item.status === 'active')) {
        try {
          const result = await api.switchWorkspace('operator');
          window.location.replace(result.path);
          return;
        } catch {
          app.error = 'Could not open the operator dashboard. Please try again.';
        }
      }
      app.role = app.session.activeRole;
      app.loading = false;
      return;
    }
    if (app.session.activeRole === 'operator' && window.location.pathname.replace(/\/$/, '') !== '/operator') {
      window.location.replace('/operator');
      return;
    }
    app.role = app.session.activeRole;
    const membership = app.session.memberships?.find((item) => item.role === app.role);
    if (membership?.status === 'active' && app.role !== 'operator') {
      stateGeneration++;
      app.loading = true;
      try {
        await refreshWorkspaces();
        const url = new URL(window.location.href);
        const requestedID = app.role === 'customer' ? requestedWorkspace(url) : url.searchParams.get('workspaceId');
        for (let page = 0; requestedID && !app.workspaces.some((item) => item.id === requestedID) && app.workspaceCursor && page < 100; page++) {
          const cursor = app.workspaceCursor;
          await loadMoreWorkspaces();
          if (app.workspaceCursor === cursor) break;
        }
        if (app.role === 'customer') {
          if (requestedID) await selectRequest(requestedID, true);
          else {
            app.customerPage = url.pathname === '/requests' ? 'requests' : url.pathname === '/profile' ? 'profile' : 'home';
            app.workspaceId = null;
            app.serverState = null;
            restoreDraft();
          }
        } else if (requestedID) await selectRequest(requestedID, true);
      } catch (error) {
        app.error = error instanceof Error ? error.message.trim() : 'Could not load your requests.';
      } finally {
        app.loading = false;
      }
    } else app.loading = false;
    if (app.providerConfig?.paymentsAvailable) {
      await loadPayPalStatus();
      await handlePayPalReturn();
    }
  }

  async function loadConfig() {
    try {
      app.providerConfig = await api.config();
      app.brandName = app.providerConfig.appName || 'Service';
    } catch (error) {
      app.error = 'We couldn’t connect right now. Please try again.';
    }
  }

  async function loadState() {
    const generation = ++stateGeneration;
    const workspace = app.workspaceId;
    app.loading = true;
    try {
      if (!workspace) return;
      const state = await api.state(workspace);
      if (generation !== stateGeneration || app.workspaceId !== workspace) return;
      app.serverState = state;
      try { await refreshWorkspaces(); } catch { app.error = 'Request loaded, but the request list could not be refreshed.'; }
      if (app.providerConfig?.paymentsAvailable) void loadPayPalStatus();
    } catch (error) {
      if (generation === stateGeneration) app.error = error instanceof Error ? error.message : 'Something went wrong.';
    } finally {
      if (generation === stateGeneration) {
        app.loading = false;
        await scrollToCurrent();
      }
    }
  }

  async function act(action: string, value = '', location?: Location, changeAmountMinor?: number, negotiation?: NegotiationInput, rating?: number, paymentAuthorizationTiming?: 'after_completion' | 'at_booking') {
    if (app.pending || app.loading || !app.workspaceId) return;
    stateGeneration++;
    app.pending = true;
    app.error = '';
    const checkoutAfterReply = action === 'agent-reply' && !app.serverState?.job.appointment && app.serverState?.job.paymentAuthorizationTiming === 'at_booking';
    try {
      const updated = await api.action(app.workspaceId, action, value, location, changeAmountMinor, negotiation, rating, paymentAuthorizationTiming);
      if (checkoutAfterReply) bookingCheckoutWorkspace = app.workspaceId;
      app.serverState = updated;
      app.newConversation = false;
      try { await refreshWorkspaces(); } catch { app.error = 'Your change was saved, but the request list could not be refreshed.'; }
      if (app.providerConfig?.paymentsAvailable) void loadPayPalStatus();
      // The assistant answers a moment later, on the server; look again soon instead of waiting for the next poll.
      if (action === 'agent-reply' || action === 'set-service-address') for (const delay of [1200, 3000, 6000, 10000]) window.setTimeout(() => void refresh(), delay);
      try { await scrollToCurrent(true, action === 'select-match' ? 'current-plan' : app.serverState?.intake?.step === 'matches' ? 'recommended-service' : undefined); }
      catch { /* The server action has already succeeded. */ }
      return true;
    } catch (error) {
      app.error = error instanceof Error ? error.message.trim() : 'That action could not be completed.';
    } finally {
      app.pending = false;
    }
  }

  async function switchWorkspace(role: NonNullable<AppState['role']>) {
    if (app.pending || !app.session?.memberships?.some((item) => item.role === role && item.status !== 'disabled')) return;
    stateGeneration++;
    app.pending = true;
    app.error = '';
    try {
      persistDraft();
      const result = await api.switchWorkspace(role);
      window.location.assign(result.path);
    } catch (error) {
      app.error = error instanceof Error ? error.message.trim() : 'Could not open that workspace.';
      app.pending = false;
    }
  }

  async function logout() {
    try {
      const result = await api.logout();
      clearAccountDrafts(app.session?.account?.id);
      window.location.assign(result.logoutUrl);
    } catch (error) {
      app.error = error instanceof Error ? error.message.trim() : 'Could not sign out.';
    }
  }

  async function beginProfessional() {
    if (app.pending || app.role !== 'customer') return;
    stateGeneration++;
    app.pending = true;
    app.error = '';
    try {
      persistDraft();
      const result = await api.beginProfessional();
      window.location.assign(result.path);
    } catch (error) {
      app.error = error instanceof Error ? error.message.trim() : 'Could not start professional onboarding.';
      app.pending = false;
    }
  }

  async function loadPayPalStatus() {
    const workspace = app.workspaceId;
    const generation = stateGeneration;
    if (!workspace) return;
    try {
      const result = await api.paypalStatus(workspace);
      if (app.workspaceId === workspace && generation === stateGeneration) app.paypalStatus = result.status;
    } catch {
      if (app.workspaceId === workspace && generation === stateGeneration) app.paypalStatus = 'unavailable';
    }
  }

  async function beginPayPalCheckout() {
    if (app.pending || app.role !== 'customer') return;
    stateGeneration++;
    app.pending = true;
    app.error = '';
    try {
      const result = await api.createPayPalOrder(app.workspaceId);
      window.location.assign(result.approvalUrl);
    } catch (error) {
      app.error = error instanceof Error ? error.message.trim() : 'Could not open PayPal checkout.';
      app.pending = false;
    }
  }

  async function handlePayPalReturn() {
    const url = new URL(window.location.href);
    const orderId = url.searchParams.get('token');
    const returnedWorkspaceID = requestedWorkspace(url);
    if (returnedWorkspaceID && app.workspaceId !== returnedWorkspaceID) {
      await selectRequest(returnedWorkspaceID);
      if (app.workspaceId !== returnedWorkspaceID) return;
    }
    if (!orderId || !url.searchParams.has('paypal_return')) {
      if (url.searchParams.has('paypal_cancelled')) {
        const currentURL = new URL(window.location.href);
        currentURL.searchParams.delete('paypal_cancelled');
        currentURL.searchParams.delete('token');
        currentURL.searchParams.delete('workspaceId');
        window.history.replaceState({}, '', currentURL.pathname + currentURL.search + currentURL.hash);
      }
      return;
    }

    stateGeneration++;
    app.pending = true;
    app.error = '';
    try {
      const result = await api.authorizePayPal(orderId, app.workspaceId);
      app.paypalStatus = result.status;
      await loadState();
    } catch (error) {
      app.error = error instanceof Error ? error.message.trim() : 'Could not confirm PayPal authorization.';
      await loadPayPalStatus();
    } finally {
      const currentURL = new URL(window.location.href);
      currentURL.searchParams.delete('paypal_return');
      currentURL.searchParams.delete('token');
      currentURL.searchParams.delete('PayerID');
      currentURL.searchParams.delete('workspaceId');
      window.history.replaceState({}, '', currentURL.pathname + currentURL.search + currentURL.hash);
      app.pending = false;
    }
  }

  async function capturePayPal() {
    if (app.pending || app.role !== 'customer') return;
    stateGeneration++;
    app.pending = true;
    app.error = '';
    try {
      const result = await api.capturePayPal(app.workspaceId);
      app.serverState = result.state;
      app.paypalStatus = result.status;
      try { await refreshWorkspaces(); } catch { app.error = 'Payment succeeded, but the request list could not be refreshed.'; }
    } catch (error) {
      app.error = error instanceof Error ? error.message.trim() : 'PayPal capture could not be confirmed.';
      await loadPayPalStatus();
    } finally {
      app.pending = false;
    }
  }

  async function reconcilePayPal() {
    if (app.pending || app.role !== 'customer') return;
    stateGeneration++;
    app.pending = true;
    app.error = '';
    try {
      const result = await api.reconcilePayPal(app.workspaceId);
      app.paypalStatus = result.status;
      app.serverState = result.state;
      try { await refreshWorkspaces(); } catch { app.error = 'Payment status was updated, but the request list could not be refreshed.'; }
    } catch (error) {
      app.error = error instanceof Error ? error.message.trim() : 'Could not reconcile PayPal status.';
    } finally {
      app.pending = false;
    }
  }

  function selectEvidenceFile(event: Event) {
    const input = event.currentTarget as HTMLInputElement;
    app.evidenceFile = input.files?.[0] ?? null;
  }

  async function uploadEvidence(workItemIds: string[] = []) {
    if (app.pending || app.role !== 'professional' || !app.evidenceFile) return;
    stateGeneration++;
    app.pending = true;
    app.error = '';
    try {
      app.serverState = await api.uploadEvidence(app.evidenceFile, app.workspaceId, workItemIds);
      app.evidenceFile = null;
      const input = document.getElementById('evidence-file') as HTMLInputElement | null;
      if (input) input.value = '';
    } catch (error) {
      app.error = error instanceof Error ? error.message.trim() : 'Evidence upload failed.';
    } finally {
      app.pending = false;
    }
  }

  async function uploadRequestPhoto(file: File) {
    if (app.pending || app.role !== 'customer' || !app.workspaceId) return false;
    stateGeneration++;
    app.pending = true;
    app.error = '';
    try {
      app.serverState = await api.uploadRequestPhoto(file, app.workspaceId);
      return true;
    } catch (error) {
      app.error = error instanceof Error ? error.message.trim() : 'The photo could not be added.';
      return false;
    } finally {
      app.pending = false;
    }
  }

  async function reviewEvidence(assetIds: string[] = []) {
    if (app.pending || app.role !== 'customer') return;
    stateGeneration++;
    app.pending = true;
    app.reviewingEvidence = true;
    app.error = '';
    try {
      app.serverState = await api.reviewEvidence(app.workspaceId, assetIds);
    } catch (error) {
      app.error = error instanceof Error ? error.message.trim() : 'SamenKlaar could not check these files right now.';
    } finally {
      app.pending = false;
      app.reviewingEvidence = false;
    }
  }

  async function sendMessage(event?: SubmitEvent, text = app.draft) {
    event?.preventDefault();
    if (app.pending) return;
    const assigned = !!app.serverState?.job.id;
    const matching = app.serverState?.intake?.step === 'matches';
    const replying = !!(app.role === 'professional' ? app.serverState?.proDialog : app.serverState?.agentDialog)?.awaiting;
    // With no question open, a customer can still ask their assistant to move or cancel a booked visit.
    const asking = app.role === 'customer' && app.messageTo === 'assistant' && !!app.serverState?.job.appointment && !app.serverState?.cancelled;
    const action = replying || asking ? 'agent-reply' : assigned ? 'send-message' : matching ? 'revise-intake' : app.newConversation || !app.serverState?.intake ? 'start-intake' : 'answer-intake';
    const sentDraft = app.draft;
    if (await act(action, text)) {
      if (text === sentDraft) { app.draft = ''; app.draftRestored = false; persistDraft(); }
    }
  }

  function rememberOpenRequests(result: { openRequests?: number; openRequestLimit?: number }) {
    if (typeof result.openRequests !== 'number') return;
    app.openRequests = result.openRequests;
    if (result.openRequestLimit) app.openRequestLimit = result.openRequestLimit;
  }
  const atRequestLimit = () => app.role === 'customer' && app.openRequests !== null && app.openRequests >= app.openRequestLimit;
  async function refreshWorkspaces() {
    if (app.role !== 'customer' && app.role !== 'professional') return;
    const result = await api.workspaces();
    rememberOpenRequests(result);
    const seen = new Set(result.workspaces.map((item) => item.id));
    const hadOlderPages = app.workspaces.length > 100;
    app.workspaces = [...result.workspaces, ...app.workspaces.filter((item) => !seen.has(item.id))];
    if (!hadOlderPages) app.workspaceCursor = result.nextCursor || null;
  }

  let loadingMore = false;
  async function loadMoreWorkspaces() {
    if (loadingMore || !app.workspaceCursor) return;
    loadingMore = true;
    try {
      const result = await api.workspaces(app.workspaceCursor);
      const seen = new Set(app.workspaces.map((item) => item.id));
      app.workspaces = [...app.workspaces, ...result.workspaces.filter((item) => !seen.has(item.id))];
      app.workspaceCursor = result.nextCursor || null;
    } catch (error) {
      app.error = error instanceof Error ? error.message.trim() : 'Could not load older requests.';
    } finally { loadingMore = false; }
  }

  async function selectRequest(id: string, replace = false) {
    if (app.pending || !id) return;
    const generation = ++stateGeneration;
    app.loading = true;
    app.error = '';
    try {
      persistDraft();
      const state = await api.state(id);
      if (generation !== stateGeneration) return;
      let paymentStatus: AppState['paypalStatus'] = 'not_started';
      if (app.providerConfig?.paymentsAvailable) {
        try { paymentStatus = (await api.paypalStatus(id)).status; }
        catch { paymentStatus = 'unavailable'; }
      }
      if (generation !== stateGeneration) return;
      app.workspaceId = id;
      app.serverState = state;
      app.paypalStatus = paymentStatus;
      app.newConversation = false;
      app.composeMode = 'message';
      restoreDraft();
      app.evidenceFile = null;
      rememberRequest(replace);
      await scrollToCurrent();
      if (app.role === 'customer') window.scrollTo?.(0, 0);
    } catch (error) {
      if (generation === stateGeneration) app.error = error instanceof Error ? error.message.trim() : 'Could not open this request.';
    } finally {
      if (generation === stateGeneration) app.loading = false;
    }
  }

  async function createRequest() {
    if (app.pending || app.role !== 'customer') return;
    // The server enforces the limit too; this saves a round trip and explains why.
    if (atRequestLimit()) {
      app.error = 'You have 3 open requests. Finish, cancel or delete one before starting another.';
      return;
    }
    stateGeneration++;
    app.pending = true;
    app.error = '';
    try {
      persistDraft();
      const result = await api.createWorkspace();
      app.workspaceId = result.workspace.workspaceId;
      rememberRequest();
      app.serverState = result.workspace;
      app.newConversation = true;
      app.composeMode = 'message';
      restoreDraft();
      await refreshWorkspaces();
      await scrollToCurrent();
    } catch (error) {
      app.error = error instanceof Error ? error.message.trim() : 'Could not start a new request.';
    } finally {
      app.pending = false;
    }
  }

  async function deleteRequest(id: string) {
    if (app.pending || !id || (app.role !== 'customer' && app.role !== 'professional')) return false;
    app.pending = true;
    app.error = '';
    try {
      await api.deleteWorkspace(id);
      app.workspaces = app.workspaces.filter((item) => item.id !== id);
    } catch (error) {
      app.error = error instanceof Error ? error.message.trim() : 'This request could not be deleted.';
      return false;
    } finally {
      app.pending = false;
    }
    if (app.workspaceId === id) {
      if (app.role === 'customer') setCustomerPage('requests');
      else showJobs();
    }
    saveDraft(draftKey(app.session?.account?.id, app.role, id), '');
    try { await refreshWorkspaces(); } catch { /* The request is gone; the next refresh updates the list. */ }
    return true;
  }

  async function startRequest(description: string) {
    const text = description.trim();
    if (app.role !== 'customer' || app.pending || text.length < 5 || text.length > 500) return;
    await createRequest();
    if (!app.workspaceId || !app.newConversation) return;
    if (await act('start-intake', text)) {
      saveDraft(draftKey(app.session?.account?.id, 'customer', null), '');
      app.draft = '';
      persistDraft();
    } else {
      app.draft = text;
      persistDraft();
    }
  }

  function chooseAnswer(text: string) {
    if (text === "I'll describe more" || text === "I’ll describe my budget") {
      focusComposer();
      return;
    }
    void sendMessage(undefined, text);
  }

  function focusComposer() {
    const id = app.composeMode === 'price' ? 'proposal-price' : app.composeMode === 'date' ? 'proposal-start' : 'message';
    ((document.getElementById(id) ?? document.getElementById('message')) as HTMLElement | null)?.focus();
  }
  function openProposal(mode: 'price' | 'date') {
    const job = app.serverState?.job;
    if (app.pending || !job?.id || app.serverState?.cancelled || job.workStarted || job.paid || (mode === 'price' && job.quoteApproved) || (mode === 'date' && !job.quoteApproved)) return;
    app.composeMode = mode;
    void scrollToCurrent().then(focusComposer);
  }
  function closeProposal() { app.composeMode = 'message'; void scrollToCurrent().then(focusComposer); }

  let refreshing = false;
  async function refresh() {
    const membership = app.session?.memberships?.find((item) => item.role === app.role);
    if (refreshing || app.pending || app.loading || !app.session?.authenticated || app.role === 'operator' || membership?.status !== 'active') return;
    refreshing = true;
    const generation = stateGeneration;
    const workspace = app.workspaceId;
    try {
      const lists = await api.workspaces();
      const state = workspace ? await api.state(workspace) : null;
      if (generation === stateGeneration && !app.pending && app.workspaceId === workspace) {
        app.refreshError = '';
        rememberOpenRequests(lists);
        const seen = new Set(lists.workspaces.map((item) => item.id));
        const hadOlderPages = app.workspaces.length > 100;
        app.workspaces = [...lists.workspaces, ...app.workspaces.filter((item) => !seen.has(item.id))];
        if (!hadOlderPages) app.workspaceCursor = lists.nextCursor || null;
        if (state) app.serverState = state;
      }
    } catch (error) {
      if (generation === stateGeneration && !app.pending && app.workspaceId === workspace) app.refreshError = 'Reconnecting to updates… Your saved changes are safe.';
    } finally { refreshing = false; }
    const job = app.serverState?.job;
    if (generation === stateGeneration && !app.refreshError && bookingCheckoutWorkspace === workspace && workspace === app.workspaceId && !app.pending && job?.appointment && job.appointmentConfirmation?.status === 'pending' && job.paymentAuthorizationTiming === 'at_booking' && job.paymentAuthorizedVersion !== job.scopeVersion && !job.workStarted && !job.paid && !app.serverState?.cancelled) {
      bookingCheckoutWorkspace = null;
      await beginPayPalCheckout();
    }
  }

  return {
    initialize,
    showHome,
    showRequests,
    showProfile,
    showJobs: () => showJobs(),
    restoreLocation,
    startRequest,
    loadState,
    refresh,
    loadMoreWorkspaces,
    selectRequest,
    createRequest,
    deleteRequest,
    act,
    switchWorkspace,
    beginProfessional,
    logout,
    sendMessage,
    chooseAnswer,
    focusComposer,
    openProposal,
    closeProposal,
    persistDraft,
    selectEvidenceFile,
    uploadEvidence,
    uploadRequestPhoto,
    reviewEvidence,
    beginPayPalCheckout,
    capturePayPal,
    reconcilePayPal,
    dismissError: () => { app.error = ''; }
  };
}
