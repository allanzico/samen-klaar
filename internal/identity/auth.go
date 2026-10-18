package identity

import (
	"context"
	"crypto/rand"
	"crypto/sha256"
	"crypto/subtle"
	"encoding/base64"
	"encoding/hex"
	"encoding/json"
	"errors"
	"fmt"
	"log/slog"
	"net/http"
	"net/url"
	"strings"
	"time"

	"github.com/coreos/go-oidc/v3/oidc"
	"golang.org/x/oauth2"
)

type Role string

const (
	Customer     Role = "customer"
	Professional Role = "professional"
	Operator     Role = "operator"
)

type User struct {
	ID          string `json:"id"`
	Locale      string `json:"locale,omitempty"`
	Email       string `json:"email,omitempty"`
	DisplayName string `json:"displayName,omitempty"`
}

type Membership struct {
	Role   Role   `json:"role"`
	Status string `json:"status"`
}

type Principal struct {
	User        User         `json:"user"`
	ActiveRole  Role         `json:"activeRole"`
	Memberships []Membership `json:"memberships"`
	ExpiresAt   time.Time    `json:"expiresAt"`
}

func (p Principal) Membership(role Role) (Membership, bool) {
	for _, membership := range p.Memberships {
		if membership.Role == role {
			return membership, true
		}
	}
	return Membership{}, false
}

type Transaction struct {
	StateHash    string
	Nonce        string
	CodeVerifier string
	Role         Role
	ExpiresAt    time.Time
}

type Repository interface {
	UpsertIdentity(context.Context, string, string, string, string) (User, error)
	EnsureMembership(context.Context, string, Role) (Membership, error)
	CreateSession(context.Context, string, string, Role, time.Time) error
	Session(context.Context, string) (Principal, error)
	ActivateRole(context.Context, string, Role) (Principal, error)
	PreferredRole(context.Context, string) (Role, error)
	DeleteSession(context.Context, string) error
	CreateOIDCTransaction(context.Context, Transaction) error
	ConsumeOIDCTransaction(context.Context, string) (Transaction, error)
}

type Config struct {
	IssuerURL    string
	ClientID     string
	ClientSecret string
	RedirectURL  string
	AppBaseURL   string
	SecureCookie bool
}

type Auth struct {
	config   Config
	repo     Repository
	provider *oidc.Provider
	verifier *oidc.IDTokenVerifier
	oauth    oauth2.Config
}

const (
	sessionCookie  = "app_session"
	stateCookie    = "oidc_state"
	sessionTTL     = 30 * 24 * time.Hour
	transactionTTL = 10 * time.Minute
)

func New(ctx context.Context, config Config, repo Repository) (*Auth, error) {
	if repo == nil {
		return nil, errors.New("identity repository is required")
	}
	for name, value := range map[string]string{
		"AUTH0_ISSUER_URL": config.IssuerURL, "AUTH0_CLIENT_ID": config.ClientID,
		"AUTH0_CLIENT_SECRET": config.ClientSecret, "AUTH0_REDIRECT_URI": config.RedirectURL,
		"APP_BASE_URL": config.AppBaseURL,
	} {
		if strings.TrimSpace(value) == "" {
			return nil, fmt.Errorf("%s is required to enable sign-in", name)
		}
	}
	issuer, err := url.Parse(config.IssuerURL)
	if err != nil || issuer.Scheme != "https" || issuer.Host == "" || issuer.RawQuery != "" || issuer.Fragment != "" {
		return nil, errors.New("AUTH0_ISSUER_URL must be an HTTPS issuer URL without a query or fragment")
	}
	redirect, err := url.Parse(config.RedirectURL)
	if err != nil || redirect.Scheme == "" || redirect.Host == "" {
		return nil, errors.New("AUTH0_REDIRECT_URI must be an absolute URL")
	}
	base, err := url.Parse(config.AppBaseURL)
	if err != nil || (base.Scheme != "https" && base.Scheme != "http") || base.Host == "" || base.Path != "" && base.Path != "/" || base.RawQuery != "" || base.Fragment != "" {
		return nil, errors.New("APP_BASE_URL must be an origin URL")
	}
	if config.SecureCookie && base.Scheme != "https" {
		return nil, errors.New("APP_BASE_URL must use HTTPS when secure cookies are enabled")
	}

	config.IssuerURL = strings.TrimRight(config.IssuerURL, "/") + "/"
	provider, err := oidc.NewProvider(ctx, config.IssuerURL)
	if err != nil {
		return nil, fmt.Errorf("discover OIDC provider: %w", err)
	}
	return &Auth{
		config: config, repo: repo, provider: provider,
		verifier: provider.Verifier(&oidc.Config{ClientID: config.ClientID}),
		oauth: oauth2.Config{
			ClientID: config.ClientID, ClientSecret: config.ClientSecret, RedirectURL: config.RedirectURL,
			Endpoint: provider.Endpoint(), Scopes: []string{oidc.ScopeOpenID, "profile", "email"},
		},
	}, nil
}

func (a *Auth) Login(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodGet {
		http.Error(w, "method not allowed", http.StatusMethodNotAllowed)
		return
	}
	// The requested destination only selects an existing membership. It never
	// provisions operator access.
	role := Customer
	if r.URL.Query().Get("workspace") == string(Operator) {
		role = Operator
	}
	w.Header().Set("Cache-Control", "no-store")
	w.Header().Set("Referrer-Policy", "no-referrer")
	// Cookies are scoped to a host, not a port. Start on the callback's origin
	// so aliases such as 127.0.0.1 cannot strand a cookie on another host.
	base, _ := url.Parse(a.config.AppBaseURL)
	if !strings.EqualFold(r.Host, base.Host) {
		login := strings.TrimRight(a.config.AppBaseURL, "/") + "/api/auth/login"
		if role == Operator {
			login += "?workspace=operator"
		}
		http.Redirect(w, r, login, http.StatusFound)
		return
	}
	state, err := randomToken(32)
	if err != nil {
		http.Error(w, "could not start sign-in", http.StatusInternalServerError)
		return
	}
	nonce, err := randomToken(32)
	if err != nil {
		http.Error(w, "could not start sign-in", http.StatusInternalServerError)
		return
	}
	verifier, err := randomToken(48)
	if err != nil {
		http.Error(w, "could not start sign-in", http.StatusInternalServerError)
		return
	}
	expires := time.Now().Add(transactionTTL)
	if err := a.repo.CreateOIDCTransaction(r.Context(), Transaction{
		StateHash: hashToken(state), Nonce: nonce, CodeVerifier: verifier, Role: role, ExpiresAt: expires,
	}); err != nil {
		http.Error(w, "could not start sign-in", http.StatusServiceUnavailable)
		return
	}
	http.SetCookie(w, &http.Cookie{Name: transactionCookie(state), Value: state, Path: "/api/auth", HttpOnly: true, Secure: a.config.SecureCookie, SameSite: http.SameSiteLaxMode, Expires: expires, MaxAge: int(time.Until(expires).Seconds())})
	url := a.oauth.AuthCodeURL(state,
		oauth2.SetAuthURLParam("nonce", nonce),
		oauth2.S256ChallengeOption(verifier),
	)
	http.Redirect(w, r, url, http.StatusFound)
}

func (a *Auth) Callback(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodGet {
		http.Error(w, "method not allowed", http.StatusMethodNotAllowed)
		return
	}
	w.Header().Set("Cache-Control", "no-store")
	w.Header().Set("Referrer-Policy", "no-referrer")
	queryState := r.URL.Query().Get("state")
	if queryState == "" || len(queryState) > 256 {
		a.loginFailed(w, r, "state", "invalid_state")
		return
	}
	cookie, cookieErr := r.Cookie(transactionCookie(queryState))
	if cookieErr != nil {
		// Allow a valid login started just before this release to finish.
		cookie, cookieErr = r.Cookie(stateCookie)
	}
	if cookieErr != nil {
		a.loginFailed(w, r, "state", "missing_state_cookie")
		return
	}
	if subtle.ConstantTimeCompare([]byte(cookie.Value), []byte(queryState)) != 1 {
		a.loginFailed(w, r, "state", "state_mismatch")
		return
	}
	clearCookie(w, cookie.Name, "/api/auth", a.config.SecureCookie)
	txn, err := a.repo.ConsumeOIDCTransaction(r.Context(), hashToken(queryState))
	if err != nil || !txn.ExpiresAt.After(time.Now()) {
		a.loginFailed(w, r, "expired", "missing_or_expired_transaction")
		return
	}
	if providerError := r.URL.Query().Get("error"); providerError != "" {
		http.Redirect(w, r, loginErrorURL(a.config.AppBaseURL, "cancelled"), http.StatusFound)
		return
	}
	code := r.URL.Query().Get("code")
	if code == "" {
		a.loginFailed(w, r, "failed", "missing_authorization_code")
		return
	}
	token, err := a.oauth.Exchange(r.Context(), code, oauth2.VerifierOption(txn.CodeVerifier))
	if err != nil {
		a.loginFailed(w, r, "failed", "code_exchange_failed")
		return
	}
	rawIDToken, ok := token.Extra("id_token").(string)
	if !ok || rawIDToken == "" {
		a.loginFailed(w, r, "failed", "missing_identity_token")
		return
	}
	idToken, err := a.verifier.Verify(r.Context(), rawIDToken)
	if err != nil {
		a.loginFailed(w, r, "failed", "invalid_identity_token")
		return
	}
	if idToken.Nonce == "" || subtle.ConstantTimeCompare([]byte(idToken.Nonce), []byte(txn.Nonce)) != 1 {
		a.loginFailed(w, r, "failed", "nonce_mismatch")
		return
	}
	var claims struct {
		Subject     string `json:"sub"`
		Email       string `json:"email"`
		DisplayName string `json:"name"`
	}
	if err := idToken.Claims(&claims); err != nil || claims.Subject == "" {
		a.loginFailed(w, r, "failed", "incomplete_identity")
		return
	}
	user, err := a.repo.UpsertIdentity(r.Context(), a.config.IssuerURL, claims.Subject, claims.Email, claims.DisplayName)
	if err != nil {
		a.loginFailed(w, r, "unavailable", "identity_persistence_failed")
		return
	}
	membership, err := a.repo.EnsureMembership(r.Context(), user.ID, Customer)
	if err != nil || membership.Status == "disabled" {
		a.loginFailed(w, r, "access", "membership_denied")
		return
	}
	role, membership, operatorDenied, err := a.loginWorkspace(r.Context(), user.ID, txn.Role)
	if err != nil {
		a.loginFailed(w, r, "unavailable", "workspace_preference_unavailable")
		return
	}
	session, err := randomToken(32)
	if err != nil {
		a.loginFailed(w, r, "unavailable", "session_generation_failed")
		return
	}
	expires := time.Now().Add(sessionTTL)
	if err := a.repo.CreateSession(r.Context(), hashToken(session), user.ID, role, expires); err != nil {
		a.loginFailed(w, r, "unavailable", "session_persistence_failed")
		return
	}
	http.SetCookie(w, &http.Cookie{Name: sessionCookie, Value: session, Path: "/", HttpOnly: true, Secure: a.config.SecureCookie, SameSite: http.SameSiteLaxMode, Expires: expires, MaxAge: int(sessionTTL.Seconds())})
	if operatorDenied {
		http.Redirect(w, r, a.config.AppBaseURL+"/operator?auth_error=access", http.StatusFound)
		return
	}
	http.Redirect(w, r, a.config.AppBaseURL+rolePath(role, membership.Status), http.StatusFound)
}

func (a *Auth) loginWorkspace(ctx context.Context, accountID string, requested Role) (Role, Membership, bool, error) {
	role, err := a.repo.PreferredRole(ctx, accountID)
	if err != nil {
		return "", Membership{}, false, err
	}
	membership, err := a.repo.EnsureMembership(ctx, accountID, role)
	if err != nil {
		return "", Membership{}, false, err
	}
	if requested == Operator {
		operator, err := a.repo.EnsureMembership(ctx, accountID, Operator)
		if err == nil && operator.Status == "active" {
			return Operator, operator, false, nil
		}
		return role, membership, true, nil
	}
	return role, membership, false, nil
}

// BeginProfessional adds only pending professional access to the same identity.
// Activation of provider actions remains subject to professional approval.
func (a *Auth) BeginProfessional(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodPost {
		http.Error(w, "method not allowed", http.StatusMethodNotAllowed)
		return
	}
	principal, ok := a.Principal(r)
	if !ok {
		http.Error(w, "sign in is required", http.StatusUnauthorized)
		return
	}
	membership, err := a.repo.EnsureMembership(r.Context(), principal.User.ID, Professional)
	if err != nil || membership.Status == "disabled" {
		http.Error(w, "professional onboarding is unavailable for this account", http.StatusForbidden)
		return
	}
	cookie, _ := r.Cookie(sessionCookie)
	principal, err = a.repo.ActivateRole(r.Context(), hashToken(cookie.Value), Professional)
	if err != nil {
		http.Error(w, "professional workspace could not be opened", http.StatusServiceUnavailable)
		return
	}
	writeIdentityJSON(w, map[string]any{"path": rolePath(Professional, membership.Status)})
}

func (a *Auth) Principal(r *http.Request) (Principal, bool) {
	cookie, err := r.Cookie(sessionCookie)
	if err != nil || cookie.Value == "" {
		return Principal{}, false
	}
	principal, err := a.repo.Session(r.Context(), hashToken(cookie.Value))
	if err != nil {
		return Principal{}, false
	}
	membership, ok := principal.Membership(principal.ActiveRole)
	if !ok || membership.Status == "disabled" {
		return Principal{}, false
	}
	return principal, true
}

func (a *Auth) SessionHandler(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodGet {
		http.Error(w, "method not allowed", http.StatusMethodNotAllowed)
		return
	}
	principal, ok := a.Principal(r)
	if !ok {
		writeIdentityJSON(w, map[string]any{"configured": true, "authenticated": false})
		return
	}
	writeIdentityJSON(w, map[string]any{"configured": true, "authenticated": true, "account": principal.User, "activeRole": principal.ActiveRole, "memberships": principal.Memberships, "expiresAt": principal.ExpiresAt})
}

func (a *Auth) ActivateRole(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodPost {
		http.Error(w, "method not allowed", http.StatusMethodNotAllowed)
		return
	}
	var input struct {
		Role Role `json:"role"`
	}
	r.Body = http.MaxBytesReader(w, r.Body, 1024)
	if json.NewDecoder(r.Body).Decode(&input) != nil {
		http.Error(w, "invalid workspace selection", http.StatusBadRequest)
		return
	}
	cookie, err := r.Cookie(sessionCookie)
	if err != nil || cookie.Value == "" {
		http.Error(w, "sign in is required", http.StatusUnauthorized)
		return
	}
	principal, err := a.repo.ActivateRole(r.Context(), hashToken(cookie.Value), input.Role)
	if err != nil {
		http.Error(w, "this account cannot access that workspace", http.StatusForbidden)
		return
	}
	membership, _ := principal.Membership(principal.ActiveRole)
	writeIdentityJSON(w, map[string]any{"activeRole": principal.ActiveRole, "status": membership.Status, "path": rolePath(principal.ActiveRole, membership.Status)})
}

func (a *Auth) Logout(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodPost {
		http.Error(w, "method not allowed", http.StatusMethodNotAllowed)
		return
	}
	if cookie, err := r.Cookie(sessionCookie); err == nil && cookie.Value != "" {
		_ = a.repo.DeleteSession(r.Context(), hashToken(cookie.Value))
	}
	clearCookie(w, sessionCookie, "/", a.config.SecureCookie)
	logout, _ := url.Parse(strings.TrimRight(a.config.IssuerURL, "/") + "/v2/logout")
	values := logout.Query()
	values.Set("client_id", a.config.ClientID)
	values.Set("returnTo", a.config.AppBaseURL)
	logout.RawQuery = values.Encode()
	writeIdentityJSON(w, map[string]string{"logoutUrl": logout.String()})
}

func (a *Auth) Configured() bool { return a != nil }

func rolePath(role Role, status string) string {
	if role == Professional && status == "pending" {
		return "/professional/pending"
	}
	return "/" + string(role)
}

func transactionCookie(state string) string {
	return stateCookie + "_" + hashToken(state)
}

func (a *Auth) loginFailed(w http.ResponseWriter, r *http.Request, code, reason string) {
	slog.Warn("sign-in rejected", "reason", reason)
	http.Redirect(w, r, loginErrorURL(a.config.AppBaseURL, code), http.StatusFound)
}

func loginErrorURL(base, code string) string {
	return strings.TrimRight(base, "/") + "/?auth_error=" + url.QueryEscape(code)
}

func randomToken(bytes int) (string, error) {
	value := make([]byte, bytes)
	if _, err := rand.Read(value); err != nil {
		return "", err
	}
	return base64.RawURLEncoding.EncodeToString(value), nil
}

func hashToken(value string) string {
	sum := sha256.Sum256([]byte(value))
	return hex.EncodeToString(sum[:])
}

func clearCookie(w http.ResponseWriter, name, path string, secure bool) {
	http.SetCookie(w, &http.Cookie{Name: name, Value: "", Path: path, HttpOnly: true, Secure: secure, SameSite: http.SameSiteLaxMode, MaxAge: -1, Expires: time.Unix(1, 0)})
}

func writeIdentityJSON(w http.ResponseWriter, value any) {
	w.Header().Set("Content-Type", "application/json; charset=utf-8")
	w.Header().Set("Cache-Control", "no-store")
	if err := json.NewEncoder(w).Encode(value); err != nil {
		http.Error(w, "could not encode response", http.StatusInternalServerError)
	}
}
