package chat

import (
	"fmt"
	"strings"

	"github.com/ETEllis/teamcode/internal/app"
	"github.com/ETEllis/teamcode/internal/config"
	"github.com/ETEllis/teamcode/internal/llm/models"
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
		{Name: "/agency status", Description: "inspect live office state"},
		{Name: "/agency genesis <intent>", Description: "record a genesis brief"},
		{Name: "/model", Description: "switch model"},
		{Name: "/theme", Description: "switch theme"},
		{Name: "/skills", Description: "list installed skills"},
		{Name: "/team", Description: "legacy team controls"},
		{Name: "/exit", Description: "exit the app"},
	}
	if overview.Running {
		commands = append(commands[:4], append([]splashCommand{{Name: "/agency stop", Description: "stop the current office runtime"}}, commands[4:]...)...)
	} else {
		commands = append(commands[:4], append([]splashCommand{{Name: "/agency bootstrap", Description: "boot the current office constitution"}}, commands[4:]...)...)
	}
	return commands
}

func splashWordmark() string {
	return strings.Join([]string{
		" ________             ___                             ",
		"/_  __/ /_  ___      /   | ____ ____  ____  _______  ",
		" / / / __ \\/ _ \\    / /| |/ __ `/ _ \\/ __ \\/ ___/ /  ",
		"/ / / / / /  __/   / ___ / /_/ /  __/ / / / /__/ /   ",
		"/_/ /_/ /_/\\___/  /_/  |_\\__, /\\___/_/ /_/\\___/_/    ",
		"                        /____/                        ",
	}, "\n")
}

func splashVersion() string {
	return fmt.Sprintf("[ the agency | teamcode runtime ]  %s", version.Version)
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

	return lipgloss.JoinVertical(lipgloss.Left, lines...)
}

func renderSplash(appState *app.App, width, height int) string {
	t := theme.CurrentTheme()
	baseStyle := styles.BaseStyle()
	overview := InspectAgencyOverview(appState)

	logoStyle := baseStyle.
		Foreground(t.Secondary()).
		Bold(true)
	versionStyle := baseStyle.
		Foreground(t.TextMuted()).
		PaddingBottom(1)
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
		Render("Solo constitution is preserved. Agency office controls reflect live runtime state.")

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
		intro,
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
