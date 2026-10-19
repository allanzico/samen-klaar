import type { Appointment } from './types';

export type PaymentTiming = 'after_completion' | 'at_booking';

// UI-only state shared by a request's stage, its next-step panel and its dialogs. Reset per request.
export const requestUi = $state({
  slotId: '',
  timing: 'after_completion' as PaymentTiming,
  timingJob: '',
  cancelling: false,
  reporting: false,
  extraWork: false,
  policy: false,
  withdrawing: false,
  deleting: false,
  photo: null as null | { src: string; caption: string; index: number; total: number }
});

export function resetRequestUi() {
  requestUi.slotId = '';
  requestUi.timing = 'after_completion';
  requestUi.timingJob = '';
  requestUi.cancelling = false;
  requestUi.reporting = false;
  requestUi.extraWork = false;
  requestUi.policy = false;
  requestUi.withdrawing = false;
  requestUi.deleting = false;
  requestUi.photo = null;
}

// The chosen slot, or the earliest one once the address is settled.
export function effectiveSlot(slots: Appointment[], addressReady: boolean): Appointment | undefined {
  return slots.find((slot) => slot.slotId === requestUi.slotId) ?? (addressReady ? slots[0] : undefined);
}
