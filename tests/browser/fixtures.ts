import type { WorkspaceState } from '../../src/lib/types';

export function emptyWorkspace(workspaceId: string): WorkspaceState {
  return {
    workspaceId, availableSlots: [],
    job: {
      id: '', title: '', customer: '', professional: '', description: '', category: '',
      scopeVersion: 1, scopeHistory: [], quoteApproved: false,
      quote: { currency: 'EUR', lines: [], totalMinor: 0, materialsPolicy: '', exclusions: [], durationMinutes: 60, expires: '' },
      needsClarification: false, clarificationQuestion: '', clarificationAnswer: '', appointment: null,
      workStarted: false, evidenceSubmitted: false, detailsRequested: false, detailAdded: false,
      completionApproved: false, paymentApprovedVersion: 0, paid: false, evidence: [], change: null, timeline: []
    }
  };
}
