export type WorkspaceRole = 'customer' | 'professional' | 'operator';
export type AccountMembership = { role: WorkspaceRole; status: 'pending' | 'active' | 'disabled' };
export type AuthSession = {
  configured: boolean;
  authenticated: boolean;
  account?: { id: string; email?: string; displayName?: string; locale?: 'en' | 'nl' };
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
  baseLocation?: Location | null;
  travelRadiusKm?: number;
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
  platformFeeReady?: boolean;
  status: string;
  connected: boolean;
  paymentsReceivable: boolean;
  primaryEmailConfirmed: boolean;
};
export type ProfessionalApplication = { profile: ProfessionalProfile; email?: string; checks: ProfessionalReviewChecks };
export type OperatorCase = { id: string; accountId: string; workspaceId: string; kind: string; description: string; reportedAt?: string; status?: string; category?: string; detail?: string; urgency?: string; location?: Location; jobId?: string; jobTitle?: string; paid?: boolean; reasonCode?: string; responsible?: string };
export type NotificationDeliveryHealth = { failed: number; unconfigured: number; stalled: number };
export type CancellationReview = { kind?: string; status: string; requestedBy: string; reason?: string; requestedAt: string; resolution?: string; resolvedAt?: string };
export type DisputeCase = { status: string; reportedBy: string; reason: string; reportedAt: string; resolution?: string; resolvedAt?: string };
export type AppointmentConfirmation = { status: string; dueAt?: string; respondedAt?: string; appointment: Appointment };

export type QuoteLine = { label: string; amountMinor: number; basis: string };
export type EvidenceRequirement = {id:string;label:string;kind:'photo'|'document'|'note'|'file'};
export type WorkItem = {id:string;description:string;kind:'photo'|'document'|'note'|'file';addedInScope:number};
export type Evidence = {
  scopeVersion?: number;
  completionRound?: number;
  workItemIds?: string[];
  assetId?: string;
  contentType?: string;
  label: string;
  description: string;
  kind: string;
};
export type ScopeChange = { status: string; description: string; amountMinor: number; proposedFrom: number };
export type ScopeSnapshot = { version: number; totalMinor: number; lines: QuoteLine[] };
export type EvidenceFinding = { scopeItem: string; evidenceIds: string[]; coverage: string; observation: string };
export type EvidenceReview = { summary: string; coverage: EvidenceFinding[]; limitations: string; reviewedAt: string };
export type JobReview = { rating: number; comment?: string; createdAt: string; professionalResponse?: string; respondedAt?: string };
export type EventItem = { text: string; kind: string; when: string };
export type UserNotification = { id: number; eventType: string; workspaceId: string; createdAt: string; readAt?: string };

export type Appointment = { slotId: string; startsAt: string; timezone: string; startUtc: string; endUtc: string };
export type NegotiationInput = { policyKind?: string; policyVersion?: string; workItemIds?: string[]; priceAmountMinor?: number; proposedAppointment?: Partial<Appointment>; proposalId?: string; choiceId?: string };
export type PriceProposal = { id: string; status: string; actor: string; amountMinor: number; note: string; scopeVersion: number; professionalReviewRequired?: boolean };
export type DateProposal = { id: string; status: string; actor: string; note: string; appointment: Appointment };
export type BookingAutonomy = { mode: 'automatic' | 'manual'; version: number; scopeVersion: number; amountMinor: number; currency: string; professionalAccountId: string; professionalProfileId: string; decidedAt: string };
export type Job = {
  platformFeeBasisPoints?: number;
  completionCheck?: EvidenceReview | null;
  completionRevision?: boolean;
  bookingAutonomyRequired?: boolean;
  bookingAutonomy?: BookingAutonomy;
  completionApprovedByAgent?: boolean;
  paymentApprovedByAgent?: boolean;
  professionalAccountId?: string;
  workItems?: WorkItem[];
  policyRequirements?: PolicyRequirement[];
  policyAcknowledgments?: { kind: string; version: string; scopeVersion: number; acknowledgedAt: string }[];
  completionApprovedAt?: string;
  paymentApprovedAt?: string;
  detailAddedAt?: string;
  completionRound?:number;
  evidenceResolutions?:{workItemId:string;scopeVersion:number;completionRound:number;rationale:string;resolvedAt:string}[];
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
  paymentAuthorizedVersion?: number;
  paymentAuthorizationTiming?: 'after_completion' | 'at_booking';
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

export type Location = { label: string; latitude: number; longitude: number; source: string; id?: string; kind?: string; city?: string; postcode?: string; country?: string; neighborhood?: string; bounds?: number[]; address?: string; selection?: string };
export type Intake = {
  matchClarifications?: number;
  detailAttempts?: number;
  locale?:string; optionIds?:string[];
  step: string;
  description: string;
  category: string;
  route: string;
  question: string;
  options: string[] | null;
  messages: { seq?: number; role: string; text: string; senderName?: string; threadProfileId?: string; audience?: string; choice?: string; agent?: string; code?: string; params?: Record<string, string> }[];
  detail: string;
  urgency: string;
  budget: string;
  location: Location | null;
  matches: { serviceName: string; serviceDescription: string; fitReason: string; currency: string; totalMinor: number; durationMinutes: number; materialsPolicy: string; exclusions: string[]; id: string; name: string; category: string; distanceKm: number; distanceKnown?: boolean; distanceApproximate?: boolean; area: string; earliestAppointment?: Appointment }[];
  needsReview: boolean;
  waitlisted?: boolean;
};

export type RequestPhoto = { assetId: string; contentType: string; label: string; addedAt?: string };

export type AgentChoice = { id: string; label: string };
export type AgentDialog = { rejectedDates?: string[]; stage?: string; awaiting: boolean; asked?: number; pending?: string; pendingId?: string; choices?: AgentChoice[] };
export type WorkspaceState = {
  completionAutomation?: 'checking' | 'needs_review' | 'releasing';
  agentDialog?: AgentDialog;
  proDialog?: AgentDialog;
  requestPhotos?: RequestPhoto[];
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
  counterpart?: string;
  category?: string;
  totalMinor?: number;
  currency?: string;
  visitStartUtc?: string;
  visitEndUtc?: string;
  visitTimezone?: string;
  confirmDueAt?: string;
  // The customer may delete it, or the professional may take it off their list. The server checks again.
  canDelete?: boolean;
};

export type ProfessionalDashboard = {
  balances: { currency: string; receivedMinor: number; platformFeesMinor?: number; awaitingPaymentMinor: number }[];
  upcomingVisits: number;
  completedJobs: number;
  awaitingConfirmation: number;
  asOf: string;
};

export type ProviderConfig = {
  appName: string;
  locationSearchAvailable?: boolean;
  automaticReleaseAvailable?: boolean;
  guidedReviewAvailable: boolean;
  evidenceUploadsAvailable: boolean;
  paymentsAvailable: boolean;
  testPayments: boolean;
};

export type AppState = {
  customerPage: 'home' | 'requests' | 'detail' | 'profile';
  serverState: WorkspaceState | null;
  workspaces: WorkspaceSummary[];
  workspaceCursor: string | null;
  // The customer's requests that are not yet approved, cancelled, started or finished, and the cap on them.
  openRequests: number | null;
  openRequestLimit: number;
  workspaceId: string | null;
  session: AuthSession | null;
  providerConfig: ProviderConfig | null;
  professionalProfile: ProfessionalProfile | null;
  brandName: string;
  role: WorkspaceRole | null;
  loading: boolean;
  pending: boolean;
  error: string;
  refreshError?: string;
  composeMode: 'message' | 'price' | 'date';
  // Who a customer's free text goes to once a visit is booked: the professional, or their own assistant.
  messageTo: 'professional' | 'assistant';
  draft: string;
  draftRestored: boolean;
  draftSaved: boolean;
  newConversation: boolean;
  clarificationAnswer: string;
  paypalStatus: string;
  evidenceFile: File | null;
  reviewingEvidence?: boolean;
  checkingCompletion?: boolean;
};

export type AppActions = {
  initialize: () => Promise<void>;
  showHome: () => void;
  showRequests: () => void;
  showProfile: () => void;
  showJobs: () => void;
  restoreLocation: () => Promise<void>;
  startRequest: (description: string) => Promise<void>;
  loadState: () => Promise<void>;
  refresh: () => Promise<void>;
  loadMoreWorkspaces: () => Promise<void>;
  selectRequest: (id: string) => Promise<void>;
  createRequest: () => Promise<void>;
  deleteRequest: (id: string) => Promise<boolean>;
  act: (action: string, value?: string, location?: Location, changeAmountMinor?: number, negotiation?: NegotiationInput, rating?: number, paymentAuthorizationTiming?: 'after_completion' | 'at_booking') => Promise<boolean | void>;
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
  uploadEvidence: (workItemIds?: string[]) => Promise<void>;
  uploadRequestPhoto: (file: File) => Promise<boolean>;
  reviewEvidence: (assetIds?: string[]) => Promise<void>;
  beginPayPalCheckout: () => Promise<void>;
  capturePayPal: () => Promise<void>;
  reconcilePayPal: () => Promise<void>;
  dismissError: () => void;
};

export type PolicyRequirement = { kind: string; version: string; requiresAcknowledgment: boolean };
export type ConsumerPolicy = { kind: 'terms' | 'cancellation' | 'withdrawal' | 'early_start'; version: string; locale: 'en' | 'nl'; title: string; body: string; requiresAcknowledgment: boolean; reviewReference?: string; publishedAt?: string };
export type CoordinationTask = {
  id: string; workspaceId: string; jobId: string; scopeVersion: number; round: number; trigger: 'completion-note' | 'post-booking-scope'; recipientRole: string;
  status: 'queued' | 'awaiting_reply' | 'queued_summary' | 'summary_ready' | 'operator_review' | 'completed' | 'obsolete';
  question: string; reply: string; summary: string; proposedAction: string; outcome: string; revision: number; createdAt: string; updatedAt: string;
};
export type CaseNote = { id: number; recipientRole?: string; note: string; createdAt: string };
export type CaseDecision = { actor: string; action: string; scopeVersion: number; at: string };
export type OperatorCaseDetail = {
  case: OperatorCase; state: WorkspaceState; assignedTo?: string; status: string; outcome?: string;
  notes: CaseNote[]; decisions: CaseDecision[]; paymentStatus: string;
};
export type ServiceEvidencePolicy = { profileId: string; requirements: EvidenceRequirement[]; proposed?: EvidenceRequirement[]; status: string; reviewedAt?: string };

// Agents act for a person only inside the mandate they set; everything else comes back as a question.
export type AgentMandate = {
  enabled: boolean; version?: number; weekdays: number[]; startHour: number; endHour: number; preferredHour?: number; timezone?: string;
  maxTotalMinor?: number; maxDaysAhead?: number; serviceAddress?: string; autoApproveExtraMinor?: number; counterPercent?: number;
  floorPercent?: number; autoConfirmVisits?: boolean; autoAcceptDates?: boolean; acceptOutsideHours?: boolean; autoApproveWork?: boolean; bookWithoutAsking?: boolean;
};
export type AgentActivity = { id: number; role: string; code: string; action?: string; params?: Record<string, string>; outcome: 'applied' | 'rejected'; error?: string; createdAt: string };
export type AgentEscalation = { id: number; workspaceId: string; role: string; code: string; params?: Record<string, string>; createdAt: string };

export type CustomerProfile = { displayName: string; email: string; phone: string; location?: Location | null };
