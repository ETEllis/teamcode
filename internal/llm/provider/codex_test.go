package provider

import (
	"strings"
	"testing"
)

func TestParseCodexOutputPrefersDoneText(t *testing.T) {
	input := []byte("{\"type\":\"response.output_text.delta\",\"delta\":\"Hello\"}\n{\"type\":\"response.output_text.done\",\"text\":\"Hello world\"}\n")
	got := parseCodexOutput(input)
	if got != "Hello world" {
		t.Fatalf("expected final done text, got %q", got)
	}
}

func TestParseCodexOutputFallsBackToDeltas(t *testing.T) {
	input := []byte("{\"type\":\"response.output_text.delta\",\"delta\":\"Hello \"}\n{\"type\":\"response.output_text.delta\",\"delta\":\"world\"}\n")
	got := parseCodexOutput(input)
	if got != "Hello world" {
		t.Fatalf("expected delta text, got %q", got)
	}
}

func TestParseCodexOutputReadsAgentMessageItem(t *testing.T) {
	input := []byte("{\"type\":\"item.completed\",\"item\":{\"type\":\"agent_message\",\"text\":\"4\"}}\n")
	got := parseCodexOutput(input)
	if got != "4" {
		t.Fatalf("expected agent message text, got %q", got)
	}
}

func TestParseCodexStreamLineEmitsDeltaAndUsage(t *testing.T) {
	state := codexStreamState{}
	events := parseCodexStreamLine([]byte("{\"type\":\"response.output_text.delta\",\"delta\":\"Hello \",\"usage\":{\"input_tokens\":10,\"output_tokens\":2}}"), &state)
	if len(events) != 1 {
		t.Fatalf("expected 1 event, got %d", len(events))
	}
	if events[0].Type != EventContentDelta || events[0].Content != "Hello " {
		t.Fatalf("unexpected event: %#v", events[0])
	}
	if state.usage.InputTokens != 10 || state.usage.OutputTokens != 2 {
		t.Fatalf("unexpected usage: %#v", state.usage)
	}
}

func TestParseCodexStreamLineEmitsRemainingAgentMessageOnly(t *testing.T) {
	state := codexStreamState{}
	parseCodexStreamLine([]byte("{\"type\":\"response.output_text.delta\",\"delta\":\"Hello \"}"), &state)
	events := parseCodexStreamLine([]byte("{\"type\":\"item.completed\",\"item\":{\"type\":\"agent_message\",\"text\":\"Hello world\"}}"), &state)
	if len(events) != 1 {
		t.Fatalf("expected 1 event, got %d", len(events))
	}
	if events[0].Content != "world" {
		t.Fatalf("expected only remaining text, got %q", events[0].Content)
	}
	if state.responseText() != "Hello world" {
		t.Fatalf("expected final response text, got %q", state.responseText())
	}
}

func TestCodexExecEnvStripsCodexParentContext(t *testing.T) {
	t.Setenv("CODEX_THREAD_ID", "thread-123")
	t.Setenv("CODEX_INTERNAL_ORIGINATOR_OVERRIDE", "Codex Desktop")
	t.Setenv("HOME", "/tmp/home")

	env := codexExecEnv("/tmp/agency-home")

	for _, entry := range env {
		if strings.HasPrefix(entry, "CODEX_THREAD_ID=") || strings.HasPrefix(entry, "CODEX_INTERNAL_ORIGINATOR_OVERRIDE=") {
			t.Fatalf("unexpected nested Codex env leaked into child process: %q", entry)
		}
	}

	foundHome := false
	for _, entry := range env {
		if entry == "HOME=/tmp/agency-home" {
			foundHome = true
			break
		}
	}
	if !foundHome {
		t.Fatal("expected HOME to be overridden for the child process")
	}
}

func TestCodexExecArgsDefaultToReadOnlySandbox(t *testing.T) {
	t.Setenv("AGENCY_CODEX_UNSANDBOXED", "")

	args := codexExecArgs("hello")
	joined := strings.Join(args, " ")

	if !strings.Contains(joined, "--sandbox read-only") {
		t.Fatalf("expected read-only sandbox args, got %q", joined)
	}
	if strings.Contains(joined, "--dangerously-bypass-approvals-and-sandbox") {
		t.Fatalf("dangerous bypass flag must not be enabled by default: %q", joined)
	}
	if args[len(args)-1] != "hello" {
		t.Fatalf("expected prompt as final arg, got %#v", args)
	}
}

func TestCodexExecArgsAllowExplicitUnsafeDeveloperMode(t *testing.T) {
	t.Setenv("AGENCY_CODEX_UNSANDBOXED", "true")

	args := codexExecArgs("hello")
	joined := strings.Join(args, " ")

	if !strings.Contains(joined, "--dangerously-bypass-approvals-and-sandbox") {
		t.Fatalf("expected explicit unsafe flag when opted in, got %q", joined)
	}
	if strings.Contains(joined, "--sandbox read-only") {
		t.Fatalf("unsafe opt-in should not also force read-only sandbox: %q", joined)
	}
}
