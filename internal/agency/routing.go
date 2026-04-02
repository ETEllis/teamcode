package agency

import (
	"context"
	"fmt"
	"log"
	"os"
	"strings"
	"time"
)

// ProviderAdapter is the interface all LLM provider adapters must implement.
type ProviderAdapter interface {
	// Name returns the provider identifier (e.g. "anthropic", "ollama").
	Name() string
	// ModelID returns the model this adapter will use.
	ModelID() string
	// Available returns true if the provider is reachable and credentials are valid.
	Available(ctx context.Context) bool
	// Execute runs a single inference and returns the result.
	Execute(ctx context.Context, req InferenceRequest) (InferenceResult, error)
}

// RoutingDecision records why a provider was selected or rejected.
type RoutingDecision struct {
	SelectedProvider string
	SelectedModel    string
	GateReason       string // empty = passed all gates
	RejectedOrder    []string
	ScoredAt         int64
}

// CredentialBroker loads and validates provider credentials from environment.
type CredentialBroker struct {
	handles []CredentialHandle
}

// NewCredentialBroker probes the environment for known provider API keys.
func NewCredentialBroker() *CredentialBroker {
	probes := []struct {
		provider string
		envKey   string
		modelID  string
	}{
		{"anthropic", "ANTHROPIC_API_KEY", "claude-haiku-4-5-20251001"},
		{"openai", "OPENAI_API_KEY", "gpt-4o-mini"},
		{"gemini", "GEMINI_API_KEY", "gemini-2.0-flash"},
		{"ollama", "", "llama3.2"},
	}

	handles := make([]CredentialHandle, 0, len(probes))
	for _, p := range probes {
		status := "valid"
		if p.envKey != "" {
			if strings.TrimSpace(os.Getenv(p.envKey)) == "" {
				status = "missing"
			}
		}
		// Ollama is local — always "valid" from credential perspective.
		handles = append(handles, CredentialHandle{
			Provider: p.provider,
			KeyRef:   p.envKey,
			Status:   status,
			ModelID:  p.modelID,
		})
	}
	return &CredentialBroker{handles: handles}
}

// ValidHandles returns only handles whose status is "valid".
func (b *CredentialBroker) ValidHandles() []CredentialHandle {
	out := make([]CredentialHandle, 0)
	for _, h := range b.handles {
		if h.Status == "valid" {
			out = append(out, h)
		}
	}
	return out
}

// Handle returns the credential handle for the given provider name.
func (b *CredentialBroker) Handle(provider string) (CredentialHandle, bool) {
	for _, h := range b.handles {
		if h.Provider == provider {
			return h, true
		}
	}
	return CredentialHandle{}, false
}

// ModelRouter performs deterministic provider selection.
// Hard gates are applied in order: capability → auth → privacy → tools → budget.
// Remaining candidates are soft-scored and the highest scorer is returned.
type ModelRouter struct {
	adapters []ProviderAdapter
	broker   *CredentialBroker
	policy   ExecutionPolicy
}

// NewModelRouter creates a router with the given adapters and policy.
func NewModelRouter(adapters []ProviderAdapter, broker *CredentialBroker, policy ExecutionPolicy) *ModelRouter {
	return &ModelRouter{
		adapters: adapters,
		broker:   broker,
		policy:   policy,
	}
}

// Route selects the best available adapter for the inference request.
// Returns an error only if no adapter passes all hard gates.
func (r *ModelRouter) Route(ctx context.Context, req InferenceRequest) (ProviderAdapter, RoutingDecision, error) {
	decision := RoutingDecision{ScoredAt: time.Now().UnixMilli()}
	candidates := make([]ProviderAdapter, 0, len(r.adapters))
	rejected := make([]string, 0)

	for _, adapter := range r.adapters {
		name := adapter.Name()

		// Gate 1: capability — adapter must be available.
		if !adapter.Available(ctx) {
			rejected = append(rejected, name+":unavailable")
			continue
		}

		// Gate 2: auth — credential must be valid.
		if h, ok := r.broker.Handle(name); ok && h.Status != "valid" {
			rejected = append(rejected, name+":no_credential")
			continue
		}

		// Gate 3: privacy — if local-only policy, reject cloud providers.
		if r.policy.PrivacyLevel == "local" && name != "ollama" {
			rejected = append(rejected, name+":privacy_gate")
			continue
		}

		// Gate 4: tools — skip (tool binding resolved at Stage 5).

		// Gate 5: budget — skip cost gate when ceiling is 0 (unconstrained).
		if r.policy.MaxCostUsd > 0 && req.Intent.CostCeilingUsd > 0 {
			if req.Intent.CostCeilingUsd < r.policy.MaxCostUsd {
				rejected = append(rejected, name+":budget_gate")
				continue
			}
		}

		candidates = append(candidates, adapter)
	}

	decision.RejectedOrder = rejected

	if len(candidates) == 0 {
		decision.GateReason = "no_candidate: " + strings.Join(rejected, ", ")
		return nil, decision, fmt.Errorf("model router: no provider passed all gates (%s)", strings.Join(rejected, "; "))
	}

	// Soft scoring: prefer local when policy says so, then lowest latency budget.
	selected := r.softScore(candidates, req)
	decision.SelectedProvider = selected.Name()
	decision.SelectedModel = selected.ModelID()

	log.Printf("model-router: selected=%s model=%s rejected=[%s] intent=%s",
		decision.SelectedProvider, decision.SelectedModel,
		strings.Join(rejected, ","), req.Intent.TaskType)

	return selected, decision, nil
}

func (r *ModelRouter) softScore(candidates []ProviderAdapter, req InferenceRequest) ProviderAdapter {
	// Simple scoring: local provider gets +10, lower latency budget gets bonus.
	best := candidates[0]
	bestScore := r.score(best, req)
	for _, c := range candidates[1:] {
		if s := r.score(c, req); s > bestScore {
			bestScore = s
			best = c
		}
	}
	return best
}

func (r *ModelRouter) score(adapter ProviderAdapter, req InferenceRequest) float64 {
	score := 0.0
	name := adapter.Name()

	// Local-first bonus.
	if r.policy.PreferLocal && name == "ollama" {
		score += 10.0
	}

	// Latency alignment: prefer fast providers for low-latency requests.
	if req.Intent.LatencyBudgetMs > 0 && req.Intent.LatencyBudgetMs < 2000 {
		if name == "ollama" {
			score += 3.0
		}
	}

	// Provider preference from AllowedProviders order (first = preferred).
	for i, p := range r.policy.AllowedProviders {
		if p == name {
			score += float64(len(r.policy.AllowedProviders)-i) * 0.5
			break
		}
	}

	return score
}
