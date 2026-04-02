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

type OpenAIAdapter struct {
	apiKey  string
	modelID string
	baseURL string
	client  *http.Client
}

func NewOpenAIAdapter() *OpenAIAdapter {
	baseURL := os.Getenv("OPENAI_BASE_URL")
	if baseURL == "" {
		baseURL = "https://api.openai.com"
	}
	modelID := os.Getenv("OPENAI_MODEL")
	if modelID == "" {
		modelID = "gpt-4o-mini"
	}
	return &OpenAIAdapter{
		apiKey:  os.Getenv("OPENAI_API_KEY"),
		modelID: modelID,
		baseURL: strings.TrimRight(baseURL, "/"),
		client:  &http.Client{Timeout: 30 * time.Second},
	}
}

func (o *OpenAIAdapter) Name() string    { return "openai" }
func (o *OpenAIAdapter) ModelID() string { return o.modelID }

func (o *OpenAIAdapter) Available(_ context.Context) bool {
	return strings.TrimSpace(o.apiKey) != ""
}

func (o *OpenAIAdapter) Execute(ctx context.Context, req InferenceRequest) (InferenceResult, error) {
	start := time.Now()
	type message struct {
		Role    string `json:"role"`
		Content string `json:"content"`
	}
	type request struct {
		Model     string    `json:"model"`
		Messages  []message `json:"messages"`
		MaxTokens int       `json:"max_tokens"`
	}
	type response struct {
		Choices []struct {
			Message struct {
				Content string `json:"content"`
			} `json:"message"`
		} `json:"choices"`
		Usage *struct {
			CompletionTokens int `json:"completion_tokens"`
		} `json:"usage,omitempty"`
		Error *struct {
			Message string `json:"message"`
		} `json:"error,omitempty"`
	}

	messages := []message{}
	if req.System != "" {
		messages = append(messages, message{Role: "system", Content: req.System})
	}
	messages = append(messages, message{Role: "user", Content: req.UserMessage})

	body, err := json.Marshal(request{
		Model:     o.modelID,
		Messages:  messages,
		MaxTokens: 512,
	})
	if err != nil {
		return InferenceResult{}, err
	}
	httpReq, err := http.NewRequestWithContext(ctx, http.MethodPost,
		o.baseURL+"/v1/chat/completions", bytes.NewReader(body))
	if err != nil {
		return InferenceResult{}, err
	}
	httpReq.Header.Set("Authorization", "Bearer "+o.apiKey)
	httpReq.Header.Set("content-type", "application/json")

	resp, err := o.client.Do(httpReq)
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
		return InferenceResult{}, fmt.Errorf("openai: %s", ar.Error.Message)
	}
	if len(ar.Choices) == 0 {
		return InferenceResult{}, fmt.Errorf("openai: empty response")
	}
	tokens := 0
	if ar.Usage != nil {
		tokens = ar.Usage.CompletionTokens
	}
	return InferenceResult{
		Text:       strings.TrimSpace(ar.Choices[0].Message.Content),
		Provider:   "openai",
		ModelID:    o.modelID,
		LatencyMs:  time.Since(start).Milliseconds(),
		TokensUsed: tokens,
	}, nil
}
