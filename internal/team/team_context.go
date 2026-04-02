package team

import (
	"context"
	"time"
)

type TeamContext struct {
	TeamName         string           `json:"teamName"`
	Leader           string           `json:"leader,omitempty"`
	Template         string           `json:"template,omitempty"`
	Constitution     string           `json:"constitution,omitempty"`
	RuntimeMode      string           `json:"runtimeMode,omitempty"`
	SharedTruth      string           `json:"sharedTruth,omitempty"`
	Charter          string           `json:"charter"`
	Goals            []Goal           `json:"goals"`
	Roles            map[string]Role  `json:"roles"`
	WorkingAgreement WorkingAgreement `json:"workingAgreement"`
	CreatedAt        int64            `json:"createdAt"`
	UpdatedAt        int64            `json:"updatedAt"`
}

type Goal struct {
	ID          string `json:"id"`
	Description string `json:"description"`
	Status      string `json:"status"`
	CreatedAt   int64  `json:"createdAt,omitempty"`
}

type Role struct {
	Name              string `json:"name"`
	Responsible       string `json:"responsible"`
	CurrentFocus      string `json:"currentFocus,omitempty"`
	Agent             string `json:"agent,omitempty"`
	Profile           string `json:"profile,omitempty"`
	Prompt            string `json:"prompt,omitempty"`
	ReportsTo         string `json:"reportsTo,omitempty"`
	CanSpawnSubagents bool   `json:"canSpawnSubagents,omitempty"`
}

type WorkingAgreement struct {
	CommitMessageFormat string   `json:"commitMessageFormat"`
	MaxWIP              int      `json:"maxWip"`
	HandoffRequires     []string `json:"handoffRequires"`
	ReviewRequired      bool     `json:"reviewRequired"`
	AllowsSubagents     bool     `json:"allowsSubagents"`
	LeadershipMode      string   `json:"leadershipMode,omitempty"`
	DelegationMode      string   `json:"delegationMode,omitempty"`
	LocalChatDefault    string   `json:"localChatDefault,omitempty"`
	ReviewRouting       string   `json:"reviewRouting,omitempty"`
	SynthesisRouting    string   `json:"synthesisRouting,omitempty"`
	AllowsPeerMessaging bool     `json:"allowsPeerMessaging,omitempty"`
	AllowsBroadcasts    bool     `json:"allowsBroadcasts,omitempty"`
	ApprovalRequiredFor []string `json:"approvalRequiredFor,omitempty"`
	WorkspaceMode       string   `json:"workspaceMode,omitempty"`
	PublicationMode     string   `json:"publicationMode,omitempty"`
	ConsensusMode       string   `json:"consensusMode,omitempty"`
	WakePolicy          string   `json:"wakePolicy,omitempty"`
}

type TeamContextService struct {
	store *store
}

func NewTeamContextService(sharedStore *store) *TeamContextService {
	return &TeamContextService{store: sharedStore}
}

func (s *TeamContextService) CreateContext(ctx context.Context, teamName, charter string, roles map[string]Role) (*TeamContext, error) {
	_ = ctx

	existing, err := s.ReadContext(context.Background(), teamName)
	if err != nil {
		return nil, err
	}
	if existing == nil {
		existing = &TeamContext{
			TeamName:     teamName,
			Constitution: "coding-office",
			RuntimeMode:  "collaborative",
			SharedTruth:  "shared-files",
			Roles:        make(map[string]Role),
			Goals:        make([]Goal, 0),
			WorkingAgreement: WorkingAgreement{
				CommitMessageFormat: "type: subject",
				MaxWIP:              3,
				HandoffRequires:     []string{"summary", "artifacts"},
				ReviewRequired:      true,
				AllowsSubagents:     true,
				LeadershipMode:      "leader-led",
				DelegationMode:      "leader-controlled",
				LocalChatDefault:    "direct",
				ReviewRouting:       "lead",
				SynthesisRouting:    "lead",
				AllowsPeerMessaging: true,
				AllowsBroadcasts:    true,
				WorkspaceMode:       "shared",
				PublicationMode:     "direct",
				ConsensusMode:       "lead-approved",
				WakePolicy:          "on-demand",
			},
			CreatedAt: time.Now().UnixMilli(),
		}
	}
	if charter != "" {
		existing.Charter = charter
	}
	if existing.Roles == nil {
		existing.Roles = make(map[string]Role)
	}
	for name, role := range roles {
		existing.Roles[name] = role
	}
	existing.UpdatedAt = time.Now().UnixMilli()
	if err := s.store.writeJSON(teamName, "team_context.json", existing); err != nil {
		return nil, err
	}
	return existing, nil
}

func (s *TeamContextService) ReadContext(ctx context.Context, teamName string) (*TeamContext, error) {
	_ = ctx
	var tc TeamContext
	if err := s.store.readJSON(teamName, "team_context.json", &tc); err != nil {
		return nil, err
	}
	if tc.TeamName == "" {
		return nil, nil
	}
	return &tc, nil
}

func (s *TeamContextService) UpdateContext(ctx context.Context, teamName string, updates map[string]any) (*TeamContext, error) {
	_ = ctx
	tc, err := s.ReadContext(context.Background(), teamName)
	if err != nil {
		return nil, err
	}
	if tc == nil {
		tc, err = s.CreateContext(context.Background(), teamName, "", nil)
		if err != nil {
			return nil, err
		}
	}
	if charter, ok := updates["charter"].(string); ok {
		tc.Charter = charter
	}
	if leader, ok := updates["leader"].(string); ok {
		tc.Leader = leader
	}
	if template, ok := updates["template"].(string); ok {
		tc.Template = template
	}
	if constitution, ok := updates["constitution"].(string); ok {
		tc.Constitution = constitution
	}
	if runtimeMode, ok := updates["runtimeMode"].(string); ok {
		tc.RuntimeMode = runtimeMode
	}
	if sharedTruth, ok := updates["sharedTruth"].(string); ok {
		tc.SharedTruth = sharedTruth
	}
	tc.UpdatedAt = time.Now().UnixMilli()
	if err := s.store.writeJSON(teamName, "team_context.json", tc); err != nil {
		return nil, err
	}
	return tc, nil
}

func (s *TeamContextService) AddRole(ctx context.Context, teamName, roleName string, role Role) (*TeamContext, error) {
	_ = ctx
	tc, err := s.CreateContext(context.Background(), teamName, "", nil)
	if err != nil {
		return nil, err
	}
	if tc.Roles == nil {
		tc.Roles = make(map[string]Role)
	}
	if role.Name == "" {
		role.Name = roleName
	}
	tc.Roles[roleName] = role
	tc.UpdatedAt = time.Now().UnixMilli()
	if err := s.store.writeJSON(teamName, "team_context.json", tc); err != nil {
		return nil, err
	}
	return tc, nil
}

func (s *TeamContextService) AssignAgentToRole(ctx context.Context, teamName, roleName, agentName string) (*TeamContext, error) {
	_ = ctx
	tc, err := s.CreateContext(context.Background(), teamName, "", nil)
	if err != nil {
		return nil, err
	}
	role := tc.Roles[roleName]
	role.Name = roleName
	role.Agent = agentName
	tc.Roles[roleName] = role
	tc.UpdatedAt = time.Now().UnixMilli()
	if err := s.store.writeJSON(teamName, "team_context.json", tc); err != nil {
		return nil, err
	}
	return tc, nil
}

func (s *TeamContextService) AddGoal(ctx context.Context, teamName string, goal Goal) (*TeamContext, error) {
	_ = ctx
	tc, err := s.CreateContext(context.Background(), teamName, "", nil)
	if err != nil {
		return nil, err
	}
	if goal.CreatedAt == 0 {
		goal.CreatedAt = time.Now().UnixMilli()
	}
	tc.Goals = append(tc.Goals, goal)
	tc.UpdatedAt = time.Now().UnixMilli()
	if err := s.store.writeJSON(teamName, "team_context.json", tc); err != nil {
		return nil, err
	}
	return tc, nil
}

func (s *TeamContextService) UpdateWorkingAgreement(ctx context.Context, teamName string, agreement WorkingAgreement) (*TeamContext, error) {
	_ = ctx
	tc, err := s.CreateContext(context.Background(), teamName, "", nil)
	if err != nil {
		return nil, err
	}
	tc.WorkingAgreement = mergeWorkingAgreement(tc.WorkingAgreement, agreement)
	tc.UpdatedAt = time.Now().UnixMilli()
	if err := s.store.writeJSON(teamName, "team_context.json", tc); err != nil {
		return nil, err
	}
	return tc, nil
}

func (s *TeamContextService) ListTeamNames(ctx context.Context) ([]string, error) {
	_ = ctx
	return s.store.listTeamNames()
}

func mergeWorkingAgreement(base, override WorkingAgreement) WorkingAgreement {
	if override.CommitMessageFormat != "" {
		base.CommitMessageFormat = override.CommitMessageFormat
	}
	if override.MaxWIP > 0 {
		base.MaxWIP = override.MaxWIP
	}
	if len(override.HandoffRequires) > 0 {
		base.HandoffRequires = override.HandoffRequires
	}
	base.ReviewRequired = override.ReviewRequired
	base.AllowsSubagents = override.AllowsSubagents
	if override.LeadershipMode != "" {
		base.LeadershipMode = override.LeadershipMode
	}
	if override.DelegationMode != "" {
		base.DelegationMode = override.DelegationMode
	}
	if override.LocalChatDefault != "" {
		base.LocalChatDefault = override.LocalChatDefault
	}
	if override.ReviewRouting != "" {
		base.ReviewRouting = override.ReviewRouting
	}
	if override.SynthesisRouting != "" {
		base.SynthesisRouting = override.SynthesisRouting
	}
	base.AllowsPeerMessaging = override.AllowsPeerMessaging
	base.AllowsBroadcasts = override.AllowsBroadcasts
	if len(override.ApprovalRequiredFor) > 0 {
		base.ApprovalRequiredFor = override.ApprovalRequiredFor
	}
	if override.WorkspaceMode != "" {
		base.WorkspaceMode = override.WorkspaceMode
	}
	if override.PublicationMode != "" {
		base.PublicationMode = override.PublicationMode
	}
	if override.ConsensusMode != "" {
		base.ConsensusMode = override.ConsensusMode
	}
	if override.WakePolicy != "" {
		base.WakePolicy = override.WakePolicy
	}
	return base
}

func (s *TeamContextService) UpdateGoalStatus(ctx context.Context, teamName, goalID, status string) (*TeamContext, error) {
	_ = ctx
	tc, err := s.CreateContext(context.Background(), teamName, "", nil)
	if err != nil {
		return nil, err
	}
	for i := range tc.Goals {
		if tc.Goals[i].ID == goalID {
			tc.Goals[i].Status = status
		}
	}
	tc.UpdatedAt = time.Now().UnixMilli()
	if err := s.store.writeJSON(teamName, "team_context.json", tc); err != nil {
		return nil, err
	}
	return tc, nil
}
