// Client-side .ics for an agreed visit. Nothing is sent anywhere, and the private address is not included.
const stamp = (iso: string) => new Date(iso).toISOString().replace(/[-:]/g, '').replace(/\.\d{3}/, '');
const escape = (text: string) => text.replace(/\\/g, '\\\\').replace(/;/g, '\\;').replace(/,/g, '\\,').replace(/\n/g, '\\n');

export function visitCalendar(id: string, summary: string, startUtc: string, endUtc: string): string {
  return ['BEGIN:VCALENDAR', 'VERSION:2.0', 'PRODID:-//SamenKlaar//Visit//EN', 'BEGIN:VEVENT', `UID:${escape(id)}@samenklaar`, `DTSTAMP:${stamp(new Date().toISOString())}`, `DTSTART:${stamp(startUtc)}`, `DTEND:${stamp(endUtc)}`, `SUMMARY:${escape(summary)}`, 'END:VEVENT', 'END:VCALENDAR'].join('\r\n');
}

export function downloadVisitCalendar(id: string, summary: string, startUtc: string, endUtc: string) {
  const url = URL.createObjectURL(new Blob([visitCalendar(id, summary, startUtc, endUtc)], { type: 'text/calendar;charset=utf-8' }));
  const link = document.createElement('a');
  link.href = url;
  link.download = 'samenklaar-visit.ics';
  link.click();
  URL.revokeObjectURL(url);
}
