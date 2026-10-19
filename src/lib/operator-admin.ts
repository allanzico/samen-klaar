export type AdminMembership = { role: 'customer' | 'professional' | 'operator'; status: 'active' | 'pending' | 'disabled'; suspended: boolean };
export type AdminProfessional = { id: string; status: string; category: string; serviceName: string; serviceArea: string; travelRadiusKm: number };
export type AdminUser = {
  id: string; email: string; displayName: string; locale: string; issuer: string; subject: string;
  createdAt: string; lastSeenAt: string; memberships: AdminMembership[]; professional?: AdminProfessional;
  customerJobs: number; professionalJobs: number; activeSessions: number; sameEmailCount: number;
};
export type AdminEvent = { id: number; actorId: string; actorName: string; action: string; role: string; previousStatus: string; nextStatus: string; reason: string; affectedSessions: number; createdAt: string };
export type AdminUserDetail = { user: AdminUser; events: AdminEvent[] };
export type AdminUserPage = { users: AdminUser[]; total: number; offset: number; hasMore: boolean };
export type AdminUserAction = { action: 'suspend-role' | 'restore-role' | 'revoke-sessions'; role: string; expectedStatus: string; reason: string };
export type AdminUserFilter = { q: string; role: string; status: string; location: string; offset: string };
export type AdminLocation = { label: string; activeProfessionals: number; pendingProfessionals: number; totalProfessionals: number; requests: number; waitlistedRequests: number; categories: string[] };
export type AdminLocationPage = { locations: AdminLocation[]; total: number; offset: number; hasMore: boolean };
