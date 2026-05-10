package agency

// defaultProviderAdapters returns all built-in provider adapters.
// Order matters for soft scoring — local providers listed first.
func defaultProviderAdapters() []ProviderAdapter {
	return []ProviderAdapter{
		NewCodexCLIAdapter(), // ChatGPT OAuth via codex CLI — no API key needed
		NewOllamaAdapter(),
		NewOpenAICompatibleAdapter(mustProviderProfile("lmstudio")),
		NewAnthropicAdapter(),
		NewOpenAIAdapter(),
		NewGeminiAdapter(),
		NewOpenAICompatibleAdapter(mustProviderProfile("openrouter")),
		NewOpenAICompatibleAdapter(mustProviderProfile("opencode")),
		NewOpenAICompatibleAdapter(mustProviderProfile("zen")),
		NewOpenAICompatibleAdapter(mustProviderProfile("go")),
		NewOpenAICompatibleAdapter(mustProviderProfile("litellm")),
		NewOpenAICompatibleAdapter(mustProviderProfile("mistral")),
		NewOpenAICompatibleAdapter(mustProviderProfile("xai")),
		NewOpenAICompatibleAdapter(mustProviderProfile("groq")),
		NewOpenAICompatibleAdapter(mustProviderProfile("together")),
		NewOpenAICompatibleAdapter(mustProviderProfile("fireworks")),
		NewOpenAICompatibleAdapter(mustProviderProfile("perplexity")),
		NewOpenAICompatibleAdapter(mustProviderProfile("cerebras")),
		NewOpenAICompatibleAdapter(mustProviderProfile("zai")),
		NewOpenAICompatibleAdapter(mustProviderProfile("qwen")),
	}
}

func BuiltinProviderAdaptersForDirector() []ProviderAdapter {
	return defaultProviderAdapters()
}

func mustProviderProfile(name string) ProviderProfile {
	profile, ok := profileByName(name)
	if !ok {
		panic("unknown provider profile: " + name)
	}
	return profile
}
