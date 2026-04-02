package chat

import (
	"context"
	"fmt"
	"math"
	"strings"
	"time"
	"unicode/utf8"

	"github.com/ETEllis/teamcode/internal/app"
	"github.com/ETEllis/teamcode/internal/message"
	"github.com/ETEllis/teamcode/internal/pubsub"
	"github.com/ETEllis/teamcode/internal/session"
	"github.com/ETEllis/teamcode/internal/tui/components/dialog"
	"github.com/ETEllis/teamcode/internal/tui/styles"
	"github.com/ETEllis/teamcode/internal/tui/theme"
	"github.com/ETEllis/teamcode/internal/tui/util"
	"github.com/charmbracelet/bubbles/key"
	"github.com/charmbracelet/bubbles/spinner"
	"github.com/charmbracelet/bubbles/viewport"
	tea "github.com/charmbracelet/bubbletea"
	"github.com/charmbracelet/lipgloss"
)

type cacheItem struct {
	width   int
	content []uiMessage
}
// AgencyBroadcastMsg is a tea message carrying a live agent broadcast.
type AgencyBroadcastMsg struct {
	app.BroadcastMsg
}

type messagesCmp struct {
	app           *app.App
	width, height int
	viewport      viewport.Model
	session       session.Session
	messages      []message.Message
	broadcasts    []app.BroadcastMsg
	bulletins     []app.BulletinRecord
	splashMode    bool
	uiMessages    []uiMessage
	currentMsgID  string
	cachedContent map[string]cacheItem
	spinner       spinner.Model
	rendering     bool
	attachments   viewport.Model
}
type renderFinishedMsg struct{}

type MessageKeys struct {
	PageDown     key.Binding
	PageUp       key.Binding
	HalfPageUp   key.Binding
	HalfPageDown key.Binding
}

var messageKeys = MessageKeys{
	PageDown: key.NewBinding(
		key.WithKeys("pgdown"),
		key.WithHelp("f/pgdn", "page down"),
	),
	PageUp: key.NewBinding(
		key.WithKeys("pgup"),
		key.WithHelp("b/pgup", "page up"),
	),
	HalfPageUp: key.NewBinding(
		key.WithKeys("ctrl+u"),
		key.WithHelp("ctrl+u", "½ page up"),
	),
	HalfPageDown: key.NewBinding(
		key.WithKeys("ctrl+d", "ctrl+d"),
		key.WithHelp("ctrl+d", "½ page down"),
	),
}

func (m *messagesCmp) Init() tea.Cmd {
	return tea.Batch(m.viewport.Init(), m.spinner.Tick)
}

func (m *messagesCmp) Update(msg tea.Msg) (tea.Model, tea.Cmd) {
	var cmds []tea.Cmd
	switch msg := msg.(type) {
	case dialog.ThemeChangedMsg:
		m.rerender()
		return m, nil
	case SessionSelectedMsg:
		if msg.ID != m.session.ID {
			cmd := m.SetSession(msg)
			return m, cmd
		}
		return m, nil
	case SessionClearedMsg:
		m.session = session.Session{}
		m.messages = make([]message.Message, 0)
		m.currentMsgID = ""
		m.rendering = false
		m.splashMode = true
		return m, util.CmdHandler(SplashModeChangedMsg{Active: true})

	case tea.KeyMsg:
		if key.Matches(msg, messageKeys.PageUp) || key.Matches(msg, messageKeys.PageDown) ||
			key.Matches(msg, messageKeys.HalfPageUp) || key.Matches(msg, messageKeys.HalfPageDown) {
			u, cmd := m.viewport.Update(msg)
			m.viewport = u
			cmds = append(cmds, cmd)
		}

	case renderFinishedMsg:
		m.rendering = false
		m.viewport.GotoBottom()
	case AgencyBroadcastMsg:
		m.broadcasts = append(m.broadcasts, msg.BroadcastMsg)
		m.rerender()
		m.viewport.GotoBottom()
	case AgencyBulletinMsg:
		m.bulletins = append(m.bulletins, msg.BulletinRecord)
		m.rerender()
		m.viewport.GotoBottom()
	case pubsub.Event[session.Session]:
		if msg.Type == pubsub.UpdatedEvent && msg.Payload.ID == m.session.ID {
			m.session = msg.Payload
			if m.session.SummaryMessageID == m.currentMsgID {
				delete(m.cachedContent, m.currentMsgID)
				m.renderView()
			}
		}
	case pubsub.Event[message.Message]:
		needsRerender := false
		if msg.Type == pubsub.CreatedEvent {
			if msg.Payload.SessionID == m.session.ID {

				messageExists := false
				for _, v := range m.messages {
					if v.ID == msg.Payload.ID {
						messageExists = true
						break
					}
				}

				if !messageExists {
					if len(m.messages) > 0 {
						lastMsgID := m.messages[len(m.messages)-1].ID
						delete(m.cachedContent, lastMsgID)
					}

					m.messages = append(m.messages, msg.Payload)
					delete(m.cachedContent, m.currentMsgID)
					m.currentMsgID = msg.Payload.ID
					needsRerender = true
				}
			}
			// There are tool calls from the child task
			for _, v := range m.messages {
				for _, c := range v.ToolCalls() {
					if c.ID == msg.Payload.SessionID {
						delete(m.cachedContent, v.ID)
						needsRerender = true
					}
				}
			}
		} else if msg.Type == pubsub.UpdatedEvent && msg.Payload.SessionID == m.session.ID {
			for i, v := range m.messages {
				if v.ID == msg.Payload.ID {
					m.messages[i] = msg.Payload
					delete(m.cachedContent, msg.Payload.ID)
					needsRerender = true
					break
				}
			}
		}
		if needsRerender {
			previousSplashMode := m.splashMode
			m.splashMode = len(m.messages) == 0
			m.renderView()
			if len(m.messages) > 0 {
				if (msg.Type == pubsub.CreatedEvent) ||
					(msg.Type == pubsub.UpdatedEvent && msg.Payload.ID == m.messages[len(m.messages)-1].ID) {
					m.viewport.GotoBottom()
				}
			}
			if previousSplashMode != m.splashMode {
				cmds = append(cmds, util.CmdHandler(SplashModeChangedMsg{Active: m.splashMode}))
			}
		}
	}

	spinner, cmd := m.spinner.Update(msg)
	m.spinner = spinner
	cmds = append(cmds, cmd)
	return m, tea.Batch(cmds...)
}

func (m *messagesCmp) IsAgentWorking() bool {
	if m.app.CoderAgent == nil {
		return false
	}
	return m.app.CoderAgent.IsSessionBusy(m.session.ID)
}

func formatTimeDifference(unixTime1, unixTime2 int64) string {
	diffSeconds := float64(math.Abs(float64(unixTime2 - unixTime1)))

	if diffSeconds < 60 {
		return fmt.Sprintf("%.1fs", diffSeconds)
	}

	minutes := int(diffSeconds / 60)
	seconds := int(diffSeconds) % 60
	return fmt.Sprintf("%dm%ds", minutes, seconds)
}

func (m *messagesCmp) renderView() {
	m.uiMessages = make([]uiMessage, 0)
	pos := 0
	baseStyle := styles.BaseStyle()

	if m.width == 0 {
		return
	}
	for inx, msg := range m.messages {
		switch msg.Role {
		case message.User:
			if cache, ok := m.cachedContent[msg.ID]; ok && cache.width == m.width {
				m.uiMessages = append(m.uiMessages, cache.content...)
				continue
			}
			userMsg := renderUserMessage(
				msg,
				msg.ID == m.currentMsgID,
				m.width,
				pos,
			)
			m.uiMessages = append(m.uiMessages, userMsg)
			m.cachedContent[msg.ID] = cacheItem{
				width:   m.width,
				content: []uiMessage{userMsg},
			}
			pos += userMsg.height + 1 // + 1 for spacing
		case message.Assistant:
			if cache, ok := m.cachedContent[msg.ID]; ok && cache.width == m.width {
				m.uiMessages = append(m.uiMessages, cache.content...)
				continue
			}
			isSummary := m.session.SummaryMessageID == msg.ID

			assistantMessages := renderAssistantMessage(
				msg,
				inx,
				m.messages,
				m.app.Messages,
				m.currentMsgID,
				isSummary,
				m.width,
				pos,
			)
			for _, msg := range assistantMessages {
				m.uiMessages = append(m.uiMessages, msg)
				pos += msg.height + 1 // + 1 for spacing
			}
			m.cachedContent[msg.ID] = cacheItem{
				width:   m.width,
				content: assistantMessages,
			}
		}
	}

	messages := make([]string, 0)
	for _, v := range m.uiMessages {
		messages = append(messages, lipgloss.JoinVertical(lipgloss.Left, v.content),
			baseStyle.
				Width(m.width).
				Render(
					"",
				),
		)
	}

	// Append live agent broadcast bubbles (iMessage-style).
	if len(m.broadcasts) > 0 {
		for _, b := range m.broadcasts {
			messages = append(messages,
				renderBroadcastBubble(b, m.width),
				baseStyle.Width(m.width).Render(""),
			)
		}
	}

	// Append bulletin timeline entries (directive→output→score).
	if len(m.bulletins) > 0 {
		for _, b := range m.bulletins {
			messages = append(messages,
				renderBulletinEntry(b, m.width),
				baseStyle.Width(m.width).Render(""),
			)
		}
	}

	m.viewport.SetContent(
		baseStyle.
			Width(m.width).
			Render(
				lipgloss.JoinVertical(
					lipgloss.Top,
					messages...,
				),
			),
	)
}

func (m *messagesCmp) View() string {
	baseStyle := styles.BaseStyle()

	if m.rendering {
		return baseStyle.
			Width(m.width).
			Render(
				lipgloss.JoinVertical(
					lipgloss.Top,
					"Loading...",
					m.working(),
					m.help(),
				),
			)
	}
	if len(m.messages) == 0 {
		content := baseStyle.
			Width(m.width).
			Height(max(0, m.height)).
			Render(
				renderSplash(m.app, m.width, m.height),
			)

		return baseStyle.Width(m.width).Render(content)
	}

	return baseStyle.
		Width(m.width).
		Render(
			lipgloss.JoinVertical(
				lipgloss.Top,
				m.viewport.View(),
				m.working(),
				m.help(),
			),
		)
}

func hasToolsWithoutResponse(messages []message.Message) bool {
	toolCalls := make([]message.ToolCall, 0)
	toolResults := make([]message.ToolResult, 0)
	for _, m := range messages {
		toolCalls = append(toolCalls, m.ToolCalls()...)
		toolResults = append(toolResults, m.ToolResults()...)
	}

	for _, v := range toolCalls {
		found := false
		for _, r := range toolResults {
			if v.ID == r.ToolCallID {
				found = true
				break
			}
		}
		if !found && v.Finished {
			return true
		}
	}
	return false
}

func hasUnfinishedToolCalls(messages []message.Message) bool {
	toolCalls := make([]message.ToolCall, 0)
	for _, m := range messages {
		toolCalls = append(toolCalls, m.ToolCalls()...)
	}
	for _, v := range toolCalls {
		if !v.Finished {
			return true
		}
	}
	return false
}

func (m *messagesCmp) working() string {
	text := ""
	if m.IsAgentWorking() && len(m.messages) > 0 {
		t := theme.CurrentTheme()
		baseStyle := styles.BaseStyle()

		task := "Thinking..."
		lastMessage := m.messages[len(m.messages)-1]
		if hasToolsWithoutResponse(m.messages) {
			task = "Waiting for tool response..."
		} else if hasUnfinishedToolCalls(m.messages) {
			task = "Building tool call..."
		} else if !lastMessage.IsFinished() {
			task = "Generating..."
		}
		if task != "" {
			text += baseStyle.
				Width(m.width).
				Foreground(t.Primary()).
				Bold(true).
				Render(fmt.Sprintf("%s %s ", m.spinner.View(), task))
		}
	}
	return text
}

func (m *messagesCmp) help() string {
	t := theme.CurrentTheme()
	baseStyle := styles.BaseStyle()

	text := ""

	if m.app.CoderAgent != nil && m.app.CoderAgent.IsBusy() {
		text += lipgloss.JoinHorizontal(
			lipgloss.Left,
			baseStyle.Foreground(t.TextMuted()).Bold(true).Render("press "),
			baseStyle.Foreground(t.Text()).Bold(true).Render("esc"),
			baseStyle.Foreground(t.TextMuted()).Bold(true).Render(" to exit cancel"),
		)
	} else {
		text += lipgloss.JoinHorizontal(
			lipgloss.Left,
			baseStyle.Foreground(t.TextMuted()).Bold(true).Render("press "),
			baseStyle.Foreground(t.Text()).Bold(true).Render("enter"),
			baseStyle.Foreground(t.TextMuted()).Bold(true).Render(" to send the message,"),
			baseStyle.Foreground(t.TextMuted()).Bold(true).Render(" write"),
			baseStyle.Foreground(t.Text()).Bold(true).Render(" \\"),
			baseStyle.Foreground(t.TextMuted()).Bold(true).Render(" and enter to add a new line"),
		)
	}
	return baseStyle.
		Width(m.width).
		Render(text)
}

func (m *messagesCmp) rerender() {
	for _, msg := range m.messages {
		delete(m.cachedContent, msg.ID)
	}
	m.renderView()
}

func (m *messagesCmp) SetSize(width, height int) tea.Cmd {
	if m.width == width && m.height == height {
		return nil
	}
	m.width = width
	m.height = height
	m.viewport.Width = width
	m.viewport.Height = height - 2
	m.attachments.Width = width + 40
	m.attachments.Height = 3
	m.rerender()
	return nil
}

func (m *messagesCmp) GetSize() (int, int) {
	return m.width, m.height
}

func (m *messagesCmp) SetSession(session session.Session) tea.Cmd {
	if m.session.ID == session.ID {
		return nil
	}
	m.session = session
	messages, err := m.app.Messages.List(context.Background(), session.ID)
	if err != nil {
		return util.ReportError(err)
	}
	m.messages = messages
	if len(m.messages) > 0 {
		m.currentMsgID = m.messages[len(m.messages)-1].ID
	} else {
		m.currentMsgID = ""
	}
	m.splashMode = len(m.messages) == 0
	delete(m.cachedContent, m.currentMsgID)
	m.rendering = true
	return tea.Batch(
		util.CmdHandler(SplashModeChangedMsg{Active: m.splashMode}),
		func() tea.Msg {
			m.renderView()
			return renderFinishedMsg{}
		},
	)
}

func (m *messagesCmp) BindingKeys() []key.Binding {
	return []key.Binding{
		m.viewport.KeyMap.PageDown,
		m.viewport.KeyMap.PageUp,
		m.viewport.KeyMap.HalfPageUp,
		m.viewport.KeyMap.HalfPageDown,
	}
}

func NewMessagesCmp(app *app.App) tea.Model {
	s := spinner.New()
	s.Spinner = spinner.Pulse
	vp := viewport.New(0, 0)
	attachmets := viewport.New(0, 0)
	vp.KeyMap.PageUp = messageKeys.PageUp
	vp.KeyMap.PageDown = messageKeys.PageDown
	vp.KeyMap.HalfPageUp = messageKeys.HalfPageUp
	vp.KeyMap.HalfPageDown = messageKeys.HalfPageDown
	return &messagesCmp{
		app:           app,
		cachedContent: make(map[string]cacheItem),
		viewport:      vp,
		spinner:       s,
		attachments:   attachmets,
		splashMode:    true,
	}
}

// roleColorPalette maps role hash index → theme color selector.
// Cycles through Primary, Secondary, Accent, Success, Warning, Info.
var roleColorIndex = []func(theme.Theme) lipgloss.AdaptiveColor{
	func(t theme.Theme) lipgloss.AdaptiveColor { return t.Primary() },
	func(t theme.Theme) lipgloss.AdaptiveColor { return t.Secondary() },
	func(t theme.Theme) lipgloss.AdaptiveColor { return t.Accent() },
	func(t theme.Theme) lipgloss.AdaptiveColor { return t.Success() },
	func(t theme.Theme) lipgloss.AdaptiveColor { return t.Warning() },
	func(t theme.Theme) lipgloss.AdaptiveColor { return t.Info() },
}

// actorColor deterministically picks a color for an actor ID.
func actorColor(actorID string) func(theme.Theme) lipgloss.AdaptiveColor {
	h := 0
	for _, r := range actorID {
		h = h*31 + int(r)
	}
	if h < 0 {
		h = -h
	}
	return roleColorIndex[h%len(roleColorIndex)]
}

// actorInitials returns 2-char uppercase initials from an actor ID.
func actorInitials(actorID string) string {
	if actorID == "" {
		return "AG"
	}
	// Use last segment of dot-separated ID (e.g. "org.developer.alice" → "alice")
	parts := strings.Split(actorID, ".")
	name := parts[len(parts)-1]
	name = strings.ToUpper(strings.TrimSpace(name))
	if utf8.RuneCountInString(name) >= 2 {
		runes := []rune(name)
		return string(runes[:2])
	}
	if len(name) == 1 {
		return name + name
	}
	return "AG"
}

// renderBroadcastBubble renders a single agent broadcast as an iMessage-style bubble.
func renderBroadcastBubble(b app.BroadcastMsg, width int) string {
	t := theme.CurrentTheme()
	base := styles.BaseStyle()

	colorFn := actorColor(b.ActorID)
	roleColor := colorFn(t)

	// Avatar (2-char initials in a colored box).
	avatarStyle := base.
		Bold(true).
		Foreground(t.Background()).
		Background(roleColor).
		Padding(0, 1)
	avatar := avatarStyle.Render(actorInitials(b.ActorID))

	// Actor name label.
	actor := b.ActorID
	if actor == "" {
		actor = "agent"
	}
	nameStyle := base.Bold(true).Foreground(roleColor)
	nameLabel := nameStyle.Render(actor)

	// Timestamp.
	ts := ""
	if b.CreatedAt > 0 {
		ts = time.UnixMilli(b.CreatedAt).Format("15:04")
	}
	tsStyle := base.Foreground(t.TextMuted())
	tsLabel := tsStyle.Render(ts)

	// Header row: avatar + name + timestamp.
	avatarWidth := lipgloss.Width(avatar)
	nameWidth := lipgloss.Width(nameLabel)
	tsWidth := lipgloss.Width(tsLabel)
	gap := width - avatarWidth - nameWidth - tsWidth - 3
	if gap < 0 {
		gap = 0
	}
	header := lipgloss.JoinHorizontal(lipgloss.Left,
		avatar,
		" ",
		nameLabel,
		strings.Repeat(" ", gap),
		tsLabel,
	)

	// Message bubble.
	bubbleWidth := width - avatarWidth - 2
	if bubbleWidth < 10 {
		bubbleWidth = 10
	}
	bubbleStyle := base.
		Width(bubbleWidth).
		BorderLeft(true).
		BorderStyle(lipgloss.ThickBorder()).
		BorderForeground(roleColor).
		Foreground(t.Text()).
		PaddingLeft(1)

	bubble := lipgloss.JoinVertical(lipgloss.Left,
		header,
		lipgloss.JoinHorizontal(lipgloss.Left,
			strings.Repeat(" ", avatarWidth+1),
			bubbleStyle.Render(b.Message),
		),
	)

	return base.Width(width).Render(bubble)
}
