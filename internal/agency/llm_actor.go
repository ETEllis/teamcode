package agency

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"os"
	"strconv"
	"strings"
	"time"
)

// ActorLLMConfig holds configuration for LLM-backed action generation.
type ActorLLMConfig struct {
	ModelID   string
	MaxTokens int
	APIKey    string
	BaseURL   string
}

// DefaultActorLLMConfig reads config from environment variables.
func DefaultActorLLMConfig() ActorLLMConfig {
	model := os.Getenv("AGENCY_LLM_MODEL")
	if model == "" {
		model = "claude-haiku-4-5-20251001"
	}
	maxTokens := 512
	if v := os.Getenv("AGENCY_LLM_MAX_TOKENS"); v != "" {
		if n, err := strconv.Atoi(v); err == nil && n > 0 {
			maxTokens = n
		}
	}
	baseURL := os.Getenv("AGENCY_LLM_BASE_URL")
	if baseURL == "" {
		baseURL = "https://api.anthropic.com"
	}
	return ActorLLMConfig{
		ModelID:   model,
		MaxTokens: maxTokens,
		APIKey:    os.Getenv("ANTHROPIC_API_KEY"),
		BaseURL:   strings.TrimRight(baseURL, "/"),
	}
}

// LLMActorProposer generates action proposals via LLM for a given observation.
type LLMActorProposer struct {
	cfg         ActorLLMConfig
	client      *http.Client
	gistVerdict *GISTVerdict
}

// NewLLMActorProposer creates a new proposer.
func NewLLMActorProposer(cfg ActorLLMConfig) *LLMActorProposer {
	return &LLMActorProposer{
		cfg:    cfg,
		client: &http.Client{Timeout: 30 * time.Second},
	}
}

// SetGISTContext attaches a GIST verdict whose Verdict string is prepended to
// the LLM system prompt and whose ExecutionIntent is stored on each proposal.
func (p *LLMActorProposer) SetGISTContext(verdict GISTVerdict) {
	p.gistVerdict = &verdict
}

// Propose generates action proposals for the given observation and signal.
// If the API key is not configured, returns a safe default proposal.
func (p *LLMActorProposer) Propose(ctx context.Context, obs ObservationSnapshot, signal WakeSignal, role RoleSpec) ([]ActionProposal, error) {
	if strings.TrimSpace(p.cfg.APIKey) == "" {
		return p.defaultProposal(obs, signal), nil
	}

	systemPrompt := p.buildSystemPrompt(obs, role)
	userMessage := p.buildUserMessage(obs, signal)

	text, err := p.callLLM(ctx, systemPrompt, userMessage)
	if err != nil {
		return p.defaultProposal(obs, signal), nil
	}

	payload := map[string]any{
		"message":     text,
		"signalKind":  string(signal.Kind),
		"signalID":    signal.ID,
		"actorRole":   obs.Actor.Role,
		"llmModel":    p.cfg.ModelID,
		"entrySource": "llm_actor_proposer",
	}
	if p.gistVerdict != nil && p.gistVerdict.ExecutionIntent != "" {
		payload["executionIntent"] = p.gistVerdict.ExecutionIntent
	}
	return []ActionProposal{
		{
			OrganizationID: obs.OrganizationID,
			ActorID:        obs.Actor.ID,
			Type:           ActionBroadcast,
			ProposedAt:     time.Now().UnixMilli(),
			Payload:        payload,
		},
	}, nil
}

func (p *LLMActorProposer) defaultProposal(obs ObservationSnapshot, signal WakeSignal) []ActionProposal {
	return []ActionProposal{
		{
			OrganizationID: obs.OrganizationID,
			ActorID:        obs.Actor.ID,
			Type:           ActionBroadcast,
			ProposedAt:     time.Now().UnixMilli(),
			Payload: map[string]any{
				"message":     "actor ready",
				"signalKind":  string(signal.Kind),
				"signalID":    signal.ID,
				"entrySource": "llm_actor_proposer.default",
			},
		},
	}
}

// BuildSystemPrompt returns the system prompt for the given observation and role.
func (p *LLMActorProposer) BuildSystemPrompt(obs ObservationSnapshot, role RoleSpec) string {
	return p.buildSystemPrompt(obs, role)
}

// BuildUserMessage returns the user message for the given observation and signal.
func (p *LLMActorProposer) BuildUserMessage(obs ObservationSnapshot, signal WakeSignal) string {
	return p.buildUserMessage(obs, signal)
}

func (p *LLMActorProposer) buildSystemPrompt(obs ObservationSnapshot, role RoleSpec) string {
	allowed := make([]string, 0, len(role.AllowedActions))
	for _, a := range role.AllowedActions {
		allowed = append(allowed, string(a))
	}
	systemPrompt := role.SystemPrompt
	if strings.TrimSpace(systemPrompt) == "" {
		systemPrompt = fmt.Sprintf("You are %s. Mission: %s", role.Name, role.Mission)
	}
	gistPrefix := ""
	if p.gistVerdict != nil && strings.TrimSpace(p.gistVerdict.Verdict) != "" {
		gistPrefix = fmt.Sprintf("GIST context: %s (confidence %.0f%%)\n\n",
			p.gistVerdict.Verdict, p.gistVerdict.Confidence*100)
	}
	return fmt.Sprintf(`%s%s

Allowed actions: %s
Shared workplace: %s
Current ledger sequence: %d

Respond with a concise status update or action message (1-3 sentences).
Be direct, professional, and specific to your role.`,
		gistPrefix,
		systemPrompt,
		strings.Join(allowed, ", "),
		obs.Resources.SharedWorkplace,
		obs.LedgerSequence,
	)
}

func (p *LLMActorProposer) buildUserMessage(obs ObservationSnapshot, signal WakeSignal) string {
	payloadStr := ""
	for k, v := range signal.Payload {
		payloadStr += fmt.Sprintf("  %s: %s\n", k, v)
	}
	return fmt.Sprintf("Signal received:\n  kind: %s\n  channel: %s\n  id: %s\n%s\nWhat is your response?",
		signal.Kind, signal.Channel, signal.ID, payloadStr)
}

type anthropicRequest struct {
	Model     string             `json:"model"`
	MaxTokens int                `json:"max_tokens"`
	System    string             `json:"system"`
	Messages  []anthropicMessage `json:"messages"`
}

type anthropicMessage struct {
	Role    string `json:"role"`
	Content string `json:"content"`
}

type anthropicResponse struct {
	Content []struct {
		Text string `json:"text"`
	} `json:"content"`
	Error *struct {
		Message string `json:"message"`
	} `json:"error,omitempty"`
}

func (p *LLMActorProposer) callLLM(ctx context.Context, system, userMsg string) (string, error) {
	req := anthropicRequest{
		Model:     p.cfg.ModelID,
		MaxTokens: p.cfg.MaxTokens,
		System:    system,
		Messages:  []anthropicMessage{{Role: "user", Content: userMsg}},
	}
	body, err := json.Marshal(req)
	if err != nil {
		return "", err
	}

	httpReq, err := http.NewRequestWithContext(ctx, http.MethodPost,
		p.cfg.BaseURL+"/v1/messages", bytes.NewReader(body))
	if err != nil {
		return "", err
	}
	httpReq.Header.Set("x-api-key", p.cfg.APIKey)
	httpReq.Header.Set("anthropic-version", "2023-06-01")
	httpReq.Header.Set("content-type", "application/json")

	resp, err := p.client.Do(httpReq)
	if err != nil {
		return "", err
	}
	defer resp.Body.Close()

	data, err := io.ReadAll(resp.Body)
	if err != nil {
		return "", err
	}

	var ar anthropicResponse
	if err := json.Unmarshal(data, &ar); err != nil {
		return "", err
	}
	if ar.Error != nil {
		return "", fmt.Errorf("anthropic api error: %s", ar.Error.Message)
	}
	if len(ar.Content) == 0 {
		return "", fmt.Errorf("empty response from anthropic")
	}
	return strings.TrimSpace(ar.Content[0].Text), nil
}
