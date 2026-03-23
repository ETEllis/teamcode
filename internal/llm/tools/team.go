package tools

import (
	"context"
	"encoding/json"
	"fmt"
	"strings"

	"github.com/ETEllis/teamcode/internal/orchestration"
	"github.com/ETEllis/teamcode/internal/team"
)

const (
	TeamCreateContextToolName = "team_create_context"
	TeamAddRoleToolName       = "team_add_role"
	TeamAssignRoleToolName    = "team_assign_role"
	TaskCreateToolName        = "task_create"
	TaskMoveToolName          = "task_move"
	HandoffCreateToolName     = "handoff_create"
	HandoffAcceptToolName     = "handoff_accept"
	InboxReadToolName         = "inbox_read"
	TeamMessageSendToolName   = "team_message_send"
	TeamBroadcastToolName     = "team_broadcast"
	TeamStatusToolName        = "team_status"
	TeammateSpawnToolName     = "teammate_spawn"
	TeammateWaitToolName      = "teammate_wait"
	SubagentSpawnToolName     = "subagent_spawn"
	SubagentWaitToolName      = "subagent_wait"
)

type TeamTool struct {
	service *team.Service
}

type TeamCreateContextParams struct {
	TeamName string               `json:"team_name"`
	Charter  string               `json:"charter"`
	Roles    map[string]team.Role `json:"roles"`
}

func NewTeamTool(service *team.Service) *TeamTool {
	return &TeamTool{service: service}
}

func (t *TeamTool) Info() ToolInfo {
	return ToolInfo{
		Name:        TeamCreateContextToolName,
		Description: "Create or update a team. Teams are persistent collaborating groups with roles, task state, mailboxes, and teammates.",
		Parameters: map[string]any{
			"team_name": "string - Name of the team",
			"charter":   "string - Team mission or operating objective",
			"roles":     "object - Optional role definitions keyed by role name",
		},
		Required: []string{"team_name"},
	}
}

func (t *TeamTool) Run(ctx context.Context, call ToolCall) (ToolResponse, error) {
	var params TeamCreateContextParams
	if err := json.Unmarshal([]byte(call.Input), &params); err != nil {
		return NewTextErrorResponse(fmt.Sprintf("error parsing parameters: %s", err)), nil
	}
	if params.TeamName == "" {
		return NewTextErrorResponse("team_name is required"), nil
	}

	tc, err := t.service.Context.CreateContext(ctx, params.TeamName, params.Charter, params.Roles)
	if err != nil {
		return NewTextErrorResponse(fmt.Sprintf("failed to create team context: %s", err)), nil
	}
	if _, err := t.service.Board.CreateBoard(ctx, params.TeamName); err != nil {
		return NewTextErrorResponse(fmt.Sprintf("failed to create team board: %s", err)), nil
	}

	return WithResponseMetadata(NewTextResponse(fmt.Sprintf("Team %s is ready", params.TeamName)), tc), nil
}

type TeamAddRoleTool struct {
	service *team.Service
}

type TeamAddRoleParams struct {
	TeamName     string `json:"team_name"`
	RoleName     string `json:"role_name"`
	Responsible  string `json:"responsible"`
	CurrentFocus string `json:"current_focus"`
}

func NewTeamAddRoleTool(service *team.Service) *TeamAddRoleTool {
	return &TeamAddRoleTool{service: service}
}

func (t *TeamAddRoleTool) Info() ToolInfo {
	return ToolInfo{
		Name:        TeamAddRoleToolName,
		Description: "Add a role definition to a team.",
		Parameters: map[string]any{
			"team_name":     "string - Name of the team",
			"role_name":     "string - Role identifier",
			"responsible":   "string - Scope of responsibility",
			"current_focus": "string - Optional current focus",
		},
		Required: []string{"team_name", "role_name", "responsible"},
	}
}

func (t *TeamAddRoleTool) Run(ctx context.Context, call ToolCall) (ToolResponse, error) {
	var params TeamAddRoleParams
	if err := json.Unmarshal([]byte(call.Input), &params); err != nil {
		return NewTextErrorResponse(fmt.Sprintf("error parsing parameters: %s", err)), nil
	}

	role := team.Role{
		Name:         params.RoleName,
		Responsible:  params.Responsible,
		CurrentFocus: params.CurrentFocus,
	}
	tc, err := t.service.Context.AddRole(ctx, params.TeamName, params.RoleName, role)
	if err != nil {
		return NewTextErrorResponse(fmt.Sprintf("failed to add role: %s", err)), nil
	}
	return WithResponseMetadata(NewTextResponse(fmt.Sprintf("Role %s added to %s", params.RoleName, params.TeamName)), tc), nil
}

type TeamAssignRoleTool struct {
	service *team.Service
}

type TeamAssignRoleParams struct {
	TeamName  string `json:"team_name"`
	RoleName  string `json:"role_name"`
	AgentName string `json:"agent_name"`
}

func NewTeamAssignRoleTool(service *team.Service) *TeamAssignRoleTool {
	return &TeamAssignRoleTool{service: service}
}

func (t *TeamAssignRoleTool) Info() ToolInfo {
	return ToolInfo{
		Name:        TeamAssignRoleToolName,
		Description: "Assign a named teammate to a team role.",
		Parameters: map[string]any{
			"team_name":  "string - Name of the team",
			"role_name":  "string - Role to assign",
			"agent_name": "string - Named teammate",
		},
		Required: []string{"team_name", "role_name", "agent_name"},
	}
}

func (t *TeamAssignRoleTool) Run(ctx context.Context, call ToolCall) (ToolResponse, error) {
	var params TeamAssignRoleParams
	if err := json.Unmarshal([]byte(call.Input), &params); err != nil {
		return NewTextErrorResponse(fmt.Sprintf("error parsing parameters: %s", err)), nil
	}

	tc, err := t.service.Context.AssignAgentToRole(ctx, params.TeamName, params.RoleName, params.AgentName)
	if err != nil {
		return NewTextErrorResponse(fmt.Sprintf("failed to assign role: %s", err)), nil
	}
	return WithResponseMetadata(NewTextResponse(fmt.Sprintf("%s now owns role %s", params.AgentName, params.RoleName)), tc), nil
}

type TaskBoardTool struct {
	service *team.Service
}

type TaskCreateParams struct {
	TeamName string `json:"team_name"`
	TaskID   string `json:"task_id"`
	Column   string `json:"column"`
}

func NewTaskBoardTool(service *team.Service) *TaskBoardTool {
	return &TaskBoardTool{service: service}
}

func (t *TaskBoardTool) Info() ToolInfo {
	return ToolInfo{
		Name:        TaskCreateToolName,
		Description: "Create a task on the team task board.",
		Parameters: map[string]any{
			"team_name": "string - Name of the team",
			"task_id":   "string - Task identifier",
			"column":    "string - Optional initial column",
		},
		Required: []string{"team_name", "task_id"},
	}
}

func (t *TaskBoardTool) Run(ctx context.Context, call ToolCall) (ToolResponse, error) {
	var params TaskCreateParams
	if err := json.Unmarshal([]byte(call.Input), &params); err != nil {
		return NewTextErrorResponse(fmt.Sprintf("error parsing parameters: %s", err)), nil
	}
	column := params.Column
	if column == "" {
		column = "backlog"
	}

	board, err := t.service.Board.AddTaskToColumn(ctx, params.TeamName, params.TaskID, column)
	if err != nil {
		return NewTextErrorResponse(fmt.Sprintf("failed to add task: %s", err)), nil
	}
	return WithResponseMetadata(NewTextResponse(fmt.Sprintf("Task %s added to %s", params.TaskID, column)), board), nil
}

type TaskMoveTool struct {
	service *team.Service
}

type TaskMoveParams struct {
	TeamName   string `json:"team_name"`
	TaskID     string `json:"task_id"`
	FromColumn string `json:"from_column"`
	ToColumn   string `json:"to_column"`
	Agent      string `json:"agent"`
}

func NewTaskMoveTool(service *team.Service) *TaskMoveTool {
	return &TaskMoveTool{service: service}
}

func (t *TaskMoveTool) Info() ToolInfo {
	return ToolInfo{
		Name:        TaskMoveToolName,
		Description: "Move a task between team board columns.",
		Parameters: map[string]any{
			"team_name":   "string - Name of the team",
			"task_id":     "string - Task identifier",
			"from_column": "string - Current column",
			"to_column":   "string - Destination column",
			"agent":       "string - Optional teammate assignment",
		},
		Required: []string{"team_name", "task_id", "to_column"},
	}
}

func (t *TaskMoveTool) Run(ctx context.Context, call ToolCall) (ToolResponse, error) {
	var params TaskMoveParams
	if err := json.Unmarshal([]byte(call.Input), &params); err != nil {
		return NewTextErrorResponse(fmt.Sprintf("error parsing parameters: %s", err)), nil
	}

	board, err := t.service.Board.MoveTask(ctx, params.TeamName, params.TaskID, params.FromColumn, params.ToColumn, params.Agent)
	if err != nil {
		return NewTextErrorResponse(fmt.Sprintf("failed to move task: %s", err)), nil
	}
	return WithResponseMetadata(NewTextResponse(fmt.Sprintf("Task %s moved to %s", params.TaskID, params.ToColumn)), board), nil
}

type HandoffCreateTool struct {
	service *team.Service
}

type HandoffCreateParams struct {
	TeamName    string   `json:"team_name"`
	TaskID      string   `json:"task_id"`
	FromAgent   string   `json:"from_agent"`
	ToAgent     string   `json:"to_agent"`
	WorkSummary string   `json:"work_summary"`
	Artifacts   []string `json:"artifacts"`
}

func NewHandoffCreateTool(service *team.Service) *HandoffCreateTool {
	return &HandoffCreateTool{service: service}
}

func (t *HandoffCreateTool) Info() ToolInfo {
	return ToolInfo{
		Name:        HandoffCreateToolName,
		Description: "Create a formal handoff between teammates.",
		Parameters: map[string]any{
			"team_name":    "string - Name of the team",
			"task_id":      "string - Task identifier",
			"from_agent":   "string - Sender",
			"to_agent":     "string - Recipient",
			"work_summary": "string - Optional summary",
			"artifacts":    "array - Optional produced artifacts",
		},
		Required: []string{"team_name", "task_id", "from_agent", "to_agent"},
	}
}

func (t *HandoffCreateTool) Run(ctx context.Context, call ToolCall) (ToolResponse, error) {
	var params HandoffCreateParams
	if err := json.Unmarshal([]byte(call.Input), &params); err != nil {
		return NewTextErrorResponse(fmt.Sprintf("error parsing parameters: %s", err)), nil
	}

	h, err := t.service.Handoff.Create(ctx, params.TeamName, params.TaskID, params.FromAgent, params.ToAgent, params.WorkSummary, params.Artifacts, nil)
	if err != nil {
		return NewTextErrorResponse(fmt.Sprintf("failed to create handoff: %s", err)), nil
	}
	return WithResponseMetadata(NewTextResponse(fmt.Sprintf("Handoff %s created", h.ID)), h), nil
}

type HandoffAcceptTool struct {
	service *team.Service
}

type HandoffAcceptParams struct {
	TeamName   string `json:"team_name"`
	HandoffID  string `json:"handoff_id"`
	AcceptedBy string `json:"accepted_by"`
}

func NewHandoffAcceptTool(service *team.Service) *HandoffAcceptTool {
	return &HandoffAcceptTool{service: service}
}

func (t *HandoffAcceptTool) Info() ToolInfo {
	return ToolInfo{
		Name:        HandoffAcceptToolName,
		Description: "Accept a pending handoff.",
		Parameters: map[string]any{
			"team_name":   "string - Name of the team",
			"handoff_id":  "string - Handoff identifier",
			"accepted_by": "string - Receiving teammate",
		},
		Required: []string{"team_name", "handoff_id", "accepted_by"},
	}
}

func (t *HandoffAcceptTool) Run(ctx context.Context, call ToolCall) (ToolResponse, error) {
	var params HandoffAcceptParams
	if err := json.Unmarshal([]byte(call.Input), &params); err != nil {
		return NewTextErrorResponse(fmt.Sprintf("error parsing parameters: %s", err)), nil
	}

	h, err := t.service.Handoff.Accept(ctx, params.TeamName, params.HandoffID, params.AcceptedBy)
	if err != nil {
		return NewTextErrorResponse(fmt.Sprintf("failed to accept handoff: %s", err)), nil
	}
	if h == nil {
		return NewTextErrorResponse("handoff not found"), nil
	}
	return WithResponseMetadata(NewTextResponse(fmt.Sprintf("Handoff %s accepted", h.ID)), h), nil
}

type InboxReadTool struct {
	service *team.Service
}

type InboxReadParams struct {
	TeamName   string `json:"team_name"`
	AgentName  string `json:"agent_name"`
	UnreadOnly bool   `json:"unread_only"`
}

func NewInboxReadTool(service *team.Service) *InboxReadTool {
	return &InboxReadTool{service: service}
}

func (t *InboxReadTool) Info() ToolInfo {
	return ToolInfo{
		Name:        InboxReadToolName,
		Description: "Read a teammate inbox for direct chatter.",
		Parameters: map[string]any{
			"team_name":   "string - Name of the team",
			"agent_name":  "string - Teammate name",
			"unread_only": "bool - Filter unread messages",
		},
		Required: []string{"team_name", "agent_name"},
	}
}

func (t *InboxReadTool) Run(ctx context.Context, call ToolCall) (ToolResponse, error) {
	var params InboxReadParams
	if err := json.Unmarshal([]byte(call.Input), &params); err != nil {
		return NewTextErrorResponse(fmt.Sprintf("error parsing parameters: %s", err)), nil
	}

	msgs, err := t.service.Inbox.ReadInbox(ctx, params.TeamName, params.AgentName, params.UnreadOnly)
	if err != nil {
		return NewTextErrorResponse(fmt.Sprintf("failed to read inbox: %s", err)), nil
	}
	if len(msgs) == 0 {
		return NewTextResponse("No messages"), nil
	}

	lines := make([]string, 0, len(msgs))
	for _, msg := range msgs {
		lines = append(lines, fmt.Sprintf("[%s] %s: %s", msg.Timestamp, msg.From_, msg.Text))
	}
	return WithResponseMetadata(NewTextResponse(strings.Join(lines, "\n")), msgs), nil
}

type TeamMessageSendTool struct {
	service *team.Service
}

type TeamMessageSendParams struct {
	TeamName  string `json:"team_name"`
	FromAgent string `json:"from_agent"`
	ToAgent   string `json:"to_agent"`
	Content   string `json:"content"`
	Summary   string `json:"summary"`
}

func NewTeamMessageSendTool(service *team.Service) *TeamMessageSendTool {
	return &TeamMessageSendTool{service: service}
}

func (t *TeamMessageSendTool) Info() ToolInfo {
	return ToolInfo{
		Name:        TeamMessageSendToolName,
		Description: "Send a direct local message between teammates.",
		Parameters: map[string]any{
			"team_name":  "string - Name of the team",
			"from_agent": "string - Sender",
			"to_agent":   "string - Recipient",
			"content":    "string - Message body",
			"summary":    "string - Optional short summary",
		},
		Required: []string{"team_name", "from_agent", "to_agent", "content"},
	}
}

func (t *TeamMessageSendTool) Run(ctx context.Context, call ToolCall) (ToolResponse, error) {
	var params TeamMessageSendParams
	if err := json.Unmarshal([]byte(call.Input), &params); err != nil {
		return NewTextErrorResponse(fmt.Sprintf("error parsing parameters: %s", err)), nil
	}
	if err := t.service.Inbox.SendMessage(ctx, params.TeamName, params.FromAgent, params.ToAgent, params.Content, params.Summary); err != nil {
		return NewTextErrorResponse(fmt.Sprintf("failed to send message: %s", err)), nil
	}
	return NewTextResponse(fmt.Sprintf("Message sent to %s", params.ToAgent)), nil
}

type TeamBroadcastTool struct {
	service *team.Service
}

type TeamBroadcastParams struct {
	TeamName  string `json:"team_name"`
	FromAgent string `json:"from_agent"`
	Content   string `json:"content"`
	Summary   string `json:"summary"`
}

func NewTeamBroadcastTool(service *team.Service) *TeamBroadcastTool {
	return &TeamBroadcastTool{service: service}
}

func (t *TeamBroadcastTool) Info() ToolInfo {
	return ToolInfo{
		Name:        TeamBroadcastToolName,
		Description: "Broadcast a message to the whole team.",
		Parameters: map[string]any{
			"team_name":  "string - Name of the team",
			"from_agent": "string - Sender",
			"content":    "string - Broadcast body",
			"summary":    "string - Optional short summary",
		},
		Required: []string{"team_name", "from_agent", "content"},
	}
}

func (t *TeamBroadcastTool) Run(ctx context.Context, call ToolCall) (ToolResponse, error) {
	var params TeamBroadcastParams
	if err := json.Unmarshal([]byte(call.Input), &params); err != nil {
		return NewTextErrorResponse(fmt.Sprintf("error parsing parameters: %s", err)), nil
	}
	if err := t.service.Inbox.Broadcast(ctx, params.TeamName, params.FromAgent, params.Content, params.Summary); err != nil {
		return NewTextErrorResponse(fmt.Sprintf("failed to broadcast: %s", err)), nil
	}
	return NewTextResponse("Broadcast delivered"), nil
}

type TeamStatusTool struct {
	service *team.Service
	manager *orchestration.Manager
}

type TeamStatusParams struct {
	TeamName string `json:"team_name"`
}

func NewTeamStatusTool(service *team.Service, manager *orchestration.Manager) *TeamStatusTool {
	return &TeamStatusTool{service: service, manager: manager}
}

func (t *TeamStatusTool) Info() ToolInfo {
	return ToolInfo{
		Name:        TeamStatusToolName,
		Description: "Read the current team snapshot: roles, members, task board, and worker status.",
		Parameters: map[string]any{
			"team_name": "string - Name of the team",
		},
		Required: []string{"team_name"},
	}
}

func (t *TeamStatusTool) Run(ctx context.Context, call ToolCall) (ToolResponse, error) {
	var params TeamStatusParams
	if err := json.Unmarshal([]byte(call.Input), &params); err != nil {
		return NewTextErrorResponse(fmt.Sprintf("error parsing parameters: %s", err)), nil
	}

	tc, err := t.service.Context.ReadContext(ctx, params.TeamName)
	if err != nil {
		return NewTextErrorResponse(fmt.Sprintf("failed to read team context: %s", err)), nil
	}
	board, err := t.service.Board.CreateBoard(ctx, params.TeamName)
	if err != nil {
		return NewTextErrorResponse(fmt.Sprintf("failed to read board: %s", err)), nil
	}
	members, err := t.service.Members.List(ctx, params.TeamName)
	if err != nil {
		return NewTextErrorResponse(fmt.Sprintf("failed to read members: %s", err)), nil
	}
	handoffs, err := t.service.Handoff.List(ctx, params.TeamName, "", "")
	if err != nil {
		return NewTextErrorResponse(fmt.Sprintf("failed to read handoffs: %s", err)), nil
	}

	snapshot := map[string]any{
		"context":  tc,
		"board":    board,
		"members":  members,
		"handoffs": handoffs,
		"workers":  t.manager.ListByTeam(params.TeamName),
	}

	lines := []string{
		fmt.Sprintf("Team: %s", params.TeamName),
		fmt.Sprintf("Roles: %d", len(tc.Roles)),
		fmt.Sprintf("Members: %d", len(members)),
		fmt.Sprintf("Open handoffs: %d", len(handoffs)),
	}
	for column, tasks := range board.Columns {
		lines = append(lines, fmt.Sprintf("%s: %d", column, len(tasks)))
	}

	return WithResponseMetadata(NewTextResponse(strings.Join(lines, "\n")), snapshot), nil
}

type workerSpawnBase struct {
	service *team.Service
	manager *orchestration.Manager
}

type workerSpawnParams struct {
	TeamName string `json:"team_name"`
	Name     string `json:"name"`
	RoleName string `json:"role_name"`
	Prompt   string `json:"prompt"`
	Title    string `json:"title"`
	Profile  string `json:"profile"`
	Wait     bool   `json:"wait"`
}

type TeammateSpawnTool struct {
	workerSpawnBase
}

func NewTeammateSpawnTool(service *team.Service, manager *orchestration.Manager) *TeammateSpawnTool {
	return &TeammateSpawnTool{workerSpawnBase{service: service, manager: manager}}
}

func (t *TeammateSpawnTool) Info() ToolInfo {
	return ToolInfo{
		Name:        TeammateSpawnToolName,
		Description: "Spawn a persistent teammate session inside a team. Teammates are distinct from subagents and can be messaged through the team mailbox.",
		Parameters: map[string]any{
			"team_name": "string - Name of the team",
			"name":      "string - Teammate name",
			"role_name": "string - Optional team role",
			"prompt":    "string - Initial task or charter for the teammate",
			"title":     "string - Optional session title",
			"profile":   "string - Optional profile (`coder` or `task`), defaults to `coder`",
			"wait":      "bool - Wait for the first run to finish",
		},
		Required: []string{"team_name", "name", "prompt"},
	}
}

func (t *TeammateSpawnTool) Run(ctx context.Context, call ToolCall) (ToolResponse, error) {
	var params workerSpawnParams
	if err := json.Unmarshal([]byte(call.Input), &params); err != nil {
		return NewTextErrorResponse(fmt.Sprintf("error parsing parameters: %s", err)), nil
	}

	worker, err := t.spawn(ctx, params, orchestration.WorkerKindTeammate)
	if err != nil {
		return NewTextErrorResponse(err.Error()), nil
	}

	label := fmt.Sprintf("Teammate %s spawned as %s", worker.Name, worker.ID)
	if params.Wait {
		label = fmt.Sprintf("Teammate %s finished with status %s", worker.Name, worker.Status)
	}
	return WithResponseMetadata(NewTextResponse(label), worker), nil
}

type SubagentSpawnTool struct {
	workerSpawnBase
}

func NewSubagentSpawnTool(service *team.Service, manager *orchestration.Manager) *SubagentSpawnTool {
	return &SubagentSpawnTool{workerSpawnBase{service: service, manager: manager}}
}

func (t *SubagentSpawnTool) Info() ToolInfo {
	return ToolInfo{
		Name:        SubagentSpawnToolName,
		Description: "Spawn a formal subagent for a bounded task. Subagents are separate from teams but can also be used by teammates.",
		Parameters: map[string]any{
			"name":    "string - Optional short label",
			"prompt":  "string - Task for the subagent",
			"title":   "string - Optional session title",
			"profile": "string - Optional profile (`coder` or `task`), defaults to `coder`",
			"wait":    "bool - Wait for completion",
		},
		Required: []string{"prompt"},
	}
}

func (t *SubagentSpawnTool) Run(ctx context.Context, call ToolCall) (ToolResponse, error) {
	var params workerSpawnParams
	if err := json.Unmarshal([]byte(call.Input), &params); err != nil {
		return NewTextErrorResponse(fmt.Sprintf("error parsing parameters: %s", err)), nil
	}

	worker, err := t.spawn(ctx, params, orchestration.WorkerKindSubagent)
	if err != nil {
		return NewTextErrorResponse(err.Error()), nil
	}

	label := fmt.Sprintf("Subagent %s spawned as %s", worker.Name, worker.ID)
	if params.Wait {
		label = fmt.Sprintf("Subagent %s finished with status %s", worker.Name, worker.Status)
	}
	return WithResponseMetadata(NewTextResponse(label), worker), nil
}

func (t *workerSpawnBase) spawn(ctx context.Context, params workerSpawnParams, kind orchestration.WorkerKind) (orchestration.Worker, error) {
	sessionID, _ := GetContextValues(ctx)
	if sessionID == "" {
		return orchestration.Worker{}, fmt.Errorf("session_id is required")
	}

	if params.Prompt == "" {
		return orchestration.Worker{}, fmt.Errorf("prompt is required")
	}
	if kind == orchestration.WorkerKindTeammate && params.TeamName == "" {
		return orchestration.Worker{}, fmt.Errorf("team_name is required")
	}

	name := params.Name
	if name == "" {
		if kind == orchestration.WorkerKindTeammate {
			name = "teammate"
		} else {
			name = "subagent"
		}
	}

	worker, err := t.manager.Spawn(ctx, orchestration.SpawnParams{
		ParentSessionID: sessionID,
		Name:            name,
		TeamName:        params.TeamName,
		RoleName:        params.RoleName,
		Kind:            kind,
		Profile:         params.Profile,
		Prompt:          params.Prompt,
		Title:           params.Title,
	})
	if err != nil {
		return orchestration.Worker{}, fmt.Errorf("failed to spawn worker: %w", err)
	}

	if kind == orchestration.WorkerKindTeammate {
		_, _ = t.service.Context.CreateContext(context.Background(), params.TeamName, "", nil)
		if params.RoleName != "" {
			_, _ = t.service.Context.AssignAgentToRole(context.Background(), params.TeamName, params.RoleName, name)
		}
		_, _ = t.service.Members.Upsert(context.Background(), params.TeamName, team.Member{
			AgentName: name,
			RoleName:  params.RoleName,
			SessionID: worker.SessionID,
			Kind:      string(kind),
			Profile:   worker.Profile,
			Status:    string(worker.Status),
		})
	}

	if params.Wait {
		worker, err = t.manager.Wait(ctx, worker.ID)
		if err != nil {
			return orchestration.Worker{}, fmt.Errorf("failed to wait for worker: %w", err)
		}
		if kind == orchestration.WorkerKindTeammate {
			_, _ = t.service.Members.Upsert(context.Background(), params.TeamName, team.Member{
				AgentName:  name,
				RoleName:   params.RoleName,
				SessionID:  worker.SessionID,
				Kind:       string(kind),
				Profile:    worker.Profile,
				Status:     string(worker.Status),
				LastResult: worker.Result,
			})
		}
	}

	return worker, nil
}

type WorkerWaitTool struct {
	manager *orchestration.Manager
	service *team.Service
	kind    orchestration.WorkerKind
}

type WorkerWaitParams struct {
	WorkerID string `json:"worker_id"`
}

func NewTeammateWaitTool(service *team.Service, manager *orchestration.Manager) *WorkerWaitTool {
	return &WorkerWaitTool{service: service, manager: manager, kind: orchestration.WorkerKindTeammate}
}

func NewSubagentWaitTool(manager *orchestration.Manager) *WorkerWaitTool {
	return &WorkerWaitTool{manager: manager, kind: orchestration.WorkerKindSubagent}
}

func (t *WorkerWaitTool) Info() ToolInfo {
	name := SubagentWaitToolName
	desc := "Wait for a spawned subagent to finish."
	if t.kind == orchestration.WorkerKindTeammate {
		name = TeammateWaitToolName
		desc = "Wait for a spawned teammate to finish its current run."
	}
	return ToolInfo{
		Name:        name,
		Description: desc,
		Parameters: map[string]any{
			"worker_id": "string - Worker identifier returned by spawn",
		},
		Required: []string{"worker_id"},
	}
}

func (t *WorkerWaitTool) Run(ctx context.Context, call ToolCall) (ToolResponse, error) {
	var params WorkerWaitParams
	if err := json.Unmarshal([]byte(call.Input), &params); err != nil {
		return NewTextErrorResponse(fmt.Sprintf("error parsing parameters: %s", err)), nil
	}
	worker, err := t.manager.Wait(ctx, params.WorkerID)
	if err != nil {
		return NewTextErrorResponse(fmt.Sprintf("failed to wait: %s", err)), nil
	}
	if t.kind == orchestration.WorkerKindTeammate && t.service != nil && worker.TeamName != "" {
		_, _ = t.service.Members.Upsert(context.Background(), worker.TeamName, team.Member{
			AgentName:  worker.Name,
			RoleName:   worker.RoleName,
			SessionID:  worker.SessionID,
			Kind:       string(worker.Kind),
			Profile:    worker.Profile,
			Status:     string(worker.Status),
			LastResult: worker.Result,
		})
	}
	return WithResponseMetadata(NewTextResponse(fmt.Sprintf("%s finished with status %s", worker.ID, worker.Status)), worker), nil
}
