import type { CustomerProfile, AgentActivity, AgentEscalation, AgentMandate, AuthSession, CaseNote, ConsumerPolicy, CoordinationTask, EvidenceRequirement, OperatorCaseDetail, ServiceEvidencePolicy, AvailabilitySlot, Location, NegotiationInput, NotificationDeliveryHealth, OperatorCase, PayPalPayoutSetupStatus, ProfessionalApplication, ProfessionalProfile, ProfessionalProfileInput, ProfessionalReviewChecks, ProviderConfig, UserNotification, WorkspaceRole, WorkspaceState, WorkspaceSummary, ProfessionalDashboard } from './types';
import type { AdminLocationPage, AdminUserAction, AdminUserDetail, AdminUserFilter, AdminUserPage } from './operator-admin';

async function fetchResponse(url: string, init?: RequestInit): Promise<Response> {
  try {
    return await fetch(url, { ...init, signal: init?.signal ?? AbortSignal.timeout(15000) });
  } catch (error) {
    if (error instanceof Error && error.name === 'TimeoutError') {
      throw new Error('The request took too long. Please try again.');
    }
    throw error;
  }
}

async function request<T>(url: string, init?: RequestInit): Promise<T> {
  const response = await fetchResponse(url, init);
  if (!response.ok) {
    if (response.status >= 500) throw new Error('SamenKlaar is temporarily unavailable. Please try again.');
    throw new Error(await response.text());
  }
  return response.json() as Promise<T>;
}

function post<T>(url: string, body: unknown): Promise<T> {
  return request<T>(url, {
    method: 'POST',
    headers: { 'Content-Type': 'application/json' },
    body: JSON.stringify(body)
  });
}

// Only these public codes are suitable for display. Provider responses and
// deployment errors may contain private details, even when they are JSON.
const payoutErrors: Record<string, string> = {
  PAYPAL_PAYOUTS_UNAVAILABLE: 'PayPal payouts are not available yet. Please contact our team.',
  PAYPAL_CONNECTION_START_FAILED: 'We couldn’t open PayPal. Please try again. If this continues, contact our team.',
  PAYPAL_CONNECTION_VERIFY_FAILED: 'We couldn’t verify your PayPal connection. Choose Check status to try again. If this continues, contact our team.',
  PAYPAL_ALREADY_CONNECTED: 'PayPal is already connected. Choose Check status to refresh.'
};

async function payoutRequest<T>(url: string, init?: RequestInit): Promise<T> {
  const response = await fetchResponse(url, init);
  if (!response.ok) {
    const body = await response.json().catch(() => null) as { code?: unknown } | null;
    const message = typeof body?.code === 'string' && Object.prototype.hasOwnProperty.call(payoutErrors, body.code) ? payoutErrors[body.code] : undefined;
    throw new Error(message ?? (response.status === 401 ? 'Please sign in again to connect PayPal.' : 'SamenKlaar is temporarily unavailable. Please try again.'));
  }
  return response.json() as Promise<T>;
}

export const api = {
  locations: (query: string, language: string, purpose: string, signal: AbortSignal) => request<{ locations: Location[]; attribution: string }>(`/api/locations?${new URLSearchParams({ q: query, language, purpose })}`, { signal }),
  config: () => request<ProviderConfig>('/api/config', { signal: AbortSignal.timeout(5000) }),
  session: () => request<AuthSession>('/api/auth/session', { signal: AbortSignal.timeout(5000) }),
  workspaces: (cursor = '') => request<{ workspaces: WorkspaceSummary[]; nextCursor?: string; openRequests?: number; openRequestLimit?: number }>(`/api/workspaces${cursor ? `?cursor=${encodeURIComponent(cursor)}` : ''}`),
  professionalDashboard: (signal?: AbortSignal) => request<ProfessionalDashboard>('/api/professional/dashboard', { signal: signal ? AbortSignal.any([signal, AbortSignal.timeout(10000)]) : AbortSignal.timeout(10000) }),
  notifications: () => request<{ notifications: UserNotification[] }>('/api/notifications'),
  markNotificationRead: (id: number) => post<{ status: string }>('/api/notifications/read', { id }),
  createWorkspace: () => post<{ workspace: WorkspaceState }>('/api/workspaces', {}),
  // Deletes a customer's unapproved request, or takes an unapproved job off a professional's list.
  deleteWorkspace: (workspaceId: string) => request<{ status: 'deleted' | 'removed' }>(`/api/workspaces/${encodeURIComponent(workspaceId)}`, { method: 'DELETE' }),
  state: (workspaceId?: string | null) => request<WorkspaceState>(`/api/state${workspaceId ? `?workspaceId=${encodeURIComponent(workspaceId)}` : ''}`),
  action: (workspaceId: string | null, action: string, value = '', location?: Location, changeAmountMinor?: number, negotiation?: NegotiationInput, rating?: number, paymentAuthorizationTiming?: 'after_completion' | 'at_booking') =>
    post<WorkspaceState>('/api/actions', { workspaceId, action, value, location, changeAmountMinor, rating, ...negotiation, paymentAuthorizationTiming }),
  switchWorkspace: (role: WorkspaceRole) => post<{ path: string }>('/api/auth/role', { role }),
  beginProfessional: () => post<{ path: string }>('/api/professional/onboarding', {}),
  logout: () => post<{ logoutUrl: string }>('/api/auth/logout', {}),
  customerProfile: () => request<CustomerProfile>('/api/customer/profile'),
  saveCustomerProfile: (profile: Omit<CustomerProfile, 'email'>) => request<CustomerProfile>('/api/customer/profile', { method: 'PUT', headers: { 'Content-Type': 'application/json' }, body: JSON.stringify(profile) }),
  professionalProfile: () => request<{ profile: ProfessionalProfile | null }>('/api/professional/profile'),
  saveProfessionalProfile: (profile: ProfessionalProfileInput) => post<{ profile: ProfessionalProfile }>('/api/professional/profile', profile),
  beginPayPalSellerOnboarding: () => payoutRequest<{ actionUrl: string }>('/api/professional/paypal/onboarding', { method: 'POST' }),
  paypalSellerStatus: (merchantIdInPayPal?: string) => payoutRequest<PayPalPayoutSetupStatus>(`/api/professional/paypal/status${merchantIdInPayPal ? `?merchantIdInPayPal=${encodeURIComponent(merchantIdInPayPal)}` : ''}`),
  professionalAvailability: () => request<{ slots: AvailabilitySlot[] }>('/api/professional/availability'),
  operatorQueue: () => request<{ cases: OperatorCase[]; professionalApplications: ProfessionalApplication[]; notificationDelivery: NotificationDeliveryHealth }>('/api/operator/queue'),
  operatorUsers: (filter: AdminUserFilter) => request<AdminUserPage>(`/api/operator/users?${new URLSearchParams(filter)}`),
  operatorUser: (id: string) => request<AdminUserDetail>(`/api/operator/users/${encodeURIComponent(id)}`),
  administerUser: (id: string, action: AdminUserAction) => post<{ status: string }>(`/api/operator/users/${encodeURIComponent(id)}`, action),
  operatorLocations: (q: string, status: string, offset: number) => request<AdminLocationPage>(`/api/operator/locations?${new URLSearchParams({ q, status, offset: String(offset) })}`),
  reviewProfessional: (id: string, decision: 'save' | 'approve' | 'reject', checks: ProfessionalReviewChecks) =>
    post<{ status: string }>(`/api/operator/professionals/${encodeURIComponent(id)}/review`, { decision, checks }),
  resolveWorkCase: (workspaceId: string, kind: string, decision: string, rationale: string) =>
    post<{ status: string }>(`/api/operator/cases/${encodeURIComponent(workspaceId)}/resolve`, { kind, decision, rationale }),
  paypalStatus: (workspaceId: string | null) => request<{ status: string }>(`/api/paypal/status?workspaceId=${encodeURIComponent(workspaceId ?? '')}`),
  createPayPalOrder: (workspaceId: string | null) => post<{ approvalUrl: string }>(`/api/paypal/create-order?workspaceId=${encodeURIComponent(workspaceId ?? '')}`, {}),
  authorizePayPal: (orderId: string, workspaceId: string | null) => post<{ status: string }>(`/api/paypal/authorize?workspaceId=${encodeURIComponent(workspaceId ?? '')}`, { orderId }),
  capturePayPal: (workspaceId: string | null) => post<{ status: string; state: WorkspaceState }>(`/api/paypal/capture?workspaceId=${encodeURIComponent(workspaceId ?? '')}`, {}),
  reconcilePayPal: (workspaceId: string | null) => post<{ status: string; state: WorkspaceState }>(`/api/paypal/reconcile?workspaceId=${encodeURIComponent(workspaceId ?? '')}`, {}),
  uploadEvidence: (file: File, workspaceId: string | null, workItemIds: string[] = []) => {
    const body = new FormData();
    body.set('file', file);
    for (const id of workItemIds) body.append('workItemId',id);
    return request<WorkspaceState>(`/api/evidence?workspaceId=${encodeURIComponent(workspaceId ?? '')}`, { method: 'POST', body });
  },
  uploadRequestPhoto: (file: File, workspaceId: string | null) => {
    const body = new FormData();
    body.set('file', file);
    return request<WorkspaceState>(`/api/request-photos?workspaceId=${encodeURIComponent(workspaceId ?? '')}`, { method: 'POST', body });
  },
  agentMandate: () => request<AgentMandate>('/api/agent/mandate'),
  saveAgentMandate: (mandate: AgentMandate) => request<AgentMandate>('/api/agent/mandate', { method: 'PUT', headers: { 'Content-Type': 'application/json' }, body: JSON.stringify(mandate) }),
  agentActivity: (workspaceId: string) => request<AgentActivity[]>(`/api/agent/activity?workspaceId=${encodeURIComponent(workspaceId)}`),
  agentEscalations: () => request<AgentEscalation[]>('/api/agent/escalations'),
  coordination: (workspaceId: string) => request<{ tasks: CoordinationTask[] }>(`/api/coordination?workspaceId=${encodeURIComponent(workspaceId)}`),
  replyCoordination: (workspaceId: string, taskId: string, reply: string) => post<{ tasks: CoordinationTask[] }>(`/api/coordination?workspaceId=${encodeURIComponent(workspaceId)}`, { taskId, reply }),
  supportNotes: (workspaceId: string) => request<{ notes: CaseNote[] }>(`/api/support/messages?workspaceId=${encodeURIComponent(workspaceId)}`),
  policies: () => request<{ policies: ConsumerPolicy[] }>('/api/policies'),
  publishPolicy: (policy: Omit<ConsumerPolicy, 'publishedAt'>) => post<{ policies: ConsumerPolicy[] }>('/api/operator/policies', policy),
  evidencePolicy: () => request<ServiceEvidencePolicy>('/api/professional/evidence-policy'),
  proposeEvidencePolicy: (requirements: EvidenceRequirement[]) => post<ServiceEvidencePolicy>('/api/professional/evidence-policy', { requirements }),
  operatorEvidencePolicies: () => request<{ policies: ServiceEvidencePolicy[] }>('/api/operator/evidence-policies'),
  reviewEvidencePolicy: (profileId: string, approve: boolean) => post<ServiceEvidencePolicy>('/api/operator/evidence-policies', { profileId, approve }),
  caseDetail: (workspaceId: string, kind: string) => request<OperatorCaseDetail>(`/api/operator/cases/${encodeURIComponent(workspaceId)}?kind=${encodeURIComponent(kind)}`),
  updateCase: (workspaceId: string, kind: string, input: { action: string; recipient?: string; note?: string; workItemId?: string }) =>
    post<OperatorCaseDetail>(`/api/operator/cases/${encodeURIComponent(workspaceId)}?kind=${encodeURIComponent(kind)}`, { Action: input.action, Recipient: input.recipient ?? '', Note: input.note ?? '', WorkItemID: input.workItemId ?? '' }),
  reviewEvidence: async (workspaceId: string | null, assetIds: string[] = []) => {
    const response = await fetchResponse(`/api/evidence/review?workspaceId=${encodeURIComponent(workspaceId ?? '')}`, {
      method: 'POST', headers: { 'Content-Type': 'application/json' }, body: JSON.stringify({ assetIds }), signal: AbortSignal.timeout(90000)
    });
    if (!response.ok) {
      const body = await response.text();
      let code: unknown;
      try { code = JSON.parse(body)?.code; } catch { /* Older servers return plain text. */ }
      if (code === 'EVIDENCE_IMAGE_UNSUPPORTED') throw new Error('This photo could not be read for comparison. Try a smaller JPEG, PNG or WebP image.');
      if (response.status >= 500 || code === 'EVIDENCE_COMPARISON_UNAVAILABLE') throw new Error('We couldn’t compare these photos. Try again, or review the completion record yourself.');
      throw new Error(body);
    }
    return response.json() as Promise<WorkspaceState>;
  }
};
