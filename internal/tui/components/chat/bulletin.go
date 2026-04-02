package chat

import (
	"fmt"
	"strings"
	"time"

	"github.com/ETEllis/teamcode/internal/app"
	"github.com/ETEllis/teamcode/internal/tui/styles"
	"github.com/ETEllis/teamcode/internal/tui/theme"
	"github.com/charmbracelet/lipgloss"
)

// AgencyBulletinMsg is a tea message carrying a performance bulletin entry.
type AgencyBulletinMsg struct {
	app.BulletinRecord
}

// renderBulletinEntry renders a single directive→output→score bulletin entry
// in a compact two-line format suitable for appending to the messages viewport.
func renderBulletinEntry(b app.BulletinRecord, width int) string {
	t := theme.CurrentTheme()
	base := styles.BaseStyle()

	// Score bar: color shifts green→yellow→red based on score value.
	scoreColor := t.Success()
	if score := parseScore(b.Score); score < 0.5 {
		scoreColor = t.Error()
	} else if score < 0.75 {
		scoreColor = t.Warning()
	}

	actor := b.ActorID
	if actor == "" {
		actor = "agent"
	}

	ts := ""
	if b.CreatedAt > 0 {
		ts = time.UnixMilli(b.CreatedAt).Format("15:04:05")
	}

	// Header: BULLETIN ▸ actor  provider/model  ts
	modelTag := ""
	if b.Provider != "" {
		modelTag = b.Provider
		if b.ModelID != "" {
			modelTag += "/" + b.ModelID
		}
	}
	headerLeft := base.Bold(true).Foreground(t.Accent()).Render("BULLETIN") +
		base.Foreground(t.TextMuted()).Render(" ▸ ") +
		base.Foreground(t.Text()).Render(actor)
	headerRight := base.Foreground(t.TextMuted()).Render(modelTag + "  " + ts)
	gap := width - lipgloss.Width(headerLeft) - lipgloss.Width(headerRight)
	if gap < 1 {
		gap = 1
	}
	header := lipgloss.JoinHorizontal(lipgloss.Left,
		headerLeft,
		strings.Repeat(" ", gap),
		headerRight,
	)

	// Directive line.
	directive := b.Directive
	if directive == "" {
		directive = "(no directive)"
	}
	if lipgloss.Width(directive) > width-4 {
		directive = directive[:width-7] + "..."
	}
	directiveLine := base.
		Foreground(t.TextMuted()).
		Italic(true).
		Render("  ↳ " + directive)

	// Output line (truncated).
	output := b.Output
	if output == "" {
		output = "(no output)"
	}
	maxOut := width - 4
	if lipgloss.Width(output) > maxOut {
		output = output[:maxOut-3] + "..."
	}
	// Score badge.
	scoreLabel := fmt.Sprintf(" ⬟ %s ", b.Score)
	scoreBadge := base.
		Bold(true).
		Foreground(t.Background()).
		Background(scoreColor).
		Render(scoreLabel)
	outputWidth := width - lipgloss.Width(scoreBadge) - 2
	if outputWidth < 10 {
		outputWidth = 10
	}
	outputLine := lipgloss.JoinHorizontal(lipgloss.Left,
		base.Width(outputWidth).Foreground(t.Text()).Render("  "+output),
		scoreBadge,
	)

	boxStyle := base.
		Width(width).
		BorderLeft(true).
		BorderStyle(lipgloss.NormalBorder()).
		BorderForeground(t.Accent()).
		PaddingLeft(1)

	return boxStyle.Render(lipgloss.JoinVertical(lipgloss.Left,
		header,
		directiveLine,
		outputLine,
	))
}

// parseScore converts a "0.75" score string to float64.
func parseScore(s string) float64 {
	if s == "" {
		return 0
	}
	var f float64
	fmt.Sscanf(s, "%f", &f)
	return f
}
