package seed

import (
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"mime/multipart"
	"net/http"
	"net/url"
	"os"
	"path/filepath"
	"strings"
	"time"
)

type importJob struct {
	ID      string `json:"id"`
	Status  string `json:"status"`
	Summary struct {
		Failed   int `json:"failed"`
		Inserted int `json:"inserted"`
	} `json:"summary"`
}

type importState struct {
	Issuer     string               `json:"issuer"`
	Connection string               `json:"connection_id"`
	Jobs       map[string]importJob `json:"jobs"`
	Submitting string               `json:"submitting,omitempty"`
}

type management struct {
	base, token string
	client      *http.Client
}

type managementError struct{ status int }

func (e managementError) Error() string {
	return fmt.Sprintf("Auth0 Management API returned HTTP %d (check token expiry, permissions and connection)", e.status)
}

func (m management) request(ctx context.Context, method, path string, body io.Reader, contentType string, result any) error {
	req, err := http.NewRequestWithContext(ctx, method, m.base+path, body)
	if err != nil {
		return errors.New("invalid Auth0 Management API URL")
	}
	req.Header.Set("Authorization", "Bearer "+m.token)
	if contentType != "" {
		req.Header.Set("Content-Type", contentType)
	}
	response, err := m.client.Do(req)
	if err != nil {
		return errors.New("Auth0 Management API request failed")
	}
	defer response.Body.Close()
	if response.StatusCode < 200 || response.StatusCode >= 300 {
		return managementError{response.StatusCode}
	}
	if err := json.NewDecoder(io.LimitReader(response.Body, 2<<20)).Decode(result); err != nil {
		return errors.New("invalid Auth0 Management API response")
	}
	return nil
}

// Import sends password hashes through Auth0's bulk import endpoint. It never
// updates existing Auth0 users or sends completion emails. Job IDs are persisted
// before polling so an interrupted run resumes instead of resubmitting a batch.
func Import(ctx context.Context, issuer, clientID, token, connection, output string) error {
	if strings.TrimSpace(token) == "" || strings.TrimSpace(connection) == "" {
		return errors.New("add AUTH0_MANAGEMENT_TOKEN and AUTH0_CONNECTION_ID to .secrets/local/seed.env")
	}
	base, err := url.Parse(issuer)
	if err != nil || base.Scheme != "https" || base.Host == "" || base.User != nil {
		return errors.New("invalid AUTH0_ISSUER_URL")
	}
	var manifest manifest
	data, err := os.ReadFile(filepath.Join(output, "manifest.json"))
	if err != nil || json.Unmarshal(data, &manifest) != nil {
		return errors.New("run make seed-pros first")
	}
	issuer = strings.TrimRight(issuer, "/") + "/"
	if manifest.Issuer != issuer || manifest.Version != version {
		return errors.New("seed manifest belongs to a different Auth0 tenant or dataset")
	}
	m := management{strings.TrimRight(issuer, "/") + "/api/v2", token, &http.Client{Timeout: 60 * time.Second, CheckRedirect: func(*http.Request, []*http.Request) error { return http.ErrUseLastResponse }}}
	return importPrepared(ctx, m, manifest, clientID, connection, output)
}

func importPrepared(ctx context.Context, m management, manifest manifest, clientID, connection, output string) error {
	issuer := manifest.Issuer
	var err error
	var conn struct {
		ID       string `json:"id"`
		Strategy string `json:"strategy"`
		Options  struct {
			RequiresUsername bool `json:"requires_username"`
			CustomDatabase   bool `json:"enabledDatabaseCustomization"`
		} `json:"options"`
	}
	if err = m.request(ctx, http.MethodGet, "/connections/"+url.PathEscape(connection), nil, "", &conn); err != nil {
		return err
	}
	if conn.ID != connection || conn.Strategy != "auth0" || conn.Options.CustomDatabase {
		return errors.New("seed import requires a standard Auth0 database connection")
	}
	enabled, err := connectionEnabled(ctx, m, connection, clientID)
	if err != nil {
		return err
	}
	if !enabled {
		return errors.New("enable this database connection for the SamenKlaar login application before importing")
	}
	path := filepath.Join(output, "auth0-import-status.json")
	state := importState{Issuer: issuer, Connection: connection, Jobs: map[string]importJob{}}
	var data []byte
	if data, err = os.ReadFile(path); err == nil {
		if json.Unmarshal(data, &state) != nil || state.Issuer != issuer || state.Connection != connection || state.Jobs == nil {
			return errors.New("Auth0 import status does not match this tenant and connection")
		}
	} else if !os.IsNotExist(err) {
		return err
	}
	if state.Submitting != "" {
		return fmt.Errorf("an earlier submission for %s has an unknown outcome; recover its job ID from Auth0 Import/Export Users before retrying", state.Submitting)
	}
	inserted := 0
	for index, file := range manifest.Files {
		if filepath.Base(file) != file {
			return errors.New("invalid import filename")
		}
		data, err := os.ReadFile(filepath.Join(output, file))
		if err != nil {
			return err
		}
		if len(data) > 400000 {
			return errors.New("Auth0 import batch exceeds the prepared size limit")
		}
		var users []authUser
		if json.Unmarshal(data, &users) != nil || len(users) == 0 {
			return errors.New("invalid Auth0 import batch")
		}
		if conn.Options.RequiresUsername {
			for i := range users {
				users[i].Username = strings.SplitN(users[i].Email, "@", 2)[0]
			}
			data, err = json.Marshal(users)
			if err != nil {
				return err
			}
			if len(data) > 400000 {
				return errors.New("Auth0 username-enabled batch exceeds the size limit")
			}
		}
		job, exists := state.Jobs[file]
		if !exists {
			var buffer bytes.Buffer
			writer := multipart.NewWriter(&buffer)
			part, err := writer.CreateFormFile("users", file)
			if err != nil {
				return err
			}
			if _, err = part.Write(data); err != nil {
				return err
			}
			for key, value := range map[string]string{"connection_id": connection, "upsert": "false", "send_completion_email": "false", "external_id": fmt.Sprintf("%s-%03d", version, index+1)} {
				if err = writer.WriteField(key, value); err != nil {
					return err
				}
			}
			if err = writer.Close(); err != nil {
				return err
			}
			// Persist intent first. A lost POST response must not trigger an
			// automatic duplicate import with an unknowable prior outcome.
			state.Submitting = file
			if err = writeJSON(path, state); err != nil {
				return err
			}
			if err = m.request(ctx, http.MethodPost, "/jobs/users-imports", &buffer, writer.FormDataContentType(), &job); err != nil {
				var rejected managementError
				if errors.As(err, &rejected) && rejected.status >= 400 && rejected.status < 500 && rejected.status != 408 {
					state.Submitting = ""
					if saveErr := writeJSON(path, state); saveErr != nil {
						return saveErr
					}
				}
				return err
			}
			if job.ID == "" {
				return errors.New("Auth0 did not return an import job ID")
			}
			state.Jobs[file] = job
			state.Submitting = ""
			if err = writeJSON(path, state); err != nil {
				return err
			}
		}
		for job.Status != "completed" && job.Status != "failed" {
			if err = m.request(ctx, http.MethodGet, "/jobs/"+url.PathEscape(job.ID), nil, "", &job); err != nil {
				return err
			}
			state.Jobs[file] = job
			if err = writeJSON(path, state); err != nil {
				return err
			}
			if job.Status == "completed" || job.Status == "failed" {
				break
			}
			select {
			case <-ctx.Done():
				return ctx.Err()
			case <-time.After(5 * time.Second):
			}
		}
		if job.Status != "completed" || job.Summary.Failed > 0 || job.Summary.Inserted != len(users) {
			return fmt.Errorf("Auth0 batch %d requires inspection: status=%s inserted=%d failed=%d expected=%d; see job %s in Import/Export Users", index+1, job.Status, job.Summary.Inserted, job.Summary.Failed, len(users), job.ID)
		}
		inserted += job.Summary.Inserted
		fmt.Printf("Auth0 batch %d/%d verified: %d users inserted\n", index+1, len(manifest.Files), job.Summary.Inserted)
	}
	fmt.Printf("Verified Auth0 imports for %d test accounts. Password login uses the normal Auth0 flow.\n", inserted)
	return nil
}

// Auth0 retired enabled_clients on the connection object in 2026. The
// dedicated endpoint uses cursor pagination and remains read-only.
func connectionEnabled(ctx context.Context, m management, connection, clientID string) (bool, error) {
	cursor := ""
	for {
		query := url.Values{"take": []string{"1000"}}
		if cursor != "" {
			query.Set("from", cursor)
		}
		var page struct {
			Clients []struct {
				ID string `json:"client_id"`
			} `json:"clients"`
			Next string `json:"next"`
		}
		if err := m.request(ctx, http.MethodGet, "/connections/"+url.PathEscape(connection)+"/clients?"+query.Encode(), nil, "", &page); err != nil {
			return false, err
		}
		for _, client := range page.Clients {
			if client.ID == clientID {
				return true, nil
			}
		}
		if page.Next == "" {
			return false, nil
		}
		if page.Next == cursor {
			return false, errors.New("Auth0 connection pagination did not advance")
		}
		cursor = page.Next
	}
}
