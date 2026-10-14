import { currentLocale, t } from './i18n.svelte';

export function localDateTime(date: Date): string {
  return new Date(date.getTime() - date.getTimezoneOffset() * 60000).toISOString().slice(0, 16);
}
export function defaultVisitEnd(start: string, duration: number): string {
  const value = new Date(start);
  return Number.isFinite(value.getTime()) ? localDateTime(new Date(value.getTime() + duration * 60000)) : '';
}
export function validateVisit(start: string, end: string, minimumMinutes: number, now = Date.now()): string {
  const startTime = new Date(start).getTime(), endTime = new Date(end).getTime();
  if (!Number.isFinite(startTime) || !Number.isFinite(endTime)) return t('Choose a valid start and end time.');
  if (startTime < now + 15 * 60000) return t('Choose a start time at least 15 minutes from now.');
  if (startTime > now + 365 * 86400000) return t('Choose a visit within the next year.');
  if (endTime <= startTime) return t('The end time must be after the start time.');
  if (endTime - startTime < minimumMinutes * 60000) return t('Allow at least {duration} minutes for this service.', { duration: minimumMinutes });
  if (endTime - startTime > 360 * 60000) return t('A visit can be at most 6 hours long.');
  return '';
}
export function formatVisit(start: string, end: string, timezone: string): string {
  const locale = currentLocale() === 'nl' ? 'nl-NL' : 'en-GB';
  const options: Intl.DateTimeFormatOptions = { timeZone: timezone, weekday: 'short', month: 'short', day: 'numeric', hour: '2-digit', minute: '2-digit' };
  return `${new Intl.DateTimeFormat(locale, options).format(new Date(start))} – ${new Intl.DateTimeFormat(locale, { timeZone: timezone, hour: '2-digit', minute: '2-digit' }).format(new Date(end))} (${timezone})`;
}

