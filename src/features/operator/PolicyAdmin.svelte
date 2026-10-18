<script lang="ts">
  import { onMount } from 'svelte';
  import Banner from '../../ui/Banner.svelte';
  import Pill from '../../ui/Pill.svelte';
  import EvidencePolicyReviews from './EvidencePolicyReviews.svelte';
  import { api } from '../../lib/api';
  import { showToast } from '../../ui/toast-state.svelte';
  import type { ConsumerPolicy } from '../../lib/types';
  import { dayMonth } from '../../lib/format';
  import { t } from '../../lib/i18n.svelte';

  // Publish consumer policy text that was reviewed outside the product. SamenKlaar never
  // drafts legal wording; a version is immutable once published and bookings keep the version they snapshot.
  let policies = $state<ConsumerPolicy[]>([]);
  let error = $state('');
  let busy = $state(false);
  let form = $state({ kind: 'early_start' as ConsumerPolicy['kind'], version: '', locale: 'en' as 'en' | 'nl', title: '', body: '', requiresAcknowledgment: true, reviewRationale: '', reviewReference: '' });
  const kinds: [ConsumerPolicy['kind'], string][] = [['early_start', 'Early start'], ['withdrawal', 'Withdrawal'], ['cancellation', 'Cancellation'], ['terms', 'Terms']];
  const ready = $derived(form.version.trim() && form.title.trim().length >= 3 && form.body.trim().length >= 10 && form.reviewRationale.trim().length >= 10 && form.reviewReference.trim());

  onMount(() => { void load(); });
  async function load() {
    try { policies = (await api.policies()).policies; }
    catch (e) { error = e instanceof Error ? e.message : 'Could not load consumer policies'; }
  }
  async function publish(event: SubmitEvent) {
    event.preventDefault();
    if (busy || !ready) return;
    busy = true;
    error = '';
    try {
      policies = (await api.publishPolicy({ ...form, version: form.version.trim(), title: form.title.trim() })).policies;
      showToast(t('Policy {version} published', { version: form.version.trim() }));
      form.version = ''; form.body = ''; form.title = ''; form.reviewRationale = ''; form.reviewReference = '';
    } catch (e) { error = e instanceof Error ? e.message.trim() : 'The policy could not be published'; }
    finally { busy = false; }
  }
</script>

<div class="admin">
  <h2 class="detail-title">{t('Policies')}</h2>
  {#if error}<Banner kind="error" text={t(error)} onDismiss={() => { error = ''; }}/>{/if}

  <section class="box card" aria-labelledby="current-title">
    <h3 id="current-title" class="small-title">{t('Published consumer policies')}</h3>
    {#if policies.length}
      <ul class="rows">
        {#each policies as policy (policy.kind)}
          <li><div><b>{policy.title}</b><p class="meta">{t(kinds.find(([k]) => k === policy.kind)?.[1] ?? policy.kind)} · {t('Version {version}', { version: policy.version })} · {policy.locale.toUpperCase()}{policy.publishedAt ? ` · ${dayMonth(policy.publishedAt)}` : ''}</p></div>
            <Pill variant={policy.requiresAcknowledgment ? 'warn' : 'neutral'} label={policy.requiresAcknowledgment ? t('Needs acknowledgment') : t('Informational')} small/></li>
        {/each}
      </ul>
    {:else}<p class="secondary-text">{t('Nothing is published. Until a reviewed policy exists, customers see no policy text and no acknowledgment is required.')}</p>{/if}
  </section>

  <form class="box card" onsubmit={publish}>
    <h3 class="small-title">{t('Publish a reviewed version')}</h3>
    <p class="secondary-text">{t('Paste the exact text approved by your policy owner or counsel. You are recorded as the reviewer; the reference points at the outside decision.')}</p>
    <div class="pair">
      <div class="field"><label for="pol-kind">{t('Policy')}</label>
        <select id="pol-kind" class="select" bind:value={form.kind}>{#each kinds as [value, label]}<option {value}>{t(label)}</option>{/each}</select></div>
      <div class="field"><label for="pol-lang">{t('Language')}</label>
        <select id="pol-lang" class="select" bind:value={form.locale}><option value="en">English</option><option value="nl">Nederlands</option></select></div>
    </div>
    <div class="pair">
      <div class="field"><label for="pol-version">{t('Version')}</label><input id="pol-version" class="input" bind:value={form.version} maxlength="80" required placeholder="2026-10-v1"/></div>
      <div class="field"><label for="pol-title">{t('Title')}</label><input id="pol-title" class="input" bind:value={form.title} maxlength="160" required/></div>
    </div>
    <div class="field"><label for="pol-body">{t('Exact reviewed text')}</label><textarea id="pol-body" class="textarea" rows="8" bind:value={form.body} maxlength="20000" required></textarea></div>
    <label class="check"><input type="checkbox" bind:checked={form.requiresAcknowledgment}/>{t('Customers must acknowledge this before work starts')}</label>
    <div class="field"><label for="pol-rationale">{t('Review rationale')}</label><textarea id="pol-rationale" class="textarea" rows="2" bind:value={form.reviewRationale} maxlength="1000" required placeholder={t('Who reviewed it and what they decided.')}></textarea></div>
    <div class="field"><label for="pol-ref">{t('Review reference')}</label><input id="pol-ref" class="input" bind:value={form.reviewReference} maxlength="1000" required placeholder={t('Ticket, memo or email reference')}/></div>
    <div class="row"><button class="btn btn-primary" disabled={busy || !ready} aria-busy={busy ? 'true' : undefined}>{t('Publish version')}</button>{#if !ready}<span class="helper">{t('Fill in every field to publish.')}</span>{/if}</div>
  </form>

  <EvidencePolicyReviews/>
</div>

<style>
  .admin { display: grid; gap: 18px; }
  .detail-title { font: 800 26px/1.1 var(--sk-fd); }
  .card { display: grid; gap: 12px; padding: 18px 22px; }
  .rows li { display: flex; align-items: center; justify-content: space-between; gap: 12px; padding: 10px 0; }
  .pair { display: grid; grid-template-columns: 1fr 1fr; gap: 12px; }
  .check { display: flex; align-items: center; gap: 10px; font-size: 14px; }
  .check input { width: 18px; height: 18px; accent-color: var(--sk-ink); }
  .row { display: flex; align-items: center; gap: 12px; }
  @media (max-width: 639px) { .pair { grid-template-columns: 1fr; } }
</style>
