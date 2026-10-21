// Missing or invalid values must stay missing, rather than becoming zero or NaN.
export function finiteNumber(value: unknown): number | null {
  if (typeof value === 'string') {
    if (!/^[+-]?\d+(?:\.\d+)?$/.test(value.trim())) return null;
    value = Number(value);
  }
  return typeof value === 'number' && Number.isFinite(value) ? value : null;
}

// Older assistant messages persisted display amounts ("€99.00" / "€ 99,00").
// Accept those alongside canonical decimal amounts without guessing at other text.
export function decimalAmount(value: unknown): number | null {
  if (typeof value !== 'string') return finiteNumber(value);
  let raw = value.trim().replace(/^(?:EUR|USD|GBP|€|\$|£)\s*/i, '').replace(/\s*(?:EUR|USD|GBP|€|\$|£)$/i, '').replace(/[\s\u00a0\u202f]/g, '');
  if (raw.includes(',')) {
    if (/^[+-]?\d{1,3}(?:\.\d{3})*,\d{1,2}$/.test(raw) || /^[+-]?\d+,\d{1,2}$/.test(raw)) raw = raw.replace(/\./g, '').replace(',', '.');
    else if (/^[+-]?\d{1,3}(?:,\d{3})+(?:\.\d{1,2})?$/.test(raw)) raw = raw.replace(/,/g, '');
    else return null;
  }
  return finiteNumber(raw);
}
