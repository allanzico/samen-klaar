<script lang="ts">
  import { untrack } from 'svelte';
  import Banner from '../../ui/Banner.svelte';
  import { t } from '../../lib/i18n.svelte';
  import { showToast } from '../../ui/toast-state.svelte';
  import { scheduleApi, type ProfessionalSchedule, type SchedulePreview, type ScheduleView } from './professional-schedule-api';

  let { initial = null, visitMinutes = 60, embedded = false, onSaved, onCancel }: { initial?: ProfessionalSchedule | null; visitMinutes?: number; embedded?: boolean; onSaved?: (view: ScheduleView) => void; onCancel?: () => void } = $props();
  const initialValue = untrack(() => initial);
  const names = ['Monday', 'Tuesday', 'Wednesday', 'Thursday', 'Friday', 'Saturday', 'Sunday'];
  let week = $state(names.map((_, index) => ({ weekday: index + 1, enabled: initialValue ? initialValue.week.some((day) => day.weekday === index + 1) : index < 6, start: initialValue?.week.find((day) => day.weekday === index + 1)?.start ?? (index === 5 ? '09:00' : '08:00'), end: initialValue?.week.find((day) => day.weekday === index + 1)?.end ?? (index === 5 ? '13:00' : '17:00') })));
  let gap = $state(initialValue?.gapMinutes ?? 30);
  let notice = $state(initialValue?.noticeMinutes ?? 1440);
  let horizon = $state(initialValue?.horizonDays ?? 28);
  let preview = $state<SchedulePreview | null>(null);
  let saving = $state(false);
  let error = $state('');
  const input = $derived<ProfessionalSchedule>({ timezone: initial?.timezone ?? 'Europe/Amsterdam', visitMinutes, gapMinutes: Number(gap), noticeMinutes: Number(notice), horizonDays: Number(horizon), week: week.filter((day) => day.enabled).map(({ weekday, start, end }) => ({ weekday, start, end })) });

  $effect(() => {
    const snapshot = $state.snapshot(input);
    let cancelled = false;
    const timer = setTimeout(() => { void scheduleApi.preview(snapshot).then((result) => { if (!cancelled) { preview = result; error = ''; } }).catch((e) => { if (!cancelled) { preview = null; error = e instanceof Error ? e.message : t('Could not preview working hours.'); } }); }, 300);
    return () => { cancelled = true; clearTimeout(timer); };
  });

  export async function save(): Promise<boolean> {
    if (saving) return false;
    saving = true; error = '';
    try { const view = await scheduleApi.save(input); showToast(t('Working hours saved')); onSaved?.(view); return true; }
    catch (e) { error = e instanceof Error ? e.message : t('Could not save working hours.'); return false; }
    finally { saving = false; }
  }
</script>

<div class="hours-editor">
  <div class="editor-main">
    <h2 class="small-title">{t('Days and hours')}</h2>
    <p class="secondary-text">{t('Booked visits stay booked. Saving changes your free times right away.')}</p>
    <div class="week">
      {#each week as day, index (day.weekday)}
        <div class="day-row">
          <label class="day-switch"><input type="checkbox" role="switch" bind:checked={day.enabled} disabled={saving} aria-label={t('Work on {day}', { day: t(names[index]) })}/><span class="track" aria-hidden="true"></span><span>{t(names[index])}</span></label>
          {#if day.enabled}<div class="range"><input class="input time-field" type="time" step="900" bind:value={day.start} disabled={saving} aria-label={t('{day}, start', { day: t(names[index]) })}/><span aria-hidden="true">{t('to')}</span><input class="input time-field" type="time" step="900" bind:value={day.end} disabled={saving} aria-label={t('{day}, end', { day: t(names[index]) })}/></div>
          {:else}<span class="secondary-text off">{t('Not working')}</span>{/if}
        </div>
      {/each}
    </div>
    <div class="timing">
      <div class="field"><label for="schedule-gap">{t('Time between visits')}</label><select id="schedule-gap" class="input" bind:value={gap} disabled={saving}>{#each [0, 15, 30, 45, 60] as value}<option value={value}>{t('{minutes} minutes', { minutes: value })}</option>{/each}{#if ![0, 15, 30, 45, 60].includes(gap)}<option value={gap}>{t('{minutes} minutes', { minutes: gap })}</option>{/if}</select></div>
      <div class="field"><label for="schedule-notice">{t('Notice before a visit')}</label><select id="schedule-notice" class="input" bind:value={notice} disabled={saving}><option value={180}>{t('3 hours')}</option><option value={1440}>{t('1 day')}</option><option value={2880}>{t('2 days')}</option>{#if ![180, 1440, 2880].includes(notice)}<option value={notice}>{t('{minutes} minutes', { minutes: notice })}</option>{/if}</select></div>
      <div class="field"><label for="schedule-horizon">{t('How far ahead')}</label><select id="schedule-horizon" class="input" bind:value={horizon} disabled={saving}>{#each [14, 28, 56] as value}<option value={value}>{t('{count} weeks', { count: value / 7 })}</option>{/each}{#if ![14, 28, 56].includes(horizon)}<option value={horizon}>{t('{count} days', { count: horizon })}</option>{/if}</select></div>
    </div>
    {#if error}<Banner kind="error" text={t(error)}/>{/if}
    {#if !embedded}<div class="actions"><button class="btn btn-primary" type="button" disabled={saving} onclick={() => void save()}>{saving ? t('Saving…') : t('Save hours')}</button>{#if onCancel}<button class="btn btn-outline" type="button" disabled={saving} onclick={onCancel}>{t('Cancel')}</button>{/if}</div>{/if}
  </div>
  <aside class="preview" aria-live="polite">
    <h2 class="small-title">{t('Times customers can book')}</h2>
    {#if preview}<p class="weekly-count">{t('{count} times a week', { count: preview.weeklyCount })}</p><p class="secondary-text">{t('{visit}-minute visits, {gap} minutes apart', { visit: visitMinutes, gap })}</p><ul class="typical">{#each preview.typicalWeek as day}<li><b>{t(names[day.weekday - 1])}</b><span>{day.starts.length ? day.starts.join(' · ') : t('Not working')}</span></li>{/each}</ul>{:else}<p role="status">{t('Checking your working hours…')}</p>{/if}
    <p class="helper">{t('Customers can book these once your profile is approved.')}</p>
  </aside>
</div>

<style>
  .hours-editor { display: grid; grid-template-columns: minmax(0, 1.5fr) minmax(220px, 1fr); gap: 28px; }
  .editor-main, .preview { display: grid; align-content: start; gap: 16px; min-width: 0; }
  .day-row { display: flex; align-items: center; justify-content: space-between; flex-wrap: wrap; gap: 12px; min-height: 66px; border-bottom: 1px solid var(--sk-line); padding: 8px 0; }
  .day-switch { display: inline-flex; align-items: center; gap: 12px; cursor: pointer; font-weight: 600; min-height: 44px; position: relative; }
  .day-switch input { position: absolute; opacity: 0; width: 44px; height: 26px; }
  .track { width: 44px; height: 26px; border-radius: 20px; background: var(--sk-surface-2); box-shadow: inset 0 0 0 1.5px var(--sk-field); position: relative; }
  .track::after { content: ''; position: absolute; width: 20px; height: 20px; top: 3px; left: 3px; background: var(--sk-surface); border-radius: 50%; box-shadow: 0 1px 2px var(--sk-shadow); }
  input:checked + .track { background: var(--sk-primary, var(--sk-brand)); box-shadow: none; }
  input:checked + .track::after { left: 21px; }
  input:focus-visible + .track { outline: 2px solid var(--sk-ink); outline-offset: 4px; }
  .range { display: flex; align-items: center; gap: 10px; }
  .time-field { width: 104px; height: 48px; text-align: center; font-variant-numeric: tabular-nums; }
  .timing { display: grid; grid-template-columns: repeat(3, minmax(0, 1fr)); gap: 12px; }
  .actions { display: flex; flex-wrap: wrap; gap: 12px; }
  .preview { padding-left: 24px; border-left: 1px solid var(--sk-line); }
  .weekly-count { font: 800 24px/1.2 var(--sk-fd); }
  .typical { display: grid; gap: 12px; }
  .typical li { display: grid; gap: 4px; font-size: 13px; }
  .typical span { color: var(--sk-muted); overflow-wrap: anywhere; }
  @media(max-width: 900px) { .hours-editor { grid-template-columns: 1fr; } .preview { border-left: 0; border-top: 1px solid var(--sk-line); padding-left: 0; padding-top: 20px; } }
  @media(max-width: 639px) { .time-field { width: 94px; } .timing { grid-template-columns: 1fr; } .day-row { gap: 6px; } }
</style>
