package team

import (
	"context"
	"time"
)

type TeamContext struct {
	TeamName         string           `json:"teamName"`
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
	Name         string `json:"name"`
	Responsible  string `json:"responsible"`
	CurrentFocus string `json:"currentFocus,omitempty"`
	Agent        string `json:"agent,omitempty"`
}

type WorkingAgreement struct {
	CommitMessageFormat string   `json:"commitMessageFormat"`
	MaxWIP              int      `json:"maxWip"`
	HandoffRequires     []string `json:"handoffRequires"`
	ReviewRequired      bool     `json:"reviewRequired"`
	ApprovalRequiredFor []string `json:"approvalRequiredFor,omitempty"`
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
			TeamName: teamName,
			Roles:    make(map[string]Role),
			Goals:    make([]Goal, 0),
			WorkingAgreement: WorkingAgreement{
				CommitMessageFormat: "type: subject",
				MaxWIP:              3,
				HandoffRequires:     []string{"summary", "artifacts"},
				ReviewRequired:      true,
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
