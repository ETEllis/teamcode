package chat

import (
	"context"
	"fmt"
	"time"

	"github.com/ETEllis/teamcode/internal/app"
	"github.com/ETEllis/teamcode/internal/tui/styles"
	"github.com/ETEllis/teamcode/internal/tui/theme"
	"github.com/charmbracelet/bubbles/key"
	tea "github.com/charmbracelet/bubbletea"
	"github.com/charmbracelet/lipgloss"
)

// ApprovalProposalMsg is a tea message carrying a pending action proposal.
type ApprovalProposalMsg struct {
	app.ProposalMsg
}

// ApprovalVotedMsg is emitted when the user casts a vote on a proposal.
type ApprovalVotedMsg struct {
	ProposalID string
	Approved   bool
}

type approvalKeys struct {
	Approve key.Binding
	Reject  key.Binding
	Up      key.Binding
	Down    key.Binding
}

var approvalKeyMap = approvalKeys{
	Approve: key.NewBinding(key.WithKeys("a"), key.WithHelp("a", "approve")),
	Reject:  key.NewBinding(key.WithKeys("r"), key.WithHelp("r", "reject")),
	Up:      key.NewBinding(key.WithKeys("up", "k"), key.WithHelp("↑/k", "up")),
	Down:    key.NewBinding(key.WithKeys("down", "j"), key.WithHelp("↓/j", "down")),
}

type ApprovalCmp struct {
	app        *app.App
	orgID      string
	width      int
	height     int
	proposals  []app.ProposalMsg
	selected   int
	approvalCh <-chan app.ProposalMsg
	cancelFn   context.CancelFunc
}

func (m *ApprovalCmp) waitForProposal() tea.Cmd {
	return func() tea.Msg {
		p, ok := <-m.approvalCh
		if !ok {
			return nil
		}
		return ApprovalProposalMsg{ProposalMsg: p}
	}
}

func (m *ApprovalCmp) Init() tea.Cmd {
	if m.app.Agency == nil || m.orgID == "" {
		return nil
	}
	ctx, cancel := context.WithCancel(context.Background())
	m.cancelFn = cancel
	ch, err := m.app.Agency.SubscribeApprovals(ctx, m.orgID)
	if err != nil || ch == nil {
		cancel()
		m.cancelFn = nil
		return nil
	}
	m.approvalCh = ch
	return m.waitForProposal()
}

func (m *ApprovalCmp) Update(msg tea.Msg) (tea.Model, tea.Cmd) {
	switch msg := msg.(type) {
	case ApprovalProposalMsg:
		// Deduplicate by proposalID.
		for _, p := range m.proposals {
			if p.ProposalID == msg.ProposalID {
				if m.approvalCh != nil {
					return m, m.waitForProposal()
				}
				return m, nil
			}
		}
		m.proposals = append(m.proposals, msg.ProposalMsg)
		if m.approvalCh != nil {
			return m, m.waitForProposal()
		}

	case tea.KeyMsg:
		if len(m.proposals) == 0 {
			return m, nil
		}
		switch {
		case key.Matches(msg, approvalKeyMap.Up):
			if m.selected > 0 {
				m.selected--
			}
		case key.Matches(msg, approvalKeyMap.Down):
			if m.selected < len(m.proposals)-1 {
				m.selected++
			}
		case key.Matches(msg, approvalKeyMap.Approve):
			return m, m.vote(true)
		case key.Matches(msg, approvalKeyMap.Reject):
			return m, m.vote(false)
		}
	}
	return m, nil
}

func (m *ApprovalCmp) vote(approved bool) tea.Cmd {
	if len(m.proposals) == 0 || m.selected >= len(m.proposals) {
		return nil
	}
	proposal := m.proposals[m.selected]
	m.proposals = append(m.proposals[:m.selected], m.proposals[m.selected+1:]...)
	if m.selected > 0 && m.selected >= len(m.proposals) {
		m.selected = len(m.proposals) - 1
	}

	return func() tea.Msg {
		if m.app.Agency != nil {
			ctx := context.Background()
			_ = m.app.Agency.SendApprovalVote(ctx, m.orgID, proposal.ProposalID, approved)
		}
		return ApprovalVotedMsg{ProposalID: proposal.ProposalID, Approved: approved}
	}
}

func (m *ApprovalCmp) View() string {
	t := theme.CurrentTheme()
	base := styles.BaseStyle()

	if len(m.proposals) == 0 {
		return base.Width(m.width).
			Foreground(t.TextMuted()).
			Render("No pending approvals")
	}

	headerStyle := base.
		Width(m.width).
		Bold(true).
		Foreground(t.Warning())

	header := headerStyle.Render(fmt.Sprintf("⏳ Pending Approvals (%d)", len(m.proposals)))
	rows := []string{header, ""}

	for i, p := range m.proposals {
		ts := ""
		if p.CreatedAt > 0 {
			ts = time.UnixMilli(p.CreatedAt).Format("15:04:05")
		}

		rowStyle := base.Width(m.width - 2).PaddingLeft(1)
		if i == m.selected {
			rowStyle = rowStyle.
				Background(t.BackgroundSecondary()).
				BorderLeft(true).
				BorderStyle(lipgloss.ThickBorder()).
				BorderForeground(t.Primary())
		}

		actor := p.ActorID
		if actor == "" {
			actor = "agent"
		}
		action := p.ActionType
		if action == "" {
			action = "unknown"
		}
		target := p.Target
		if target != "" {
			action = action + " → " + target
		}

		line := fmt.Sprintf("%s  %s  %s", actor, action, ts)
		rows = append(rows, rowStyle.Render(line))
	}

	rows = append(rows, "",
		base.Foreground(t.TextMuted()).Render("[a] approve  [r] reject  [↑↓] navigate"),
	)

	return base.Width(m.width).Render(
		lipgloss.JoinVertical(lipgloss.Left, rows...),
	)
}

func (m *ApprovalCmp) SetSize(width, height int) tea.Cmd {
	m.width = width
	m.height = height
	return nil
}

func (m *ApprovalCmp) GetSize() (int, int) {
	return m.width, m.height
}

func (m *ApprovalCmp) BindingKeys() []key.Binding {
	return []key.Binding{
		approvalKeyMap.Approve,
		approvalKeyMap.Reject,
		approvalKeyMap.Up,
		approvalKeyMap.Down,
	}
}

// HasPendingApprovals returns true when there are proposals waiting for a vote.
func (m *ApprovalCmp) HasPendingApprovals() bool {
	return len(m.proposals) > 0
}

func NewApprovalCmp(a *app.App, orgID string) *ApprovalCmp {
	return &ApprovalCmp{
		app:   a,
		orgID: orgID,
	}
}
