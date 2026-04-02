package dialog

import (
	"fmt"
	"strings"

	"github.com/ETEllis/teamcode/internal/tui/layout"
	"github.com/ETEllis/teamcode/internal/tui/styles"
	"github.com/ETEllis/teamcode/internal/tui/theme"
	"github.com/ETEllis/teamcode/internal/tui/util"
	"github.com/charmbracelet/bubbles/key"
	tea "github.com/charmbracelet/bubbletea"
	"github.com/charmbracelet/lipgloss"
)

type AgencyDialogData struct {
	ProductName         string
	CurrentConstitution string
	SoloConstitution    string
	OfficeMode          string
	ConsensusMode       string
	SharedWorkplace     string
	RedisAddress        string
	LedgerPath          string
	LastEvent           string
	Blueprint           string
	TeamTemplate        string
	Governance          string
	WorkspaceMode       string
	Running             bool
	DefaultQuorum       int
	TeamName            string
	Leader              string
	UnreadDirect        int
	UnreadBroadcasts    int
	PendingHandoffs     int
	MemberCount         int
	WorkerCount         int
	ActiveWorkerCount   int
	BoardReady          int
	BoardActive         int
	BoardReview         int
	BoardDone           int
	Thread              []string
	RequiredGates       []string
	Constitutions       []string
}

type AgencyDialog interface {
	tea.Model
	layout.Bindings
	SetData(AgencyDialogData)
}

type agencyDialogCmp struct {
	width  int
	height int
	data   AgencyDialogData
}

type agencyDialogKeyMap struct {
	Close key.Binding
}

var agencyDialogKeys = agencyDialogKeyMap{
	Close: key.NewBinding(
		key.WithKeys("esc"),
		key.WithHelp("esc", "close"),
	),
}

func (a *agencyDialogCmp) Init() tea.Cmd {
	return nil
}

func (a *agencyDialogCmp) SetData(data AgencyDialogData) {
	a.data = data
}

func (a *agencyDialogCmp) Update(msg tea.Msg) (tea.Model, tea.Cmd) {
	switch msg := msg.(type) {
	case tea.KeyMsg:
		if key.Matches(msg, agencyDialogKeys.Close) {
			return a, util.CmdHandler(CloseAgencyDialogMsg{})
		}
	case tea.WindowSizeMsg:
		a.width = msg.Width
		a.height = msg.Height
	}
	return a, nil
}

func (a *agencyDialogCmp) View() string {
	t := theme.CurrentTheme()
	baseStyle := styles.BaseStyle()
	contentWidth := max(56, min(88, a.width-18))

	title := baseStyle.
		Foreground(t.Primary()).
		Bold(true).
		Width(contentWidth).
		Render(emptyString(a.data.ProductName, "The Agency"))

	status := "stopped"
	if a.data.Running {
		status = "running"
	}

	lines := []string{
		baseStyle.Width(contentWidth).Render(fmt.Sprintf("Constitution: %s", emptyString(a.data.CurrentConstitution, a.data.SoloConstitution))),
		baseStyle.Width(contentWidth).Render(fmt.Sprintf("Office: %s (%s)", status, emptyString(a.data.OfficeMode, "runtime"))),
	}
	if a.data.TeamName != "" {
		lines = append(lines, baseStyle.Width(contentWidth).Render(fmt.Sprintf("Office name: %s", a.data.TeamName)))
	}
	if a.data.Governance != "" {
		lines = append(lines, baseStyle.Foreground(t.TextMuted()).Width(contentWidth).Render("Governance: "+a.data.Governance))
	}
	if a.data.ConsensusMode != "" {
		lines = append(lines, baseStyle.Foreground(t.TextMuted()).Width(contentWidth).Render("Consensus: "+a.data.ConsensusMode))
	}
	if a.data.Blueprint != "" {
		lines = append(lines, baseStyle.Foreground(t.TextMuted()).Width(contentWidth).Render("Blueprint: "+a.data.Blueprint))
	} else if a.data.TeamTemplate != "" {
		lines = append(lines, baseStyle.Foreground(t.TextMuted()).Width(contentWidth).Render("Blueprint: "+a.data.TeamTemplate))
	}
	if a.data.WorkspaceMode != "" {
		lines = append(lines, baseStyle.Foreground(t.TextMuted()).Width(contentWidth).Render("Workspace: "+a.data.WorkspaceMode))
	}
	if a.data.SharedWorkplace != "" {
		lines = append(lines, baseStyle.Foreground(t.TextMuted()).Width(contentWidth).Render("Shared workplace: "+a.data.SharedWorkplace))
	}
	if a.data.LedgerPath != "" {
		lines = append(lines, baseStyle.Foreground(t.TextMuted()).Width(contentWidth).Render("Ledger: "+a.data.LedgerPath))
	}
	if a.data.LastEvent != "" {
		lines = append(lines, baseStyle.Foreground(t.TextMuted()).Width(contentWidth).Render("Last event: "+a.data.LastEvent))
	}

	if a.data.TeamName != "" || a.data.MemberCount > 0 || a.data.WorkerCount > 0 {
		lines = append(lines, "")
		lines = append(lines, baseStyle.Foreground(t.TextMuted()).Bold(true).Width(contentWidth).Render("Organization"))
		lines = append(lines,
			baseStyle.Width(contentWidth).Render(fmt.Sprintf("Lead: %s", emptyString(a.data.Leader, "unassigned"))),
			baseStyle.Width(contentWidth).Render(fmt.Sprintf("Roster: %d  Workers: %d  Active: %d", a.data.MemberCount, a.data.WorkerCount, a.data.ActiveWorkerCount)),
			baseStyle.Foreground(t.TextMuted()).Width(contentWidth).Render(fmt.Sprintf("Signals: %d direct / %d broadcast / %d handoff", a.data.UnreadDirect, a.data.UnreadBroadcasts, a.data.PendingHandoffs)),
			baseStyle.Foreground(t.TextMuted()).Width(contentWidth).Render(fmt.Sprintf("Board: ready %d  active %d  review %d  done %d", a.data.BoardReady, a.data.BoardActive, a.data.BoardReview, a.data.BoardDone)),
		)
	}

	if len(a.data.Thread) > 0 {
		lines = append(lines, "")
		lines = append(lines, baseStyle.Foreground(t.TextMuted()).Bold(true).Width(contentWidth).Render("Orientation"))
		for _, line := range a.data.Thread {
			lines = append(lines, baseStyle.Foreground(t.TextMuted()).Width(contentWidth).Render("• "+line))
		}
	}

	if len(a.data.RequiredGates) > 0 || len(a.data.Constitutions) > 0 {
		lines = append(lines, "")
		lines = append(lines, baseStyle.Foreground(t.TextMuted()).Bold(true).Width(contentWidth).Render("Runtime"))
		if a.data.DefaultQuorum > 0 {
			lines = append(lines, baseStyle.Foreground(t.TextMuted()).Width(contentWidth).Render(fmt.Sprintf("Default quorum: %d", a.data.DefaultQuorum)))
		}
		if len(a.data.RequiredGates) > 0 {
			lines = append(lines, baseStyle.Foreground(t.TextMuted()).Width(contentWidth).Render("Gates: "+strings.Join(a.data.RequiredGates, ", ")))
		}
		if len(a.data.Constitutions) > 0 {
			lines = append(lines, baseStyle.Foreground(t.TextMuted()).Width(contentWidth).Render("Constitutions: "+strings.Join(a.data.Constitutions, ", ")))
		}
	}

	lines = append(lines, "")
	lines = append(lines, baseStyle.Foreground(t.TextMuted()).Width(contentWidth).Render("Commands: /agency status, /agency bootstrap, /agency stop, /agency genesis <intent>"))

	content := lipgloss.JoinVertical(lipgloss.Left, append([]string{title}, lines...)...)
	return baseStyle.Padding(1, 2).
		Border(lipgloss.RoundedBorder()).
		BorderBackground(t.Background()).
		BorderForeground(t.TextMuted()).
		Width(contentWidth + 4).
		Render(content)
}

func (a *agencyDialogCmp) BindingKeys() []key.Binding {
	return layout.KeyMapToSlice(agencyDialogKeys)
}

func emptyString(value, fallback string) string {
	if strings.TrimSpace(value) == "" {
		return fallback
	}
	return value
}

func NewAgencyDialogCmp() AgencyDialog {
	return &agencyDialogCmp{}
}
