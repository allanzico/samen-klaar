<script lang="ts">
  import type { Appointment } from '../../lib/types';
  import { longDay, time, timezoneCity } from '../../lib/format';
  import { effectiveSlot, requestUi } from '../../lib/request-ui.svelte';
  import { t } from '../../lib/i18n.svelte';

  // Handoff 6.16: day headings with pill slots; the selected slot is pressed. A released slot shows struck through.
  let { slots, disabled = false, unavailable }: { slots: Appointment[]; disabled?: boolean; unavailable?: Appointment } = $props();
  let showAll = $state(false);
  const selected = $derived(effectiveSlot(slots, !disabled));
  const days = $derived.by(() => {
    const groups = new Map<string, { day: string; slots: (Appointment & { gone?: boolean })[] }>();
    const list: (Appointment & { gone?: boolean })[] = [...slots];
    if (unavailable && !slots.some((s) => s.startUtc === unavailable.startUtc)) list.push({ ...unavailable, gone: true });
    list.sort((a, b) => a.startUtc.localeCompare(b.startUtc));
    for (const slot of list) {
      const day = longDay(slot.startUtc, slot.timezone);
      const group = groups.get(day) ?? { day, slots: [] };
      group.slots.push(slot);
      groups.set(day, group);
    }
    const all = [...groups.values()];
    return showAll ? all : all.slice(0, 3);
  });
  const dayCount = $derived(new Set(slots.map((slot) => longDay(slot.startUtc, slot.timezone))).size);
</script>

<div class="picker" role="group" aria-label={t('Visit times')}>
  <div class="days">
    {#each days as group (group.day)}
      <div class="day">
        <h3>{group.day}</h3>
        <div class="slots">
          {#each group.slots as slot (slot.startUtc)}
            {#if slot.gone}
              <button type="button" class="slot gone" disabled aria-label={t('{time}, no longer available', { time: time(slot.startUtc, slot.timezone) })}>{time(slot.startUtc, slot.timezone)}</button>
            {:else}
              <button type="button" class="slot" aria-pressed={selected?.slotId === slot.slotId} {disabled} onclick={() => { requestUi.slotId = slot.slotId; }}>{time(slot.startUtc, slot.timezone)}</button>
            {/if}
          {/each}
        </div>
      </div>
    {/each}
  </div>
  <div class="foot">
    <p class="secondary-text">{#if selected}{t('Ends {time}, {city} time', { time: time(selected.endUtc, selected.timezone), city: timezoneCity(selected.timezone) })}{:else if slots[0]}{t('Times in {city} time', { city: timezoneCity(slots[0].timezone) })}{/if}</p>
    {#if dayCount > 3}<button type="button" class="btn btn-quiet" aria-expanded={showAll} onclick={() => { showAll = !showAll; }}>{showAll ? t('Fewer days') : t('More days')}</button>{/if}
  </div>
</div>

<style>
  .picker { display: grid; gap: 12px; margin-top: 10px; }
  .days { display: grid; grid-template-columns: repeat(auto-fit, minmax(min(150px, 100%), 1fr)); gap: 18px; }
  h3 { margin-bottom: 8px; font: 600 13px/1.3 var(--sk-ft); }
  .slots { display: flex; flex-wrap: wrap; gap: 8px; }
  .slot { min-width: 76px; min-height: 40px; padding: 0 14px; border: 0; border-radius: var(--sk-r-pill); background: var(--sk-surface); box-shadow: inset 0 0 0 1.5px var(--sk-field); color: var(--sk-ink); font: 600 14px/1 var(--sk-ft); font-variant-numeric: tabular-nums; }
  .slot:hover:not(:disabled) { background: var(--sk-ground); }
  .slot[aria-pressed="true"] { background: var(--sk-primary); color: var(--sk-on-primary); box-shadow: inset 0 0 0 1.5px var(--sk-primary); }
  .slot:disabled:not(.gone) { opacity: .55; }
  .slot.gone { background: var(--sk-surface-2); box-shadow: none; border: 1.5px dashed var(--sk-dash); color: var(--sk-muted); text-decoration: line-through; }
  .foot { display: flex; flex-wrap: wrap; align-items: center; justify-content: space-between; gap: 8px; }
  @media (max-width: 639px) { .days { grid-template-columns: 1fr; } }
</style>
