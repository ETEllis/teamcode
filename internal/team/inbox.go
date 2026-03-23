package team

import (
	"context"
	"time"
)

type InboxMessage struct {
	From_     string `json:"from"`
	Text      string `json:"text"`
	Timestamp string `json:"timestamp"`
	Read      bool   `json:"read"`
	Summary   string `json:"summary,omitempty"`
	Color     string `json:"color,omitempty"`
}

type InboxService struct {
	store *store
}

func NewInboxService(sharedStore *store) *InboxService {
	return &InboxService{store: sharedStore}
}

func (s *InboxService) ReadInbox(ctx context.Context, teamName, agentName string, unreadOnly bool) ([]InboxMessage, error) {
	_ = ctx
	messages, err := s.store.readInbox(teamName, agentName)
	if err != nil {
		return nil, err
	}
	if !unreadOnly {
		return messages, nil
	}
	filtered := make([]InboxMessage, 0, len(messages))
	for _, message := range messages {
		if !message.Read {
			filtered = append(filtered, message)
		}
	}
	return filtered, nil
}

func (s *InboxService) SendMessage(ctx context.Context, teamName, from, to, content, summary string) error {
	_ = ctx
	messages, err := s.store.readInbox(teamName, to)
	if err != nil {
		return err
	}
	messages = append(messages, InboxMessage{
		From_:     from,
		Text:      content,
		Timestamp: time.Now().Format(time.RFC3339),
		Read:      false,
		Summary:   summary,
	})
	return s.store.writeInbox(teamName, to, messages)
}

func (s *InboxService) Broadcast(ctx context.Context, teamName, from, content, summary string) error {
	_ = ctx
	recipients, err := s.recipients(teamName, from)
	if err != nil {
		return err
	}
	for _, recipient := range recipients {
		if err := s.SendMessage(context.Background(), teamName, from, recipient, content, summary); err != nil {
			return err
		}
	}
	return nil
}

func (s *InboxService) recipients(teamName, sender string) ([]string, error) {
	contextSvc := NewTeamContextService(s.store)
	memberSvc := NewMemberService(s.store)

	seen := map[string]struct{}{}
	recipients := make([]string, 0)

	tc, err := contextSvc.ReadContext(context.Background(), teamName)
	if err != nil {
		return nil, err
	}
	if tc != nil {
		for _, role := range tc.Roles {
			if role.Agent == "" || role.Agent == sender {
				continue
			}
			if _, ok := seen[role.Agent]; ok {
				continue
			}
			seen[role.Agent] = struct{}{}
			recipients = append(recipients, role.Agent)
		}
	}

	members, err := memberSvc.List(context.Background(), teamName)
	if err != nil {
		return nil, err
	}
	for _, member := range members {
		if member.AgentName == "" || member.AgentName == sender {
			continue
		}
		if _, ok := seen[member.AgentName]; ok {
			continue
		}
		seen[member.AgentName] = struct{}{}
		recipients = append(recipients, member.AgentName)
	}

	return recipients, nil
}
