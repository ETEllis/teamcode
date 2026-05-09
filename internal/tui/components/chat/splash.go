package chat

import (
	"bytes"
	"fmt"
	"os"
	"strings"

	"github.com/ETEllis/teamcode/internal/app"
	"github.com/ETEllis/teamcode/internal/config"
	"github.com/ETEllis/teamcode/internal/llm/models"
	"github.com/ETEllis/teamcode/internal/session"
	"github.com/ETEllis/teamcode/internal/tui/styles"
	"github.com/ETEllis/teamcode/internal/tui/theme"
	"github.com/ETEllis/teamcode/internal/version"
	"github.com/charmbracelet/lipgloss"
)

type splashCommand struct {
	Name        string
	Description string
}

func splashCommands(overview AgencyOverview) []splashCommand {
	commands := []splashCommand{
		{Name: "/help", Description: "show help"},
		{Name: "/sessions", Description: "list sessions"},
		{Name: "/new", Description: "start a new session"},
		{Name: "/agency status", Description: "check office status"},
		{Name: "/agency genesis <intent>", Description: "shape the office around a goal"},
		{Name: "/model", Description: "switch model"},
		{Name: "/theme", Description: "switch theme"},
		{Name: "/skills", Description: "list installed skills"},
		{Name: "/team", Description: "advanced office controls"},
		{Name: "/exit", Description: "exit the app"},
	}
	if overview.Running {
		commands = append(commands[:4], append([]splashCommand{{Name: "/agency stop", Description: "stop the office"}}, commands[4:]...)...)
	} else {
		commands = append(commands[:4], append([]splashCommand{{Name: "/agency bootstrap", Description: "start the office"}}, commands[4:]...)...)
	}
	return commands
}

func splashWordmark() string {
	return strings.Join([]string{
		"    ___    ______ ______ _   __ ______ __  __",
		"   /   |  / ____// ____// | / // ____// / / /",
		"  / /| | / / __ / __/  /  |/ // /    / /_/ / ",
		" / ___ |/ /_/ // /___ / /|  // /___ / __  /  ",
		"/_/  |_|\\____//_____//_/ |_/ \\____//_/ /_/   ",
	}, "\n")
}

func splashVersion() string {
	return fmt.Sprintf("[ local office / governed agents ]  %s", version.Version)
}

func splashModelLabel() string {
	cfg := config.Get()
	if cfg == nil {
		return "No model"
	}

	agentCfg, ok := cfg.Agents[config.AgentCoder]
	if !ok {
		return "No model"
	}

	model, ok := models.SupportedModels[agentCfg.Model]
	if !ok {
		return string(agentCfg.Model)
	}

	provider := strings.ToUpper(string(model.Provider[:1])) + string(model.Provider[1:])
	return fmt.Sprintf("%s %s", provider, model.Name)
}

type providerEntry struct {
	label  string
	envKey string
	local  bool
}

func splashProviderLines(width int) []string {
	t := theme.CurrentTheme()
	baseStyle := styles.BaseStyle()

	entries := []providerEntry{
		{label: "Codex", envKey: "", local: true}, // auth via ~/.codex/auth.json
		{label: "Anthropic", envKey: "ANTHROPIC_API_KEY"},
		{label: "OpenAI", envKey: "OPENAI_API_KEY"},
		{label: "Gemini", envKey: "GEMINI_API_KEY"},
		{label: "Ollama", envKey: "", local: true},
	}

	anyKey := false
	lines := []string{
		baseStyle.Foreground(t.TextMuted()).Bold(true).Width(width).Render("Providers"),
	}
	for _, e := range entries {
		var icon, detail string
		var fg lipgloss.TerminalColor
		if e.local {
			fg = t.TextMuted()
			switch e.label {
			case "Codex":
				home, _ := os.UserHomeDir()
				authFile := home + "/.codex/auth.json"
				if data, err := os.ReadFile(authFile); err == nil && bytes.Contains(data, []byte("access_token")) {
					icon = "✓"
					detail = "ChatGPT OAuth  (~/.codex/auth.json)"
					fg = t.Text()
					anyKey = true
				} else {
					icon = "✗"
					detail = "run: codex login"
				}
			case "Ollama":
				icon = "◌"
				ollamaModel := os.Getenv("OLLAMA_MODEL")
				if ollamaModel == "" {
					ollamaModel = "llama3.2"
				}
				detail = fmt.Sprintf("local — %s (ollama serve)", ollamaModel)
			default:
				icon = "◌"
				detail = "local"
			}
		} else if strings.TrimSpace(os.Getenv(e.envKey)) != "" {
			icon = "✓"
			detail = e.envKey
			if e.envKey == "OPENAI_API_KEY" {
				model := os.Getenv("OPENAI_MODEL")
				if model == "" {
					model = "gpt-4o-mini"
				}
				detail = fmt.Sprintf("%s  model=%s", e.envKey, model)
			}
			fg = t.Text()
			anyKey = true
		} else {
			icon = "✗"
			detail = e.envKey
			fg = t.TextMuted()
		}
		lines = append(lines, baseStyle.Foreground(fg).Width(width).Render(
			fmt.Sprintf("  %s %-12s  %s", icon, e.label, detail),
		))
	}
	if !anyKey {
		lines = append(lines, "")
		lines = append(lines, baseStyle.Foreground(t.Warning()).Width(width).Render(
			"  No provider is ready yet",
		))
		lines = append(lines, baseStyle.Foreground(t.TextMuted()).Width(width).Render(
			"  Run: scripts/setup  to connect one",
		))
	}
	return lines
}

func splashState(overview AgencyOverview, width int) string {
	t := theme.CurrentTheme()
	baseStyle := styles.BaseStyle()

	lines := []string{
		baseStyle.Width(width).Render(fmt.Sprintf("Constitution: %s", emptyFallback(overview.CurrentConstitution, overview.SoloConstitution))),
	}

	if overview.HasOfficeState {
		status := "stopped"
		if overview.Running {
			status = "running"
		}
		lines = append(lines, baseStyle.Width(width).Render(fmt.Sprintf("Office: %s (%s)", status, emptyFallback(overview.OfficeMode, "runtime"))))
	}

	if overview.TeamName != "" {
		lines = append(lines,
			baseStyle.Width(width).Render(fmt.Sprintf("Office name: %s", overview.TeamName)),
			baseStyle.Width(width).Render(fmt.Sprintf("Lead: %s", emptyFallback(overview.Leader, "unassigned"))),
		)
	}

	if overview.HasTeamSnapshot {
		lines = append(lines,
			baseStyle.Foreground(t.TextMuted()).Width(width).Render(fmt.Sprintf("Signals: %d direct / %d broadcast / %d handoff", overview.UnreadDirect, overview.UnreadBroadcasts, overview.PendingHandoffs)),
			baseStyle.Foreground(t.TextMuted()).Width(width).Render(fmt.Sprintf("Board: ready %d  active %d  review %d  done %d",
				overview.BoardSummary["ready"],
				overview.BoardSummary["in_progress"],
				overview.BoardSummary["in_review"],
				overview.BoardSummary["done"],
			)),
		)
	}

	if len(overview.Thread) > 0 {
		lines = append(lines, "")
		lines = append(lines, baseStyle.Foreground(t.TextMuted()).Bold(true).Width(width).Render("Orientation"))
		for _, line := range overview.Thread {
			lines = append(lines, baseStyle.Foreground(t.TextMuted()).Width(width).Render("• "+line))
		}
	}

	lines = append(lines, "")
	lines = append(lines, splashProviderLines(width)...)

	return lipgloss.JoinVertical(lipgloss.Left, lines...)
}

func renderSplash(appState *app.App, activeSession session.Session, width, height int) string {
	t := theme.CurrentTheme()
	baseStyle := styles.BaseStyle()
	overview := InspectAgencyOverview(appState)

	logoStyle := baseStyle.
		Foreground(t.Primary()).
		Bold(true)
	versionStyle := baseStyle.
		Foreground(t.TextMuted()).
		PaddingBottom(1)
	brandLineStyle := baseStyle.
		Foreground(t.Secondary()).
		Bold(true)
	commandStyle := baseStyle.Foreground(t.TextMuted())
	commandNameStyle := baseStyle.Foreground(t.Text()).Bold(true)

	commandRows := make([]string, 0, len(splashCommands(overview)))
	for _, command := range splashCommands(overview) {
		commandRows = append(commandRows, lipgloss.JoinHorizontal(
			lipgloss.Left,
			commandNameStyle.Render(command.Name),
			commandStyle.Render(" "+command.Description),
		))
	}

	intro := baseStyle.
		Foreground(t.TextMuted()).
		Align(lipgloss.Center).
		Render("Tell Agency what needs building, fixing, reviewing, or untangling. Use /agency when you want the staffed office, ledger, approvals, and scheduled roles.")

	brandLine := brandLineStyle.
		Align(lipgloss.Center).
		Render("signal gold / relay cyan / ledger ink")

	sessionStatus := ""
	if activeSession.ID != "" {
		label := activeSession.Title
		if strings.TrimSpace(label) == "" {
			label = "New Session"
		}
		sessionStatus = styles.BaseStyle().
			Foreground(t.Primary()).
			Bold(true).
			Align(lipgloss.Center).
			Render("Fresh session ready: " + label)
	}

	stateWidth := min(max(44, width/2), 74)
	stateCard := styles.BaseStyle().
		Width(stateWidth).
		Padding(1, 2).
		Border(lipgloss.RoundedBorder()).
		BorderForeground(t.BorderDim()).
		Render(splashState(overview, stateWidth-4))

	content := lipgloss.JoinVertical(
		lipgloss.Center,
		logoStyle.Render(splashWordmark()),
		versionStyle.Render(splashVersion()),
		brandLine,
		intro,
		sessionStatus,
		"",
		stateCard,
		"",
		lipgloss.JoinVertical(lipgloss.Center, commandRows...),
	)

	return lipgloss.Place(
		width,
		max(0, height),
		lipgloss.Center,
		lipgloss.Center,
		content,
		lipgloss.WithWhitespaceBackground(t.Background()),
	)
}
