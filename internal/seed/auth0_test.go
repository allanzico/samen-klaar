package seed

import (
	"context"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"net/http/httptest"
	"path/filepath"
	"strings"
	"testing"
)

func TestImportContractResumesAndDoesNotResetUsersOrSendEmails(t *testing.T) {
	output := t.TempDir()
	m := manifest{Version: version, Issuer: "https://tenant.example/", Files: []string{"batch.json"}}
	users := []authUser{{ID: "relay-seed-customer1", Email: "customer1@relay.com", PasswordHash: "test-hash", EmailVerified: true}}
	if err := writeJSON(filepath.Join(output, "batch.json"), users); err != nil {
		t.Fatal(err)
	}
	posts, gets := 0, 0
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.Header.Get("Authorization") != "Bearer test-token" {
			t.Error("missing management authorization")
		}
		switch r.URL.Path {
		case "/connections/con_test":
			fmt.Fprint(w, `{"id":"con_test","strategy":"auth0","enabled_clients":["samen-klaar-client"],"options":{"requires_username":true}}`)
		case "/connections/con_test/clients":
			if r.URL.Query().Get("from") == "next-page" {
				fmt.Fprint(w, `{"clients":[{"client_id":"samen-klaar-client"}]}`)
			} else {
				fmt.Fprint(w, `{"clients":[{"client_id":"other-client"}],"next":"next-page"}`)
			}
		case "/jobs/users-imports":
			posts++
			if posts == 1 {
				w.WriteHeader(http.StatusForbidden)
				fmt.Fprint(w, "test-token must not appear in errors")
				return
			}
			if err := r.ParseMultipartForm(1 << 20); err != nil {
				t.Error(err)
				w.WriteHeader(400)
				return
			}
			if r.FormValue("upsert") != "false" || r.FormValue("send_completion_email") != "false" || r.FormValue("connection_id") != "con_test" {
				t.Error("unsafe import settings")
			}
			file, _, err := r.FormFile("users")
			if err != nil {
				t.Error(err)
				w.WriteHeader(400)
				return
			}
			defer file.Close()
			var imported []authUser
			if err = json.NewDecoder(file).Decode(&imported); err != nil || len(imported) != 1 {
				t.Error("invalid payload")
			} else if imported[0].Username != "customer1" || imported[0].ID != "relay-seed-customer1" || imported[0].PasswordHash != "test-hash" {
				t.Error("identity/hash changed")
			}
			w.WriteHeader(http.StatusAccepted)
			fmt.Fprint(w, `{"id":"job_test","status":"pending"}`)
		case "/jobs/job_test":
			gets++
			fmt.Fprint(w, `{"id":"job_test","status":"completed","summary":{"inserted":1,"failed":0}}`)
		default:
			t.Error("unexpected path")
			w.WriteHeader(404)
		}
	}))
	defer server.Close()
	manager := management{server.URL, "test-token", server.Client()}
	err := importPrepared(context.Background(), manager, m, "samen-klaar-client", "con_test", output)
	if err == nil || strings.Contains(err.Error(), "test-token") {
		t.Fatal("rejection missing or token leaked")
	}
	if err = importPrepared(context.Background(), manager, m, "samen-klaar-client", "con_test", output); err != nil {
		t.Fatal(err)
	}
	if err = importPrepared(context.Background(), manager, m, "samen-klaar-client", "con_test", output); err != nil {
		t.Fatal(err)
	}
	if posts != 2 || gets != 1 {
		t.Fatalf("resubmitted completed job: posts=%d gets=%d", posts, gets)
	}
}

func TestImportUnknownPostOutcomeIsNotAutomaticallyResubmitted(t *testing.T) {
	output := t.TempDir()
	m := manifest{Version: version, Issuer: "https://tenant.example/", Files: []string{"batch.json"}}
	if err := writeJSON(filepath.Join(output, "batch.json"), []authUser{{ID: "relay-seed-customer1", Email: "customer1@relay.com"}}); err != nil {
		t.Fatal(err)
	}
	posts := 0
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if strings.HasSuffix(r.URL.Path, "/clients") {
			fmt.Fprint(w, `{"clients":[{"client_id":"samen-klaar-client"}]}`)
			return
		}
		if strings.HasPrefix(r.URL.Path, "/connections/") {
			fmt.Fprint(w, `{"id":"con_test","strategy":"auth0","enabled_clients":["samen-klaar-client"]}`)
			return
		}
		posts++
		_, _ = io.Copy(io.Discard, r.Body)
		w.WriteHeader(500)
	}))
	defer server.Close()
	manager := management{server.URL, "test-token", server.Client()}
	if err := importPrepared(context.Background(), manager, m, "samen-klaar-client", "con_test", output); err == nil {
		t.Fatal("failure accepted")
	}
	if err := importPrepared(context.Background(), manager, m, "samen-klaar-client", "con_test", output); err == nil || !strings.Contains(err.Error(), "unknown outcome") {
		t.Fatal("unknown outcome resubmitted")
	}
	if posts != 1 {
		t.Fatal("duplicate submission")
	}
}
