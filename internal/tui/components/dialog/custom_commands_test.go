package dialog

import (
	"os"
	"path/filepath"
	"regexp"
	"strings"
	"testing"
)

func TestNamedArgPattern(t *testing.T) {
	testCases := []struct {
		input    string
		expected []string
	}{
		{
			input:    "This is a test with $ARGUMENTS placeholder",
			expected: []string{"ARGUMENTS"},
		},
		{
			input:    "This is a test with $FOO and $BAR placeholders",
			expected: []string{"FOO", "BAR"},
		},
		{
			input:    "This is a test with $FOO_BAR and $BAZ123 placeholders",
			expected: []string{"FOO_BAR", "BAZ123"},
		},
		{
			input:    "This is a test with no placeholders",
			expected: []string{},
		},
		{
			input:    "This is a test with $FOO appearing twice: $FOO",
			expected: []string{"FOO"},
		},
		{
			input:    "This is a test with $1INVALID placeholder",
			expected: []string{},
		},
	}

	for _, tc := range testCases {
		matches := namedArgPattern.FindAllStringSubmatch(tc.input, -1)

		// Extract unique argument names
		argNames := make([]string, 0)
		argMap := make(map[string]bool)

		for _, match := range matches {
			argName := match[1] // Group 1 is the name without $
			if !argMap[argName] {
				argMap[argName] = true
				argNames = append(argNames, argName)
			}
		}

		// Check if we got the expected number of arguments
		if len(argNames) != len(tc.expected) {
			t.Errorf("Expected %d arguments, got %d for input: %s", len(tc.expected), len(argNames), tc.input)
			continue
		}

		// Check if we got the expected argument names
		for _, expectedArg := range tc.expected {
			found := false
			for _, actualArg := range argNames {
				if actualArg == expectedArg {
					found = true
					break
				}
			}
			if !found {
				t.Errorf("Expected argument %s not found in %v for input: %s", expectedArg, argNames, tc.input)
			}
		}
	}
}

func TestRegexPattern(t *testing.T) {
	pattern := regexp.MustCompile(`\$([A-Z][A-Z0-9_]*)`)

	validMatches := []string{
		"$FOO",
		"$BAR",
		"$FOO_BAR",
		"$BAZ123",
		"$ARGUMENTS",
	}

	invalidMatches := []string{
		"$foo",
		"$1BAR",
		"$_FOO",
		"FOO",
		"$",
	}

	for _, valid := range validMatches {
		if !pattern.MatchString(valid) {
			t.Errorf("Expected %s to match, but it didn't", valid)
		}
	}

	for _, invalid := range invalidMatches {
		if pattern.MatchString(invalid) {
			t.Errorf("Expected %s not to match, but it did", invalid)
		}
	}
}

func TestCandidateCommandDirsIncludesLegacySingularPaths(t *testing.T) {
	got := candidateCommandDirs("/tmp/config", "teamcode", "opencode")
	want := []string{
		"/tmp/config/teamcode/commands",
		"/tmp/config/teamcode/command",
		"/tmp/config/opencode/commands",
		"/tmp/config/opencode/command",
	}

	if len(got) != len(want) {
		t.Fatalf("expected %d paths, got %d: %v", len(want), len(got), got)
	}

	for i, expected := range want {
		if got[i] != expected {
			t.Fatalf("path %d mismatch: expected %q, got %q", i, expected, got[i])
		}
	}
}

func TestExpandEnvironmentReferencesPreservesUnknownPlaceholders(t *testing.T) {
	home := t.TempDir()
	oldHome, hadHome := os.LookupEnv("HOME")
	if err := os.Setenv("HOME", home); err != nil {
		t.Fatalf("set HOME: %v", err)
	}
	defer func() {
		if hadHome {
			_ = os.Setenv("HOME", oldHome)
			return
		}
		_ = os.Unsetenv("HOME")
	}()

	got := expandEnvironmentReferences("@$HOME/workflows/help.md\n$TASK\n$MISSING")
	want := "@" + home + "/workflows/help.md\n$TASK\n$MISSING"
	if got != want {
		t.Fatalf("expected %q, got %q", want, got)
	}
}

func TestExpandEnvironmentReferencesFallsBackToUserHomeDir(t *testing.T) {
	oldHome, hadHome := os.LookupEnv("HOME")
	if err := os.Unsetenv("HOME"); err != nil {
		t.Fatalf("unset HOME: %v", err)
	}
	defer func() {
		if hadHome {
			_ = os.Setenv("HOME", oldHome)
			return
		}
		_ = os.Unsetenv("HOME")
	}()

	home, err := currentUserHomeDir()
	if err != nil {
		t.Fatalf("user home dir: %v", err)
	}

	got := expandEnvironmentReferences("@$HOME/workflows/help.md")
	want := "@" + home + "/workflows/help.md"
	if got != want {
		t.Fatalf("expected %q, got %q", want, got)
	}
}

func TestExtractCommandFrontmatter(t *testing.T) {
	content := "---\ndescription: Demo command\nargument-hint: [task]\n---\nbody"

	meta, body := extractCommandFrontmatter(content)
	if meta.Description != "Demo command" {
		t.Fatalf("expected description to be parsed, got %q", meta.Description)
	}
	if meta.ArgumentHint != "[task]" {
		t.Fatalf("expected argument hint to be parsed, got %q", meta.ArgumentHint)
	}
	if body != "body" {
		t.Fatalf("expected body %q, got %q", "body", body)
	}
}

func TestRenderCommandPromptInlinesExecutionContext(t *testing.T) {
	dir := t.TempDir()
	reference := filepath.Join(dir, "help.md")
	if err := os.WriteFile(reference, []byte("# Help\nRun the listed commands."), 0o644); err != nil {
		t.Fatalf("write reference: %v", err)
	}

	content := strings.Join([]string{
		"---",
		"description: Demo help",
		"---",
		"<objective>",
		"Display the help reference.",
		"</objective>",
		"",
		"<execution_context>",
		"@" + reference,
		"</execution_context>",
		"",
		"<process>",
		"Output the reference directly.",
		"</process>",
	}, "\n")

	got := renderCommandPrompt(content, filepath.Join(dir, "demo.md"), "/demo help", "help")
	if strings.Contains(got, "<objective>") {
		t.Fatalf("expected legacy tags to be resolved, got %q", got)
	}
	if !strings.Contains(got, "User invocation: /demo help") {
		t.Fatalf("expected invocation context, got %q", got)
	}
	if !strings.Contains(got, "# Help") {
		t.Fatalf("expected referenced content to be inlined, got %q", got)
	}
}

func TestLoadCommandsFromDirUsesFrontmatterDescriptionAndInvocationArgs(t *testing.T) {
	dir := t.TempDir()
	commandPath := filepath.Join(dir, "debug.md")
	content := strings.Join([]string{
		"---",
		"description: Debug issue",
		"---",
		"Investigate: $ARGUMENTS",
	}, "\n")
	if err := os.WriteFile(commandPath, []byte(content), 0o644); err != nil {
		t.Fatalf("write command: %v", err)
	}

	commands, err := loadCommandsFromDir(dir, UserCommandPrefix)
	if err != nil {
		t.Fatalf("load commands: %v", err)
	}
	if len(commands) != 1 {
		t.Fatalf("expected 1 command, got %d", len(commands))
	}
	if commands[0].Description != "Debug issue" {
		t.Fatalf("expected frontmatter description, got %q", commands[0].Description)
	}

	command := commands[0]
	command.Invocation = "/debug failing build"
	command.ArgsText = "failing build"
	msg := command.Handler(command)()
	runMsg, ok := msg.(CommandRunCustomMsg)
	if !ok {
		t.Fatalf("expected CommandRunCustomMsg, got %T", msg)
	}
	if strings.Contains(runMsg.Content, "$ARGUMENTS") {
		t.Fatalf("expected invocation args to be applied, got %q", runMsg.Content)
	}
	if !strings.Contains(runMsg.Content, "failing build") {
		t.Fatalf("expected invocation args in command content, got %q", runMsg.Content)
	}
}

func TestLoadCommandsFromDirExpandsEnvReferencesAfterInliningContext(t *testing.T) {
	dir := t.TempDir()
	home := t.TempDir()
	oldHome, hadHome := os.LookupEnv("HOME")
	if err := os.Setenv("HOME", home); err != nil {
		t.Fatalf("set HOME: %v", err)
	}
	defer func() {
		if hadHome {
			_ = os.Setenv("HOME", oldHome)
			return
		}
		_ = os.Unsetenv("HOME")
	}()

	reference := filepath.Join(dir, "reference.md")
	if err := os.WriteFile(reference, []byte("Path: $HOME/docs"), 0o644); err != nil {
		t.Fatalf("write reference: %v", err)
	}

	commandPath := filepath.Join(dir, "help.md")
	content := strings.Join([]string{
		"---",
		"description: Help",
		"---",
		"<execution_context>",
		"@" + reference,
		"</execution_context>",
	}, "\n")
	if err := os.WriteFile(commandPath, []byte(content), 0o644); err != nil {
		t.Fatalf("write command: %v", err)
	}

	commands, err := loadCommandsFromDir(dir, UserCommandPrefix)
	if err != nil {
		t.Fatalf("load commands: %v", err)
	}

	msg := commands[0].Handler(commands[0])()
	runMsg, ok := msg.(CommandRunCustomMsg)
	if !ok {
		t.Fatalf("expected CommandRunCustomMsg, got %T", msg)
	}
	if strings.Contains(runMsg.Content, "$HOME") {
		t.Fatalf("expected HOME to be expanded after inlining, got %q", runMsg.Content)
	}
	if !strings.Contains(runMsg.Content, filepath.Join(home, "docs")) {
		t.Fatalf("expected expanded HOME path in command content, got %q", runMsg.Content)
	}
}
