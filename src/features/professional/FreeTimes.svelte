<script lang="ts">
  import { onMount } from 'svelte';
  import Icon from '../../ui/Icon.svelte';
  import Pill from '../../ui/Pill.svelte';
  import Banner from '../../ui/Banner.svelte';
  import WorkingHoursEditor from './WorkingHoursEditor.svelte';
  import DaysOffDialog from './DaysOffDialog.svelte';
  import { api } from '../../lib/api';
  import type { ProfessionalProfile } from '../../lib/types';
  import { longDay, time } from '../../lib/format';
  import { showToast } from '../../ui/toast-state.svelte';
  import { t } from '../../lib/i18n.svelte';
  import { scheduleApi, type ScheduleView, type AvailabilityWindow } from './professional-schedule-api';

  let { compact = false }: { compact?: boolean } = $props();
  let profile = $state<ProfessionalProfile | null>(null);
  let schedule = $state<ScheduleView>({ schedule: null, daysOff: [], weeklyCount: 0, added: 0, removed: 0 });
  let window = $state<AvailabilityWindow>({ slots: [], removed: [], daysOff: [] });
  let editor = $state<WorkingHoursEditor>();
  let editing = $state(false);
  let loading = $state(true);
  let saving = $state(false);
  let error = $state('');
  let page = $state(0);
  let daysOffOpen = $state(false);
  const timezone = $derived(schedule.schedule?.timezone ?? 'Europe/Amsterdam');
  const duration = $derived(profile?.durationMinutes || 60);
  const horizon = $derived(schedule.schedule?.horizonDays ?? 90);
  const from = $derived(addDays(dateKey(new Date(), timezone), page * 7));
  const free = $derived(window.slots.filter((slot) => slot.status === 'available').length);
  const booked = $derived(window.slots.filter((slot) => slot.status === 'booked').length);
  const days = $derived(Array.from({ length: Math.min(7, horizon - page * 7) }, (_, i) => addDays(from, i)));

  function dateKey(value: Date, zone: string): string {
    const parts = new Intl.DateTimeFormat('en-CA', { timeZone: zone, year: 'numeric', month: '2-digit', day: '2-digit' }).formatToParts(value);
    return ['year', 'month', 'day'].map((type) => parts.find((part) => part.type === type)?.value).join('-');
  }
  function addDays(day: string, count: number): string { const value = new Date(day + 'T12:00:00Z'); value.setUTCDate(value.getUTCDate() + count); return value.toISOString().slice(0, 10); }
  onMount(() => { void load(); });
  async function load() {
    loading = true; error = '';
    try { [profile, schedule] = await Promise.all([api.professionalProfile().then((result) => result.profile), scheduleApi.get()]); if (!compact) await loadWindow(); }
    catch (e) { error = e instanceof Error ? e.message : t('Could not load your free times.'); }
    finally { loading = false; }
  }
  async function loadWindow() { window = await scheduleApi.window(from, 7); }
  async function changePage(value: number) { if (saving) return; saving = true; error = ''; try { page = value; await loadWindow(); } catch (e) { error = e instanceof Error ? e.message : t('Could not load your free times.'); } finally { saving = false; } }
  async function refresh() { try { schedule = await scheduleApi.get(); page = Math.min(page, Math.floor(((schedule.schedule?.horizonDays ?? 90) - 1) / 7)); await loadWindow(); } catch (e) { error = e instanceof Error ? e.message : t('Could not load your free times.'); } }
  async function changed(view: ScheduleView) { schedule = view; editing = false; page = 0; if (!compact) await refresh(); }
  export async function saveWorkingHours(): Promise<boolean> { return editor ? await editor.save() : false; }
  async function removeDayOff(id: string) { if (saving) return; saving = true; error = ''; try { await scheduleApi.removeDaysOff(id); await refresh(); showToast(t('Days off removed')); } catch (e) { error = e instanceof Error ? e.message : t('Could not remove days off.'); } finally { saving = false; } }

</script>

<div class="times" class:compact>
  {#if !compact}<header><h1 class="page-title times-title">{editing ? t('Working hours') : t('Free times')}</h1><p class="lead">{t('Customers book these times. Your working hours create them.')}</p></header>{/if}
  {#if error}<Banner kind="error" text={t(error)}/>{/if}
  {#if loading}<p class="status" role="status"><span class="spinner" aria-hidden="true"></span>{t('Loading your free times…')}</p>
  {:else if compact || editing}
    {#if !compact}<button class="btn btn-quiet back" type="button" onclick={() => { editing = false; }}>{t('Back to free times')}</button>{/if}
    <WorkingHoursEditor bind:this={editor} initial={schedule.schedule} visitMinutes={duration} embedded={compact} onSaved={(view) => void changed(view)} onCancel={compact ? undefined : () => { editing = false; }}/>
  {:else}
    <div class="grid">
      <div class="col">
        <section class="box card" aria-labelledby="working-hours-title"><h2 id="working-hours-title" class="small-title">{t('Working hours')}</h2>
          {#if schedule.schedule}<ul class="hours-summary">{#each schedule.schedule.week as day}<li><b>{t(['Monday', 'Tuesday', 'Wednesday', 'Thursday', 'Friday', 'Saturday', 'Sunday'][day.weekday - 1])}</b><span>{day.start} {t('to')} {day.end}</span></li>{/each}</ul><p class="secondary-text">{t('{visit}-minute visits, {gap} minutes apart', { visit: duration, gap: schedule.schedule.gapMinutes })}. {t('{count} times a week', { count: schedule.weeklyCount })}</p><p class="helper">{t('{notice} minutes’ notice, up to {days} days ahead.', { notice: schedule.schedule.noticeMinutes, days: schedule.schedule.horizonDays })}</p>
          {:else}<p>{t('Set your working hours so customers can book you directly.')}</p>{/if}
          <button class="btn btn-outline" type="button" onclick={() => { editing = true; }}>{schedule.schedule ? t('Edit working hours') : t('Set working hours')}</button>
        </section>
        {#if schedule.schedule}<section class="box card" aria-labelledby="days-off-title"><h2 id="days-off-title" class="small-title">{t('Days off')}</h2><p class="secondary-text">{t('No free times on these days. Booked visits stay booked.')}</p><ul class="rows off-list">{#each schedule.daysOff as day (day.id)}<li><span>{longDay(day.firstDay + 'T12:00:00Z', timezone)}{#if day.lastDay !== day.firstDay} {t('to')} {longDay(day.lastDay + 'T12:00:00Z', timezone)}{/if}{#if day.note}<small>{day.note}</small>{/if}</span><button class="icon-btn" type="button" disabled={saving} aria-label={t('Remove days off from {day}', { day: day.firstDay })} onclick={() => void removeDayOff(day.id)}><Icon name="close" size={16}/></button></li>{/each}</ul><button class="btn btn-outline" type="button" onclick={() => { daysOffOpen = true; }}>{t('Add days off')}</button></section>{/if}
      </div>
      <div class="col">
        <section class="box card upcoming" aria-labelledby="upcoming-title"><div class="card-head"><h2 id="upcoming-title" class="small-title">{t('Upcoming times')}</h2><span class="tags"><Pill variant="progress" label={t('{count} free', { count: free })}/>{#if booked}<Pill variant="booked" label={t('{count} booked', { count: booked })} icon="calendar"/>{/if}</span></div>
          <p class="helper">{t('Your working hours and days off determine these times. Booked visits stay booked.')}</p>
          <ul class="rows days">{#each days as day (day)}{@const list = window.slots.filter((slot) => dateKey(new Date(slot.startUtc), timezone) === day)}{@const off = window.daysOff.some((range) => day >= range.firstDay && day <= range.lastDay)}<li><span class="day">{longDay(day + 'T12:00:00Z', timezone)}</span><span class="pills">{#each list as slot (slot.slotId)}{#if slot.status === 'booked'}<span class="slot booked"><Icon name="calendar" size={14}/>{time(slot.startUtc, slot.timezone)}<span class="visually-hidden">, {t('booked')}</span></span>{:else}<span class="slot free">{time(slot.startUtc, slot.timezone)}</span>{/if}{/each}{#if off}<Pill variant="neutral" label={t('Day off')}/>{:else if !list.length}<span class="secondary-text">{t('No free times')}</span>{/if}</span></li>{/each}</ul>
          <div class="pager">{#if page > 0}<button class="btn btn-quiet" type="button" disabled={saving} onclick={() => void changePage(page - 1)}>{t('Show the previous 7 days')}</button>{/if}{#if (page + 1) * 7 < horizon}<button class="btn btn-quiet" type="button" disabled={saving} onclick={() => void changePage(page + 1)}>{t('Show the next 7 days')}</button>{/if}</div>
        </section>

      </div>
    </div>
  {/if}
</div>
<DaysOffDialog bind:open={daysOffOpen} {timezone} onSaved={() => void refresh()}/>

<style>
  .times { display: grid; gap: 24px; max-width: 1120px; margin: 0 auto; padding: 56px 48px 72px; }
  .times.compact { padding: 0; }
  .times-title { font-size: 36px; }
  .lead { margin-top: 10px; font-size: 15px; color: var(--sk-muted); }
  .grid { display: grid; grid-template-columns: minmax(280px, 0.85fr) minmax(0, 1.4fr); gap: 20px; align-items: start; }
  .col, .card { display: grid; gap: 16px; min-width: 0; }
  .card { padding: 20px 22px; }
  .card > .btn { justify-self: start; }
  .card-head { display: flex; flex-wrap: wrap; justify-content: space-between; align-items: center; gap: 10px; }
  .tags { display: flex; gap: 6px; }
  .status { display: flex; align-items: center; gap: 10px; }
  .days li { display: grid; gap: 10px; padding: 14px 0; }
  .day { font-size: 13px; font-weight: 600; }
  .pills { display: flex; flex-wrap: wrap; gap: 8px; align-items: center; }
  .slot { display: inline-flex; align-items: center; gap: 6px; min-height: 40px; padding: 0 14px; border: 0; border-radius: var(--sk-r-pill); font: 600 14px/1 var(--sk-ft); font-variant-numeric: tabular-nums; }
  .slot.free { background: var(--sk-soft); }
  .slot.booked { background: var(--sk-signal); color: var(--sk-on-signal); }
  .pager { display: flex; flex-wrap: wrap; gap: 8px; }
  .hours-summary { display: grid; gap: 8px; }
  .hours-summary li, .off-list li { display: flex; align-items: center; justify-content: space-between; gap: 16px; }
  .off-list li { padding: 10px 0; }
  .off-list small { display: block; color: var(--sk-muted); }
  .back { justify-self: start; }
  @media (max-width: 900px) { .grid { grid-template-columns: 1fr; } }
  @media (max-width: 639px) { .times:not(.compact) { padding: 28px 16px 40px; } .times-title { font-size: 32px; } .card { padding: 18px 16px; } }
</style>
