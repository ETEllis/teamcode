package team

import (
	"context"
	"time"

	"github.com/google/uuid"
)

type Handoff struct {
	ID                 string   `json:"id"`
	TaskID             string   `json:"taskId"`
	FromAgent          string   `json:"fromAgent"`
	ToAgent            string   `json:"toAgent"`
	Status             string   `json:"status"`
	WorkSummary        string   `json:"workSummary"`
	Artifacts          []string `json:"artifacts"`
	AcceptanceCriteria []string `json:"acceptanceCriteria"`
	CreatedAt          int64    `json:"createdAt"`
	AcceptedAt         *int64   `json:"acceptedAt,omitempty"`
	AcceptedBy         string   `json:"acceptedBy,omitempty"`
	RejectionReason    string   `json:"rejectionReason,omitempty"`
}

type HandoffService struct {
	store *store
}

func NewHandoffService(sharedStore *store) *HandoffService {
	return &HandoffService{store: sharedStore}
}

func (s *HandoffService) Create(ctx context.Context, teamName, taskID, fromAgent, toAgent, workSummary string, artifacts, criteria []string) (*Handoff, error) {
	_ = ctx
	handoffs, err := s.load(teamName)
	if err != nil {
		return nil, err
	}
	now := time.Now().UnixMilli()
	h := Handoff{
		ID:                 uuid.NewString(),
		TaskID:             taskID,
		FromAgent:          fromAgent,
		ToAgent:            toAgent,
		Status:             "pending",
		WorkSummary:        workSummary,
		Artifacts:          artifacts,
		AcceptanceCriteria: criteria,
		CreatedAt:          now,
	}
	handoffs = append(handoffs, h)
	if err := s.store.writeJSON(teamName, "handoffs.json", handoffs); err != nil {
		return nil, err
	}
	return &h, nil
}

func (s *HandoffService) Read(ctx context.Context, teamName, handoffID string) (*Handoff, error) {
	_ = ctx
	handoffs, err := s.load(teamName)
	if err != nil {
		return nil, err
	}
	for _, handoff := range handoffs {
		if handoff.ID == handoffID {
			copy := handoff
			return &copy, nil
		}
	}
	return nil, nil
}

func (s *HandoffService) Accept(ctx context.Context, teamName, handoffID, acceptedBy string) (*Handoff, error) {
	_ = ctx
	handoffs, err := s.load(teamName)
	if err != nil {
		return nil, err
	}
	now := time.Now().UnixMilli()
	for i := range handoffs {
		if handoffs[i].ID == handoffID {
			handoffs[i].Status = "accepted"
			handoffs[i].AcceptedBy = acceptedBy
			handoffs[i].AcceptedAt = &now
			if err := s.store.writeJSON(teamName, "handoffs.json", handoffs); err != nil {
				return nil, err
			}
			copy := handoffs[i]
			return &copy, nil
		}
	}
	return nil, nil
}

func (s *HandoffService) Reject(ctx context.Context, teamName, handoffID, rejectedBy, reason string) (*Handoff, error) {
	_ = ctx
	handoffs, err := s.load(teamName)
	if err != nil {
		return nil, err
	}
	for i := range handoffs {
		if handoffs[i].ID == handoffID {
			handoffs[i].Status = "rejected"
			handoffs[i].AcceptedBy = rejectedBy
			handoffs[i].RejectionReason = reason
			if err := s.store.writeJSON(teamName, "handoffs.json", handoffs); err != nil {
				return nil, err
			}
			copy := handoffs[i]
			return &copy, nil
		}
	}
	return nil, nil
}

func (s *HandoffService) List(ctx context.Context, teamName string, status, toAgent string) ([]Handoff, error) {
	_ = ctx
	handoffs, err := s.load(teamName)
	if err != nil {
		return nil, err
	}
	filtered := make([]Handoff, 0, len(handoffs))
	for _, handoff := range handoffs {
		if status != "" && handoff.Status != status {
			continue
		}
		if toAgent != "" && handoff.ToAgent != toAgent {
			continue
		}
		filtered = append(filtered, handoff)
	}
	return filtered, nil
}

func (s *HandoffService) GetPendingForAgent(ctx context.Context, teamName, agent string) ([]Handoff, error) {
	return s.List(ctx, teamName, "pending", agent)
}

func (s *HandoffService) load(teamName string) ([]Handoff, error) {
	var handoffs []Handoff
	if err := s.store.readJSON(teamName, "handoffs.json", &handoffs); err != nil {
		return nil, err
	}
	return handoffs, nil
}
