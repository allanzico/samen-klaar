package ai

import (
	"context"
	"encoding/json"
	"io"
	"net/http"
	"strings"
	"testing"

	core "github.com/allanzico/samen-klaar/internal/ai"
)

type roundTripFunc func(*http.Request) (*http.Response, error)

func (fn roundTripFunc) RoundTrip(request *http.Request) (*http.Response, error) {
	return fn(request)
}

func TestGeminiRetriesTemporaryUnavailableResponse(t *testing.T) {
	attempts := 0
	client := &http.Client{Transport: roundTripFunc(func(request *http.Request) (*http.Response, error) {
		attempts++
		status, body := http.StatusServiceUnavailable, `{"error":{"code":503,"message":"temporarily unavailable","status":"UNAVAILABLE"}}`
		if attempts > 1 {
			status, body = http.StatusOK, `{"candidates":[{"content":{"role":"model","parts":[{"text":"{\"ok\":\"yes\"}"}]}}]}`
		}
		return &http.Response{StatusCode: status, Header: http.Header{"Content-Type": []string{"application/json"}}, Body: io.NopCloser(strings.NewReader(body)), Request: request}, nil
	})}
	model, err := newGeminiWithHTTPClient(context.Background(), "test-key", "test-model", client)
	if err != nil {
		t.Fatal(err)
	}
	result, err := model.GenerateJSON(context.Background(), core.GenerationRequest{
		Prompt: "Return ok", MaxOutputTokens: 32,
		Schema: map[string]any{"type": "object", "properties": map[string]any{"ok": map[string]any{"type": "string"}}},
	})
	if err != nil || string(result) != `{"ok":"yes"}` || attempts != 2 {
		t.Fatalf("temporary 503 should recover on retry: attempts=%d result=%q err=%v", attempts, result, err)
	}
}

func TestGeminiDoesNotRetryCredentialFailure(t *testing.T) {
	attempts := 0
	client := &http.Client{Transport: roundTripFunc(func(request *http.Request) (*http.Response, error) {
		attempts++
		return &http.Response{
			StatusCode: http.StatusForbidden,
			Header:     http.Header{"Content-Type": []string{"application/json"}},
			Body:       io.NopCloser(strings.NewReader(`{"error":{"code":403,"message":"forbidden","status":"PERMISSION_DENIED"}}`)),
			Request:    request,
		}, nil
	})}
	model, err := newGeminiWithHTTPClient(context.Background(), "test-key", "test-model", client)
	if err != nil {
		t.Fatal(err)
	}
	_, err = model.GenerateJSON(context.Background(), core.GenerationRequest{Prompt: "Return ok", MaxOutputTokens: 32})
	if err == nil || attempts != 1 {
		t.Fatalf("credential failure should stop immediately: attempts=%d err=%v", attempts, err)
	}
}

func TestNewValidatesEffort(t *testing.T) {
	if _, err := New(context.Background(), "gemini", "test-key", "test-model", "low"); err == nil {
		t.Fatal("effort with a non-claude provider should be rejected")
	}
	if _, err := New(context.Background(), "claude", "test-key", "test-model", "turbo"); err == nil {
		t.Fatal("unknown effort level should be rejected")
	}
	if _, err := New(context.Background(), "claude", "test-key", "test-model", " High "); err != nil {
		t.Fatalf("valid effort should be accepted: %v", err)
	}
}

func TestAnthropicSendsEffortOnlyWhenSet(t *testing.T) {
	for _, effort := range []string{"", "low"} {
		var sent struct {
			OutputConfig map[string]any `json:"output_config"`
		}
		model := &anthropicModel{key: "test-key", model: "test-model", effort: effort, client: &http.Client{Transport: roundTripFunc(func(request *http.Request) (*http.Response, error) {
			if err := json.NewDecoder(request.Body).Decode(&sent); err != nil {
				t.Fatal(err)
			}
			return &http.Response{StatusCode: http.StatusOK, Body: io.NopCloser(strings.NewReader(`{"content":[{"type":"text","text":"{\"ok\":\"yes\"}"}]}`)), Request: request}, nil
		})}}
		if _, err := model.GenerateJSON(context.Background(), core.GenerationRequest{Prompt: "Return ok", MaxOutputTokens: 32}); err != nil {
			t.Fatal(err)
		}
		got, present := sent.OutputConfig["effort"]
		if effort == "" && present || effort != "" && got != effort {
			t.Fatalf("effort %q: output_config = %v", effort, sent.OutputConfig)
		}
	}
}
