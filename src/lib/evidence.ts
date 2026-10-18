import type { Evidence, Job, WorkItem } from './types';

// Mirrors CurrentEvidence / MissingEvidence in internal/workflow/evidence_policy.go. The server
// decides; this only explains to people which agreed proof is still outstanding.
export type ItemState = 'added' | 'missing' | 'resolved';

export const evidenceRound = (job: Job): number => (job.completionRound && job.completionRound > 0 ? job.completionRound : 1);

export function isCurrentEvidence(job: Job, e: Evidence): boolean {
  if (!e.completionRound && !e.scopeVersion) return !(job.workItems?.length) && evidenceRound(job) === 1;
  return e.scopeVersion === job.scopeVersion && e.completionRound === evidenceRound(job);
}

export function workItemState(job: Job, item: WorkItem): ItemState {
  if (job.evidenceResolutions?.some((r) => r.workItemId === item.id && r.scopeVersion === job.scopeVersion && r.completionRound === evidenceRound(job))) return 'resolved';
  const covered = job.evidence.some((e) => isCurrentEvidence(job, e)
    && (e.kind === item.kind || (item.kind === 'file' && !!e.assetId))
    && (e.kind === 'note' || (!!e.assetId && !!e.contentType))
    && (e.workItemIds ?? []).includes(item.id));
  return covered ? 'added' : 'missing';
}

export function workItems(job: Job): { item: WorkItem; state: ItemState }[] {
  return (job.workItems ?? []).map((item) => ({ item, state: workItemState(job, item) }));
}

export const missingItems = (job: Job) => workItems(job).filter((entry) => entry.state === 'missing');
