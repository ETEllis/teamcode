package provider

import (
	"bufio"
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"io"
	"os"
	"os/exec"
	"path/filepath"
	"strings"

	"github.com/ETEllis/teamcode/internal/config"
	"github.com/ETEllis/teamcode/internal/llm/tools"
	"github.com/ETEllis/teamcode/internal/message"
)

type codexClient struct {
	providerOptions providerClientOptions
	binaryPath      string
	authFile        string
}

type CodexClient ProviderClient

func newCodexClient(opts providerClientOptions) CodexClient {
	home, _ := os.UserHomeDir()
	return &codexClient{
		providerOptions: opts,
		binaryPath:      findCodexBinary(),
		authFile:        filepath.Join(home, ".codex", "auth.json"),
	}
}

func findCodexBinary() string {
	if path, err := exec.LookPath("codex"); err == nil {
		return path
	}
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
	for _, candidate := range candidates {
		if _, err := os.Stat(candidate); err == nil {
			return candidate
		}
	}
	return ""
}

func (c *codexClient) send(ctx context.Context, messages []message.Message, _ []tools.BaseTool) (*ProviderResponse, error) {
	text, err := c.runCodex(ctx, messages)
	if err != nil {
		return nil, err
	}
	return &ProviderResponse{
		Content:      text,
		FinishReason: message.FinishReasonEndTurn,
	}, nil
}

func (c *codexClient) stream(ctx context.Context, messages []message.Message, _ []tools.BaseTool) <-chan ProviderEvent {
	events := make(chan ProviderEvent, 16)
	go func() {
		defer close(events)
		if err := c.streamCodex(ctx, messages, events); err != nil {
			events <- ProviderEvent{Type: EventError, Error: err}
		}
	}()
	return events
}

func (c *codexClient) runCodex(ctx context.Context, messages []message.Message) (string, error) {
	if c.binaryPath == "" {
		return "", fmt.Errorf("codex binary not found; run scripts/setup or install @openai/codex")
	}
	if !c.hasAuthToken() {
		return "", fmt.Errorf("codex login not found; run codex login")
	}

	prompt := codexPrompt(c.providerOptions.systemMessage, messages)
	execHome, err := c.prepareExecHome()
	if err != nil {
		return "", err
	}
	cmd := exec.CommandContext(ctx, c.binaryPath, codexExecArgs(prompt)...)
	cmd.Env = codexExecEnv(execHome)

	output, err := cmd.Output()
	if err != nil {
		if ee, ok := err.(*exec.ExitError); ok && len(ee.Stderr) > 0 {
			return "", fmt.Errorf("codex exec: %w — %s", err, strings.TrimSpace(string(ee.Stderr)))
		}
		return "", fmt.Errorf("codex exec: %w", err)
	}

	text := parseCodexOutput(output)
	if text == "" {
		text = strings.TrimSpace(string(output))
	}
	if text == "" {
		return "", fmt.Errorf("codex returned empty response")
	}
	return text, nil
}

func (c *codexClient) streamCodex(ctx context.Context, messages []message.Message, events chan<- ProviderEvent) error {
	if c.binaryPath == "" {
		return fmt.Errorf("codex binary not found; run scripts/setup or install @openai/codex")
	}
	if !c.hasAuthToken() {
		return fmt.Errorf("codex login not found; run codex login")
	}

	prompt := codexPrompt(c.providerOptions.systemMessage, messages)
	execHome, err := c.prepareExecHome()
	if err != nil {
		return err
	}
	cmd := exec.CommandContext(ctx, c.binaryPath, codexExecArgs(prompt)...)
	cmd.Env = codexExecEnv(execHome)

	stdout, err := cmd.StdoutPipe()
	if err != nil {
		return fmt.Errorf("codex stdout pipe: %w", err)
	}
	stderr, err := cmd.StderrPipe()
	if err != nil {
		return fmt.Errorf("codex stderr pipe: %w", err)
	}

	var stderrBuf bytes.Buffer
	go func() {
		_, _ = io.Copy(&stderrBuf, stderr)
	}()

	if err := cmd.Start(); err != nil {
		return fmt.Errorf("codex exec start: %w", err)
	}

	state := codexStreamState{}
	scanner := bufio.NewScanner(stdout)
	scanner.Buffer(make([]byte, 0, 64*1024), 8*1024*1024)
	for scanner.Scan() {
		for _, event := range parseCodexStreamLine(scanner.Bytes(), &state) {
			select {
			case <-ctx.Done():
				_ = cmd.Wait()
				return ctx.Err()
			case events <- event:
			}
		}
	}
	if err := scanner.Err(); err != nil {
		_ = cmd.Wait()
		return fmt.Errorf("codex stdout read: %w", err)
	}

	if err := cmd.Wait(); err != nil {
		if state.finalError != "" {
			return fmt.Errorf("codex exec: %s", state.finalError)
		}
		if stderr := strings.TrimSpace(stderrBuf.String()); stderr != "" {
			return fmt.Errorf("codex exec: %w — %s", err, stderr)
		}
		return fmt.Errorf("codex exec: %w", err)
	}

	if state.finalError != "" {
		return fmt.Errorf("codex exec: %s", state.finalError)
	}

	content := state.responseText()
	if content == "" {
		return fmt.Errorf("codex returned empty response")
	}

	events <- ProviderEvent{
		Type: EventComplete,
		Response: &ProviderResponse{
			Content:      content,
			Usage:        state.usage,
			FinishReason: message.FinishReasonEndTurn,
		},
	}
	return nil
}

func (c *codexClient) hasAuthToken() bool {
	data, err := os.ReadFile(c.authFile)
	if err != nil {
		return false
	}
	var auth struct {
		Tokens struct {
			AccessToken string `json:"access_token"`
		} `json:"tokens"`
	}
	if err := json.Unmarshal(data, &auth); err != nil {
		return false
	}
	return strings.TrimSpace(auth.Tokens.AccessToken) != ""
}

func (c *codexClient) prepareExecHome() (string, error) {
	baseDir := "."
	if cfg := config.Get(); cfg != nil && cfg.Data.Directory != "" {
		baseDir = cfg.Data.Directory
	}
	if !filepath.IsAbs(baseDir) {
		baseDir = filepath.Join(config.WorkingDirectory(), baseDir)
	}

	homeDir := filepath.Join(baseDir, "codex-home")
	codexDir := filepath.Join(homeDir, ".codex")
	if err := os.MkdirAll(codexDir, 0o755); err != nil {
		return "", fmt.Errorf("prepare codex home: %w", err)
	}

	auth, err := os.ReadFile(c.authFile)
	if err != nil {
		return "", fmt.Errorf("read codex auth: %w", err)
	}
	if err := os.WriteFile(filepath.Join(codexDir, "auth.json"), auth, 0o600); err != nil {
		return "", fmt.Errorf("write codex auth: %w", err)
	}

	return homeDir, nil
}

func codexExecArgs(prompt string) []string {
	args := []string{"exec", "--json", "--ephemeral"}
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

func codexPrompt(systemMessage string, messages []message.Message) string {
	var b strings.Builder
	if strings.TrimSpace(systemMessage) != "" {
		b.WriteString(systemMessage)
		b.WriteString("\n\n")
	}
	for _, msg := range messages {
		var content strings.Builder
		if text := strings.TrimSpace(msg.Content().String()); text != "" {
			content.WriteString(text)
		}
		for _, result := range msg.ToolResults() {
			if content.Len() > 0 {
				content.WriteString("\n")
			}
			content.WriteString("Tool ")
			content.WriteString(result.Name)
			content.WriteString(": ")
			content.WriteString(result.Content)
		}
		if content.Len() == 0 {
			continue
		}
		b.WriteString(strings.ToUpper(string(msg.Role)))
		b.WriteString(": ")
		b.WriteString(content.String())
		b.WriteString("\n\n")
	}
	return strings.TrimSpace(b.String())
}

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

		if strings.Contains(eventType, ".done") || strings.HasSuffix(eventType, "done") {
			for _, field := range []string{"text", "content", "output"} {
				if v, ok := event[field].(string); ok && strings.TrimSpace(v) != "" {
					finalText = v
					break
				}
			}
		}

		if strings.Contains(eventType, "delta") {
			for _, field := range []string{"delta", "text", "content"} {
				if v, ok := event[field].(string); ok && v != "" {
					deltas = append(deltas, v)
					break
				}
			}
		}

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

type codexStreamState struct {
	deltas      strings.Builder
	emittedText strings.Builder
	finalText   string
	finalError  string
	usage       TokenUsage
}

func (s *codexStreamState) responseText() string {
	if strings.TrimSpace(s.finalText) != "" {
		return strings.TrimSpace(s.finalText)
	}
	return strings.TrimSpace(s.deltas.String())
}

func parseCodexStreamLine(raw []byte, state *codexStreamState) []ProviderEvent {
	raw = bytes.TrimSpace(raw)
	if len(raw) == 0 || raw[0] != '{' {
		return nil
	}

	var event map[string]any
	if err := json.Unmarshal(raw, &event); err != nil {
		return nil
	}

	if usage, ok := event["usage"].(map[string]any); ok {
		state.usage = TokenUsage{
			InputTokens:         int64Value(usage["input_tokens"]),
			OutputTokens:        int64Value(usage["output_tokens"]),
			CacheCreationTokens: int64Value(usage["cached_input_tokens"]),
			CacheReadTokens:     int64Value(usage["cache_read_input_tokens"]),
		}
	}

	var events []ProviderEvent
	eventType, _ := event["type"].(string)
	switch eventType {
	case "error":
		if message, ok := event["message"].(string); ok && strings.TrimSpace(message) != "" {
			state.finalError = strings.TrimSpace(message)
		}
	case "turn.failed":
		if errPayload, ok := event["error"].(map[string]any); ok {
			if message, ok := errPayload["message"].(string); ok && strings.TrimSpace(message) != "" {
				state.finalError = strings.TrimSpace(message)
			}
		}
	}
	if delta := codexEventText(eventType, event); delta != "" {
		state.deltas.WriteString(delta)
		state.emittedText.WriteString(delta)
		events = append(events, ProviderEvent{Type: EventContentDelta, Content: delta})
	}

	if item, ok := event["item"].(map[string]any); ok {
		itemType, _ := item["type"].(string)
		switch itemType {
		case "agent_message":
			if text, ok := item["text"].(string); ok && strings.TrimSpace(text) != "" {
				state.finalText = text
				if remainder := strings.TrimPrefix(text, state.emittedText.String()); remainder != "" {
					state.emittedText.WriteString(remainder)
					events = append(events, ProviderEvent{Type: EventContentDelta, Content: remainder})
				}
			}
		case "message_delta":
			if text, ok := item["text"].(string); ok && text != "" {
				state.deltas.WriteString(text)
				state.emittedText.WriteString(text)
				events = append(events, ProviderEvent{Type: EventContentDelta, Content: text})
			}
		}
	}

	return events
}

func codexEventText(eventType string, event map[string]any) string {
	if !(strings.Contains(eventType, "delta") || strings.Contains(eventType, ".done") || strings.HasSuffix(eventType, "done")) {
		return ""
	}
	for _, field := range []string{"delta", "text", "content", "output"} {
		if v, ok := event[field].(string); ok && v != "" {
			return v
		}
	}
	return ""
}

func int64Value(value any) int64 {
	switch v := value.(type) {
	case float64:
		return int64(v)
	case int64:
		return v
	case int:
		return int64(v)
	default:
		return 0
	}
}

func codexExecEnv(homeDir string) []string {
	env := os.Environ()
	filtered := make([]string, 0, len(env))
	homeSet := false
	for _, entry := range env {
		key := entry
		if idx := strings.Index(entry, "="); idx >= 0 {
			key = entry[:idx]
		}
		// Prevent nested Codex sessions from inheriting the parent desktop/thread
		// context and leaking outer-agent process instructions into Agency replies.
		if strings.HasPrefix(key, "CODEX_") {
			continue
		}
		if key == "HOME" {
			filtered = append(filtered, "HOME="+homeDir)
			homeSet = true
			continue
		}
		filtered = append(filtered, entry)
	}
	if !homeSet {
		filtered = append(filtered, "HOME="+homeDir)
	}
	return filtered
}
