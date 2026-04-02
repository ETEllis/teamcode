package agency

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"os"
	"strings"
	"time"

	
)

type AnthropicAdapter struct {
	apiKey  string
	modelID string
	baseURL string
	client  *http.Client
}

func NewAnthropicAdapter() *AnthropicAdapter {
	baseURL := os.Getenv("AGENCY_LLM_BASE_URL")
	if baseURL == "" {
		baseURL = "https://api.anthropic.com"
	}
	modelID := os.Getenv("AGENCY_LLM_MODEL")
	if modelID == "" {
		modelID = "claude-haiku-4-5-20251001"
	}
	return &AnthropicAdapter{
		apiKey:  os.Getenv("ANTHROPIC_API_KEY"),
		modelID: modelID,
		baseURL: strings.TrimRight(baseURL, "/"),
		client:  &http.Client{Timeout: 30 * time.Second},
	}
}

func (a *AnthropicAdapter) Name() string    { return "anthropic" }
func (a *AnthropicAdapter) ModelID() string { return a.modelID }

func (a *AnthropicAdapter) Available(_ context.Context) bool {
	return strings.TrimSpace(a.apiKey) != ""
}

func (a *AnthropicAdapter) Execute(ctx context.Context, req InferenceRequest) (InferenceResult, error) {
	start := time.Now()
	type message struct {
		Role    string `json:"role"`
		Content string `json:"content"`
	}
	type request struct {
		Model     string    `json:"model"`
		MaxTokens int       `json:"max_tokens"`
		System    string    `json:"system"`
		Messages  []message `json:"messages"`
	}
	type response struct {
		Content []struct {
			Text string `json:"text"`
		} `json:"content"`
		Usage *struct {
			OutputTokens int `json:"output_tokens"`
		} `json:"usage,omitempty"`
		Error *struct {
			Message string `json:"message"`
		} `json:"error,omitempty"`
	}

	maxTokens := 512
	body, err := json.Marshal(request{
		Model:     a.modelID,
		MaxTokens: maxTokens,
		System:    req.System,
		Messages:  []message{{Role: "user", Content: req.UserMessage}},
	})
	if err != nil {
		return InferenceResult{}, err
	}
	httpReq, err := http.NewRequestWithContext(ctx, http.MethodPost,
		a.baseURL+"/v1/messages", bytes.NewReader(body))
	if err != nil {
		return InferenceResult{}, err
	}
	httpReq.Header.Set("x-api-key", a.apiKey)
	httpReq.Header.Set("anthropic-version", "2023-06-01")
	httpReq.Header.Set("content-type", "application/json")

	resp, err := a.client.Do(httpReq)
	if err != nil {
		return InferenceResult{}, err
	}
	defer resp.Body.Close()
	data, err := io.ReadAll(resp.Body)
	if err != nil {
		return InferenceResult{}, err
	}
	var ar response
	if err := json.Unmarshal(data, &ar); err != nil {
		return InferenceResult{}, err
	}
	if ar.Error != nil {
		return InferenceResult{}, fmt.Errorf("anthropic: %s", ar.Error.Message)
	}
	if len(ar.Content) == 0 {
		return InferenceResult{}, fmt.Errorf("anthropic: empty response")
	}
	tokens := 0
	if ar.Usage != nil {
		tokens = ar.Usage.OutputTokens
	}
	return InferenceResult{
		Text:       strings.TrimSpace(ar.Content[0].Text),
		Provider:   "anthropic",
		ModelID:    a.modelID,
		LatencyMs:  time.Since(start).Milliseconds(),
		TokensUsed: tokens,
	}, nil
}
