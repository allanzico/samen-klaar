package ai

import (
	"bytes"
	"context"
	"encoding/base64"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"net/http"
	"strings"
	"time"

	core "github.com/allanzico/samen-klaar/internal/ai"
	"google.golang.org/genai"
)

type constructor func(context.Context, string, string) (core.ModelClient, error)

var providers = map[string]constructor{
	"gemini":    newGemini,
	"openai":    newOpenAI,
	"anthropic": newAnthropic,
	"claude":    newAnthropic,
}

func New(ctx context.Context, provider, key, model string) (*core.Coordinator, error) {
	provider = strings.ToLower(strings.TrimSpace(provider))
	if provider == "" {
		provider = "gemini"
	}
	key = strings.TrimSpace(key)
	model = strings.TrimSpace(model)
	if key == "" {
		return nil, errors.New("AI_PROVIDER_KEY is required")
	}
	if model == "" {
		return nil, errors.New("AI_MODEL is required")
	}
	create, ok := providers[provider]
	if !ok {
		return nil, fmt.Errorf("unsupported AI_PROVIDER %q; supported providers are gemini, openai, claude, and anthropic", provider)
	}
	client, err := create(ctx, key, model)
	if err != nil {
		return nil, err
	}
	return core.New(provider, model, client), nil
}

type geminiModel struct {
	client *genai.Client
	model  string
}

func newGemini(ctx context.Context, key, model string) (core.ModelClient, error) {
	return newGeminiWithHTTPClient(ctx, key, model, nil)
}

func newGeminiWithHTTPClient(ctx context.Context, key, model string, httpClient *http.Client) (core.ModelClient, error) {
	attempts, initialDelay, maxDelay, jitter := int32(3), 0.5, 2.0, 0.25
	client, err := genai.NewClient(ctx, &genai.ClientConfig{
		APIKey: key, Backend: genai.BackendGeminiAPI, HTTPClient: httpClient,
		HTTPOptions: genai.HTTPOptions{RetryOptions: &genai.HTTPRetryOptions{
			Attempts: &attempts, InitialDelay: &initialDelay, MaxDelay: &maxDelay, Jitter: &jitter,
			HTTPStatusCodes: []int32{408, 429, 500, 502, 503, 504},
		}},
	})
	if err != nil {
		return nil, errors.New("could not initialize Gemini provider")
	}
	return &geminiModel{client: client, model: model}, nil
}

func (g *geminiModel) GenerateJSON(ctx context.Context, request core.GenerationRequest) ([]byte, error) {
	parts := make([]*genai.Part, 0, len(request.Attachments)+1)
	parts = append(parts, genai.NewPartFromText(request.Prompt))
	for _, attachment := range request.Attachments {
		parts = append(parts, genai.NewPartFromBytes(attachment.Data, attachment.ContentType))
	}
	config := &genai.GenerateContentConfig{
		SystemInstruction:  &genai.Content{Parts: []*genai.Part{{Text: request.System}}},
		ResponseMIMEType:   "application/json",
		ResponseJsonSchema: request.Schema,
		MaxOutputTokens:    int32(request.MaxOutputTokens),
	}
	content := genai.NewContentFromParts(parts, genai.RoleUser)
	result, err := g.client.Models.GenerateContent(ctx, g.model, []*genai.Content{content}, config)
	if err != nil {
		var apiError genai.APIError
		if errors.As(err, &apiError) {
			return nil, fmt.Errorf("AI provider returned HTTP %d", apiError.Code)
		}
		return nil, errors.New("AI provider request did not return a response")
	}
	text := strings.TrimSpace(result.Text())
	if text == "" {
		return nil, errors.New("Gemini returned no structured content")
	}
	return []byte(text), nil
}

type openAIModel struct {
	key    string
	model  string
	client *http.Client
}

func newOpenAI(_ context.Context, key, model string) (core.ModelClient, error) {
	return &openAIModel{key: key, model: model, client: &http.Client{Timeout: 75 * time.Second}}, nil
}

func (o *openAIModel) GenerateJSON(ctx context.Context, request core.GenerationRequest) ([]byte, error) {
	content := []map[string]any{{"type": "input_text", "text": request.Prompt}}
	for index, attachment := range request.Attachments {
		encoded := base64.StdEncoding.EncodeToString(attachment.Data)
		if attachment.ContentType == "application/pdf" {
			content = append(content, map[string]any{
				"type": "input_file", "filename": fmt.Sprintf("evidence-%d.pdf", index+1), "file_data": "data:application/pdf;base64," + encoded,
			})
		} else {
			content = append(content, map[string]any{
				"type": "input_image", "image_url": "data:" + attachment.ContentType + ";base64," + encoded, "detail": "auto",
			})
		}
	}
	payload := map[string]any{
		"model":             o.model,
		"instructions":      request.System,
		"input":             []map[string]any{{"role": "user", "content": content}},
		"max_output_tokens": request.MaxOutputTokens,
		"text": map[string]any{"format": map[string]any{
			"type": "json_schema", "name": "provider_response", "strict": true, "schema": request.Schema,
		}},
	}
	data, err := postJSON(ctx, o.client, "https://api.openai.com/v1/responses", o.key, "", "", payload)
	if err != nil {
		return nil, err
	}
	var result struct {
		Output []struct {
			Content []struct {
				Type string `json:"type"`
				Text string `json:"text"`
			} `json:"content"`
		} `json:"output"`
	}
	if err := json.Unmarshal(data, &result); err != nil {
		return nil, errors.New("OpenAI returned an invalid response")
	}
	for _, item := range result.Output {
		for _, part := range item.Content {
			if part.Type == "output_text" && strings.TrimSpace(part.Text) != "" {
				return []byte(part.Text), nil
			}
		}
	}
	return nil, errors.New("OpenAI returned no structured content")
}

type anthropicModel struct {
	key    string
	model  string
	client *http.Client
}

func newAnthropic(_ context.Context, key, model string) (core.ModelClient, error) {
	return &anthropicModel{key: key, model: model, client: &http.Client{Timeout: 75 * time.Second}}, nil
}

func (a *anthropicModel) GenerateJSON(ctx context.Context, request core.GenerationRequest) ([]byte, error) {
	content := []map[string]any{{"type": "text", "text": request.Prompt}}
	for _, attachment := range request.Attachments {
		source := map[string]any{
			"type": "base64", "media_type": attachment.ContentType, "data": base64.StdEncoding.EncodeToString(attachment.Data),
		}
		if attachment.ContentType == "application/pdf" {
			content = append(content, map[string]any{"type": "document", "source": source})
		} else {
			content = append(content, map[string]any{"type": "image", "source": source})
		}
	}
	payload := map[string]any{
		"model":         a.model,
		"max_tokens":    request.MaxOutputTokens,
		"system":        request.System,
		"messages":      []map[string]any{{"role": "user", "content": content}},
		"output_config": map[string]any{"format": map[string]any{"type": "json_schema", "schema": request.Schema}},
	}
	data, err := postJSON(ctx, a.client, "https://api.anthropic.com/v1/messages", a.key, "anthropic-version", "2023-06-01", payload)
	if err != nil {
		return nil, err
	}
	var result struct {
		Content []struct {
			Type string `json:"type"`
			Text string `json:"text"`
		} `json:"content"`
	}
	if err := json.Unmarshal(data, &result); err != nil {
		return nil, errors.New("Anthropic returned an invalid response")
	}
	for _, block := range result.Content {
		if block.Type == "text" && strings.TrimSpace(block.Text) != "" {
			return []byte(block.Text), nil
		}
	}
	return nil, errors.New("Anthropic returned no structured content")
}

func postJSON(ctx context.Context, client *http.Client, endpoint, key, extraHeader, extraValue string, payload any) ([]byte, error) {
	encoded, err := json.Marshal(payload)
	if err != nil {
		return nil, errors.New("could not encode AI request")
	}
	req, err := http.NewRequestWithContext(ctx, http.MethodPost, endpoint, bytes.NewReader(encoded))
	if err != nil {
		return nil, errors.New("could not prepare AI request")
	}
	req.Header.Set("Authorization", "Bearer "+key)
	if extraHeader == "anthropic-version" {
		req.Header.Del("Authorization")
		req.Header.Set("x-api-key", key)
		req.Header.Set(extraHeader, extraValue)
	}
	req.Header.Set("Content-Type", "application/json")
	response, err := client.Do(req)
	if err != nil {
		return nil, errors.New("AI provider request did not return a response")
	}
	defer response.Body.Close()
	data, err := io.ReadAll(io.LimitReader(response.Body, 2<<20))
	if err != nil {
		return nil, errors.New("could not read AI provider response")
	}
	if response.StatusCode < 200 || response.StatusCode >= 300 {
		return nil, fmt.Errorf("AI provider returned HTTP %d", response.StatusCode)
	}
	return data, nil
}
