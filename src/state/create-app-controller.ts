import { draftKey, readDraft, saveDraft, clearAccountDrafts } from '../lib/drafts';
import { api } from '../lib/api';
import type { AppActions, AppState, Location, NegotiationInput } from '../lib/types';

type ScrollToCurrent = (smooth?: boolean, targetID?: string) => Promise<void>;

export function createAppActions(app: AppState, scrollToCurrent: ScrollToCurrent): AppActions {
  let stateGeneration = 0;
  function currentDraftKey() { return draftKey(app.session?.account?.id, app.role, app.workspaceId); }
  function persistDraft() { app.draftRestored = false; app.draftSaved = saveDraft(currentDraftKey(), app.draft); }
  function rememberRequest() {
    const url = new URL(window.location.href);
    if (app.workspaceId) url.searchParams.set('workspaceId', app.workspaceId);
    window.history.replaceState({}, '', url.pathname + url.search + url.hash);
  }
  function restoreDraft() { app.draft = readDraft(currentDraftKey()); app.draftRestored = !!app.draft; app.draftSaved = true; }
  async function initialize() {
    await Promise.all([
      loadConfig(),
      api.session().then((session) => { app.session = session; app.loading = session.authenticated; }).catch(() => {
        app.error = 'We couldn’t check your sign-in. You can try signing in again.';
      })
    ]);
    if (!app.session?.authenticated || !app.session.activeRole) {
      app.loading = false;
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
        const requestedID = url.searchParams.get('workspaceId');
        for (let page = 0; requestedID && !app.workspaces.some((item) => item.id === requestedID) && app.workspaceCursor && page < 100; page++) {
          const cursor = app.workspaceCursor;
          await loadMoreWorkspaces();
          if (app.workspaceCursor === cursor) break;
        }
        const selected = app.workspaces.find((item) => item.id === requestedID)
          ?? app.workspaces.find((item) => item.status !== 'paid' && item.status !== 'cancelled')
          ?? app.workspaces[0];
        if (selected) await selectRequest(selected.id);
        else if (app.role === 'customer') await createRequest();
      } catch (error) {
        app.error = error instanceof Error ? error.message.trim() : 'Could not load your requests.';
      } finally {
        app.loading = false;
      }
    } else app.loading = false;
    if (app.providerConfig?.paymentProvider === 'paypal_sandbox') {
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
      if (app.providerConfig?.paymentProvider === 'paypal_sandbox') void loadPayPalStatus();
    } catch (error) {
      if (generation === stateGeneration) app.error = error instanceof Error ? error.message : 'Something went wrong.';
    } finally {
      if (generation === stateGeneration) {
        app.loading = false;
        await scrollToCurrent();
      }
    }
  }

  async function act(action: string, value = '', location?: Location, changeAmountMinor?: number, negotiation?: NegotiationInput, rating?: number) {
    if (app.pending || app.loading || !app.workspaceId) return;
    stateGeneration++;
    app.pending = true;
    app.error = '';
    try {
      const updated = await api.action(app.workspaceId, action, value, location, changeAmountMinor, negotiation, rating);
      app.serverState = updated;
      app.newConversation = false;
      try { await refreshWorkspaces(); } catch { app.error = 'Your change was saved, but the request list could not be refreshed.'; }
      if (app.providerConfig?.paymentProvider === 'paypal_sandbox') void loadPayPalStatus();
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
    const returnedWorkspaceID = url.searchParams.get('workspaceId');
    if (returnedWorkspaceID) {
      await selectRequest(returnedWorkspaceID);
      if (app.workspaceId !== returnedWorkspaceID) return;
    }
    if (!orderId || !url.searchParams.has('paypal_return')) {
      if (url.searchParams.has('paypal_cancelled')) {
        url.searchParams.delete('paypal_cancelled');
        url.searchParams.delete('token');
        window.history.replaceState({}, '', url.pathname + url.search + url.hash);
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
      url.searchParams.delete('paypal_return');
      url.searchParams.delete('token');
      url.searchParams.delete('PayerID');
      window.history.replaceState({}, '', url.pathname + url.search + url.hash);
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

  async function uploadEvidence() {
    if (app.pending || app.role !== 'professional' || !app.evidenceFile) return;
    stateGeneration++;
    app.pending = true;
    app.error = '';
    try {
      app.serverState = await api.uploadEvidence(app.evidenceFile, app.workspaceId);
      app.evidenceFile = null;
      const input = document.getElementById('evidence-file') as HTMLInputElement | null;
      if (input) input.value = '';
    } catch (error) {
      app.error = error instanceof Error ? error.message.trim() : 'Evidence upload failed.';
    } finally {
      app.pending = false;
    }
  }

  async function reviewEvidence(assetIds: string[] = []) {
    if (app.pending || app.role !== 'customer') return;
    stateGeneration++;
    app.pending = true;
    app.error = '';
    try {
      app.serverState = await api.reviewEvidence(app.workspaceId, assetIds);
    } catch (error) {
      app.error = error instanceof Error ? error.message.trim() : 'AI evidence review failed.';
    } finally {
      app.pending = false;
    }
  }

  async function sendMessage(event?: SubmitEvent, text = app.draft) {
    event?.preventDefault();
    if (app.pending) return;
    const assigned = !!app.serverState?.job.id;
    const matching = app.serverState?.intake?.step === 'matches';
    const action = assigned ? 'send-message' : matching ? 'revise-intake' : app.newConversation || !app.serverState?.intake ? 'start-intake' : 'answer-intake';
    const sentDraft = app.draft;
    if (await act(action, text)) {
      if (text === sentDraft) { app.draft = ''; app.draftRestored = false; persistDraft(); }
    }
  }

  async function refreshWorkspaces() {
    if (app.role !== 'customer' && app.role !== 'professional') return;
    const result = await api.workspaces();
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

  async function selectRequest(id: string) {
    if (app.pending || !id) return;
    const generation = ++stateGeneration;
    app.loading = true;
    app.error = '';
    try {
      persistDraft();
      const state = await api.state(id);
      if (generation !== stateGeneration) return;
      let paymentStatus: AppState['paypalStatus'] = 'not_started';
      if (app.providerConfig?.paymentProvider === 'paypal_sandbox') {
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
      rememberRequest();
      await scrollToCurrent();
    } catch (error) {
      if (generation === stateGeneration) app.error = error instanceof Error ? error.message.trim() : 'Could not open this request.';
    } finally {
      if (generation === stateGeneration) app.loading = false;
    }
  }

  async function createRequest() {
    if (app.pending || app.role !== 'customer') return;
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
        const seen = new Set(lists.workspaces.map((item) => item.id));
        const hadOlderPages = app.workspaces.length > 100;
        app.workspaces = [...lists.workspaces, ...app.workspaces.filter((item) => !seen.has(item.id))];
        if (!hadOlderPages) app.workspaceCursor = lists.nextCursor || null;
        if (state) app.serverState = state;
      }
    } catch (error) {
      app.error = error instanceof Error ? error.message.trim() : 'Could not refresh the request.';
    } finally { refreshing = false; }
  }

  return {
    initialize,
    loadState,
    refresh,
    loadMoreWorkspaces,
    selectRequest,
    createRequest,
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
    reviewEvidence,
    beginPayPalCheckout,
    capturePayPal,
    reconcilePayPal,
    dismissError: () => { app.error = ''; }
  };
}
