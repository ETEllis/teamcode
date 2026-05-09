package agency

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"time"
)

// CodexCLIAdapter runs inference via the Codex CLI (`codex exec`) using the
// ChatGPT OAuth credentials stored at ~/.codex/auth.json. No API key required —
// authentication is handled by `codex login` before first use.
type CodexCLIAdapter struct {
	binaryPath string
	authFile   string
}

func NewCodexCLIAdapter() *CodexCLIAdapter {
	home, _ := os.UserHomeDir()
	return &CodexCLIAdapter{
		binaryPath: findCodexBinary(),
		authFile:   filepath.Join(home, ".codex", "auth.json"),
	}
}

func findCodexBinary() string {
	if path, err := exec.LookPath("codex"); err == nil {
		return path
	}
	// Common install locations for npm global packages
	candidates := []string{
		"/usr/local/bin/codex",
		"/opt/homebrew/bin/codex",
	}
	if home, err := os.UserHomeDir(); err == nil {
		candidates = append(candidates,
			filepath.Join(home, ".local", "bin", "codex"),
			filepath.Join(home, ".npm-global", "bin", "codex"),
		)
	}
	for _, c := range candidates {
		if _, err := os.Stat(c); err == nil {
			return c
		}
	}
	return ""
}

func (c *CodexCLIAdapter) Name() string    { return "codex" }
func (c *CodexCLIAdapter) ModelID() string { return "codex-cli" }

// Available returns true when the codex binary is present and auth.json has an
// access token from a prior `codex login`.
func (c *CodexCLIAdapter) Available(_ context.Context) bool {
	if c.binaryPath == "" {
		return false
	}
	data, err := os.ReadFile(c.authFile)
	if err != nil {
		return false
	}
	var auth struct {
		Tokens struct {
			AccessToken string `json:"access_token"`
		} `json:"tokens"`
		AuthMode string `json:"auth_mode"`
	}
	if err := json.Unmarshal(data, &auth); err != nil {
		return false
	}
	return strings.TrimSpace(auth.Tokens.AccessToken) != ""
}

// Execute runs `codex exec --json` in a read-only sandbox by default and parses
// the resulting newline-delimited JSON event stream.
func (c *CodexCLIAdapter) Execute(ctx context.Context, req InferenceRequest) (InferenceResult, error) {
	start := time.Now()

	prompt := req.UserMessage
	if strings.TrimSpace(req.System) != "" {
		prompt = req.System + "\n\n" + req.UserMessage
	}

	cmd := exec.CommandContext(ctx, c.binaryPath, codexExecArgs(prompt)...)
	// Inherit environment so codex can find its config/auth files.
	cmd.Env = os.Environ()

	output, err := cmd.Output()
	if err != nil {
		if ee, ok := err.(*exec.ExitError); ok && len(ee.Stderr) > 0 {
			return InferenceResult{}, fmt.Errorf("codex exec: %w — %s", err, strings.TrimSpace(string(ee.Stderr)))
		}
		return InferenceResult{}, fmt.Errorf("codex exec: %w", err)
	}

	text := parseCodexOutput(output)
	if text == "" {
		// Fallback: treat raw stdout as the response
		text = strings.TrimSpace(string(output))
	}
	if text == "" {
		return InferenceResult{}, fmt.Errorf("codex: empty response")
	}

	return InferenceResult{
		Text:      text,
		Provider:  "codex",
		ModelID:   c.ModelID(),
		LatencyMs: time.Since(start).Milliseconds(),
	}, nil
}

func codexExecArgs(prompt string) []string {
	args := []string{"exec", "--json"}
	if allowUnsafeCodexExec() {
		args = append(args, "--dangerously-bypass-approvals-and-sandbox")
	} else {
		args = append(args, "--sandbox", "read-only")
	}
	return append(args, prompt)
}

func allowUnsafeCodexExec() bool {
	switch strings.ToLower(strings.TrimSpace(os.Getenv("AGENCY_CODEX_UNSANDBOXED"))) {
	case "1", "true", "yes":
		return true
	default:
		return false
	}
}

// parseCodexOutput parses the newline-delimited JSON event stream produced by
// `codex exec --json`. It follows the OpenAI Responses API event format,
// preferring the final "done" event text and falling back to accumulated deltas.
func parseCodexOutput(data []byte) string {
	var deltas []string
	var finalText string

	for _, raw := range bytes.Split(data, []byte("\n")) {
		raw = bytes.TrimSpace(raw)
		if len(raw) == 0 || raw[0] != '{' {
			continue
		}
		var event map[string]any
		if err := json.Unmarshal(raw, &event); err != nil {
			continue
		}
		eventType, _ := event["type"].(string)

		if item, ok := event["item"].(map[string]any); ok {
			itemType, _ := item["type"].(string)
			switch itemType {
			case "agent_message":
				if text, ok := item["text"].(string); ok && strings.TrimSpace(text) != "" {
					finalText = text
				}
			case "message_delta":
				if text, ok := item["text"].(string); ok && text != "" {
					deltas = append(deltas, text)
				}
			}
		}

		// OpenAI Responses API: response.output_text.done has the full text.
		if strings.Contains(eventType, ".done") || strings.HasSuffix(eventType, "done") {
			for _, field := range []string{"text", "content", "output"} {
				if v, ok := event[field].(string); ok && strings.TrimSpace(v) != "" {
					finalText = v
					break
				}
			}
		}

		// Accumulate streaming deltas.
		if strings.Contains(eventType, "delta") {
			for _, field := range []string{"delta", "text", "content"} {
				if v, ok := event[field].(string); ok && v != "" {
					deltas = append(deltas, v)
					break
				}
			}
		}

		// Fallback: any event carrying a substantial text payload.
		if finalText == "" && !strings.Contains(eventType, "delta") {
			for _, field := range []string{"message", "content", "text", "output"} {
				if v, ok := event[field].(string); ok && len(strings.TrimSpace(v)) > 4 {
					finalText = v
					break
				}
			}
		}
	}

	if finalText != "" {
		return strings.TrimSpace(finalText)
	}
	return strings.TrimSpace(strings.Join(deltas, ""))
}
