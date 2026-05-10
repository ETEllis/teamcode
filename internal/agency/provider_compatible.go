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

type ProviderProfile struct {
	Name           string `json:"name"`
	DisplayName    string `json:"displayName"`
	Kind           string `json:"kind"`
	Priority       string `json:"priority"`
	APIKeyEnv      string `json:"apiKeyEnv,omitempty"`
	BaseURLEnv     string `json:"baseUrlEnv,omitempty"`
	ModelEnv       string `json:"modelEnv,omitempty"`
	DefaultBaseURL string `json:"defaultBaseUrl,omitempty"`
	DefaultModel   string `json:"defaultModel"`
	Local          bool   `json:"local"`
	Adapter        bool   `json:"adapter,omitempty"`
	Notes          string `json:"notes,omitempty"`
}

func BuiltinProviderProfiles() []ProviderProfile {
	return []ProviderProfile{
		{Name: "codex", DisplayName: "Codex CLI", Kind: "oauth-cli", Priority: "P0", DefaultModel: "codex-cli", Local: false, Notes: "ChatGPT OAuth via `codex login`; no API key required by Agency."},
		{Name: "ollama", DisplayName: "Ollama", Kind: "local", Priority: "P0", BaseURLEnv: "OLLAMA_BASE_URL", ModelEnv: "OLLAMA_MODEL", DefaultBaseURL: "http://localhost:11434", DefaultModel: "llama3.2", Local: true},
		{Name: "lmstudio", DisplayName: "LM Studio", Kind: "openai-compatible", Priority: "P0", BaseURLEnv: "LM_STUDIO_BASE_URL", ModelEnv: "LM_STUDIO_MODEL", DefaultBaseURL: "http://localhost:1234", DefaultModel: "local-model", Local: true},
		{Name: "openai", DisplayName: "OpenAI", Kind: "native", Priority: "P0", APIKeyEnv: "OPENAI_API_KEY", BaseURLEnv: "OPENAI_BASE_URL", ModelEnv: "OPENAI_MODEL", DefaultBaseURL: "https://api.openai.com", DefaultModel: "gpt-4o-mini"},
		{Name: "anthropic", DisplayName: "Anthropic", Kind: "native", Priority: "P0", APIKeyEnv: "ANTHROPIC_API_KEY", ModelEnv: "AGENCY_LLM_MODEL", DefaultBaseURL: "https://api.anthropic.com", DefaultModel: "claude-haiku-4-5-20251001", Notes: "Optional official API path; Agency does not emulate or bypass Claude account controls."},
		{Name: "gemini", DisplayName: "Gemini", Kind: "native", Priority: "P0", APIKeyEnv: "GEMINI_API_KEY", ModelEnv: "GEMINI_MODEL", DefaultModel: "gemini-2.0-flash"},
		{Name: "openrouter", DisplayName: "OpenRouter", Kind: "openai-compatible", Priority: "P0", APIKeyEnv: "OPENROUTER_API_KEY", BaseURLEnv: "OPENROUTER_BASE_URL", ModelEnv: "OPENROUTER_MODEL", DefaultBaseURL: "https://openrouter.ai/api", DefaultModel: "openai/gpt-4o-mini"},
		{Name: "opencode", DisplayName: "OpenCode models", Kind: "openai-compatible", Priority: "P0", APIKeyEnv: "OPENCODE_API_KEY", BaseURLEnv: "OPENCODE_BASE_URL", ModelEnv: "OPENCODE_MODEL", DefaultModel: "opencode/default", Notes: "First-class user-configured OpenCode model profile; set OPENCODE_BASE_URL to the compatible endpoint."},
		{Name: "zen", DisplayName: "Zen", Kind: "openai-compatible", Priority: "P0", APIKeyEnv: "ZEN_API_KEY", BaseURLEnv: "ZEN_BASE_URL", ModelEnv: "ZEN_MODEL", DefaultModel: "zen-default", Notes: "First-class user-configured Zen model profile; set ZEN_BASE_URL to the compatible endpoint."},
		{Name: "go", DisplayName: "Go", Kind: "openai-compatible", Priority: "P0", APIKeyEnv: "GO_API_KEY", BaseURLEnv: "GO_BASE_URL", ModelEnv: "GO_MODEL", DefaultModel: "go-default", Notes: "First-class user-configured Go model profile; set GO_BASE_URL to the compatible endpoint."},
		{Name: "litellm", DisplayName: "LiteLLM proxy", Kind: "openai-compatible", Priority: "P0-adapter", APIKeyEnv: "LITELLM_API_KEY", BaseURLEnv: "LITELLM_BASE_URL", ModelEnv: "LITELLM_MODEL", DefaultModel: "gpt-4o-mini", Adapter: true, Notes: "Self-hosted gateway profile; set LITELLM_BASE_URL to enable."},
		{Name: "mistral", DisplayName: "Mistral", Kind: "openai-compatible", Priority: "P1", APIKeyEnv: "MISTRAL_API_KEY", BaseURLEnv: "MISTRAL_BASE_URL", ModelEnv: "MISTRAL_MODEL", DefaultBaseURL: "https://api.mistral.ai", DefaultModel: "mistral-small-latest"},
		{Name: "xai", DisplayName: "xAI", Kind: "openai-compatible", Priority: "P1", APIKeyEnv: "XAI_API_KEY", BaseURLEnv: "XAI_BASE_URL", ModelEnv: "XAI_MODEL", DefaultBaseURL: "https://api.x.ai", DefaultModel: "grok-3-mini"},
		{Name: "groq", DisplayName: "Groq", Kind: "openai-compatible", Priority: "P1", APIKeyEnv: "GROQ_API_KEY", BaseURLEnv: "GROQ_BASE_URL", ModelEnv: "GROQ_MODEL", DefaultBaseURL: "https://api.groq.com/openai", DefaultModel: "llama-3.3-70b-versatile"},
		{Name: "together", DisplayName: "Together AI", Kind: "openai-compatible", Priority: "P1", APIKeyEnv: "TOGETHER_API_KEY", BaseURLEnv: "TOGETHER_BASE_URL", ModelEnv: "TOGETHER_MODEL", DefaultBaseURL: "https://api.together.xyz", DefaultModel: "meta-llama/Llama-3.3-70B-Instruct-Turbo"},
		{Name: "fireworks", DisplayName: "Fireworks AI", Kind: "openai-compatible", Priority: "P1", APIKeyEnv: "FIREWORKS_API_KEY", BaseURLEnv: "FIREWORKS_BASE_URL", ModelEnv: "FIREWORKS_MODEL", DefaultBaseURL: "https://api.fireworks.ai/inference", DefaultModel: "accounts/fireworks/models/llama-v3p1-8b-instruct"},
		{Name: "perplexity", DisplayName: "Perplexity", Kind: "openai-compatible", Priority: "P1", APIKeyEnv: "PERPLEXITY_API_KEY", BaseURLEnv: "PERPLEXITY_BASE_URL", ModelEnv: "PERPLEXITY_MODEL", DefaultBaseURL: "https://api.perplexity.ai", DefaultModel: "sonar-pro"},
		{Name: "cerebras", DisplayName: "Cerebras", Kind: "openai-compatible", Priority: "P2", APIKeyEnv: "CEREBRAS_API_KEY", BaseURLEnv: "CEREBRAS_BASE_URL", ModelEnv: "CEREBRAS_MODEL", DefaultBaseURL: "https://api.cerebras.ai", DefaultModel: "llama3.1-8b"},
		{Name: "zai", DisplayName: "Z.ai / GLM", Kind: "openai-compatible", Priority: "P2", APIKeyEnv: "ZAI_API_KEY", BaseURLEnv: "ZAI_BASE_URL", ModelEnv: "ZAI_MODEL", DefaultModel: "glm-4.5", Notes: "Set ZAI_BASE_URL to the current OpenAI-compatible endpoint before enabling."},
		{Name: "qwen", DisplayName: "Qwen / DashScope", Kind: "openai-compatible", Priority: "P2", APIKeyEnv: "DASHSCOPE_API_KEY", BaseURLEnv: "DASHSCOPE_BASE_URL", ModelEnv: "QWEN_MODEL", DefaultModel: "qwen-plus", Notes: "Uses API-key/OpenAI-compatible path; free Qwen Code OAuth is not assumed."},
	}
}

func profileByName(name string) (ProviderProfile, bool) {
	for _, profile := range BuiltinProviderProfiles() {
		if profile.Name == name {
			return profile, true
		}
	}
	return ProviderProfile{}, false
}

type OpenAICompatibleAdapter struct {
	profile ProviderProfile
	apiKey  string
	baseURL string
	modelID string
	client  *http.Client
}

func NewOpenAICompatibleAdapter(profile ProviderProfile) *OpenAICompatibleAdapter {
	baseURL := firstEnvValue(profile.BaseURLEnv, profile.DefaultBaseURL)
	modelID := firstEnvValue(profile.ModelEnv, profile.DefaultModel)
	return &OpenAICompatibleAdapter{
		profile: profile,
		apiKey:  firstEnvValue(profile.APIKeyEnv, ""),
		baseURL: strings.TrimRight(baseURL, "/"),
		modelID: modelID,
		client:  &http.Client{Timeout: 30 * time.Second},
	}
}

func (a *OpenAICompatibleAdapter) Name() string    { return a.profile.Name }
func (a *OpenAICompatibleAdapter) ModelID() string { return a.modelID }

func (a *OpenAICompatibleAdapter) Available(ctx context.Context) bool {
	if a.baseURL == "" {
		return false
	}
	if !a.profile.Local && a.profile.APIKeyEnv != "" && strings.TrimSpace(a.apiKey) == "" {
		return false
	}
	if a.profile.Local {
		req, err := http.NewRequestWithContext(ctx, http.MethodGet, a.baseURL+"/v1/models", nil)
		if err != nil {
			return false
		}
		resp, err := a.client.Do(req)
		if err != nil {
			return false
		}
		defer resp.Body.Close()
		return resp.StatusCode >= 200 && resp.StatusCode < 500
	}
	return true
}

func (a *OpenAICompatibleAdapter) Execute(ctx context.Context, req InferenceRequest) (InferenceResult, error) {
	start := time.Now()
	type message struct {
		Role    string `json:"role"`
		Content string `json:"content"`
	}
	type request struct {
		Model     string    `json:"model"`
		Messages  []message `json:"messages"`
		MaxTokens int       `json:"max_tokens,omitempty"`
		Stream    bool      `json:"stream"`
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
		Model:     a.modelID,
		Messages:  messages,
		MaxTokens: 512,
		Stream:    false,
	})
	if err != nil {
		return InferenceResult{}, err
	}
	httpReq, err := http.NewRequestWithContext(ctx, http.MethodPost, a.baseURL+"/v1/chat/completions", bytes.NewReader(body))
	if err != nil {
		return InferenceResult{}, err
	}
	if strings.TrimSpace(a.apiKey) != "" {
		httpReq.Header.Set("Authorization", "Bearer "+a.apiKey)
	}
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
		return InferenceResult{}, fmt.Errorf("%s: %s", a.profile.Name, ar.Error.Message)
	}
	if len(ar.Choices) == 0 {
		return InferenceResult{}, fmt.Errorf("%s: empty response", a.profile.Name)
	}
	tokens := 0
	if ar.Usage != nil {
		tokens = ar.Usage.CompletionTokens
	}
	return InferenceResult{
		Text:       strings.TrimSpace(ar.Choices[0].Message.Content),
		Provider:   a.profile.Name,
		ModelID:    a.modelID,
		LatencyMs:  time.Since(start).Milliseconds(),
		TokensUsed: tokens,
	}, nil
}

func firstEnvValue(envKey string, fallback string) string {
	if strings.TrimSpace(envKey) == "" {
		return fallback
	}
	if value := strings.TrimSpace(os.Getenv(envKey)); value != "" {
		return value
	}
	return fallback
}
