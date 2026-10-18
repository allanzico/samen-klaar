import { currentLocale, t } from './i18n.svelte';

const locale = () => (currentLocale() === 'nl' ? 'nl-NL' : 'en-GB');

// Money always uses nl-NL formatting ("€ 95,00", non-breaking space) and tabular figures in CSS.
export function money(minor: number, currency = 'EUR'): string {
  const code = /^[A-Za-z]{3}$/.test(currency) ? currency.toUpperCase() : 'EUR';
  return new Intl.NumberFormat('nl-NL', { style: 'currency', currency: code }).format(minor / 100);
}

export function time(iso: string, timeZone?: string): string {
  return new Intl.DateTimeFormat(locale(), { timeZone, hour: '2-digit', minute: '2-digit' }).format(new Date(iso));
}
// "Mon 19 Oct"
export function shortDay(iso: string, timeZone?: string): string {
  return new Intl.DateTimeFormat(locale(), { timeZone, weekday: 'short', day: 'numeric', month: 'short' }).format(new Date(iso));
}
// "Monday 19 October"
export function longDay(iso: string, timeZone?: string): string {
  return new Intl.DateTimeFormat(locale(), { timeZone, weekday: 'long', day: 'numeric', month: 'long' }).format(new Date(iso));
}
// "19 Oct"
export function dayMonth(iso: string): string {
  const date = new Date(iso);
  return Number.isNaN(date.getTime()) ? '' : new Intl.DateTimeFormat(locale(), { day: 'numeric', month: 'short' }).format(date);
}
// "Mon 09:00"
export function shortVisit(iso: string, timeZone?: string): string {
  const weekday = new Intl.DateTimeFormat(locale(), { timeZone, weekday: 'short' }).format(new Date(iso));
  return `${weekday} ${time(iso, timeZone)}`;
}
// "Mon 19 Oct, 09:00 to 10:00"
export function visitRange(startIso: string, endIso: string, timeZone?: string): string {
  return t('{day}, {start} to {end}', { day: shortDay(startIso, timeZone), start: time(startIso, timeZone), end: time(endIso, timeZone) });
}
// "Mon 19 Oct, 09:00"
export function dayTime(iso: string, timeZone?: string): string {
  return `${shortDay(iso, timeZone)}, ${time(iso, timeZone)}`;
}
// "Today", "Friday 16 October" for chat day dividers; "14:12" or "16 Oct, 14:12" for meta lines.
export function sameDay(a: Date, b: Date): boolean {
  return a.getFullYear() === b.getFullYear() && a.getMonth() === b.getMonth() && a.getDate() === b.getDate();
}
export function stamp(iso: string): string {
  const date = new Date(iso);
  if (Number.isNaN(date.getTime())) return '';
  return sameDay(date, new Date()) ? time(iso) : `${dayMonth(iso)}, ${time(iso)}`;
}
export function weekdayShort(iso: string, timeZone?: string): string {
  return new Intl.DateTimeFormat(locale(), { timeZone, weekday: 'short' }).format(new Date(iso));
}
export function dayNumber(iso: string, timeZone?: string): string {
  return new Intl.DateTimeFormat(locale(), { timeZone, day: 'numeric' }).format(new Date(iso));
}
export function timezoneCity(timeZone: string): string {
  return timeZone.split('/').pop()?.replace(/_/g, ' ') ?? timeZone;
}
