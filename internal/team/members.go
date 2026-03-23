package team

import (
	"context"
	"time"
)

type Member struct {
	AgentName  string `json:"agentName"`
	RoleName   string `json:"roleName,omitempty"`
	SessionID  string `json:"sessionId,omitempty"`
	Kind       string `json:"kind,omitempty"`
	Profile    string `json:"profile,omitempty"`
	Status     string `json:"status,omitempty"`
	LastResult string `json:"lastResult,omitempty"`
	UpdatedAt  int64  `json:"updatedAt"`
	CreatedAt  int64  `json:"createdAt"`
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
			if member.Profile == "" {
				member.Profile = members[i].Profile
			}
			if member.Status == "" {
				member.Status = members[i].Status
			}
			if member.LastResult == "" {
				member.LastResult = members[i].LastResult
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
