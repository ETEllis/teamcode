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

type GeminiAdapter struct {
	apiKey  string
	modelID string
	client  *http.Client
}

func NewGeminiAdapter() *GeminiAdapter {
	modelID := os.Getenv("GEMINI_MODEL")
	if modelID == "" {
		modelID = "gemini-2.0-flash"
	}
	return &GeminiAdapter{
		apiKey:  os.Getenv("GEMINI_API_KEY"),
		modelID: modelID,
		client:  &http.Client{Timeout: 30 * time.Second},
	}
}

func (g *GeminiAdapter) Name() string    { return "gemini" }
func (g *GeminiAdapter) ModelID() string { return g.modelID }

func (g *GeminiAdapter) Available(_ context.Context) bool {
	return strings.TrimSpace(g.apiKey) != ""
}

func (g *GeminiAdapter) Execute(ctx context.Context, req InferenceRequest) (InferenceResult, error) {
	start := time.Now()
	type part struct {
		Text string `json:"text"`
	}
	type content struct {
		Parts []part `json:"parts"`
		Role  string `json:"role,omitempty"`
	}
	type request struct {
		Contents          []content `json:"contents"`
		SystemInstruction *content  `json:"systemInstruction,omitempty"`
	}
	type response struct {
		Candidates []struct {
			Content struct {
				Parts []struct {
					Text string `json:"text"`
				} `json:"parts"`
			} `json:"content"`
		} `json:"candidates"`
		Error *struct {
			Message string `json:"message"`
		} `json:"error,omitempty"`
	}

	gemReq := request{
		Contents: []content{
			{Parts: []part{{Text: req.UserMessage}}, Role: "user"},
		},
	}
	if req.System != "" {
		gemReq.SystemInstruction = &content{Parts: []part{{Text: req.System}}}
	}

	body, err := json.Marshal(gemReq)
	if err != nil {
		return InferenceResult{}, err
	}
	url := fmt.Sprintf(
		"https://generativelanguage.googleapis.com/v1beta/models/%s:generateContent?key=%s",
		g.modelID, g.apiKey,
	)
	httpReq, err := http.NewRequestWithContext(ctx, http.MethodPost, url, bytes.NewReader(body))
	if err != nil {
		return InferenceResult{}, err
	}
	httpReq.Header.Set("content-type", "application/json")

	resp, err := g.client.Do(httpReq)
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
		return InferenceResult{}, fmt.Errorf("gemini: %s", ar.Error.Message)
	}
	if len(ar.Candidates) == 0 || len(ar.Candidates[0].Content.Parts) == 0 {
		return InferenceResult{}, fmt.Errorf("gemini: empty response")
	}
	return InferenceResult{
		Text:      strings.TrimSpace(ar.Candidates[0].Content.Parts[0].Text),
		Provider:  "gemini",
		ModelID:   g.modelID,
		LatencyMs: time.Since(start).Milliseconds(),
	}, nil
}
