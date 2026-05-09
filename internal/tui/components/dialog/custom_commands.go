package dialog

import (
	"fmt"
	"os"
	"os/user"
	"path/filepath"
	"regexp"
	"strings"

	"github.com/ETEllis/teamcode/internal/config"
	"github.com/ETEllis/teamcode/internal/tui/util"
	tea "github.com/charmbracelet/bubbletea"
)

// Command prefix constants
const (
	UserCommandPrefix    = "user:"
	ProjectCommandPrefix = "project:"
	SkillCommandPrefix   = "skill:"
)

// namedArgPattern is a regex pattern to find named arguments in the format $NAME
var namedArgPattern = regexp.MustCompile(`\$([A-Z][A-Z0-9_]*)`)

// LoadCustomCommands loads custom commands from both XDG_CONFIG_HOME and project data directory
func LoadCustomCommands() ([]Command, error) {
	cfg := config.Get()
	if cfg == nil {
		return nil, fmt.Errorf("config not loaded")
	}

	var commands []Command
	seen := map[string]bool{}

	appendCommands := func(next []Command) {
		for _, command := range next {
			if seen[command.ID] {
				continue
			}
			seen[command.ID] = true
			commands = append(commands, command)
		}
	}

	// Load user commands from XDG_CONFIG_HOME/teamcode/commands first, then legacy opencode paths.
	xdgConfigHome := os.Getenv("XDG_CONFIG_HOME")
	if xdgConfigHome == "" {
		// Default to ~/.config if XDG_CONFIG_HOME is not set
		home, err := os.UserHomeDir()
		if err == nil {
			xdgConfigHome = filepath.Join(home, ".config")
		}
	}

	if xdgConfigHome != "" {
		userCommandsDirs := candidateCommandDirs(xdgConfigHome, "teamcode", "opencode")
		for _, userCommandsDir := range userCommandsDirs {
			userCommands, err := loadCommandsFromDir(userCommandsDir, UserCommandPrefix)
			if err != nil {
				fmt.Printf("Warning: failed to load user commands from %s: %v\n", userCommandsDir, err)
				continue
			}
			appendCommands(userCommands)
		}
	}

	// Load commands from $HOME/.teamcode/commands first, then legacy $HOME/.opencode/commands.
	home, err := os.UserHomeDir()
	if err == nil {
		homeCommandsDirs := candidateCommandDirs(home, ".teamcode", ".opencode")
		for _, homeCommandsDir := range homeCommandsDirs {
			homeCommands, err := loadCommandsFromDir(homeCommandsDir, UserCommandPrefix)
			if err != nil {
				fmt.Printf("Warning: failed to load home commands from %s: %v\n", homeCommandsDir, err)
				continue
			}
			appendCommands(homeCommands)
		}
	}

	// Load project commands from data directory
	projectCommandsDir := filepath.Join(cfg.Data.Directory, "commands")
	projectCommands, err := loadCommandsFromDir(projectCommandsDir, ProjectCommandPrefix)
	if err != nil {
		// Log error but return what we have so far
		fmt.Printf("Warning: failed to load project commands: %v\n", err)
	} else {
		appendCommands(projectCommands)
	}

	skillCommands, err := loadSkillCommands()
	if err != nil {
		fmt.Printf("Warning: failed to load skills: %v\n", err)
	} else {
		appendCommands(skillCommands)
	}

	return commands, nil
}

func candidateCommandDirs(root string, appNames ...string) []string {
	var dirs []string
	seen := map[string]bool{}
	for _, appName := range appNames {
		for _, dirName := range []string{"commands", "command"} {
			path := filepath.Join(root, appName, dirName)
			if seen[path] {
				continue
			}
			seen[path] = true
			dirs = append(dirs, path)
		}
	}
	return dirs
}

func loadSkillCommands() ([]Command, error) {
	roots, err := skillRoots()
	if err != nil {
		return nil, err
	}

	var commands []Command
	seen := map[string]bool{}
	for _, root := range roots {
		err := filepath.Walk(root, func(path string, info os.FileInfo, err error) error {
			if err != nil {
				return err
			}
			if info.IsDir() || info.Name() != "SKILL.md" {
				return nil
			}

			skillDir := filepath.Dir(path)
			skillName := filepath.Base(skillDir)
			if seen[skillName] {
				return nil
			}
			seen[skillName] = true

			content := fmt.Sprintf("Use the skill defined at %s for this task. Read the skill first, then help with:\n$TASK", path)
			commands = append(commands, Command{
				ID:          SkillCommandPrefix + skillName,
				Title:       SkillCommandPrefix + skillName,
				Description: fmt.Sprintf("Installed skill from %s", path),
				Handler: func(cmd Command) tea.Cmd {
					return util.CmdHandler(ShowMultiArgumentsDialogMsg{
						CommandID: cmd.ID,
						Content:   content,
						ArgNames:  []string{"TASK"},
					})
				},
			})
			return nil
		})
		if err != nil {
			return nil, err
		}
	}

	return commands, nil
}

func skillRoots() ([]string, error) {
	var roots []string
	seen := map[string]bool{}
	appendRoot := func(path string) {
		path = strings.TrimSpace(path)
		if path == "" || seen[path] {
			return
		}
		if info, err := os.Stat(path); err == nil && info.IsDir() {
			seen[path] = true
			roots = append(roots, path)
		}
	}

	if codexHome := os.Getenv("CODEX_HOME"); codexHome != "" {
		appendRoot(filepath.Join(codexHome, "skills"))
	}

	home, err := os.UserHomeDir()
	if err != nil {
		return nil, err
	}
	appendRoot(filepath.Join(home, ".codex", "skills"))
	appendRoot(filepath.Join(home, ".agents", "skills"))
	return roots, nil
}

// loadCommandsFromDir loads commands from a specific directory with the given prefix
func loadCommandsFromDir(commandsDir string, prefix string) ([]Command, error) {
	// Check if the commands directory exists
	if _, err := os.Stat(commandsDir); os.IsNotExist(err) {
		// Create the commands directory if it doesn't exist
		if err := os.MkdirAll(commandsDir, 0755); err != nil {
			return nil, fmt.Errorf("failed to create commands directory %s: %w", commandsDir, err)
		}
		// Return empty list since we just created the directory
		return []Command{}, nil
	}

	var commands []Command

	// Walk through the commands directory and load all .md files
	err := filepath.Walk(commandsDir, func(path string, info os.FileInfo, err error) error {
		if err != nil {
			return err
		}

		// Skip directories
		if info.IsDir() {
			return nil
		}

		// Only process markdown files
		if !strings.HasSuffix(strings.ToLower(info.Name()), ".md") {
			return nil
		}

		// Read the file content
		content, err := os.ReadFile(path)
		if err != nil {
			return fmt.Errorf("failed to read command file %s: %w", path, err)
		}
		expandedContent := expandEnvironmentReferences(string(content))
		metadata, _ := extractCommandFrontmatter(expandedContent)

		// Get the command ID from the file name without the .md extension
		commandID := strings.TrimSuffix(info.Name(), filepath.Ext(info.Name()))

		// Get relative path from commands directory
		relPath, err := filepath.Rel(commandsDir, path)
		if err != nil {
			return fmt.Errorf("failed to get relative path for %s: %w", path, err)
		}

		// Create the command ID from the relative path
		// Replace directory separators with colons
		commandIDPath := strings.ReplaceAll(filepath.Dir(relPath), string(filepath.Separator), ":")
		if commandIDPath != "." {
			commandID = commandIDPath + ":" + commandID
		}

		// Create a command
		command := Command{
			ID:          prefix + commandID,
			Title:       prefix + commandID,
			Description: commandDescription(metadata.Description, relPath),
			Handler: func(cmd Command) tea.Cmd {
				commandContent := renderCommandPrompt(expandedContent, path, cmd.Invocation, cmd.ArgsText)
				commandContent = applyInvocationDefaults(commandContent, cmd.ArgsText)
				commandContent = expandEnvironmentReferences(commandContent)

				// Check for named arguments
				matches := namedArgPattern.FindAllStringSubmatch(commandContent, -1)
				if len(matches) > 0 {
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

					// Show multi-arguments dialog for all named arguments
					return util.CmdHandler(ShowMultiArgumentsDialogMsg{
						CommandID: cmd.ID,
						Content:   commandContent,
						ArgNames:  argNames,
					})
				}

				// No arguments needed, run command directly
				return util.CmdHandler(CommandRunCustomMsg{
					Content: commandContent,
					Args:    nil, // No arguments
				})
			},
		}

		commands = append(commands, command)
		return nil
	})

	if err != nil {
		return nil, fmt.Errorf("failed to load custom commands from %s: %w", commandsDir, err)
	}

	return commands, nil
}

func expandEnvironmentReferences(content string) string {
	return os.Expand(content, func(name string) string {
		if value, ok := os.LookupEnv(name); ok {
			return value
		}
		switch name {
		case "HOME":
			if home, err := currentUserHomeDir(); err == nil {
				return home
			}
		case "XDG_CONFIG_HOME":
			if configDir, err := currentUserConfigDir(); err == nil {
				return configDir
			}
		}
		return "$" + name
	})
}

func currentUserHomeDir() (string, error) {
	if home, ok := os.LookupEnv("HOME"); ok && strings.TrimSpace(home) != "" {
		return home, nil
	}
	current, err := user.Current()
	if err != nil || strings.TrimSpace(current.HomeDir) == "" {
		return "", fmt.Errorf("home directory unavailable")
	}
	return current.HomeDir, nil
}

func currentUserConfigDir() (string, error) {
	if configDir, ok := os.LookupEnv("XDG_CONFIG_HOME"); ok && strings.TrimSpace(configDir) != "" {
		return configDir, nil
	}
	home, err := currentUserHomeDir()
	if err != nil {
		return "", err
	}
	return filepath.Join(home, ".config"), nil
}

type commandMetadata struct {
	Description  string
	ArgumentHint string
}

func commandDescription(description, relPath string) string {
	description = strings.TrimSpace(description)
	if description != "" {
		return description
	}
	return fmt.Sprintf("Custom command from %s", relPath)
}

func extractCommandFrontmatter(content string) (commandMetadata, string) {
	var metadata commandMetadata
	normalized := strings.ReplaceAll(content, "\r\n", "\n")
	if !strings.HasPrefix(normalized, "---\n") {
		return metadata, content
	}

	rest := normalized[len("---\n"):]
	end := strings.Index(rest, "\n---\n")
	endLen := len("\n---\n")
	if end == -1 {
		if strings.HasSuffix(rest, "\n---") {
			end = len(rest) - len("\n---")
			endLen = len("\n---")
		} else {
			return metadata, content
		}
	}

	for _, line := range strings.Split(rest[:end], "\n") {
		key, value, ok := strings.Cut(line, ":")
		if !ok {
			continue
		}
		key = strings.TrimSpace(strings.ToLower(key))
		value = strings.Trim(strings.TrimSpace(value), `"'`)
		switch key {
		case "description":
			metadata.Description = value
		case "argument-hint":
			metadata.ArgumentHint = value
		}
	}

	return metadata, rest[end+endLen:]
}

func renderCommandPrompt(content, sourcePath, invocation, argsText string) string {
	_, body := extractCommandFrontmatter(content)
	body = strings.TrimSpace(body)
	if !hasLegacyCommandSpec(body) {
		return wrapCommandPrompt(body, sourcePath, invocation, argsText)
	}

	objective := commandSection(body, "objective")
	context := commandSection(body, "context")
	executionContext := resolveExecutionContext(commandSection(body, "execution_context"), sourcePath)
	process := commandSection(body, "process")
	successCriteria := commandSection(body, "success_criteria")

	lines := commandInvocationHeader(sourcePath, invocation, argsText)
	lines = append(lines, "Follow the command specification below exactly.", "")
	if objective != "" {
		lines = append(lines, "Objective:", objective, "")
	}
	if context != "" {
		lines = append(lines, "Context:", context, "")
	}
	if executionContext != "" {
		lines = append(lines, "Referenced context:", executionContext, "")
	}
	if process != "" {
		lines = append(lines, "Process:", process, "")
	}
	if successCriteria != "" {
		lines = append(lines, "Success criteria:", successCriteria, "")
	}

	return strings.TrimSpace(strings.Join(lines, "\n"))
}

func hasLegacyCommandSpec(content string) bool {
	for _, name := range []string{"objective", "context", "execution_context", "process", "success_criteria"} {
		if commandSection(content, name) != "" {
			return true
		}
	}
	return false
}

func commandSection(content, name string) string {
	pattern := fmt.Sprintf(`(?s)<%s>\s*(.*?)\s*</%s>`, regexp.QuoteMeta(name), regexp.QuoteMeta(name))
	match := regexp.MustCompile(pattern).FindStringSubmatch(content)
	if len(match) < 2 {
		return ""
	}
	return strings.TrimSpace(match[1])
}

func resolveExecutionContext(section, sourcePath string) string {
	section = strings.TrimSpace(section)
	if section == "" {
		return ""
	}

	var blocks []string
	for _, line := range strings.Split(section, "\n") {
		ref := strings.TrimSpace(line)
		if !strings.HasPrefix(ref, "@") {
			if ref != "" {
				blocks = append(blocks, ref)
			}
			continue
		}
		resolvedPath := resolveCommandReferencePath(strings.TrimPrefix(ref, "@"), sourcePath)
		content, err := os.ReadFile(resolvedPath)
		if err != nil {
			blocks = append(blocks, fmt.Sprintf("Reference: %s\n[unable to read: %v]", resolvedPath, err))
			continue
		}
		blocks = append(blocks, fmt.Sprintf("Reference: %s\n%s", resolvedPath, strings.TrimSpace(string(content))))
	}
	return strings.TrimSpace(strings.Join(blocks, "\n\n"))
}

func resolveCommandReferencePath(reference, sourcePath string) string {
	reference = strings.TrimSpace(reference)
	if filepath.IsAbs(reference) {
		return reference
	}
	if strings.HasPrefix(reference, ".") {
		return filepath.Join(config.WorkingDirectory(), reference)
	}
	if sourcePath == "" {
		return reference
	}
	return filepath.Join(filepath.Dir(sourcePath), reference)
}

func wrapCommandPrompt(body, sourcePath, invocation, argsText string) string {
	lines := commandInvocationHeader(sourcePath, invocation, argsText)
	if strings.TrimSpace(body) != "" {
		lines = append(lines, "", strings.TrimSpace(body))
	}
	return strings.TrimSpace(strings.Join(lines, "\n"))
}

func commandInvocationHeader(sourcePath, invocation, argsText string) []string {
	lines := []string{"You are executing an Agency custom command."}
	if sourcePath != "" {
		lines = append(lines, "Command file: "+sourcePath)
	}
	if invocation != "" {
		lines = append(lines, "User invocation: "+invocation)
	}
	if argsText != "" {
		lines = append(lines, "Trailing args: "+argsText)
	}
	return lines
}

func applyInvocationDefaults(content, argsText string) string {
	if strings.TrimSpace(argsText) == "" {
		return content
	}
	return strings.ReplaceAll(content, "$ARGUMENTS", argsText)
}

// CommandRunCustomMsg is sent when a custom command is executed
type CommandRunCustomMsg struct {
	Content string
	Args    map[string]string // Map of argument names to values
}
