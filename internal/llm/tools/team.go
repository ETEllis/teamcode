package tools

import (
	"context"
	"encoding/json"
	"fmt"
	"slices"
	"strings"

	"github.com/ETEllis/teamcode/internal/config"
	"github.com/ETEllis/teamcode/internal/orchestration"
	"github.com/ETEllis/teamcode/internal/team"
)

const (
	TeamBootstrapToolName     = "team_bootstrap"
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
	AgencyGenesisToolName     = "agency_genesis"
	OfficeStatusToolName      = "office_status"
)

type TeamTool struct {
	service *team.Service
}

type TeamBootstrapRole struct {
	Name              string `json:"name"`
	Responsible       string `json:"responsible"`
	CurrentFocus      string `json:"current_focus"`
	Profile           string `json:"profile"`
	Prompt            string `json:"prompt"`
	ReportsTo         string `json:"reports_to"`
	CanSpawnSubagents bool   `json:"can_spawn_subagents"`
}

type TeamBootstrapParams struct {
	TeamName         string                 `json:"team_name"`
	Objective        string                 `json:"objective"`
	LeadName         string                 `json:"lead_name"`
	TemplateName     string                 `json:"template_name"`
	Roles            []TeamBootstrapRole    `json:"roles"`
	SpawnTeammates   *bool                  `json:"spawn_teammates"`
	WorkingAgreement *team.WorkingAgreement `json:"working_agreement"`
}

type TeamBootstrapTool struct {
	workerSpawnBase
}

type AgencyGenesisParams struct {
	OfficeName       string                 `json:"office_name"`
	Objective        string                 `json:"objective"`
	ConstitutionName string                 `json:"constitution_name"`
	LeadName         string                 `json:"lead_name"`
	Roles            []TeamBootstrapRole    `json:"roles"`
	SpawnAgents      *bool                  `json:"spawn_agents"`
	WorkingAgreement *team.WorkingAgreement `json:"working_agreement"`
	OfficeMode       string                 `json:"office_mode"`
	Schedule         string                 `json:"schedule"`
	GenesisBrief     string                 `json:"genesis_brief"`
}

type AgencyGenesisTool struct {
	base    *TeamBootstrapTool
	service *team.Service
}

type OfficeStatusTool struct {
	service *team.Service
	manager *orchestration.Manager
}

func NewTeamBootstrapTool(service *team.Service, manager *orchestration.Manager) *TeamBootstrapTool {
	return &TeamBootstrapTool{workerSpawnBase{service: service, manager: manager}}
}

func NewAgencyGenesisTool(service *team.Service, manager *orchestration.Manager) *AgencyGenesisTool {
	return &AgencyGenesisTool{
		base:    NewTeamBootstrapTool(service, manager),
		service: service,
	}
}

func NewOfficeStatusTool(service *team.Service, manager *orchestration.Manager) *OfficeStatusTool {
	return &OfficeStatusTool{service: service, manager: manager}
}

func (t *TeamBootstrapTool) Info() ToolInfo {
	return ToolInfo{
		Name:        TeamBootstrapToolName,
		Description: "Bootstrap a persistent team or office with specialist roles, a shared board, and optional agent sessions. Prefer this over manually wiring a coordinated multi-agent workspace when the user asks for structured collaboration.",
		Parameters: map[string]any{
			"team_name":         "string - Name of the team",
			"objective":         "string - Shared mission for the team",
			"lead_name":         "string - Optional leader name, defaults to `team-lead`",
			"template_name":     "string - Optional configured team template or compatibility constitution template",
			"roles":             "array - Optional specialist role objects `{name,responsible,current_focus,profile,prompt}`",
			"spawn_teammates":   "bool - When true, immediately spawn the lead and specialist teammates",
			"working_agreement": "object - Optional working agreement override for leadership, delegation, review, and messaging rules",
		},
		Required: []string{"team_name", "objective"},
	}
}

func (t *AgencyGenesisTool) Info() ToolInfo {
	return ToolInfo{
		Name:        AgencyGenesisToolName,
		Description: "Stand up an Agency office from a mission, constitution, roster, and optional schedule. This is the Agency-native genesis surface for persistent coordinated work.",
		Parameters: map[string]any{
			"office_name":       "string - Name of the office to create or reconfigure",
			"objective":         "string - Shared mission for the office",
			"constitution_name": "string - Optional Agency constitution such as solo, coding-office, or full-agency",
			"lead_name":         "string - Optional office lead name, defaults to `office-lead`",
			"roles":             "array - Optional role objects `{name,responsible,current_focus,profile,prompt}`",
			"spawn_agents":      "bool - When true, immediately spawn the office lead and specialist agents",
			"working_agreement": "object - Optional override for leadership, delegation, review, and messaging rules",
			"office_mode":       "string - Optional office runtime mode or governance note",
			"schedule":          "string - Optional schedule, cadence, or office-hours note",
			"genesis_brief":     "string - Optional extra genesis context to record in the office charter",
		},
		Required: []string{"office_name", "objective"},
	}
}

func defaultBootstrapRoles(objective string) []TeamBootstrapRole {
	return []TeamBootstrapRole{
		{
			Name:         "lead",
			Responsible:  "Own the plan, coordinate the office, assign work, keep the board accurate, and decide when to spawn bounded agents.",
			CurrentFocus: objective,
			Profile:      "coder",
			Prompt:       fmt.Sprintf("You are the office lead. Coordinate the work, break the objective into tasks, keep agents aligned, use direct chatter and broadcasts deliberately, and spawn bounded agents when focused execution helps.\n\nObjective: %s", objective),
		},
		{
			Name:         "implementer",
			Responsible:  "Build the main code changes and keep implementation moving.",
			CurrentFocus: objective,
			Profile:      "coder",
			Prompt:       fmt.Sprintf("You are the implementation specialist. Build the main changes for the team objective, report progress clearly, and hand off for review when a meaningful slice is ready.\n\nObjective: %s", objective),
		},
		{
			Name:         "reviewer",
			Responsible:  "Review quality, tests, regressions, and release readiness.",
			CurrentFocus: "verification",
			Profile:      "coder",
			Prompt:       fmt.Sprintf("You are the reviewer. Focus on test coverage, regressions, packaging, and integration quality. Send direct feedback, formal handoffs, and readiness signals when review is complete.\n\nObjective: %s", objective),
		},
		{
			Name:         "researcher",
			Responsible:  "Trace code paths, compare upstream behavior, and gather evidence for the team.",
			CurrentFocus: "analysis",
			Profile:      "coder",
			Prompt:       fmt.Sprintf("You are the researcher. Gather codebase evidence, compare behavior with references when needed, and summarize findings so the rest of the team can move faster.\n\nObjective: %s", objective),
		},
	}
}

func (t *AgencyGenesisTool) Run(ctx context.Context, call ToolCall) (ToolResponse, error) {
	var params AgencyGenesisParams
	if err := json.Unmarshal([]byte(call.Input), &params); err != nil {
		return NewTextErrorResponse(fmt.Sprintf("error parsing parameters: %s", err)), nil
	}
	if params.OfficeName == "" {
		return NewTextErrorResponse("office_name is required"), nil
	}
	if params.Objective == "" {
		return NewTextErrorResponse("objective is required"), nil
	}

	templateName := constitutionTemplateName(params.ConstitutionName)
	bootstrap := TeamBootstrapParams{
		TeamName:         params.OfficeName,
		Objective:        params.Objective,
		LeadName:         params.LeadName,
		TemplateName:     templateName,
		Roles:            params.Roles,
		SpawnTeammates:   params.SpawnAgents,
		WorkingAgreement: agencyWorkingAgreement(params.ConstitutionName, params.OfficeMode, params.Schedule, params.WorkingAgreement),
	}
	if bootstrap.LeadName == "" {
		bootstrap.LeadName = "office-lead"
	}

	payload, err := json.Marshal(bootstrap)
	if err != nil {
		return ToolResponse{}, fmt.Errorf("failed to marshal agency genesis params: %w", err)
	}
	resp, err := t.base.Run(ctx, ToolCall{
		ID:    call.ID,
		Name:  TeamBootstrapToolName,
		Input: string(payload),
	})
	if err != nil {
		return resp, err
	}

	charter := agencyCharter(params.Objective, params.ConstitutionName, params.OfficeMode, params.Schedule, params.GenesisBrief)
	updates := map[string]any{"charter": charter}
	if params.ConstitutionName != "" {
		updates["template"] = params.ConstitutionName
	}
	if _, updateErr := t.service.Context.UpdateContext(ctx, params.OfficeName, updates); updateErr != nil {
		return NewTextErrorResponse(fmt.Sprintf("failed to update office charter: %s", updateErr)), nil
	}

	lines := []string{
		fmt.Sprintf("Office %s initialized", params.OfficeName),
		fmt.Sprintf("Objective: %s", params.Objective),
	}
	if params.ConstitutionName != "" {
		lines = append(lines, fmt.Sprintf("Constitution: %s", params.ConstitutionName))
	}
	if params.OfficeMode != "" {
		lines = append(lines, fmt.Sprintf("Office mode: %s", params.OfficeMode))
	}
	if params.Schedule != "" {
		lines = append(lines, fmt.Sprintf("Schedule: %s", params.Schedule))
	}
	if resp.Content != "" {
		lines = append(lines, resp.Content)
	}
	resp.Content = strings.Join(lines, "\n")
	return resp, nil
}

func (t *TeamBootstrapTool) Run(ctx context.Context, call ToolCall) (ToolResponse, error) {
	var params TeamBootstrapParams
	if err := json.Unmarshal([]byte(call.Input), &params); err != nil {
		return NewTextErrorResponse(fmt.Sprintf("error parsing parameters: %s", err)), nil
	}
	if params.TeamName == "" {
		return NewTextErrorResponse("team_name is required"), nil
	}
	if params.Objective == "" {
		return NewTextErrorResponse("objective is required"), nil
	}

	templateName, template, err := resolveBootstrapTemplate(params.TemplateName)
	if err != nil {
		return NewTextErrorResponse(err.Error()), nil
	}

	leadName := params.LeadName
	if leadName == "" {
		leadName = "team-lead"
	}
	spawnTeammates := spawnDefault(template)
	if params.SpawnTeammates != nil {
		spawnTeammates = *params.SpawnTeammates
	}

	roles := params.Roles
	if len(roles) == 0 {
		roles = templateRoles(template, params.Objective, leadName)
	}
	if len(roles) == 0 {
		roles = defaultBootstrapRoles(params.Objective)
	}

	roleMap := make(map[string]team.Role, len(roles))
	for _, role := range roles {
		if role.Name == "" {
			return NewTextErrorResponse("each role requires a name"), nil
		}
		roleMap[role.Name] = team.Role{
			Name:              role.Name,
			Responsible:       role.Responsible,
			CurrentFocus:      role.CurrentFocus,
			Profile:           defaultProfile(role.Profile),
			Prompt:            role.Prompt,
			ReportsTo:         role.ReportsTo,
			CanSpawnSubagents: role.CanSpawnSubagents,
		}
	}

	tc, err := t.service.Context.CreateContext(ctx, params.TeamName, params.Objective, roleMap)
	if err != nil {
		return NewTextErrorResponse(fmt.Sprintf("failed to create team context: %s", err)), nil
	}
	tc, err = t.service.Context.UpdateContext(ctx, params.TeamName, map[string]any{
		"leader":   leadName,
		"template": templateName,
	})
	if err != nil {
		return NewTextErrorResponse(fmt.Sprintf("failed to update team leader: %s", err)), nil
	}
	agreement := templateAgreement(template)
	if params.WorkingAgreement != nil {
		agreement = *params.WorkingAgreement
	}
	tc, err = t.service.Context.UpdateWorkingAgreement(ctx, params.TeamName, agreement)
	if err != nil {
		return NewTextErrorResponse(fmt.Sprintf("failed to update team policies: %s", err)), nil
	}
	if _, err := t.service.Board.CreateBoard(ctx, params.TeamName); err != nil {
		return NewTextErrorResponse(fmt.Sprintf("failed to create team board: %s", err)), nil
	}
	_, _ = t.service.Context.AddGoal(ctx, params.TeamName, team.Goal{
		ID:          "primary-objective",
		Description: params.Objective,
		Status:      "active",
	})
	_, _ = t.service.Board.AddTaskToColumn(ctx, params.TeamName, "plan-team-execution", "ready")
	_, _ = t.service.Board.AddTaskToColumn(ctx, params.TeamName, "implement-objective", "backlog")
	_, _ = t.service.Board.AddTaskToColumn(ctx, params.TeamName, "review-and-ship", "backlog")

	bootstrapSummary := []string{
		fmt.Sprintf("Team %s bootstrapped", params.TeamName),
		fmt.Sprintf("Template: %s", templateName),
		fmt.Sprintf("Leader: %s", leadName),
		fmt.Sprintf("Roles: %d", len(roleMap)),
	}

	var workers []orchestration.Worker
	if spawnTeammates {
		for _, role := range roles {
			memberName := role.Name
			if role.Name == "lead" {
				memberName = leadName
			}
			worker, spawnErr := t.spawn(ctx, workerSpawnParams{
				TeamName: params.TeamName,
				Name:     memberName,
				RoleName: role.Name,
				Prompt:   role.Prompt,
				Title:    "Team member: " + memberName,
				Profile:  role.Profile,
				Wait:     false,
			}, orchestration.WorkerKindTeammate)
			if spawnErr != nil {
				return NewTextErrorResponse(fmt.Sprintf("failed to spawn %s: %s", memberName, spawnErr)), nil
			}
			workers = append(workers, worker)
			reportsTo := roleMap[role.Name].ReportsTo
			if reportsTo == "" && role.Name != "lead" {
				reportsTo = leadName
			}
			if role.Name == "lead" {
				reportsTo = ""
			}
			_, _ = t.service.Members.Upsert(context.Background(), params.TeamName, team.Member{
				AgentName:         memberName,
				RoleName:          role.Name,
				SessionID:         worker.SessionID,
				Kind:              string(worker.Kind),
				Profile:           worker.Profile,
				Status:            string(worker.Status),
				ReportsTo:         reportsTo,
				Leader:            role.Name == "lead",
				CanSpawnSubagents: roleMap[role.Name].CanSpawnSubagents,
			})
			if role.Name != "lead" {
				_, _ = t.service.Context.AssignAgentToRole(context.Background(), params.TeamName, role.Name, memberName)
			}
		}
		bootstrapSummary = append(bootstrapSummary, fmt.Sprintf("Spawned agents: %d", len(workers)))
	}

	return WithResponseMetadata(NewTextResponse(strings.Join(bootstrapSummary, "\n")), map[string]any{
		"context": tc,
		"workers": workers,
	}), nil
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
		Description: "Create or update a persistent office or team context. Use this compatibility surface when you want low-level control instead of full agency_genesis.",
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
		Description: "Add a role definition to an office or team constitution.",
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
		Description: "Assign a named office agent or teammate to a configured role.",
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
		Description: "Create a work item on the shared office or team task board.",
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
		Description: "Move a work item between shared board columns and optionally note the assigned office agent.",
		Parameters: map[string]any{
			"team_name":   "string - Name of the team",
			"task_id":     "string - Task identifier",
			"from_column": "string - Current column",
			"to_column":   "string - Destination column",
			"agent":       "string - Optional office agent assignment",
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
		Description: "Create a formal handoff between office agents or teammates.",
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
			"accepted_by": "string - Receiving office agent",
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
		Description: "Read an office agent inbox for direct chatter and handoff coordination.",
		Parameters: map[string]any{
			"team_name":   "string - Name of the team",
			"agent_name":  "string - Office agent name",
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
		Description: "Send a direct local message between office agents or teammates.",
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
		Description: "Broadcast a message to the whole office or team.",
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
		Description: "Read the current team or office snapshot: roles, members, task board, handoffs, and worker status.",
		Parameters: map[string]any{
			"team_name": "string - Name of the team",
		},
		Required: []string{"team_name"},
	}
}

func (t *OfficeStatusTool) Info() ToolInfo {
	return ToolInfo{
		Name:        OfficeStatusToolName,
		Description: "Read the current Agency office snapshot: constitution, governance, roster, task board, handoffs, and worker status.",
		Parameters: map[string]any{
			"team_name": "string - Office name",
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
	if tc == nil {
		tc, err = t.service.Context.CreateContext(ctx, params.TeamName, "", nil)
		if err != nil {
			return NewTextErrorResponse(fmt.Sprintf("failed to create missing team context: %s", err)), nil
		}
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
		fmt.Sprintf("Template: %s", tc.Template),
		fmt.Sprintf("Leader: %s", tc.Leader),
		fmt.Sprintf("Leadership: %s", tc.WorkingAgreement.LeadershipMode),
		fmt.Sprintf("Roles: %d", len(tc.Roles)),
		fmt.Sprintf("Members: %d", len(members)),
		fmt.Sprintf("Open handoffs: %d", len(handoffs)),
	}
	for column, tasks := range board.Columns {
		lines = append(lines, fmt.Sprintf("%s: %d", column, len(tasks)))
	}

	return WithResponseMetadata(NewTextResponse(strings.Join(lines, "\n")), snapshot), nil
}

func (t *OfficeStatusTool) Run(ctx context.Context, call ToolCall) (ToolResponse, error) {
	var params TeamStatusParams
	if err := json.Unmarshal([]byte(call.Input), &params); err != nil {
		return NewTextErrorResponse(fmt.Sprintf("error parsing parameters: %s", err)), nil
	}

	tc, err := t.service.Context.ReadContext(ctx, params.TeamName)
	if err != nil {
		return NewTextErrorResponse(fmt.Sprintf("failed to read office context: %s", err)), nil
	}
	if tc == nil {
		tc, err = t.service.Context.CreateContext(ctx, params.TeamName, "", nil)
		if err != nil {
			return NewTextErrorResponse(fmt.Sprintf("failed to create missing office context: %s", err)), nil
		}
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

	cfg := config.Get()
	active := config.ActiveConstitution(cfg)
	defaultSchedule := ""
	soloConstitution := ""
	if active.DefaultSchedule != "" {
		defaultSchedule = active.DefaultSchedule
	} else if cfg != nil {
		defaultSchedule = cfg.Agency.Schedules.DefaultCadence
	}
	if cfg != nil {
		soloConstitution = cfg.Agency.SoloConstitution
	}

	snapshot := map[string]any{
		"context":            tc,
		"board":              board,
		"members":            members,
		"handoffs":           handoffs,
		"workers":            t.manager.ListByTeam(params.TeamName),
		"constitution":       tc.Template,
		"activeConstitution": active.Name,
		"soloConstitution":   soloConstitution,
		"runtimeMode":        active.RuntimeMode,
		"entryMode":          active.EntryMode,
		"consensusMode":      active.Policies.ConsensusMode,
	}

	lines := []string{
		fmt.Sprintf("Office: %s", params.TeamName),
		fmt.Sprintf("Constitution: %s", tc.Template),
		fmt.Sprintf("Lead: %s", tc.Leader),
		fmt.Sprintf("Governance: %s", tc.WorkingAgreement.LeadershipMode),
		fmt.Sprintf("Delegation: %s", tc.WorkingAgreement.DelegationMode),
		fmt.Sprintf("Members: %d", len(members)),
		fmt.Sprintf("Open handoffs: %d", len(handoffs)),
	}
	if cfg != nil && cfg.Agency.Office.Mode != "" {
		lines = append(lines, fmt.Sprintf("Office mode: %s", cfg.Agency.Office.Mode))
	}
	if soloConstitution != "" {
		lines = append(lines, fmt.Sprintf("Solo constitution: %s", soloConstitution))
	}
	if active.RuntimeMode != "" {
		lines = append(lines, fmt.Sprintf("Runtime mode: %s", active.RuntimeMode))
	}
	if active.EntryMode != "" {
		lines = append(lines, fmt.Sprintf("Entry mode: %s", active.EntryMode))
	}
	if active.Policies.ConsensusMode != "" {
		lines = append(lines, fmt.Sprintf("Consensus mode: %s", active.Policies.ConsensusMode))
	}
	if defaultSchedule != "" {
		lines = append(lines, fmt.Sprintf("Default schedule: %s", defaultSchedule))
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
		Description: "Spawn a persistent office agent session inside an office or team. This compatibility surface creates durable named collaborators that appear in office status, mailboxes, handoffs, and role assignment.",
		Parameters: map[string]any{
			"team_name": "string - Name of the team",
			"name":      "string - Office agent name",
			"role_name": "string - Optional office role",
			"prompt":    "string - Initial mission or charter for the office agent",
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

	label := fmt.Sprintf("Office agent %s spawned as %s", worker.Name, worker.ID)
	if params.Wait {
		label = fmt.Sprintf("Office agent %s finished with status %s", worker.Name, worker.Status)
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
		Description: "Spawn a bounded delegated agent for a scoped task. Delegates are separate from durable office agents but can be launched from solo or office workflows.",
		Parameters: map[string]any{
			"name":    "string - Optional short label",
			"prompt":  "string - Task for the bounded delegate",
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

	label := fmt.Sprintf("Delegate %s spawned as %s", worker.Name, worker.ID)
	if params.Wait {
		label = fmt.Sprintf("Delegate %s finished with status %s", worker.Name, worker.Status)
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
			name = "office-agent"
		} else {
			name = "delegate"
		}
	}

	teamName := params.TeamName
	reportsTo := ""
	canSpawnSubagents := true
	if teamName == "" {
		inferredTeam, member, err := t.service.Members.FindBySessionID(context.Background(), sessionID)
		if err != nil {
			return orchestration.Worker{}, fmt.Errorf("failed to inspect worker membership: %w", err)
		}
		if member != nil {
			teamName = inferredTeam
			reportsTo = member.AgentName
			canSpawnSubagents = member.CanSpawnSubagents
		}
	}
	if kind == orchestration.WorkerKindSubagent && teamName != "" && !canSpawnSubagents {
		return orchestration.Worker{}, fmt.Errorf("subagent spawning is disabled for this teammate")
	}

	worker, err := t.manager.Spawn(ctx, orchestration.SpawnParams{
		ParentSessionID: sessionID,
		Name:            name,
		TeamName:        teamName,
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
		_, _ = t.service.Context.CreateContext(context.Background(), teamName, "", nil)
		if params.RoleName != "" {
			_, _ = t.service.Context.AssignAgentToRole(context.Background(), teamName, params.RoleName, name)
		}
		leader := params.RoleName == "lead"
		if reportsTo == "" && !leader {
			tc, _ := t.service.Context.ReadContext(context.Background(), teamName)
			if tc != nil {
				reportsTo = tc.Leader
			}
		}
		roleCanSpawn := kind == orchestration.WorkerKindTeammate
		if role, err := t.teamRole(teamName, params.RoleName); err == nil && role != nil {
			roleCanSpawn = role.CanSpawnSubagents
		}
		_, _ = t.service.Members.Upsert(context.Background(), teamName, team.Member{
			AgentName:         name,
			RoleName:          params.RoleName,
			SessionID:         worker.SessionID,
			Kind:              string(kind),
			Profile:           worker.Profile,
			Status:            string(worker.Status),
			ReportsTo:         reportsTo,
			Leader:            leader,
			CanSpawnSubagents: roleCanSpawn,
		})
	}

	if params.Wait {
		worker, err = t.manager.Wait(ctx, worker.ID)
		if err != nil {
			return orchestration.Worker{}, fmt.Errorf("failed to wait for worker: %w", err)
		}
		if kind == orchestration.WorkerKindTeammate {
			_, _ = t.service.Members.Upsert(context.Background(), teamName, team.Member{
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

func (t *workerSpawnBase) teamRole(teamName, roleName string) (*team.Role, error) {
	if teamName == "" || roleName == "" {
		return nil, nil
	}
	tc, err := t.service.Context.ReadContext(context.Background(), teamName)
	if err != nil || tc == nil {
		return nil, err
	}
	role, ok := tc.Roles[roleName]
	if !ok {
		return nil, nil
	}
	return &role, nil
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
	desc := "Wait for a spawned bounded delegate to finish."
	if t.kind == orchestration.WorkerKindTeammate {
		name = TeammateWaitToolName
		desc = "Wait for a spawned office agent to finish its current run."
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

func resolveBootstrapTemplate(name string) (string, config.TeamTemplate, error) {
	cfg := config.Get()
	if cfg == nil {
		return "", config.TeamTemplate{}, fmt.Errorf("config is not loaded")
	}
	teamCfg := cfg.Team
	if name == "" {
		name = teamCfg.DefaultTemplate
	}
	if name == "" {
		name = "leader-led"
	}
	tmpl, ok := teamCfg.Templates[name]
	if !ok {
		keys := make([]string, 0, len(teamCfg.Templates))
		for key := range teamCfg.Templates {
			keys = append(keys, key)
		}
		slices.Sort(keys)
		return "", config.TeamTemplate{}, fmt.Errorf("unknown team template %q; available templates: %s", name, strings.Join(keys, ", "))
	}
	return name, tmpl, nil
}

func templateRoles(tmpl config.TeamTemplate, objective, leadName string) []TeamBootstrapRole {
	roles := make([]TeamBootstrapRole, 0, len(tmpl.Roles))
	for _, role := range tmpl.Roles {
		rolePrompt := role.Prompt
		if rolePrompt == "" {
			rolePrompt = fmt.Sprintf("You are %s. %s\n\nObjective: %s", role.Name, role.Responsible, objective)
		}
		roleName := role.Name
		if roleName == "" {
			continue
		}
		if roleName == "lead" && leadName != "" {
			rolePrompt = fmt.Sprintf("%s\n\nYou are named %s for this team.", rolePrompt, leadName)
		}
		roles = append(roles, TeamBootstrapRole{
			Name:              roleName,
			Responsible:       role.Responsible,
			CurrentFocus:      defaultFocus(role.CurrentFocus, objective),
			Profile:           defaultProfile(role.Profile),
			Prompt:            rolePrompt,
			ReportsTo:         role.ReportsTo,
			CanSpawnSubagents: role.CanSpawnSubagents != nil && *role.CanSpawnSubagents,
		})
	}
	return roles
}

func templateAgreement(tmpl config.TeamTemplate) team.WorkingAgreement {
	agreement := team.WorkingAgreement{
		CommitMessageFormat: tmpl.Policies.CommitMessageFormat,
		HandoffRequires:     tmpl.Policies.HandoffRequires,
		LeadershipMode:      tmpl.LeadershipMode,
		DelegationMode:      tmpl.Policies.DelegationMode,
		LocalChatDefault:    tmpl.Policies.LocalChatDefault,
		ReviewRouting:       tmpl.Policies.ReviewRouting,
		SynthesisRouting:    tmpl.Policies.SynthesisRouting,
	}
	if tmpl.Policies.MaxWIP != nil {
		agreement.MaxWIP = *tmpl.Policies.MaxWIP
	}
	if tmpl.Policies.ReviewRequired != nil {
		agreement.ReviewRequired = *tmpl.Policies.ReviewRequired
	}
	if tmpl.Policies.AllowsSubagents != nil {
		agreement.AllowsSubagents = *tmpl.Policies.AllowsSubagents
	}
	if tmpl.Policies.AllowsPeerMessaging != nil {
		agreement.AllowsPeerMessaging = *tmpl.Policies.AllowsPeerMessaging
	}
	if tmpl.Policies.AllowsBroadcasts != nil {
		agreement.AllowsBroadcasts = *tmpl.Policies.AllowsBroadcasts
	}
	return agreement
}

func spawnDefault(tmpl config.TeamTemplate) bool {
	if tmpl.SpawnTeammates == nil {
		return true
	}
	return *tmpl.SpawnTeammates
}

func defaultFocus(value, objective string) string {
	if value != "" {
		return value
	}
	return objective
}

func defaultProfile(value string) string {
	if value == "" {
		return "coder"
	}
	return value
}

func constitutionTemplateName(name string) string {
	if name == "" {
		return ""
	}
	cfg := config.Get()
	if cfg == nil {
		return name
	}
	if constitution, ok := cfg.Agency.Constitutions[name]; ok && constitution.TeamTemplate != "" {
		return constitution.TeamTemplate
	}
	return name
}

func agencyWorkingAgreement(constitutionName, officeMode, schedule string, override *team.WorkingAgreement) *team.WorkingAgreement {
	cfg := config.Get()
	agreement := team.WorkingAgreement{}
	if cfg != nil {
		if constitution, ok := cfg.Agency.Constitutions[constitutionName]; ok {
			if constitution.Governance != "" {
				agreement.LeadershipMode = constitution.Governance
			}
			if constitution.Policies.SpawnMode != "" {
				agreement.DelegationMode = constitution.Policies.SpawnMode
			}
			if constitution.Policies.PublicationPolicy != "" {
				agreement.ReviewRouting = constitution.Policies.PublicationPolicy
			}
			if constitution.Policies.ConsensusMode != "" {
				agreement.SynthesisRouting = constitution.Policies.ConsensusMode
			}
			if constitution.Policies.DefaultQuorum > 0 {
				agreement.MaxWIP = constitution.Policies.DefaultQuorum
			}
		}
	}
	if officeMode != "" {
		agreement.LocalChatDefault = officeMode
	}
	if schedule != "" {
		agreement.HandoffRequires = []string{"summary", "artifacts", "shift-handoff"}
	}
	if override == nil && isZeroWorkingAgreement(agreement) {
		return nil
	}
	if override == nil {
		return &agreement
	}
	merged := *override
	if merged.LeadershipMode == "" {
		merged.LeadershipMode = agreement.LeadershipMode
	}
	if merged.DelegationMode == "" {
		merged.DelegationMode = agreement.DelegationMode
	}
	if merged.ReviewRouting == "" {
		merged.ReviewRouting = agreement.ReviewRouting
	}
	if merged.SynthesisRouting == "" {
		merged.SynthesisRouting = agreement.SynthesisRouting
	}
	if merged.LocalChatDefault == "" {
		merged.LocalChatDefault = agreement.LocalChatDefault
	}
	if len(merged.HandoffRequires) == 0 {
		merged.HandoffRequires = agreement.HandoffRequires
	}
	if merged.MaxWIP == 0 {
		merged.MaxWIP = agreement.MaxWIP
	}
	return &merged
}

func agencyCharter(objective, constitutionName, officeMode, schedule, genesisBrief string) string {
	lines := []string{objective}
	if constitutionName != "" {
		lines = append(lines, fmt.Sprintf("Constitution: %s", constitutionName))
	}
	if officeMode != "" {
		lines = append(lines, fmt.Sprintf("Office mode: %s", officeMode))
	}
	if schedule != "" {
		lines = append(lines, fmt.Sprintf("Schedule: %s", schedule))
	}
	if genesisBrief != "" {
		lines = append(lines, fmt.Sprintf("Genesis brief: %s", genesisBrief))
	}
	return strings.Join(lines, "\n")
}

func isZeroWorkingAgreement(agreement team.WorkingAgreement) bool {
	return agreement.CommitMessageFormat == "" &&
		agreement.MaxWIP == 0 &&
		len(agreement.HandoffRequires) == 0 &&
		!agreement.ReviewRequired &&
		!agreement.AllowsSubagents &&
		agreement.LeadershipMode == "" &&
		agreement.DelegationMode == "" &&
		agreement.LocalChatDefault == "" &&
		agreement.ReviewRouting == "" &&
		agreement.SynthesisRouting == "" &&
		!agreement.AllowsPeerMessaging &&
		!agreement.AllowsBroadcasts &&
		len(agreement.ApprovalRequiredFor) == 0
}
