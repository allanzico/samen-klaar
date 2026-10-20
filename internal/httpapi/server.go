package httpapi

import (
	"context"
	"errors"
	"io"
	"net"
	"net/http"
	"net/url"
	"strings"
	"sync"
	"sync/atomic"
	"time"

	"github.com/allanzico/samen-klaar/internal/adapters/paypal"
	"github.com/allanzico/samen-klaar/internal/identity"
	"github.com/allanzico/samen-klaar/internal/matching"
	"github.com/allanzico/samen-klaar/internal/workflow"
)

type ProviderConfig struct {
	AIProvider       string `json:"aiProvider"`
	AIModel          string `json:"aiModel"`
	EvidenceProvider string `json:"evidenceProvider"`
	PaymentProvider  string `json:"paymentProvider"`
}

type IntakeAI interface {
	matching.Selector
	Clarify(context.Context, string, string) (workflow.IntakeSuggestion, error)
	ReviewEvidence(context.Context, workflow.Job, []workflow.EvidenceBlob) (*workflow.EvidenceReview, error)
}
type EvidenceStorage interface {
	Put(context.Context, string, string, string, []byte) error
	Get(context.Context, string, string) (io.ReadCloser, string, error)
	Delete(context.Context, string, string)
}
type PaymentGateway interface {
	CreateOrder(context.Context, workflow.State) (string, string, error)
	Authorize(context.Context, workflow.State, string) error
	Capture(context.Context, workflow.State) error
	Reconcile(context.Context, workflow.State) (string, error)
	Status(context.Context, workflow.State) (string, error)
	CaptureID(context.Context, workflow.State) (string, error)
	VoidCurrent(context.Context, workflow.State) error
	VerifiedAuthorization(context.Context, workflow.State) (paypal.IntentRecord, error)
}

type RefundGateway interface {
	Refund(context.Context, workflow.State) (string, error)
}

type PayPalMarketplace interface {
	BeginSellerOnboarding(context.Context, string) (string, error)
	SellerOnboardingStatus(context.Context, string, string) (workflow.PayPalPayoutSetupStatus, error)
}

type PaymentWebhookReceiver interface {
	ReceiveWebhook(context.Context, http.Header, []byte) (bool, error)
}

type Dependencies struct {
	Locations     LocationSearch
	AppName       string
	WebDirectory  string
	Store         workflow.StateStore
	Auth          *identity.Auth
	Sessions      SessionResolver
	AppBaseURL    string
	Providers     ProviderConfig
	AI            IntakeAI
	Evidence      EvidenceStorage
	Payments      PaymentGateway
	Refunds       RefundGateway
	Marketplace   PayPalMarketplace
	Webhook       PaymentWebhookReceiver
	Professionals ProfessionalService
	Directory     ProfessionalDirectory
	Operators     OperatorService
	Agents        AgentStore
	Readiness     func(context.Context) error
	// ClientIPHeader names the header the edge proxy overwrites with the
	// connection peer (default X-Real-IP, as set by the bundled Caddy gateway).
	ClientIPHeader string
}

type Server struct {
	locations       LocationSearch
	locationKey     []byte
	appName         string
	webDirectory    string
	store           workflow.StateStore
	auth            *identity.Auth
	sessions        SessionResolver
	appOrigin       string
	clientIPHeader  string
	providers       ProviderConfig
	ai              IntakeAI
	r2              EvidenceStorage
	paypal          PaymentGateway
	refunds         RefundGateway
	marketplace     PayPalMarketplace
	webhook         PaymentWebhookReceiver
	professionals   ProfessionalService
	directory       ProfessionalDirectory
	operators       OperatorService
	agents          AgentStore
	agentLocks      sync.Map
	interpretations sync.Map // workspace|role|seq -> *workflow.ReplyIntent
	interpretFailed sync.Map // the same key -> time.Time of the last failed model call
	readiness       func(context.Context) error
	limiter         *rateLimiter
	draining        atomic.Bool
}

func New(deps Dependencies) *Server {
	appName := deps.AppName
	if appName == "" {
		appName = "Service"
	}
	origin := ""
	if parsed, err := url.Parse(deps.AppBaseURL); err == nil && parsed.Scheme != "" && parsed.Host != "" {
		origin = parsed.Scheme + "://" + parsed.Host
	}
	sessions := deps.Sessions
	if sessions == nil && deps.Auth != nil {
		sessions = deps.Auth
	}
	return &Server{locations: deps.Locations, locationKey: newLocationKey(), appName: appName, webDirectory: deps.WebDirectory, store: deps.Store, auth: deps.Auth, sessions: sessions, appOrigin: origin, providers: deps.Providers, ai: deps.AI, r2: deps.Evidence, paypal: deps.Payments, refunds: deps.Refunds, marketplace: deps.Marketplace, webhook: deps.Webhook, professionals: deps.Professionals, directory: deps.Directory, operators: deps.Operators, agents: deps.Agents, readiness: deps.Readiness, limiter: newRateLimiter(), clientIPHeader: clientIPHeaderName(deps.ClientIPHeader)}
}

func (s *Server) Handler() http.Handler {
	mux := http.NewServeMux()
	mux.HandleFunc("/api/health/live", func(w http.ResponseWriter, _ *http.Request) {
		writeJSON(w, map[string]any{"status": "ok"})
	})
	mux.HandleFunc("/api/health", s.readinessHandler)
	mux.HandleFunc("/api/health/ready", s.readinessHandler)
	mux.HandleFunc("/api/config", s.configHandler)
	mux.Handle("/api/locations", s.protect(false, identity.Customer, identity.Professional)(http.HandlerFunc(s.locationSearchHandler)))
	mux.Handle("/api/auth/login", s.limitByClientIP("auth-login", 0.1, 15, s.authHandler(func(w http.ResponseWriter, r *http.Request, auth *identity.Auth) { auth.Login(w, r) })))
	mux.Handle("/api/auth/callback", s.limitByClientIP("auth-callback", 0.5, 20, s.authHandler(func(w http.ResponseWriter, r *http.Request, auth *identity.Auth) { auth.Callback(w, r) })))
	mux.HandleFunc("/api/auth/session", s.sessionHandler)
	mux.Handle("/api/auth/role", s.protect(false, identity.Customer, identity.Professional, identity.Operator)(http.HandlerFunc(s.roleHandler)))
	mux.Handle("/api/professional/onboarding", s.protect(false, identity.Customer)(s.authHandler(func(w http.ResponseWriter, r *http.Request, auth *identity.Auth) { auth.BeginProfessional(w, r) })))
	mux.HandleFunc("/api/auth/logout", s.authHandler(func(w http.ResponseWriter, r *http.Request, auth *identity.Auth) { auth.Logout(w, r) }))
	mux.Handle("/api/coordination", s.protect(false, identity.Customer, identity.Professional)(http.HandlerFunc(s.coordinationHandler)))
	mux.Handle("/api/request-plan", s.protect(false, identity.Customer)(http.HandlerFunc(s.parentRequestHandler)))
	mux.Handle("/api/workspaces", s.protect(false, identity.Customer, identity.Professional)(http.HandlerFunc(s.workspacesHandler)))
	mux.Handle("/api/workspaces/{id}", s.protect(false, identity.Customer, identity.Professional)(http.HandlerFunc(s.workspaceHandler)))
	mux.Handle("/api/notifications", s.protect(false, identity.Customer, identity.Professional)(http.HandlerFunc(s.notificationListHandler)))
	mux.Handle("/api/notifications/read", s.protect(true, identity.Customer, identity.Professional)(http.HandlerFunc(s.notificationReadHandler)))
	mux.Handle("/api/state", s.protect(false, identity.Customer, identity.Professional)(http.HandlerFunc(s.stateHandler)))
	mux.Handle("/api/agent/mandate", s.protect(false, identity.Customer, identity.Professional)(http.HandlerFunc(s.agentMandateHandler)))
	mux.Handle("/api/agent/activity", s.protect(false, identity.Customer, identity.Professional)(http.HandlerFunc(s.agentActivityHandler)))
	mux.Handle("/api/agent/escalations", s.protect(false, identity.Customer, identity.Professional)(http.HandlerFunc(s.agentEscalationsHandler)))
	mux.Handle("/api/actions", s.protect(true, identity.Customer, identity.Professional)(http.HandlerFunc(s.actionHandler)))
	mux.Handle("/api/paypal/create-order", s.protect(true, identity.Customer)(http.HandlerFunc(s.paypalCreateOrderHandler)))
	mux.Handle("/api/paypal/authorize", s.protect(true, identity.Customer)(http.HandlerFunc(s.paypalAuthorizeHandler)))
	mux.Handle("/api/paypal/capture", s.protect(true, identity.Customer)(http.HandlerFunc(s.paypalCaptureHandler)))
	mux.Handle("/api/paypal/reconcile", s.protect(true, identity.Customer)(http.HandlerFunc(s.paypalReconcileHandler)))
	mux.Handle("/api/paypal/status", s.protect(false, identity.Customer)(http.HandlerFunc(s.paypalStatusHandler)))
	mux.HandleFunc("/api/paypal/webhook", s.paypalWebhookHandler)
	mux.Handle("/api/evidence", s.protect(true, identity.Professional)(http.HandlerFunc(s.evidenceUploadHandler)))
	mux.Handle("/api/evidence/{id}", s.protect(false, identity.Customer, identity.Professional)(http.HandlerFunc(s.evidenceGetHandler)))
	mux.Handle("/api/request-photos", s.protect(true, identity.Customer)(http.HandlerFunc(s.requestPhotoUploadHandler)))
	mux.Handle("/api/request-photos/{id}", s.protect(false, identity.Customer, identity.Professional)(http.HandlerFunc(s.requestPhotoGetHandler)))
	mux.Handle("/api/evidence/review", s.protect(true, identity.Customer)(http.HandlerFunc(s.evidenceReviewHandler)))
	mux.Handle("/api/professional/profile", s.protect(false, identity.Professional)(http.HandlerFunc(s.professionalProfileHandler)))
	mux.Handle("/api/professional/paypal/onboarding", s.protect(false, identity.Professional)(http.HandlerFunc(s.paypalSellerOnboardingHandler)))
	mux.Handle("/api/professional/paypal/status", s.protect(false, identity.Professional)(http.HandlerFunc(s.paypalSellerStatusHandler)))
	mux.Handle("/api/professional/schedule", s.protect(false, identity.Professional)(http.HandlerFunc(s.professionalScheduleHandler)))
	mux.Handle("/api/professional/schedule/preview", s.protect(false, identity.Professional)(http.HandlerFunc(s.professionalSchedulePreviewHandler)))
	mux.Handle("/api/professional/days-off", s.protect(false, identity.Professional)(http.HandlerFunc(s.professionalDaysOffHandler)))
	mux.Handle("/api/professional/days-off/", s.protect(false, identity.Professional)(http.HandlerFunc(s.professionalDaysOffHandler)))
	mux.Handle("/api/professional/days-off/preview", s.protect(false, identity.Professional)(http.HandlerFunc(s.professionalDaysOffPreviewHandler)))
	mux.Handle("/api/professional/availability/", s.protect(false, identity.Professional)(http.HandlerFunc(s.professionalAvailabilityMaintenanceHandler)))
	mux.Handle("/api/professional/availability", s.protect(false, identity.Professional)(http.HandlerFunc(s.professionalAvailabilityHandler)))
	mux.Handle("/api/operator/cases/{workspaceId}", s.protect(false, identity.Operator)(http.HandlerFunc(s.operatorCaseDetailHandler)))
	mux.Handle("/api/operator/cases/{workspaceId}/evidence/{id}", s.protect(false, identity.Operator)(http.HandlerFunc(s.operatorEvidenceHandler)))
	mux.Handle("/api/operator/evidence-policies", s.protect(false, identity.Operator)(http.HandlerFunc(s.operatorEvidencePolicyHandler)))
	mux.Handle("/api/professional/evidence-policy", s.protect(false, identity.Professional)(http.HandlerFunc(s.professionalEvidencePolicyHandler)))
	mux.Handle("/api/policies", s.protect(false, identity.Customer, identity.Professional, identity.Operator)(http.HandlerFunc(s.consumerPoliciesHandler)))
	mux.Handle("/api/operator/policies", s.protect(true, identity.Operator)(http.HandlerFunc(s.operatorPolicyPublishHandler)))
	mux.Handle("/api/support/messages", s.protect(false, identity.Customer, identity.Professional)(http.HandlerFunc(s.participantSupportHandler)))
	mux.Handle("/api/customer/profile", s.protect(false, identity.Customer)(http.HandlerFunc(s.customerProfileHandler)))
	mux.Handle("/api/account/locale", s.protect(false, identity.Customer, identity.Professional, identity.Operator)(http.HandlerFunc(s.accountLocaleHandler)))
	mux.Handle("/api/operator/queue", s.protect(false, identity.Operator)(http.HandlerFunc(s.operatorQueueHandler)))
	mux.Handle("/api/operator/users", s.protect(true, identity.Operator)(http.HandlerFunc(s.operatorUsersHandler)))
	mux.Handle("/api/operator/users/{id}", s.protect(true, identity.Operator)(http.HandlerFunc(s.operatorUserHandler)))
	mux.Handle("/api/operator/locations", s.protect(true, identity.Operator)(http.HandlerFunc(s.operatorLocationsHandler)))
	mux.Handle("/api/operator/professionals/{id}/review", s.protect(true, identity.Operator)(http.HandlerFunc(s.professionalReviewHandler)))
	mux.Handle("/api/operator/cases/{workspaceId}/resolve", s.protect(true, identity.Operator)(http.HandlerFunc(s.resolveWorkCaseHandler)))
	return s.securityHeaders(frontend(mux, s.webDirectory))
}

// securityHeaders keeps the browser protections in the application so they hold
// behind any proxy (the bundled Caddy gateway or a platform router).
func (s *Server) securityHeaders(next http.Handler) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		h := w.Header()
		h.Set("X-Content-Type-Options", "nosniff")
		h.Set("X-Frame-Options", "DENY")
		h.Set("Referrer-Policy", "strict-origin-when-cross-origin")
		h.Set("Content-Security-Policy", "frame-ancestors 'none'; base-uri 'self'; object-src 'none'")
		if strings.HasPrefix(s.appOrigin, "https://") {
			h.Set("Strict-Transport-Security", "max-age=31536000; includeSubDomains")
		}
		next.ServeHTTP(w, r)
	})
}

func (s *Server) paypalWebhookHandler(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodPost {
		http.Error(w, "method not allowed", http.StatusMethodNotAllowed)
		return
	}
	if s.webhook == nil {
		http.Error(w, "PayPal webhooks are not configured", http.StatusServiceUnavailable)
		return
	}
	r.Body = http.MaxBytesReader(w, r.Body, 1<<20)
	body, err := io.ReadAll(r.Body)
	if err != nil {
		http.Error(w, "invalid PayPal webhook request", http.StatusRequestEntityTooLarge)
		return
	}
	ctx, cancel := context.WithTimeout(r.Context(), 25*time.Second)
	defer cancel()
	valid, err := s.webhook.ReceiveWebhook(ctx, r.Header, body)
	if err != nil {
		http.Error(w, "PayPal webhook could not be verified or queued", http.StatusServiceUnavailable)
		return
	}
	if !valid {
		http.Error(w, "PayPal webhook signature is invalid", http.StatusUnauthorized)
		return
	}
	w.WriteHeader(http.StatusAccepted)
}

// BeginShutdown withdraws readiness before HTTP requests and workers drain.
func (s *Server) BeginShutdown() { s.draining.Store(true) }

func (s *Server) readinessHandler(w http.ResponseWriter, r *http.Request) {
	ready := !s.draining.Load() && s.store != nil && s.auth != nil && s.ai != nil && s.r2 != nil && s.paypal != nil && s.professionals != nil && s.directory != nil && s.operators != nil && s.readiness != nil
	if ready {
		ctx, cancel := context.WithTimeout(r.Context(), 2*time.Second)
		ready = s.readiness(ctx) == nil
		cancel()
	}
	status := "ready"
	w.Header().Set("Content-Type", "application/json")
	if !ready {
		status = "not_ready"
		w.WriteHeader(http.StatusServiceUnavailable)
	}
	writeJSON(w, map[string]any{"status": status, "ready": ready})
}

type SessionResolver interface {
	Principal(*http.Request) (identity.Principal, bool)
}

type RoutedStateStore interface {
	LoadFor(context.Context, string, string) (workflow.State, error)
	ApplyFor(context.Context, string, string, workflow.Action) (workflow.State, error)
}

type principalContextKey struct{}

func principalForRequest(r *http.Request) identity.Principal {
	principal, _ := r.Context().Value(principalContextKey{}).(identity.Principal)
	return principal
}

func (s *Server) protect(activeOnly bool, roles ...identity.Role) func(http.Handler) http.Handler {
	return func(next http.Handler) http.Handler {
		return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			if s.sessions == nil {
				http.Error(w, "account sign-in is not configured", http.StatusServiceUnavailable)
				return
			}
			principal, ok := s.sessions.Principal(r)
			if !ok {
				http.Error(w, "sign in to continue", http.StatusUnauthorized)
				return
			}
			allowed := false
			for _, role := range roles {
				if principal.ActiveRole == role {
					allowed = true
					break
				}
			}
			if !allowed {
				http.Error(w, "this workspace cannot access that resource", http.StatusForbidden)
				return
			}
			membership, found := principal.Membership(principal.ActiveRole)
			if !found || membership.Status == "disabled" || activeOnly && membership.Status != "active" {
				http.Error(w, "workspace access is not active", http.StatusForbidden)
				return
			}
			if r.Method != http.MethodGet && r.Method != http.MethodHead && r.Method != http.MethodOptions && s.appOrigin != "" && r.Header.Get("Origin") != s.appOrigin {
				http.Error(w, "request origin is not allowed", http.StatusForbidden)
				return
			}
			bucket, perSecond, burst := "write", 0.5, 60
			if r.Method == http.MethodGet || r.Method == http.MethodHead || r.Method == http.MethodOptions {
				bucket, perSecond, burst = "read", 2, 120
			}
			if !s.limiter.allow("account:"+principal.User.ID+":"+bucket, perSecond, burst, time.Now()) {
				w.Header().Set("Retry-After", "2")
				http.Error(w, "too many requests; try again shortly", http.StatusTooManyRequests)
				return
			}
			if r.URL.Path == "/api/evidence/review" && !s.limiter.allow("ai:"+principal.User.ID, 0.2, 20, time.Now()) {
				w.Header().Set("Retry-After", "5")
				http.Error(w, "Please wait a moment before trying again", http.StatusTooManyRequests)
				return
			}
			ctx := context.WithValue(r.Context(), principalContextKey{}, principal)
			next.ServeHTTP(w, r.WithContext(ctx))
		})
	}
}

func (s *Server) authHandler(handler func(http.ResponseWriter, *http.Request, *identity.Auth)) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		if s.auth == nil {
			http.Error(w, "account sign-in is not configured", http.StatusServiceUnavailable)
			return
		}
		if r.Method != http.MethodGet && s.appOrigin != "" && r.Header.Get("Origin") != s.appOrigin {
			http.Error(w, "request origin is not allowed", http.StatusForbidden)
			return
		}
		handler(w, r, s.auth)
	}
}

// limitByClientIP protects public authentication routes. The production
// gateway overwrites X-Real-IP from the connection peer before proxying, so
// client-supplied forwarding headers cannot choose the bucket.
func (s *Server) limitByClientIP(bucket string, perSecond float64, burst int, next http.Handler) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		ip := clientIP(r, s.clientIPHeader)
		if !s.limiter.allow("client-ip:"+bucket+":"+ip, perSecond, burst, time.Now()) {
			w.Header().Set("Retry-After", "2")
			http.Error(w, "too many requests; try again shortly", http.StatusTooManyRequests)
			return
		}
		next.ServeHTTP(w, r)
	})
}

func clientIPHeaderName(configured string) string {
	if configured = strings.TrimSpace(configured); configured != "" {
		return configured
	}
	return "X-Real-IP"
}

func clientIP(r *http.Request, header string) string {
	if ip := net.ParseIP(strings.TrimSpace(r.Header.Get(header))); ip != nil {
		return ip.String()
	}
	peer := r.RemoteAddr
	if host, _, err := net.SplitHostPort(peer); err == nil {
		peer = host
	}
	if ip := net.ParseIP(strings.TrimSpace(peer)); ip != nil {
		return ip.String()
	}
	return "unknown"
}

func (s *Server) sessionHandler(w http.ResponseWriter, r *http.Request) {
	if s.auth == nil {
		writeJSON(w, map[string]any{"configured": false, "authenticated": false})
		return
	}
	s.auth.SessionHandler(w, r)
}

func (s *Server) roleHandler(w http.ResponseWriter, r *http.Request) {
	if s.auth == nil {
		http.Error(w, "account sign-in is not configured", http.StatusServiceUnavailable)
		return
	}
	if s.appOrigin != "" && r.Header.Get("Origin") != s.appOrigin {
		http.Error(w, "request origin is not allowed", http.StatusForbidden)
		return
	}
	s.auth.ActivateRole(w, r)
}

func (s *Server) workspacesHandler(w http.ResponseWriter, r *http.Request) {
	store, ok := s.store.(workflow.WorkspaceStore)
	if !ok {
		http.Error(w, "request workspaces are not configured", http.StatusServiceUnavailable)
		return
	}
	principal := principalForRequest(r)
	ctx, cancel := context.WithTimeout(r.Context(), 5*time.Second)
	defer cancel()
	switch r.Method {
	case http.MethodGet:
		if pager, ok := s.store.(interface {
			ListWorkspacesPage(context.Context, string, string, string) ([]workflow.WorkspaceSummary, string, error)
		}); ok {
			items, nextCursor, err := pager.ListWorkspacesPage(ctx, principal.User.ID, string(principal.ActiveRole), r.URL.Query().Get("cursor"))
			if err != nil {
				http.Error(w, "request list could not be loaded", http.StatusServiceUnavailable)
				return
			}
			response := map[string]any{"workspaces": items, "nextCursor": nextCursor}
			if !s.addOpenRequests(ctx, w, principal, response) {
				return
			}
			writeJSON(w, response)
			return
		}
		items, err := store.ListWorkspaces(ctx, principal.User.ID, string(principal.ActiveRole))
		if err != nil {
			http.Error(w, "request list could not be loaded", http.StatusServiceUnavailable)
			return
		}
		response := map[string]any{"workspaces": items}
		if !s.addOpenRequests(ctx, w, principal, response) {
			return
		}
		writeJSON(w, response)
	case http.MethodPost:
		if principal.ActiveRole != identity.Customer {
			http.Error(w, "only customers can create service requests", http.StatusForbidden)
			return
		}
		state, err := store.CreateWorkspace(ctx, principal.User.ID)
		if errors.Is(err, workflow.ErrOpenRequestLimit) {
			http.Error(w, err.Error(), http.StatusConflict)
			return
		}
		if err != nil {
			http.Error(w, "request workspace could not be created", http.StatusServiceUnavailable)
			return
		}
		writeJSON(w, map[string]any{"workspace": state})
	default:
		http.Error(w, "method not allowed", http.StatusMethodNotAllowed)
	}
}

type State = workflow.State
type Action = workflow.Action
type Job = workflow.Job
type Evidence = workflow.Evidence
type EvidenceReview = workflow.EvidenceReview
type EvidenceBlob = workflow.EvidenceBlob
