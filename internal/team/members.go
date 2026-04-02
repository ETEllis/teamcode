package team

import (
	"context"
	"time"
)

type Member struct {
	AgentName         string   `json:"agentName"`
	RoleName          string   `json:"roleName,omitempty"`
	SessionID         string   `json:"sessionId,omitempty"`
	WorkerID          string   `json:"workerId,omitempty"`
	Kind              string   `json:"kind,omitempty"`
	Profile           string   `json:"profile,omitempty"`
	Status            string   `json:"status,omitempty"`
	LastResult        string   `json:"lastResult,omitempty"`
	ReportsTo         string   `json:"reportsTo,omitempty"`
	ParentWorkerID    string   `json:"parentWorkerId,omitempty"`
	RootWorkerID      string   `json:"rootWorkerId,omitempty"`
	Lineage           []string `json:"lineage,omitempty"`
	LineageDepth      int      `json:"lineageDepth,omitempty"`
	StateScope        string   `json:"stateScope,omitempty"`
	CommitmentState   string   `json:"commitmentState,omitempty"`
	WorkspaceMode     string   `json:"workspaceMode,omitempty"`
	WakeState         string   `json:"wakeState,omitempty"`
	LastSignalAt      int64    `json:"lastSignalAt,omitempty"`
	LastHeartbeatAt   int64    `json:"lastHeartbeatAt,omitempty"`
	Leader            bool     `json:"leader,omitempty"`
	CanSpawnSubagents bool     `json:"canSpawnSubagents,omitempty"`
	UpdatedAt         int64    `json:"updatedAt"`
	CreatedAt         int64    `json:"createdAt"`
}

type MemberService struct {
	store *store
}

func NewMemberService(sharedStore *store) *MemberService {
	return &MemberService{store: sharedStore}
}

func (s *MemberService) List(ctx context.Context, teamName string) ([]Member, error) {
	_ = ctx
	var members []Member
	if err := s.store.readJSON(teamName, "members.json", &members); err != nil {
		return nil, err
	}
	return members, nil
}

func (s *MemberService) Upsert(ctx context.Context, teamName string, member Member) (*Member, error) {
	_ = ctx
	members, err := s.List(context.Background(), teamName)
	if err != nil {
		return nil, err
	}

	now := time.Now().UnixMilli()
	member.UpdatedAt = now
	if member.CreatedAt == 0 {
		member.CreatedAt = now
	}

	updated := false
	for i := range members {
		if members[i].AgentName == member.AgentName {
			if member.RoleName == "" {
				member.RoleName = members[i].RoleName
			}
			if member.SessionID == "" {
				member.SessionID = members[i].SessionID
			}
			if member.Kind == "" {
				member.Kind = members[i].Kind
			}
			if member.WorkerID == "" {
				member.WorkerID = members[i].WorkerID
			}
			if member.Profile == "" {
				member.Profile = members[i].Profile
			}
			if member.Status == "" {
				member.Status = members[i].Status
			}
			if member.LastResult == "" {
				member.LastResult = members[i].LastResult
			}
			if member.ReportsTo == "" {
				member.ReportsTo = members[i].ReportsTo
			}
			if member.ParentWorkerID == "" {
				member.ParentWorkerID = members[i].ParentWorkerID
			}
			if member.RootWorkerID == "" {
				member.RootWorkerID = members[i].RootWorkerID
			}
			if len(member.Lineage) == 0 {
				member.Lineage = members[i].Lineage
			}
			if member.LineageDepth == 0 {
				member.LineageDepth = members[i].LineageDepth
			}
			if member.StateScope == "" {
				member.StateScope = members[i].StateScope
			}
			if member.CommitmentState == "" {
				member.CommitmentState = members[i].CommitmentState
			}
			if member.WorkspaceMode == "" {
				member.WorkspaceMode = members[i].WorkspaceMode
			}
			if member.WakeState == "" {
				member.WakeState = members[i].WakeState
			}
			if member.LastSignalAt == 0 {
				member.LastSignalAt = members[i].LastSignalAt
			}
			if member.LastHeartbeatAt == 0 {
				member.LastHeartbeatAt = members[i].LastHeartbeatAt
			}
			if !member.Leader {
				member.Leader = members[i].Leader
			}
			if !member.CanSpawnSubagents {
				member.CanSpawnSubagents = members[i].CanSpawnSubagents
			}
			member.CreatedAt = members[i].CreatedAt
			members[i] = member
			updated = true
			break
		}
	}
	if !updated {
		members = append(members, member)
	}

	if err := s.store.writeJSON(teamName, "members.json", members); err != nil {
		return nil, err
	}
	return &member, nil
}

func (s *MemberService) FindBySessionID(ctx context.Context, sessionID string) (string, *Member, error) {
	_ = ctx
	teamNames, err := s.store.listTeamNames()
	if err != nil {
		return "", nil, err
	}
	for _, teamName := range teamNames {
		members, err := s.List(context.Background(), teamName)
		if err != nil {
			return "", nil, err
		}
		for _, member := range members {
			if member.SessionID == sessionID {
				copy := member
				return teamName, &copy, nil
			}
		}
	}
	return "", nil, nil
}
