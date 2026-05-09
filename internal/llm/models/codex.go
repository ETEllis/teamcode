package models

const (
	ProviderCodex ModelProvider = "codex"

	CodexCLI ModelID = "codex-cli"
)

var CodexModels = map[ModelID]Model{
	CodexCLI: {
		ID:                  CodexCLI,
		Name:                "Codex CLI",
		Provider:            ProviderCodex,
		APIModel:            "codex-cli",
		ContextWindow:       200_000,
		DefaultMaxTokens:    20000,
		SupportsAttachments: false,
	},
}
