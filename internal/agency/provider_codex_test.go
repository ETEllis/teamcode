package agency

import (
	"strings"
	"testing"
)

func TestAgencyCodexExecArgsDefaultToReadOnlySandbox(t *testing.T) {
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

func TestAgencyCodexExecArgsAllowExplicitUnsafeDeveloperMode(t *testing.T) {
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
