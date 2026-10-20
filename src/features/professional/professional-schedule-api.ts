export type WorkingRange = { weekday: number; start: string; end: string };
export type ProfessionalSchedule = { timezone: string; visitMinutes: number; gapMinutes: number; noticeMinutes: number; horizonDays: number; week: WorkingRange[] };
export type ProfessionalDayOff = { id: string; firstDay: string; lastDay: string; note: string };
export type ScheduleView = { schedule: ProfessionalSchedule | null; daysOff: ProfessionalDayOff[]; weeklyCount: number; added: number; removed: number };
export type SchedulePreview = { weeklyCount: number; typicalWeek: { weekday: number; starts: string[] }[] };
export type ScheduleSlot = { slotId: string; startUtc: string; endUtc: string; timezone: string; status: string; source: 'manual' | 'schedule' };
export type AvailabilityWindow = { slots: ScheduleSlot[]; removed: { startUtc: string; endUtc: string }[]; daysOff: ProfessionalDayOff[] };
export type DaysOffPreview = { removedCount: number; bookedVisits: { jobId: string; workspaceId: string; title: string; startUtc: string; endUtc: string }[] };

async function request<T>(path: string, method = 'GET', body?: unknown): Promise<T> {
  const response = await fetch(`/api/professional/${path}`, { method, headers: body === undefined ? undefined : { 'Content-Type': 'application/json' }, body: body === undefined ? undefined : JSON.stringify(body) });
  if (!response.ok) throw new Error(response.status >= 500 ? 'SamenKlaar is temporarily unavailable. Please try again.' : (await response.text()).trim());
  return (response.status === 204 ? undefined : await response.json()) as T;
}

export const scheduleApi = {
  get: () => request<ScheduleView>('schedule'),
  save: (schedule: ProfessionalSchedule) => request<ScheduleView>('schedule', 'PUT', schedule),
  preview: (schedule: ProfessionalSchedule) => request<SchedulePreview>('schedule/preview', 'POST', schedule),
  window: (from: string, days = 7) => request<AvailabilityWindow>(`availability?from=${encodeURIComponent(from)}&days=${days}`),
  previewDaysOff: (firstDay: string, lastDay: string) => request<DaysOffPreview>('days-off/preview', 'POST', { firstDay, lastDay }),
  addDaysOff: (firstDay: string, lastDay: string, note: string) => request<{ dayOff: ProfessionalDayOff; removed: number }>('days-off', 'POST', { firstDay, lastDay, note }),
  removeDaysOff: (id: string) => request<{ added: number }>(`days-off/${encodeURIComponent(id)}`, 'DELETE')
};
