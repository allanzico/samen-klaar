import type { ConsumerPolicy, Job, PolicyRequirement } from './types';

// Policies recorded with a booking that still need the customer's explicit acknowledgment.
export function pendingPolicies(job: Job): PolicyRequirement[] {
  return (job.policyRequirements ?? []).filter((required) => required.requiresAcknowledgment
    && !(job.policyAcknowledgments ?? []).some((ack) => ack.kind === required.kind && ack.version === required.version && ack.scopeVersion === job.scopeVersion));
}

export const policyFor = (policies: ConsumerPolicy[], kind: string, locale: string): ConsumerPolicy | undefined =>
  policies.find((policy) => policy.kind === kind && policy.locale === locale) ?? policies.find((policy) => policy.kind === kind);
