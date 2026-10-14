export type WorkspaceRole = 'customer' | 'professional' | 'operator';
export type AccountMembership = { role: WorkspaceRole; status: 'pending' | 'active' | 'disabled' };
export type AuthSession = {
  configured: boolean;
  authenticated: boolean;
  account?: { id: string; email?: string; displayName?: string };
  activeRole?: WorkspaceRole;
  memberships?: AccountMembership[];
  expiresAt?: string;
};
export type ProfessionalProfile = {
  id: string;
  displayName: string;
  category: string;
  serviceName: string;
  serviceDescription: string;
  serviceArea: string;
  latitude: number;
  longitude: number;
  currency: string;
  priceMinor: number;
  calloutMinor: number;
  durationMinutes: number;
  materialsPolicy: string;
  exclusions: string[];
  status: 'pending' | 'active' | 'rejected' | 'disabled';
};
export type AvailabilitySlot = { slotId: string; startsAt: string; startUtc: string; endUtc: string; timezone: string; status: string };
export type ProfessionalProfileInput = Omit<ProfessionalProfile, 'id' | 'status'>;
export type ProfessionalReviewChecks = {
  identityBusiness: 'pending' | 'verified';
  paypalOnboarding: 'pending' | 'verified';
  qualification: 'pending' | 'verified' | 'not_required';
  insurance: 'pending' | 'verified' | 'not_required';
  qualificationRationale: string;
  insuranceRationale: string;
};
export type PayPalPayoutSetupStatus = {
  status: string;
  connected: boolean;
  paymentsReceivable: boolean;
  primaryEmailConfirmed: boolean;
};
export type ProfessionalApplication = { profile: ProfessionalProfile; email?: string; checks: ProfessionalReviewChecks };
export type OperatorCase = { id: string; accountId: string; workspaceId: string; kind: string; description: string; reportedAt?: string; status?: string; category?: string; detail?: string; urgency?: string; location?: Location; jobId?: string; jobTitle?: string; paid?: boolean };
export type NotificationDeliveryHealth = { failed: number; unconfigured: number; stalled: number };
export type CancellationReview = { status: string; requestedBy: string; reason?: string; requestedAt: string; resolution?: string; resolvedAt?: string };
export type DisputeCase = { status: string; reportedBy: string; reason: string; reportedAt: string; resolution?: string; resolvedAt?: string };
export type AppointmentConfirmation = { status: string; dueAt?: string; respondedAt?: string; appointment: Appointment };

export type QuoteLine = { label: string; amountMinor: number; basis: string };
export type Evidence = {
  assetId?: string;
  contentType?: string;
  label: string;
  description: string;
  kind: string;
};
export type ScopeChange = { status: string; description: string; amountMinor: number; proposedFrom: number };
export type ScopeSnapshot = { version: number; totalMinor: number; lines: QuoteLine[] };
export type EvidenceFinding = { scopeItem: string; evidenceIds: string[]; coverage: string; observation: string };
export type EvidenceReview = { summary: string; coverage: EvidenceFinding[]; limitations: string; model: string; reviewedAt: string };
export type JobReview = { rating: number; comment?: string; createdAt: string; professionalResponse?: string; respondedAt?: string };
export type EventItem = { text: string; kind: string; when: string };
export type UserNotification = { id: number; eventType: string; workspaceId: string; createdAt: string; readAt?: string };

export type Appointment = { slotId: string; startsAt: string; timezone: string; startUtc: string; endUtc: string };
export type NegotiationInput = { priceAmountMinor?: number; proposedAppointment?: Partial<Appointment>; proposalId?: string };
export type PriceProposal = { id: string; status: string; actor: string; amountMinor: number; note: string; scopeVersion: number };
export type DateProposal = { id: string; status: string; actor: string; note: string; appointment: Appointment };
export type Job = {
  priceProposal?: PriceProposal;
  dateProposal?: DateProposal;
  dispute?: DisputeCase | null;
  appointmentConfirmation?: AppointmentConfirmation | null;
  id: string;
  professionalProfileId?: string;
  location?: Location | null;
  serviceAddress?: string;
  title: string;
  serviceDescription?: string;
  customer: string;
  professional: string;
  description: string;
  category: string;
  scopeVersion: number;
  scopeHistory: ScopeSnapshot[];
  quote: {
    currency: string;
    lines: QuoteLine[];
    totalMinor: number;
    materialsPolicy: string;
    exclusions: string[];
    durationMinutes: number;
    expires: string;
  };
  quoteApproved: boolean;
  needsClarification: boolean;
  clarificationQuestion: string;
  clarificationAnswer: string;
  intakeRoute?: string;
  appointment: Appointment | null;
  workStarted: boolean;
  evidenceSubmitted: boolean;
  detailsRequested: boolean;
  detailAdded: boolean;
  completionApproved: boolean;
  paymentApprovedVersion: number;
  paid: boolean;
  paymentRefunded?: boolean;
  paymentTransactionId?: string;
  refundTransactionId?: string;
  evidence: Evidence[];
  evidenceReview?: EvidenceReview | null;
  review?: JobReview | null;
  change: ScopeChange | null;
  timeline: EventItem[];
};

export type Location = { label: string; latitude: number; longitude: number; source: string };
export type Intake = {
  step: string;
  description: string;
  category: string;
  route: string;
  question: string;
  options: string[] | null;
  messages: { role: string; text: string; senderName?: string; threadProfileId?: string }[];
  detail: string;
  urgency: string;
  budget: string;
  location: Location | null;
  matches: { serviceName: string; serviceDescription: string; fitReason: string; currency: string; totalMinor: number; durationMinutes: number; materialsPolicy: string; exclusions: string[]; id: string; name: string; category: string; distanceKm: number; area: string; earliestAppointment?: Appointment }[];
  needsReview: boolean;
  waitlisted?: boolean;
};

export type WorkspaceState = {
  cancelled?: boolean;
  cancelledBy?: string;
	cancellationReview?: CancellationReview | null;
	workspaceId: string;
  intake?: Intake;
  job: Job;
  availableSlots: Appointment[];
};

export type WorkspaceSummary = {
	id: string;
	jobId?: string;
	title: string;
	status: string;
  statusLabel?: string;
	role: WorkspaceRole;
	updatedAt: string;
};

export type ProviderConfig = {
  appName: string;
  aiProvider: string;
  aiModel: string;
  evidenceProvider: string;
  paymentProvider: string;
};

export type AppState = {
  serverState: WorkspaceState | null;
  workspaces: WorkspaceSummary[];
  workspaceCursor: string | null;
  workspaceId: string | null;
  session: AuthSession | null;
  providerConfig: ProviderConfig | null;
  professionalProfile: ProfessionalProfile | null;
  brandName: string;
  role: WorkspaceRole | null;
  loading: boolean;
  pending: boolean;
  error: string;
  composeMode: 'message' | 'price' | 'date';
  draft: string;
  draftRestored: boolean;
  draftSaved: boolean;
  newConversation: boolean;
  clarificationAnswer: string;
  paypalStatus: string;
  evidenceFile: File | null;
};

export type AppActions = {
  initialize: () => Promise<void>;
  loadState: () => Promise<void>;
  refresh: () => Promise<void>;
  loadMoreWorkspaces: () => Promise<void>;
  selectRequest: (id: string) => Promise<void>;
  createRequest: () => Promise<void>;
  act: (action: string, value?: string, location?: Location, changeAmountMinor?: number, negotiation?: NegotiationInput, rating?: number) => Promise<boolean | void>;
  switchWorkspace: (role: WorkspaceRole) => Promise<void>;
  beginProfessional: () => Promise<void>;
  logout: () => Promise<void>;
  sendMessage: (event?: SubmitEvent, text?: string) => Promise<void>;
  chooseAnswer: (text: string) => void;
  focusComposer: () => void;
  openProposal: (mode: 'price' | 'date') => void;
  closeProposal: () => void;
  persistDraft: () => void;
  selectEvidenceFile: (event: Event) => void;
  uploadEvidence: () => Promise<void>;
  reviewEvidence: (assetIds?: string[]) => Promise<void>;
  beginPayPalCheckout: () => Promise<void>;
  capturePayPal: () => Promise<void>;
  reconcilePayPal: () => Promise<void>;
  dismissError: () => void;
};
