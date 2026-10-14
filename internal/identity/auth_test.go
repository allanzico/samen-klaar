package identity

import (
	"context"
	"errors"
	"net/http"
	"net/http/httptest"
	"net/url"
	"strings"
	"testing"
	"time"

	"golang.org/x/oauth2"
)

type transactionRepository struct {
	Repository
	transactions map[string]Transaction
	consumed     int
}

func (repo *transactionRepository) CreateOIDCTransaction(_ context.Context, txn Transaction) error {
	repo.transactions[txn.StateHash] = txn
	return nil
}

func (repo *transactionRepository) ConsumeOIDCTransaction(_ context.Context, hash string) (Transaction, error) {
	repo.consumed++
	txn, ok := repo.transactions[hash]
	if !ok {
		return Transaction{}, errors.New("transaction missing")
	}
	delete(repo.transactions, hash)
	return txn, nil
}

func testAuth() (*Auth, *transactionRepository) {
	repo := &transactionRepository{transactions: make(map[string]Transaction)}
	return &Auth{
		config: Config{AppBaseURL: "http://localhost:8090"},
		repo:   repo,
		oauth: oauth2.Config{
			ClientID: "test-client", RedirectURL: "http://localhost:8090/api/auth/callback",
			Endpoint: oauth2.Endpoint{AuthURL: "https://identity.example/authorize"},
		},
	}, repo
}

func startLogin(t *testing.T, auth *Auth) (string, *http.Cookie) {
	t.Helper()
	response := httptest.NewRecorder()
	auth.Login(response, httptest.NewRequest(http.MethodGet, auth.config.AppBaseURL+"/api/auth/login", nil))
	if response.Code != http.StatusFound {
		t.Fatalf("login status = %d", response.Code)
	}
	target, err := url.Parse(response.Header().Get("Location"))
	if err != nil {
		t.Fatal(err)
	}
	state := target.Query().Get("state")
	cookies := response.Result().Cookies()
	if state == "" || len(cookies) != 1 {
		t.Fatal("login must issue one browser cookie and a state parameter")
	}
	cookie := cookies[0]
	if !cookie.HttpOnly || cookie.SameSite != http.SameSiteLaxMode || cookie.Path != "/api/auth" || cookie.MaxAge <= 0 {
		t.Fatal("login cookie must have bounded lifetime and protect the auth path")
	}
	if cookie.Secure != auth.config.SecureCookie {
		t.Fatal("login must preserve the environment's secure-cookie policy")
	}
	if target.Query().Get("code_challenge_method") != "S256" || target.Query().Get("code_challenge") == "" || target.Query().Get("nonce") == "" {
		t.Fatal("login must retain PKCE and nonce protection")
	}
	return state, cookie
}

// A provider cancellation validates browser state without exchanging a real
// authorization code or creating an account/session.
func cancelLogin(auth *Auth, state string, cookies ...*http.Cookie) *httptest.ResponseRecorder {
	request := httptest.NewRequest(http.MethodGet, auth.config.AppBaseURL+"/api/auth/callback?error=access_denied&state="+url.QueryEscape(state), nil)
	for _, cookie := range cookies {
		request.AddCookie(cookie)
	}
	response := httptest.NewRecorder()
	auth.Callback(response, request)
	return response
}

func TestLoginCanonicalizesHostBeforeCreatingTransaction(t *testing.T) {
	auth, repo := testAuth()
	response := httptest.NewRecorder()
	auth.Login(response, httptest.NewRequest(http.MethodGet, "http://127.0.0.1:8090/api/auth/login?role=professional", nil))
	if response.Code != http.StatusFound || response.Header().Get("Location") != "http://localhost:8090/api/auth/login" {
		t.Fatal("an alias must redirect to the configured callback host and use the single login endpoint")
	}
	if len(response.Result().Cookies()) != 0 || len(repo.transactions) != 0 {
		t.Fatal("must not strand a login cookie or transaction on an alias host")
	}
}

func TestConcurrentLoginsDoNotOverwriteEachOthersState(t *testing.T) {
	auth, repo := testAuth()
	first, firstCookie := startLogin(t, auth)
	second, secondCookie := startLogin(t, auth)
	if firstCookie.Name == secondCookie.Name {
		t.Fatal("separate attempts must have distinct cookies")
	}
	for _, state := range []string{second, first} {
		response := cancelLogin(auth, state, firstCookie, secondCookie)
		if response.Code != http.StatusFound || response.Header().Get("Location") != "http://localhost:8090/?auth_error=cancelled" {
			t.Fatal("each attempt must remain independently valid")
		}
		cookies := response.Result().Cookies()
		if len(cookies) != 1 || cookies[0].Name != transactionCookie(state) || cookies[0].MaxAge != -1 {
			t.Fatal("only the completed attempt's cookie may be cleared")
		}
	}
	if repo.consumed != 2 || len(repo.transactions) != 0 {
		t.Fatal("each completed attempt must be consumed exactly once")
	}
	replay := cancelLogin(auth, first, firstCookie)
	if replay.Header().Get("Location") != "http://localhost:8090/?auth_error=expired" {
		t.Fatal("a consumed transaction must not be replayable")
	}
}

func TestInvalidStateDoesNotConsumeOrClearAnotherLogin(t *testing.T) {
	for _, kind := range []string{"missing-cookie", "mismatch", "missing-state", "oversized-state"} {
		t.Run(kind, func(t *testing.T) {
			auth, repo := testAuth()
			state, cookie := startLogin(t, auth)
			var cookies []*http.Cookie
			switch kind {
			case "mismatch":
				copy := *cookie
				copy.Value = "other-attempt"
				cookies = append(cookies, &copy)
			case "missing-state":
				state = ""
				cookies = append(cookies, cookie)
			case "oversized-state":
				state = strings.Repeat("x", 257)
				cookies = append(cookies, cookie)
			}
			response := cancelLogin(auth, state, cookies...)
			if response.Code != http.StatusFound || response.Header().Get("Location") != "http://localhost:8090/?auth_error=state" {
				t.Fatal("invalid state must return to the retry page")
			}
			if repo.consumed != 0 || len(repo.transactions) != 1 || len(response.Result().Cookies()) != 0 {
				t.Fatal("invalid callbacks must not disturb a legitimate sign-in attempt")
			}
			if response.Header().Get("Cache-Control") != "no-store" {
				t.Fatal("callback responses must not be cached")
			}
		})
	}
}

func TestExpiredTransactionIsRejected(t *testing.T) {
	auth, repo := testAuth()
	state, cookie := startLogin(t, auth)
	txn := repo.transactions[hashToken(state)]
	txn.ExpiresAt = time.Now().Add(-time.Minute)
	repo.transactions[hashToken(state)] = txn
	response := cancelLogin(auth, state, cookie)
	if response.Header().Get("Location") != "http://localhost:8090/?auth_error=expired" {
		t.Fatal("expired transactions must return to the retry page")
	}
}

func TestInFlightLegacyLoginAndSecureCookiePolicy(t *testing.T) {
	auth, _ := testAuth()
	auth.config.AppBaseURL = "https://samen-klaar.example"
	auth.config.SecureCookie = true
	state, cookie := startLogin(t, auth)
	cookie.Name = stateCookie
	response := cancelLogin(auth, state, cookie)
	if response.Header().Get("Location") != "https://samen-klaar.example/?auth_error=cancelled" {
		t.Fatal("a valid login started before deployment must be able to complete")
	}
	cookies := response.Result().Cookies()
	if len(cookies) != 1 || cookies[0].Name != stateCookie || !cookies[0].Secure {
		t.Fatal("only the completed legacy cookie may be cleared, retaining Secure")
	}
}

func TestLoginParametersCannotRequestAnotherRole(t *testing.T) {
	for _, role := range []string{"professional", "operator", "unknown"} {
		auth, repo := testAuth()
		response := httptest.NewRecorder()
		auth.Login(response, httptest.NewRequest(http.MethodGet, auth.config.AppBaseURL+"/api/auth/login?role="+role, nil))
		if response.Code != http.StatusFound {
			t.Fatal("unified login failed")
		}
		for _, txn := range repo.transactions {
			if txn.Role != Customer {
				t.Fatal("login URL controlled an account grant")
			}
		}
	}
}

type workspaceRepository struct {
	Repository
	role   Role
	status string
}

func (r *workspaceRepository) PreferredRole(context.Context, string) (Role, error) {
	return r.role, nil
}
func (r *workspaceRepository) EnsureMembership(_ context.Context, _ string, role Role) (Membership, error) {
	return Membership{Role: role, Status: r.status}, nil
}
func TestLoginRestoresPreferredWorkspace(t *testing.T) {
	for _, role := range []Role{Customer, Professional, Operator} {
		auth := &Auth{repo: &workspaceRepository{role: role, status: "active"}}
		chosen, membership, err := auth.loginWorkspace(context.Background(), "same-account")
		if err != nil || chosen != role || membership.Role != role {
			t.Fatal("login did not restore persisted role")
		}
	}
}
