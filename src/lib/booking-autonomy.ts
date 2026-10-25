import type { Job } from './types';

export const autonomyChoiceKey = (job: Job) => `${job.id}|${job.scopeVersion}|${job.quote.totalMinor}|${job.quote.currency}|${job.professionalProfileId ?? ''}`;
export function automaticReleaseOn(job: Job) {
  const grant = job.bookingAutonomy;
  return grant?.mode === 'automatic' && !!grant.decidedAt && grant.professionalAccountId === job.professionalAccountId && grant.scopeVersion === job.scopeVersion && grant.amountMinor === job.quote.totalMinor && grant.currency === job.quote.currency && grant.professionalProfileId === job.professionalProfileId;
}
export function needsAutonomyChoice(job: Job) {
  return !!job.bookingAutonomyRequired && job.bookingAutonomy?.mode !== 'manual' && !automaticReleaseOn(job);
}
