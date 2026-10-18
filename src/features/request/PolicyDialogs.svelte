<script lang="ts">
  import Modal from '../../ui/Modal.svelte';
  import Banner from '../../ui/Banner.svelte';
  import { api } from '../../lib/api';
  import type { AppActions, AppState, ConsumerPolicy, WorkspaceState } from '../../lib/types';
  import { pendingPolicies, policyFor } from '../../lib/policy';
  import { requestUi } from '../../lib/request-ui.svelte';
  import { currentLocale, t } from '../../lib/i18n.svelte';

  // G08: the reviewed policy text the customer is asked to acknowledge before work starts, and the
  // self-service withdrawal request. SamenKlaar only shows text an operator published with a review
  // reference; it never writes legal wording itself, and a withdrawal goes to a person to decide.
  let { app, actions, workspace }: { app: AppState; actions: AppActions; workspace: WorkspaceState } = $props();
  let policies = $state<ConsumerPolicy[]>([]);
  let loaded = $state(false);
  let read = $state(false);
  let note = $state('');
  const job = $derived(workspace.job);
  const pending = $derived(pendingPolicies(job)[0]);
  const policy = $derived(pending ? policies.find((p) => p.kind === pending.kind && p.version === pending.version) : undefined);
  const withdrawal = $derived(policyFor(policies, 'withdrawal', currentLocale()));

  $effect(() => {
    if (!(requestUi.policy || requestUi.withdrawing) || loaded) return;
    api.policies().then((r) => { policies = r.policies; loaded = true; }, () => { policies = []; loaded = true; });
  });
  $effect(() => { if (!requestUi.policy) read = false; });

  async function acknowledge() {
    if (!pending || !read) return;
    if (await actions.act('acknowledge-policy', '', undefined, undefined, { policyKind: pending.kind, policyVersion: pending.version })) requestUi.policy = false;
  }
  async function withdraw() {
    if (note.trim().length < 3) return;
    if (await actions.act('request-withdrawal', note.trim())) { requestUi.withdrawing = false; note = ''; }
  }
</script>

<Modal bind:open={requestUi.policy} title={policy?.title ?? t('Before work starts')}>
  {#if !loaded}<p class="status" role="status"><span class="spinner" aria-hidden="true"></span>{t('Loading…')}</p>
  {:else if policy}
    <!-- svelte-ignore a11y_no_noninteractive_tabindex (The scrolling policy text must be reachable by keyboard.) -->
    <div class="policy" tabindex="0" role="region" aria-label={policy.title}>{policy.body}</div>
    <p class="helper">{t('Reviewed text, version {version}.', { version: policy.version })}</p>
    <label class="check"><input type="checkbox" bind:checked={read}/>{t('I have read this and agree work can start on the booked day.')}</label>
  {:else}
    <Banner kind="warn" text={t('This version of the terms isn’t available to read here. Contact our team before work starts.')}/>
  {/if}
  {#snippet footer()}
    <button type="button" class="btn btn-primary" disabled={app.pending || !policy || !read} onclick={acknowledge}>{t('Acknowledge and continue')}</button>
    <button type="button" class="btn btn-outline" onclick={() => { requestUi.policy = false; }}>{t('Not now')}</button>
  {/snippet}
</Modal>

<Modal bind:open={requestUi.withdrawing} title={t('Withdraw from this service')}>
  {#if !loaded}<p class="status" role="status"><span class="spinner" aria-hidden="true"></span>{t('Loading…')}</p>
  {:else if withdrawal}
    <!-- svelte-ignore a11y_no_noninteractive_tabindex (The scrolling policy text must be reachable by keyboard.) -->
    <div class="policy" tabindex="0" role="region" aria-label={withdrawal.title}>{withdrawal.body}</div>
  {:else}
    <p class="secondary-text">{t('Our team reviews every withdrawal request. Nothing is cancelled or refunded until a person decides.')}</p>
  {/if}
  <div class="field">
    <label for="withdraw-note">{t('Why are you withdrawing?')}</label>
    <textarea id="withdraw-note" class="textarea" rows="3" minlength="3" maxlength="500" bind:value={note} disabled={app.pending}></textarea>
    {#if note.trim().length < 3}<p class="field-help">{t('Write at least 3 characters to continue')}</p>{/if}
  </div>
  {#snippet footer()}
    <button type="button" class="btn btn-primary" disabled={app.pending || note.trim().length < 3} onclick={withdraw}>{t('Request withdrawal review')}</button>
    <button type="button" class="btn btn-outline" onclick={() => { requestUi.withdrawing = false; }}>{t('Keep the service')}</button>
  {/snippet}
</Modal>

<style>
  .policy { max-height: 280px; overflow: auto; padding: 12px 14px; border-radius: var(--sk-r-card); background: var(--sk-ground); font-size: 13px; line-height: 1.55; white-space: pre-wrap; }
  .check { display: flex; align-items: flex-start; gap: 10px; font-size: 14px; }
  .check input { width: 18px; height: 18px; margin-top: 2px; accent-color: var(--sk-ink); }
  .status { display: flex; align-items: center; gap: 10px; }
</style>
