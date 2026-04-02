package agency

// defaultProviderAdapters returns all built-in provider adapters.
// Order matters for soft scoring — local providers listed first.
func defaultProviderAdapters() []ProviderAdapter {
	return []ProviderAdapter{
		NewOllamaAdapter(),
		NewAnthropicAdapter(),
		NewOpenAIAdapter(),
		NewGeminiAdapter(),
	}
}
