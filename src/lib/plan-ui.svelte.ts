import { currentLocale } from './i18n.svelte';
import type { Appointment } from './types';

// UI-only state shared between the plan page, the next-step panel and the agreement summary.
export const planUi = $state({ slotId: '', cancelling: false, reportingIssue: false });

export function resetPlanUi() {
  planUi.slotId = '';
  planUi.cancelling = false;
  planUi.reportingIssue = false;
}

const locale = () => (currentLocale() === 'nl' ? 'nl-NL' : 'en-GB');

// "Thu 09:00" in the visit's own timezone.
export function shortVisit(startUtc: string, timezone: string): string {
  const date = new Date(startUtc);
  const day = new Intl.DateTimeFormat(locale(), { timeZone: timezone, weekday: 'short' }).format(date);
  const time = new Intl.DateTimeFormat(locale(), { timeZone: timezone, hour: '2-digit', minute: '2-digit' }).format(date);
  return `${day} ${time}`;
}

export function dayAndTime(startUtc: string, endUtc: string, timezone: string): string {
  const date = new Date(startUtc);
  const day = new Intl.DateTimeFormat(locale(), { timeZone: timezone, weekday: 'short', day: 'numeric', month: 'short' }).format(date);
  const time = (value: string) => new Intl.DateTimeFormat(locale(), { timeZone: timezone, hour: '2-digit', minute: '2-digit' }).format(new Date(value));
  return `${day}, ${time(startUtc)}–${time(endUtc)}`;
}

// The chosen slot, or the earliest one once the address is settled (handoff: "earliest preselected").
export function effectiveSlot(slots: Appointment[], addressReady: boolean): Appointment | undefined {
  return slots.find((slot) => slot.slotId === planUi.slotId) ?? (addressReady ? slots[0] : undefined);
}
