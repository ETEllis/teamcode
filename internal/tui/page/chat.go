package page

import (
	"context"
	"fmt"
	"os/exec"
	"strings"
	"time"

	"github.com/ETEllis/teamcode/internal/agency"
	"github.com/ETEllis/teamcode/internal/app"
	"github.com/ETEllis/teamcode/internal/completions"
	"github.com/ETEllis/teamcode/internal/config"
	"github.com/ETEllis/teamcode/internal/message"
	"github.com/ETEllis/teamcode/internal/session"
	"github.com/ETEllis/teamcode/internal/tui/components/chat"
	"github.com/ETEllis/teamcode/internal/tui/components/dialog"
	"github.com/ETEllis/teamcode/internal/tui/layout"
	"github.com/ETEllis/teamcode/internal/tui/util"
	"github.com/charmbracelet/bubbles/key"
	tea "github.com/charmbracelet/bubbletea"
	"github.com/charmbracelet/lipgloss"
)

var ChatPage PageID = "chat"

type chatPage struct {
	app                  *app.App
	editor               layout.Container
	messages             layout.Container
	layout               layout.SplitPaneLayout
	session              session.Session
	completionDialog     dialog.CompletionDialog
	showCompletionDialog bool
	broadcastCh <-chan app.BroadcastMsg
	bulletinCh  <-chan app.BulletinRecord
	approval    *chat.ApprovalCmp
	orgID       string
}

type ChatKeyMap struct {
	ShowCompletionDialog key.Binding
	NewSession           key.Binding
	Cancel               key.Binding
}

var keyMap = ChatKeyMap{
	ShowCompletionDialog: key.NewBinding(
		key.WithKeys("@"),
		key.WithHelp("@", "Complete"),
	),
	NewSession: key.NewBinding(
		key.WithKeys("ctrl+n"),
		key.WithHelp("ctrl+n", "new session"),
	),
	Cancel: key.NewBinding(
		key.WithKeys("esc"),
		key.WithHelp("esc", "cancel"),
	),
}

func (p *chatPage) Init() tea.Cmd {
	cmds := []tea.Cmd{
		p.layout.Init(),
		p.completionDialog.Init(),
	}
	if p.app.Agency != nil {
		org, _ := p.app.Agency.InspectOrganization()
		if org.Constitution.Name != "" || org.CurrentConstitution != "" {
			orgID := org.CurrentConstitution
			if orgID == "" {
				orgID = org.Constitution.Name
			}
			p.orgID = orgID
			ch, _ := p.app.Agency.SubscribeBroadcasts(context.Background(), orgID)
			if ch != nil {
				p.broadcastCh = ch
				cmds = append(cmds, p.waitForBroadcast())
			}
			// Init approval panel.
			p.approval = chat.NewApprovalCmp(p.app, orgID)
			if initCmd := p.approval.Init(); initCmd != nil {
				cmds = append(cmds, initCmd)
			}
			// Subscribe to bulletin channel.
			bch, _ := p.app.Agency.SubscribeBulletin(context.Background(), orgID)
			if bch != nil {
				p.bulletinCh = bch
				cmds = append(cmds, p.waitForBulletin())
			}
		}
	}
	return tea.Batch(cmds...)
}

func (p *chatPage) waitForBroadcast() tea.Cmd {
	return func() tea.Msg {
		b, ok := <-p.broadcastCh
		if !ok {
			return nil
		}
		return chat.AgencyBroadcastMsg{BroadcastMsg: b}
	}
}

func (p *chatPage) waitForBulletin() tea.Cmd {
	return func() tea.Msg {
		b, ok := <-p.bulletinCh
		if !ok {
			return nil
		}
		return chat.AgencyBulletinMsg{BulletinRecord: b}
	}
}

func (p *chatPage) Update(msg tea.Msg) (tea.Model, tea.Cmd) {
	var cmds []tea.Cmd
	switch msg := msg.(type) {
	case chat.AgencyBroadcastMsg:
		// Forward to messages component and re-queue the next wait.
		u, cmd := p.messages.Update(msg)
		p.messages = u.(layout.Container)
		cmds = append(cmds, cmd)
		if p.broadcastCh != nil {
			cmds = append(cmds, p.waitForBroadcast())
		}
		// Fire-and-forget TTS in background.
		go p.playTTS(msg.Message, msg.ActorID)
		return p, tea.Batch(cmds...)

	case chat.AgencyBulletinMsg:
		u, cmd := p.messages.Update(msg)
		p.messages = u.(layout.Container)
		cmds = append(cmds, cmd)
		if p.bulletinCh != nil {
			cmds = append(cmds, p.waitForBulletin())
		}
		return p, tea.Batch(cmds...)

	case chat.ApprovalProposalMsg:
		if p.approval != nil {
			u, cmd := p.approval.Update(msg)
			p.approval = u.(*chat.ApprovalCmp)
			cmds = append(cmds, cmd)
			// Show approval panel in right rail when there are pending items.
			if p.approval.HasPendingApprovals() {
				approvalContainer := layout.NewContainer(p.approval, layout.WithPadding(1, 1, 1, 1))
				cmds = append(cmds, tea.Batch(p.layout.SetRightPanel(approvalContainer), approvalContainer.Init()))
			}
		}
		return p, tea.Batch(cmds...)

	case chat.ApprovalVotedMsg:
		if p.approval != nil {
			u, cmd := p.approval.Update(msg)
			p.approval = u.(*chat.ApprovalCmp)
			cmds = append(cmds, cmd)
			// Clear approval panel when queue is empty.
			if !p.approval.HasPendingApprovals() {
				cmds = append(cmds, p.clearSidebar())
			}
		}
		return p, tea.Batch(cmds...)
	case tea.WindowSizeMsg:
		cmd := p.layout.SetSize(msg.Width, msg.Height)
		cmds = append(cmds, cmd)
	case dialog.CompletionDialogCloseMsg:
		p.showCompletionDialog = false
	case chat.SendMsg:
		cmd := p.sendMessage(msg.Text, msg.Attachments)
		if cmd != nil {
			return p, cmd
		}
	case dialog.CommandRunCustomMsg:
		// Check if the agent is busy before executing custom commands
		if p.app.CoderAgent != nil && p.app.CoderAgent.IsBusy() {
			return p, util.ReportWarn("Agent is busy, please wait before executing a command...")
		}

		// Process the command content with arguments if any
		content := msg.Content
		if msg.Args != nil {
			// Replace all named arguments with their values
			for name, value := range msg.Args {
				placeholder := "$" + name
				content = strings.ReplaceAll(content, placeholder, value)
			}
		}

		// Handle custom command execution
		cmd := p.sendMessage(content, nil)
		if cmd != nil {
			return p, cmd
		}
	case chat.SessionSelectedMsg:
		p.session = msg
	case chat.SplashModeChangedMsg:
		if msg.Active {
			return p, p.clearSidebar()
		}
		if p.session.ID != "" {
			return p, p.setSidebar()
		}
		return p, nil
	case tea.KeyMsg:
		switch {
		case key.Matches(msg, keyMap.ShowCompletionDialog):
			p.showCompletionDialog = true
			// Continue sending keys to layout->chat
		case key.Matches(msg, keyMap.NewSession):
			p.session = session.Session{}
			return p, tea.Batch(
				p.clearSidebar(),
				util.CmdHandler(chat.SessionClearedMsg{}),
			)
		case key.Matches(msg, keyMap.Cancel):
			if p.session.ID != "" && p.app.CoderAgent != nil {
				// Cancel the current session's generation process
				// This allows users to interrupt long-running operations
				p.app.CoderAgent.Cancel(p.session.ID)
				return p, nil
			}
		}
	}
	if p.showCompletionDialog {
		context, contextCmd := p.completionDialog.Update(msg)
		p.completionDialog = context.(dialog.CompletionDialog)
		cmds = append(cmds, contextCmd)

		// Doesn't forward event if enter key is pressed
		if keyMsg, ok := msg.(tea.KeyMsg); ok {
			if keyMsg.String() == "enter" {
				return p, tea.Batch(cmds...)
			}
		}
	}

	u, cmd := p.layout.Update(msg)
	cmds = append(cmds, cmd)
	p.layout = u.(layout.SplitPaneLayout)

	return p, tea.Batch(cmds...)
}

func (p *chatPage) setSidebar() tea.Cmd {
	sidebarContainer := layout.NewContainer(
		chat.NewSidebarCmp(p.session, p.app.History, p.app.Agency, p.app.Team, p.app.Workers),
		layout.WithPadding(1, 1, 1, 1),
	)
	return tea.Batch(p.layout.SetRightPanel(sidebarContainer), sidebarContainer.Init())
}

func (p *chatPage) clearSidebar() tea.Cmd {
	return p.layout.ClearRightPanel()
}

func (p *chatPage) sendMessage(text string, attachments []message.Attachment) tea.Cmd {
	if slashCmd := p.resolveSlashCommand(strings.TrimSpace(text)); slashCmd != nil {
		return slashCmd
	}

	if p.app.CoderAgent == nil {
		return util.ReportError(fmt.Errorf("no AI provider configured: please set ANTHROPIC_API_KEY, OPENAI_API_KEY, or another supported provider API key"))
	}

	var cmds []tea.Cmd
	if p.session.ID == "" {
		session, err := p.app.Sessions.Create(context.Background(), "New Session")
		if err != nil {
			return util.ReportError(err)
		}

		p.session = session
		cmds = append(cmds, util.CmdHandler(chat.SessionSelectedMsg(session)))
	}

	_, err := p.app.CoderAgent.Run(context.Background(), p.session.ID, text, attachments...)
	if err != nil {
		return util.ReportError(err)
	}
	return tea.Batch(cmds...)
}

func (p *chatPage) resolveSlashCommand(text string) tea.Cmd {
	if !strings.HasPrefix(text, "/") {
		return nil
	}

	commandText := strings.TrimSpace(strings.TrimPrefix(text, "/"))
	if commandText == "" {
		return util.CmdHandler(dialog.ShowCommandDialogMsg{})
	}
	parts := strings.Fields(commandText)
	commandName := parts[0]

	switch commandName {
	case "commands":
		return util.CmdHandler(dialog.ShowCommandDialogMsg{})
	case "sessions":
		return util.CmdHandler(dialog.ShowSessionDialogMsg{})
	case "new":
		p.session = session.Session{}
		return tea.Batch(
			p.clearSidebar(),
			util.CmdHandler(chat.SessionClearedMsg{}),
		)
	case "init":
		return util.CmdHandler(chat.SendMsg{Text: dialog.InitProjectPrompt()})
	case "models":
		return util.CmdHandler(dialog.ShowModelDialogMsg{})
	case "model":
		return util.CmdHandler(dialog.ShowModelDialogMsg{})
	case "theme":
		return util.CmdHandler(dialog.ShowThemeDialogMsg{})
	case "compact":
		return util.CmdHandler(dialog.StartCompactSessionMsg{})
	case "skills":
		return util.CmdHandler(chat.SendMsg{Text: dialog.ListSkillsPrompt()})
	case "skill":
		if len(parts) == 1 {
			return util.CmdHandler(chat.SendMsg{Text: dialog.ListSkillsPrompt()})
		}
		task := ""
		if len(parts) > 2 {
			task = strings.Join(parts[2:], " ")
		}
		return util.CmdHandler(chat.SendMsg{Text: dialog.SkillUsePrompt(parts[1], task)})
	case "help":
		return util.CmdHandler(dialog.ShowHelpDialogMsg{})
	case "exit":
		return util.CmdHandler(dialog.ShowQuitDialogMsg{})
	case "team":
		if len(parts) == 1 {
			return util.CmdHandler(chat.SendMsg{Text: dialog.TeamStatusPrompt()})
		}
		switch parts[1] {
		case "status":
			return util.CmdHandler(chat.SendMsg{Text: dialog.TeamStatusPrompt()})
		case "bootstrap":
			templateName := ""
			if len(parts) > 2 {
				templateName = strings.Join(parts[2:], " ")
			}
			return util.CmdHandler(chat.SendMsg{Text: dialog.TeamBootstrapPrompt(templateName)})
		case "templates":
			return util.CmdHandler(chat.SendMsg{Text: dialog.TeamTemplatesPrompt()})
		}
	case "agency":
		if len(parts) == 1 {
			return util.CmdHandler(dialog.ShowAgencyDialogMsg{})
		}
		switch parts[1] {
		case "status":
			return util.CmdHandler(dialog.ShowAgencyDialogMsg{})
		case "genesis":
			intent := ""
			if len(parts) > 2 {
				intent = strings.Join(parts[2:], " ")
			}
			return util.CmdHandler(dialog.StartAgencyGenesisMsg{Intent: intent})
		case "bootstrap":
			constitutionName := ""
			if len(parts) > 2 {
				constitutionName = strings.Join(parts[2:], " ")
			}
			return util.CmdHandler(dialog.BootAgencyOfficeMsg{Constitution: constitutionName})
		case "stop":
			return util.CmdHandler(dialog.StopAgencyOfficeMsg{})
		case "templates", "blueprints", "constitutions":
			return util.CmdHandler(dialog.ShowAgencyDialogMsg{})
		}
	}

	commands, err := dialog.LoadCustomCommands()
	if err != nil {
		return util.ReportError(err)
	}

	for _, command := range commands {
		if commandName == command.ID ||
			commandName == strings.TrimPrefix(command.ID, dialog.UserCommandPrefix) ||
			commandName == strings.TrimPrefix(command.ID, dialog.ProjectCommandPrefix) {
			return command.Handler(command)
		}
	}

	return util.ReportWarn(fmt.Sprintf("Unknown command: /%s", commandName))
}

func (p *chatPage) SetSize(width, height int) tea.Cmd {
	return p.layout.SetSize(width, height)
}

func (p *chatPage) GetSize() (int, int) {
	return p.layout.GetSize()
}

func (p *chatPage) View() string {
	layoutView := p.layout.View()

	if p.showCompletionDialog {
		_, layoutHeight := p.layout.GetSize()
		editorWidth, editorHeight := p.editor.GetSize()

		p.completionDialog.SetWidth(editorWidth)
		overlay := p.completionDialog.View()

		layoutView = layout.PlaceOverlay(
			0,
			layoutHeight-editorHeight-lipgloss.Height(overlay),
			overlay,
			layoutView,
			false,
		)
	}

	return layoutView
}

func (p *chatPage) BindingKeys() []key.Binding {
	bindings := layout.KeyMapToSlice(keyMap)
	bindings = append(bindings, p.messages.BindingKeys()...)
	bindings = append(bindings, p.editor.BindingKeys()...)
	return bindings
}

// playTTS fires TTS for an agent broadcast message. Called in a goroutine — never blocks TUI.
func (p *chatPage) playTTS(text, actorID string) {
	if text == "" {
		return
	}
	baseDir := config.WorkingDirectory()
	cmd, args, ok := agency.PlatformTTSCommand(baseDir)
	if !ok {
		return
	}

	// Replace placeholders. Voice is resolved from actor role suffix.
	voiceID := agency.VoiceIDForRole(actorID)
	finalArgs := make([]string, len(args))
	for i, a := range args {
		a = strings.ReplaceAll(a, "{voice}", voiceID)
		a = strings.ReplaceAll(a, "{text}", text)
		// For {output} use a temp path derived from timestamp.
		a = strings.ReplaceAll(a, "{output}", fmt.Sprintf("/tmp/agency-tts-%d.wav", time.Now().UnixMilli()))
		finalArgs[i] = a
	}

	c := exec.Command(cmd, finalArgs...)
	_ = c.Run() // fire and forget; errors are silently ignored
}

func NewChatPage(app *app.App) tea.Model {
	cg := completions.NewFileAndFolderContextGroup()
	completionDialog := dialog.NewCompletionDialogCmp(cg)

	messagesContainer := layout.NewContainer(
		chat.NewMessagesCmp(app),
		layout.WithPadding(1, 1, 0, 1),
	)
	editorContainer := layout.NewContainer(
		chat.NewEditorCmp(app),
		layout.WithBorder(true, false, false, false),
	)
	return &chatPage{
		app:              app,
		editor:           editorContainer,
		messages:         messagesContainer,
		completionDialog: completionDialog,
		layout: layout.NewSplitPane(
			layout.WithLeftPanel(messagesContainer),
			layout.WithBottomPanel(editorContainer),
		),
	}
}
