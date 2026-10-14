import type { AuthSession, AvailabilitySlot, Location, NegotiationInput, NotificationDeliveryHealth, OperatorCase, PayPalPayoutSetupStatus, ProfessionalApplication, ProfessionalProfile, ProfessionalProfileInput, ProfessionalReviewChecks, ProviderConfig, UserNotification, WorkspaceRole, WorkspaceState, WorkspaceSummary } from './types';

async function request<T>(url: string, init?: RequestInit): Promise<T> {
  const response = await fetch(url, init);
  if (!response.ok) throw new Error(await response.text());
  return response.json() as Promise<T>;
}

function post<T>(url: string, body: unknown): Promise<T> {
  return request<T>(url, {
    method: 'POST',
    headers: { 'Content-Type': 'application/json' },
    body: JSON.stringify(body)
  });
}

export const api = {
  config: () => request<ProviderConfig>('/api/config', { signal: AbortSignal.timeout(5000) }),
  session: () => request<AuthSession>('/api/auth/session', { signal: AbortSignal.timeout(5000) }),
  workspaces: (cursor = '') => request<{ workspaces: WorkspaceSummary[]; nextCursor?: string }>(`/api/workspaces${cursor ? `?cursor=${encodeURIComponent(cursor)}` : ''}`),
  notifications: () => request<{ notifications: UserNotification[] }>('/api/notifications'),
  markNotificationRead: (id: number) => post<{ status: string }>('/api/notifications/read', { id }),
  createWorkspace: () => post<{ workspace: WorkspaceState }>('/api/workspaces', {}),
  state: (workspaceId?: string | null) => request<WorkspaceState>(`/api/state${workspaceId ? `?workspaceId=${encodeURIComponent(workspaceId)}` : ''}`),
  action: (workspaceId: string | null, action: string, value = '', location?: Location, changeAmountMinor?: number, negotiation?: NegotiationInput, rating?: number) =>
    post<WorkspaceState>('/api/actions', { workspaceId, action, value, location, changeAmountMinor, rating, ...negotiation }),
  switchWorkspace: (role: WorkspaceRole) => post<{ path: string }>('/api/auth/role', { role }),
  beginProfessional: () => post<{ path: string }>('/api/professional/onboarding', {}),
  logout: () => post<{ logoutUrl: string }>('/api/auth/logout', {}),
  professionalProfile: () => request<{ profile: ProfessionalProfile | null }>('/api/professional/profile'),
  saveProfessionalProfile: (profile: ProfessionalProfileInput) => post<{ profile: ProfessionalProfile }>('/api/professional/profile', profile),
  beginPayPalSellerOnboarding: () => post<{ actionUrl: string }>('/api/professional/paypal/onboarding', {}),
  paypalSellerStatus: (merchantIdInPayPal?: string) => request<PayPalPayoutSetupStatus>(`/api/professional/paypal/status${merchantIdInPayPal ? `?merchantIdInPayPal=${encodeURIComponent(merchantIdInPayPal)}` : ''}`),
  professionalAvailability: () => request<{ slots: AvailabilitySlot[] }>('/api/professional/availability'),
  addProfessionalAvailability: (slot: { startUtc: string; endUtc: string; timezone: string }) =>
    post<{ id: string; status: string }>('/api/professional/availability', slot),
  operatorQueue: () => request<{ cases: OperatorCase[]; professionalApplications: ProfessionalApplication[]; notificationDelivery: NotificationDeliveryHealth }>('/api/operator/queue'),
  reviewProfessional: (id: string, decision: 'save' | 'approve' | 'reject', checks: ProfessionalReviewChecks) =>
    post<{ status: string }>(`/api/operator/professionals/${encodeURIComponent(id)}/review`, { decision, checks }),
  resolveWorkCase: (workspaceId: string, kind: string, decision: string, rationale: string) =>
    post<{ status: string }>(`/api/operator/cases/${encodeURIComponent(workspaceId)}/resolve`, { kind, decision, rationale }),
  paypalStatus: (workspaceId: string | null) => request<{ status: string }>(`/api/paypal/status?workspaceId=${encodeURIComponent(workspaceId ?? '')}`),
  createPayPalOrder: (workspaceId: string | null) => post<{ approvalUrl: string }>(`/api/paypal/create-order?workspaceId=${encodeURIComponent(workspaceId ?? '')}`, {}),
  authorizePayPal: (orderId: string, workspaceId: string | null) => post<{ status: string }>(`/api/paypal/authorize?workspaceId=${encodeURIComponent(workspaceId ?? '')}`, { orderId }),
  capturePayPal: (workspaceId: string | null) => post<{ status: string; state: WorkspaceState }>(`/api/paypal/capture?workspaceId=${encodeURIComponent(workspaceId ?? '')}`, {}),
  reconcilePayPal: (workspaceId: string | null) => post<{ status: string; state: WorkspaceState }>(`/api/paypal/reconcile?workspaceId=${encodeURIComponent(workspaceId ?? '')}`, {}),
  uploadEvidence: (file: File, workspaceId: string | null) => {
    const body = new FormData();
    body.set('file', file);
    return request<WorkspaceState>(`/api/evidence?workspaceId=${encodeURIComponent(workspaceId ?? '')}`, { method: 'POST', body });
  },
  reviewEvidence: (workspaceId: string | null, assetIds: string[] = []) => post<WorkspaceState>(`/api/evidence/review?workspaceId=${encodeURIComponent(workspaceId ?? '')}`, { assetIds })
};
