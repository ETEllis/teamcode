package chat

import (
	"context"
	"fmt"
	"sort"
	"strings"

	"github.com/ETEllis/teamcode/internal/app"
	"github.com/ETEllis/teamcode/internal/config"
	"github.com/ETEllis/teamcode/internal/diff"
	"github.com/ETEllis/teamcode/internal/history"
	"github.com/ETEllis/teamcode/internal/llm/models"
	"github.com/ETEllis/teamcode/internal/pubsub"
	"github.com/ETEllis/teamcode/internal/session"
	"github.com/ETEllis/teamcode/internal/tui/styles"
	"github.com/ETEllis/teamcode/internal/tui/theme"
	tea "github.com/charmbracelet/bubbletea"
	"github.com/charmbracelet/lipgloss"

	"github.com/ETEllis/teamcode/internal/orchestration"
	"github.com/ETEllis/teamcode/internal/team"
)

type sidebarCmp struct {
	width, height int
	session       session.Session
	history       history.Service
	agency        *app.AgencyService
	team          *team.Service
	workers       *orchestration.Manager
	modFiles      map[string]struct {
		additions int
		removals  int
	}
}

func (m *sidebarCmp) Init() tea.Cmd {
	if m.history != nil {
		ctx := context.Background()
		// Subscribe to file events
		filesCh := m.history.Subscribe(ctx)

		// Initialize the modified files map
		m.modFiles = make(map[string]struct {
			additions int
			removals  int
		})

		// Load initial files and calculate diffs
		m.loadModifiedFiles(ctx)

		// Return a command that will send file events to the Update method
		return func() tea.Msg {
			return <-filesCh
		}
	}
	return nil
}

func (m *sidebarCmp) Update(msg tea.Msg) (tea.Model, tea.Cmd) {
	switch msg := msg.(type) {
	case SessionSelectedMsg:
		if msg.ID != m.session.ID {
			m.session = msg
			ctx := context.Background()
			m.loadModifiedFiles(ctx)
		}
	case pubsub.Event[session.Session]:
		if msg.Type == pubsub.UpdatedEvent {
			if m.session.ID == msg.Payload.ID {
				m.session = msg.Payload
			}
		}
	case pubsub.Event[history.File]:
		if msg.Payload.SessionID == m.session.ID {
			// Process the individual file change instead of reloading all files
			ctx := context.Background()
			m.processFileChanges(ctx, msg.Payload)

			// Return a command to continue receiving events
			return m, func() tea.Msg {
				ctx := context.Background()
				filesCh := m.history.Subscribe(ctx)
				return <-filesCh
			}
		}
	}
	return m, nil
}

func (m *sidebarCmp) View() string {
	baseStyle := styles.BaseStyle()

	return baseStyle.
		Width(m.width).
		PaddingLeft(4).
		PaddingRight(2).
		Height(m.height - 1).
		Render(
			lipgloss.JoinVertical(
				lipgloss.Top,
				header(m.width),
				" ",
				m.sessionSection(),
				" ",
				m.providerSection(),
				" ",
				m.agencySection(),
				" ",
				lspsConfigured(m.width),
				" ",
				m.modifiedFiles(),
			),
		)
}

func (m *sidebarCmp) sessionSection() string {
	t := theme.CurrentTheme()
	baseStyle := styles.BaseStyle()

	sessionKey := baseStyle.
		Foreground(t.Primary()).
		Bold(true).
		Render("Session")

	sessionValue := baseStyle.
		Foreground(t.Text()).
		Width(m.width - lipgloss.Width(sessionKey)).
		Render(fmt.Sprintf(": %s", m.session.Title))

	return lipgloss.JoinHorizontal(
		lipgloss.Left,
		sessionKey,
		sessionValue,
	)
}

func (m *sidebarCmp) providerSection() string {
	cfg := config.Get()
	t := theme.CurrentTheme()
	baseStyle := styles.BaseStyle()

	title := baseStyle.
		Width(m.width).
		Foreground(t.Primary()).
		Bold(true).
		Render("Models")

	if cfg == nil {
		return lipgloss.JoinVertical(lipgloss.Left, title)
	}

	current := "Unavailable"
	if agentCfg, ok := cfg.Agents[config.AgentCoder]; ok {
		if model, ok := models.SupportedModels[agentCfg.Model]; ok {
			current = fmt.Sprintf("%s (%s)", model.Name, model.Provider)
		}
	}

	enabledProviders := make([]string, 0)
	for provider, providerCfg := range cfg.Providers {
		if providerCfg.Disabled {
			continue
		}
		enabledProviders = append(enabledProviders, string(provider))
	}
	sort.Strings(enabledProviders)

	lines := []string{
		baseStyle.Width(m.width).Render("Current: " + current),
	}
	if len(enabledProviders) == 0 {
		lines = append(lines, baseStyle.Foreground(t.TextMuted()).Width(m.width).Render("Enabled: none configured"))
	} else {
		lines = append(lines, baseStyle.Foreground(t.TextMuted()).Width(m.width).Render("Enabled: "+strings.Join(enabledProviders, ", ")))
	}

	return lipgloss.JoinVertical(lipgloss.Left, append([]string{title}, lines...)...)
}

func (m *sidebarCmp) agencySection() string {
	cfg := config.Get()
	t := theme.CurrentTheme()
	baseStyle := styles.BaseStyle()

	title := baseStyle.
		Width(m.width).
		Foreground(t.Primary()).
		Bold(true).
		Render("Agency")

	if cfg == nil || !config.TeamHUDEnabled(cfg) || m.team == nil {
		return lipgloss.JoinVertical(lipgloss.Left, title)
	}

	overview := InspectAgencyOverview(nil)
	if m.team != nil {
		overview = InspectAgencyOverview(&app.App{
			Agency:  m.agency,
			Team:    m.team,
			Workers: m.workers,
		})
	}

	if !overview.HasOfficeState && !overview.HasTeamSnapshot {
		return lipgloss.JoinVertical(
			lipgloss.Left,
			title,
			baseStyle.Width(m.width).Render(fmt.Sprintf("Constitution: %s", emptyFallback(overview.SoloConstitution, "solo"))),
			baseStyle.Foreground(t.TextMuted()).Width(m.width).Render("Office: stopped"),
		)
	}

	lines := []string{
		baseStyle.Width(m.width).Render(fmt.Sprintf("Constitution: %s", emptyFallback(overview.CurrentConstitution, overview.SoloConstitution))),
	}
	if overview.OfficeMode != "" {
		state := "stopped"
		if overview.Running {
			state = "running"
		}
		lines = append(lines, baseStyle.Width(m.width).Render(fmt.Sprintf("Office: %s (%s)", state, overview.OfficeMode)))
	}
	if overview.TeamName != "" {
		lines = append(lines, baseStyle.Width(m.width).Render(fmt.Sprintf("Office name: %s", overview.TeamName)))
	}
	if overview.Governance != "" {
		lines = append(lines, baseStyle.Foreground(t.TextMuted()).Width(m.width).Render("Governance: "+overview.Governance))
	}
	if overview.Topology != "" && overview.Topology != overview.Governance {
		lines = append(lines, baseStyle.Foreground(t.TextMuted()).Width(m.width).Render("Topology: "+overview.Topology))
	}
	if overview.ConsensusMode != "" {
		lines = append(lines, baseStyle.Foreground(t.TextMuted()).Width(m.width).Render("Consensus: "+overview.ConsensusMode))
	}
	if overview.Blueprint != "" {
		lines = append(lines, baseStyle.Foreground(t.TextMuted()).Width(m.width).Render("Blueprint: "+overview.Blueprint))
	} else if overview.TeamTemplate != "" {
		lines = append(lines, baseStyle.Foreground(t.TextMuted()).Width(m.width).Render("Template: "+overview.TeamTemplate))
	}
	if overview.HasTeamSnapshot {
		lines = append(lines,
			baseStyle.Width(m.width).Render(fmt.Sprintf("Lead: %s", emptyFallback(overview.Leader, "unassigned"))),
			baseStyle.Width(m.width).Render(fmt.Sprintf("Roster: %d  Workers: %d  Active: %d", overview.MemberCount, overview.WorkerCount, overview.ActiveWorkerCount)),
			baseStyle.Width(m.width).Render(fmt.Sprintf("Signals: %d direct / %d broadcast / %d handoff", overview.UnreadDirect, overview.UnreadBroadcasts, overview.PendingHandoffs)),
			baseStyle.Width(m.width).Render(fmt.Sprintf("Board: ready %d  active %d  review %d  done %d",
				overview.BoardSummary["ready"],
				overview.BoardSummary["in_progress"],
				overview.BoardSummary["in_review"],
				overview.BoardSummary["done"],
			)),
		)
	} else if overview.ManufacturedRoles > 0 {
		lines = append(lines,
			baseStyle.Width(m.width).Render(fmt.Sprintf("Manufactured roles: %d", overview.ManufacturedRoles)),
		)
		if overview.GenesisSummary != "" {
			lines = append(lines, baseStyle.Foreground(t.TextMuted()).Width(m.width).Render(truncateSidebarLine(m.width, overview.GenesisSummary)))
		}
	}

	if len(overview.Thread) > 0 {
		threadTitle := baseStyle.
			Width(m.width).
			Foreground(t.TextMuted()).
			Bold(true).
			Render("Thread" + threadTitleSuffix(overview.ThreadSource))
		lines = append(lines, threadTitle)
		for _, line := range overview.Thread {
			lines = append(lines, baseStyle.Foreground(t.TextMuted()).Width(m.width).Render("• "+truncateSidebarLine(m.width-2, line)))
		}
	}

	return lipgloss.JoinVertical(lipgloss.Left, append([]string{title}, lines...)...)
}

func truncateSidebarLine(width int, value string) string {
	if width <= 0 {
		return value
	}
	maxWidth := max(16, width-1)
	if lipgloss.Width(value) <= maxWidth {
		return value
	}
	return value[:maxWidth-3] + "..."
}

func threadTitleSuffix(source string) string {
	switch source {
	case "runtime":
		return " (live)"
	case "genesis":
		return " (planned)"
	default:
		return ""
	}
}

func (m *sidebarCmp) modifiedFile(filePath string, additions, removals int) string {
	t := theme.CurrentTheme()
	baseStyle := styles.BaseStyle()

	stats := ""
	if additions > 0 && removals > 0 {
		additionsStr := baseStyle.
			Foreground(t.Success()).
			PaddingLeft(1).
			Render(fmt.Sprintf("+%d", additions))

		removalsStr := baseStyle.
			Foreground(t.Error()).
			PaddingLeft(1).
			Render(fmt.Sprintf("-%d", removals))

		content := lipgloss.JoinHorizontal(lipgloss.Left, additionsStr, removalsStr)
		stats = baseStyle.Width(lipgloss.Width(content)).Render(content)
	} else if additions > 0 {
		additionsStr := fmt.Sprintf(" %s", baseStyle.
			PaddingLeft(1).
			Foreground(t.Success()).
			Render(fmt.Sprintf("+%d", additions)))
		stats = baseStyle.Width(lipgloss.Width(additionsStr)).Render(additionsStr)
	} else if removals > 0 {
		removalsStr := fmt.Sprintf(" %s", baseStyle.
			PaddingLeft(1).
			Foreground(t.Error()).
			Render(fmt.Sprintf("-%d", removals)))
		stats = baseStyle.Width(lipgloss.Width(removalsStr)).Render(removalsStr)
	}

	filePathStr := baseStyle.Render(filePath)

	return baseStyle.
		Width(m.width).
		Render(
			lipgloss.JoinHorizontal(
				lipgloss.Left,
				filePathStr,
				stats,
			),
		)
}

func (m *sidebarCmp) modifiedFiles() string {
	t := theme.CurrentTheme()
	baseStyle := styles.BaseStyle()

	modifiedFiles := baseStyle.
		Width(m.width).
		Foreground(t.Primary()).
		Bold(true).
		Render("Modified Files:")

	// If no modified files, show a placeholder message
	if m.modFiles == nil || len(m.modFiles) == 0 {
		message := "No modified files"
		remainingWidth := m.width - lipgloss.Width(message)
		if remainingWidth > 0 {
			message += strings.Repeat(" ", remainingWidth)
		}
		return baseStyle.
			Width(m.width).
			Render(
				lipgloss.JoinVertical(
					lipgloss.Top,
					modifiedFiles,
					baseStyle.Foreground(t.TextMuted()).Render(message),
				),
			)
	}

	// Sort file paths alphabetically for consistent ordering
	var paths []string
	for path := range m.modFiles {
		paths = append(paths, path)
	}
	sort.Strings(paths)

	// Create views for each file in sorted order
	var fileViews []string
	for _, path := range paths {
		stats := m.modFiles[path]
		fileViews = append(fileViews, m.modifiedFile(path, stats.additions, stats.removals))
	}

	return baseStyle.
		Width(m.width).
		Render(
			lipgloss.JoinVertical(
				lipgloss.Top,
				modifiedFiles,
				lipgloss.JoinVertical(
					lipgloss.Left,
					fileViews...,
				),
			),
		)
}

func (m *sidebarCmp) SetSize(width, height int) tea.Cmd {
	m.width = width
	m.height = height
	return nil
}

func (m *sidebarCmp) GetSize() (int, int) {
	return m.width, m.height
}

func NewSidebarCmp(session session.Session, history history.Service, agencyService *app.AgencyService, teamService *team.Service, workers *orchestration.Manager) tea.Model {
	return &sidebarCmp{
		session: session,
		history: history,
		agency:  agencyService,
		team:    teamService,
		workers: workers,
	}
}

func (m *sidebarCmp) loadModifiedFiles(ctx context.Context) {
	if m.history == nil || m.session.ID == "" {
		return
	}

	// Get all latest files for this session
	latestFiles, err := m.history.ListLatestSessionFiles(ctx, m.session.ID)
	if err != nil {
		return
	}

	// Get all files for this session (to find initial versions)
	allFiles, err := m.history.ListBySession(ctx, m.session.ID)
	if err != nil {
		return
	}

	// Clear the existing map to rebuild it
	m.modFiles = make(map[string]struct {
		additions int
		removals  int
	})

	// Process each latest file
	for _, file := range latestFiles {
		// Skip if this is the initial version (no changes to show)
		if file.Version == history.InitialVersion {
			continue
		}

		// Find the initial version for this specific file
		var initialVersion history.File
		for _, v := range allFiles {
			if v.Path == file.Path && v.Version == history.InitialVersion {
				initialVersion = v
				break
			}
		}

		// Skip if we can't find the initial version
		if initialVersion.ID == "" {
			continue
		}
		if initialVersion.Content == file.Content {
			continue
		}

		// Calculate diff between initial and latest version
		_, additions, removals := diff.GenerateDiff(initialVersion.Content, file.Content, file.Path)

		// Only add to modified files if there are changes
		if additions > 0 || removals > 0 {
			// Remove working directory prefix from file path
			displayPath := file.Path
			workingDir := config.WorkingDirectory()
			displayPath = strings.TrimPrefix(displayPath, workingDir)
			displayPath = strings.TrimPrefix(displayPath, "/")

			m.modFiles[displayPath] = struct {
				additions int
				removals  int
			}{
				additions: additions,
				removals:  removals,
			}
		}
	}
}

func (m *sidebarCmp) processFileChanges(ctx context.Context, file history.File) {
	// Skip if this is the initial version (no changes to show)
	if file.Version == history.InitialVersion {
		return
	}

	// Find the initial version for this file
	initialVersion, err := m.findInitialVersion(ctx, file.Path)
	if err != nil || initialVersion.ID == "" {
		return
	}

	// Skip if content hasn't changed
	if initialVersion.Content == file.Content {
		// If this file was previously modified but now matches the initial version,
		// remove it from the modified files list
		displayPath := getDisplayPath(file.Path)
		delete(m.modFiles, displayPath)
		return
	}

	// Calculate diff between initial and latest version
	_, additions, removals := diff.GenerateDiff(initialVersion.Content, file.Content, file.Path)

	// Only add to modified files if there are changes
	if additions > 0 || removals > 0 {
		displayPath := getDisplayPath(file.Path)
		m.modFiles[displayPath] = struct {
			additions int
			removals  int
		}{
			additions: additions,
			removals:  removals,
		}
	} else {
		// If no changes, remove from modified files
		displayPath := getDisplayPath(file.Path)
		delete(m.modFiles, displayPath)
	}
}

// Helper function to find the initial version of a file
func (m *sidebarCmp) findInitialVersion(ctx context.Context, path string) (history.File, error) {
	// Get all versions of this file for the session
	fileVersions, err := m.history.ListBySession(ctx, m.session.ID)
	if err != nil {
		return history.File{}, err
	}

	// Find the initial version
	for _, v := range fileVersions {
		if v.Path == path && v.Version == history.InitialVersion {
			return v, nil
		}
	}

	return history.File{}, fmt.Errorf("initial version not found")
}

// Helper function to get the display path for a file
func getDisplayPath(path string) string {
	workingDir := config.WorkingDirectory()
	displayPath := strings.TrimPrefix(path, workingDir)
	return strings.TrimPrefix(displayPath, "/")
}
