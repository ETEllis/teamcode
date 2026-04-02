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

type OllamaAdapter struct {
	baseURL string
	modelID string
	client  *http.Client
}

func NewOllamaAdapter() *OllamaAdapter {
	baseURL := os.Getenv("OLLAMA_BASE_URL")
	if baseURL == "" {
		baseURL = "http://localhost:11434"
	}
	modelID := os.Getenv("OLLAMA_MODEL")
	if modelID == "" {
		modelID = "llama3.2"
	}
	return &OllamaAdapter{
		baseURL: strings.TrimRight(baseURL, "/"),
		modelID: modelID,
		client:  &http.Client{Timeout: 60 * time.Second},
	}
}

func (o *OllamaAdapter) Name() string    { return "ollama" }
func (o *OllamaAdapter) ModelID() string { return o.modelID }

func (o *OllamaAdapter) Available(ctx context.Context) bool {
	req, err := http.NewRequestWithContext(ctx, http.MethodGet, o.baseURL+"/api/tags", nil)
	if err != nil {
		return false
	}
	resp, err := o.client.Do(req)
	if err != nil {
		return false
	}
	defer resp.Body.Close()
	return resp.StatusCode == http.StatusOK
}

func (o *OllamaAdapter) Execute(ctx context.Context, req InferenceRequest) (InferenceResult, error) {
	start := time.Now()
	type message struct {
		Role    string `json:"role"`
		Content string `json:"content"`
	}
	type request struct {
		Model    string    `json:"model"`
		Messages []message `json:"messages"`
		Stream   bool      `json:"stream"`
	}
	type response struct {
		Message *struct {
			Content string `json:"content"`
		} `json:"message,omitempty"`
		Error string `json:"error,omitempty"`
	}

	messages := []message{}
	if req.System != "" {
		messages = append(messages, message{Role: "system", Content: req.System})
	}
	messages = append(messages, message{Role: "user", Content: req.UserMessage})

	body, err := json.Marshal(request{
		Model:    o.modelID,
		Messages: messages,
		Stream:   false,
	})
	if err != nil {
		return InferenceResult{}, err
	}
	httpReq, err := http.NewRequestWithContext(ctx, http.MethodPost,
		o.baseURL+"/api/chat", bytes.NewReader(body))
	if err != nil {
		return InferenceResult{}, err
	}
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
	if ar.Error != "" {
		return InferenceResult{}, fmt.Errorf("ollama: %s", ar.Error)
	}
	if ar.Message == nil {
		return InferenceResult{}, fmt.Errorf("ollama: empty response")
	}
	return InferenceResult{
		Text:      strings.TrimSpace(ar.Message.Content),
		Provider:  "ollama",
		ModelID:   o.modelID,
		LatencyMs: time.Since(start).Milliseconds(),
	}, nil
}
