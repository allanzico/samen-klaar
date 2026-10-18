// Layout is CSS-only; this flag only decides whether a menu opens as a popover or a phone sheet.
export const media = $state({ phone: false });

if (typeof window !== 'undefined' && window.matchMedia) {
  const query = window.matchMedia('(max-width: 639px)');
  media.phone = query.matches;
  query.addEventListener('change', (event) => { media.phone = event.matches; });
}
