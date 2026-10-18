<script lang="ts">
  import Icon from '../../ui/Icon.svelte';
  import type { Intake } from '../../lib/types';
  import { t } from '../../lib/i18n.svelte';

  // B04: "Your job so far", built from the intake state the app already has.
  let { intake, onEdit, busy }: { intake?: Intake; onEdit: () => void; busy: boolean } = $props();
  const order = ['detail', 'urgency', 'budget', 'location', 'matches'];
  const at = $derived(intake ? Math.max(0, order.indexOf(intake.step === 'waitlist' ? 'matches' : intake.step)) : -1);
  type Row = { label: string; value: string; state: 'done' | 'current' | 'todo' };
  const rows = $derived.by((): Row[] => {
    const state = (index: number, value: string): Row['state'] => (value ? 'done' : index === at ? 'current' : index < at ? 'done' : 'todo');
    const list: Row[] = [];
    if (intake?.category) list.push({ label: t('Trade'), value: t(intake.category), state: 'done' });
    list.push({ label: t('Problem'), value: intake?.description ?? '', state: intake?.description ? 'done' : 'current' });
    list.push({ label: t('Details'), value: intake?.detail ?? '', state: state(0, intake?.detail ?? '') });
    list.push({ label: t('When'), value: intake?.urgency ? t(intake.urgency) : '', state: state(1, intake?.urgency ?? '') });
    list.push({ label: t('Budget'), value: intake?.budget ? t(intake.budget) : '', state: state(2, intake?.budget ?? '') });
    list.push({ label: t('Area'), value: intake?.location?.label ?? '', state: state(3, intake?.location?.label ?? '') });
    return list;
  });
</script>

<section class="box so-far" aria-labelledby="so-far-title">
  <div class="head"><h2 id="so-far-title" class="small-title">{t('Your job so far')}</h2>{#if intake}<button type="button" class="btn btn-quiet" disabled={busy} onclick={onEdit}>{t('Edit')}</button>{/if}</div>
  <ul>
    {#each rows as row (row.label)}
      <li class={row.state}>
        <span class="mark" aria-hidden="true">{#if row.state === 'done'}<Icon name="check" size={12}/>{/if}</span>
        <span><small>{row.label}</small><b>{row.value || (row.state === 'current' ? t('Asking now') : t('Next'))}</b></span>
      </li>
    {/each}
  </ul>
</section>

<style>
  .so-far { display: grid; gap: 8px; padding: 20px; }
  .head { display: flex; justify-content: space-between; align-items: center; }
  ul { display: grid; gap: 14px; }
  li { display: grid; grid-template-columns: 20px 1fr; gap: 10px; align-items: start; }
  .mark { display: grid; place-items: center; width: 20px; height: 20px; margin-top: 2px; border-radius: 50%; box-shadow: inset 0 0 0 1.5px var(--sk-field); }
  .done .mark { background: var(--sk-soft); box-shadow: none; }
  .current .mark { box-shadow: inset 0 0 0 2px var(--sk-ink); }
  .current .mark::after { content: ""; width: 8px; height: 8px; border-radius: 50%; background: var(--sk-ink); }
  small { display: block; font-size: 12px; color: var(--sk-muted); }
  b { display: block; font-size: 14px; font-weight: 600; overflow-wrap: anywhere; display: -webkit-box; -webkit-line-clamp: 3; line-clamp: 3; -webkit-box-orient: vertical; overflow: hidden; }
  .todo b { color: var(--sk-muted); font-weight: 500; }
</style>
