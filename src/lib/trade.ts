import { t } from './i18n.svelte';

export type Trade = 'plumber' | 'carpenter' | 'electrician' | 'professional';

export function tradeOf(category?: string): Trade {
  const value = (category ?? '').toLowerCase();
  if (value.includes('plumb')) return 'plumber';
  if (value.includes('carp')) return 'carpenter';
  if (value.includes('electr')) return 'electrician';
  return 'professional';
}

// Whole strings per trade so every phrase has a Dutch entry and none are stitched together.
export function tradeName(trade: Trade, plural = false): string {
  const names: Record<Trade, [string, string]> = {
    plumber: ['plumber', 'plumbers'],
    carpenter: ['carpenter', 'carpenters'],
    electrician: ['electrician', 'electricians'],
    professional: ['professional', 'professionals']
  };
  return t(names[trade][plural ? 1 : 0]);
}

export function fitsTitle(trade: Trade): string {
  const titles: Record<Trade, string> = {
    plumber: 'A plumber who fits',
    carpenter: 'A carpenter who fits',
    electrician: 'An electrician who fits',
    professional: 'A professional who fits'
  };
  return t(titles[trade]);
}

export function coverTitle(trade: Trade, count: number, area: string): string {
  return t(count === 1 ? '{count} {trade} covers {area}' : '{count} {trade} cover {area}', { count, trade: tradeName(trade, count !== 1), area });
}

export function initials(name: string): string {
  const words = name.trim().split(/\s+/).filter(Boolean);
  return (words.length > 1 ? [words[0], words[words.length - 1]] : [words[0] ?? '']).map((word) => word.charAt(0).toUpperCase()).join('') || '?';
}

// "Sanne van der Berg" becomes "Sanne B." — a first name and initial is all a professional needs.
export function shortName(name: string): string {
  const words = name.trim().split(/\s+/).filter(Boolean);
  return words.length > 1 ? `${words[0]} ${words[words.length - 1].charAt(0).toUpperCase()}.` : words[0] ?? name;
}
