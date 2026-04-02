package team

import (
	"context"
	"sort"

	"github.com/ETEllis/teamcode/internal/orchestration"
)

type Snapshot struct {
	TeamName         string                 `json:"teamName"`
	Context          *TeamContext           `json:"context,omitempty"`
	Board            *TaskBoard             `json:"board,omitempty"`
	BoardSummary     map[string]int         `json:"boardSummary,omitempty"`
	Members          []Member               `json:"members,omitempty"`
	Agency           *AgencySnapshot        `json:"agency,omitempty"`
	Office           *OfficeState           `json:"office,omitempty"`
	RuntimeRecords   []RuntimeRecord        `json:"runtimeRecords,omitempty"`
	RuntimeSummary   RuntimeSummary         `json:"runtimeSummary,omitempty"`
	PendingHandoffs  int                    `json:"pendingHandoffs,omitempty"`
	UnreadDirect     int                    `json:"unreadDirect,omitempty"`
	UnreadBroadcasts int                    `json:"unreadBroadcasts,omitempty"`
	Workers          []orchestration.Worker `json:"workers,omitempty"`
}

type AgencySnapshot struct {
	Constitution          string `json:"constitution,omitempty"`
	RuntimeMode           string `json:"runtimeMode,omitempty"`
	SharedTruth           string `json:"sharedTruth,omitempty"`
	WorkspaceMode         string `json:"workspaceMode,omitempty"`
	PublicationMode       string `json:"publicationMode,omitempty"`
	ConsensusMode         string `json:"consensusMode,omitempty"`
	ActiveRuntimeRecords  int    `json:"activeRuntimeRecords,omitempty"`
	LocalRuntimeRecords   int    `json:"localRuntimeRecords,omitempty"`
	CommittedRuntimeCount int    `json:"committedRuntimeRecords,omitempty"`
}

func (s *Service) ListTeamNames(ctx context.Context) ([]string, error) {
	return s.Context.ListTeamNames(ctx)
}

func (s *Service) MostRecentTeamName(ctx context.Context) (string, error) {
	teamNames, err := s.ListTeamNames(ctx)
	if err != nil {
		return "", err
	}
	var (
		selected string
		updated  int64
	)
	for _, teamName := range teamNames {
		tc, err := s.Context.ReadContext(ctx, teamName)
		if err != nil || tc == nil {
			continue
		}
		if tc.UpdatedAt >= updated {
			selected = teamName
			updated = tc.UpdatedAt
		}
	}
	return selected, nil
}

func (s *Service) Snapshot(ctx context.Context, teamName string, workers []orchestration.Worker) (*Snapshot, error) {
	tc, err := s.Context.ReadContext(ctx, teamName)
	if err != nil {
		return nil, err
	}
	if tc == nil {
		return nil, nil
	}
	board, err := s.Board.CreateBoard(ctx, teamName)
	if err != nil {
		return nil, err
	}
	members, err := s.Members.List(ctx, teamName)
	if err != nil {
		return nil, err
	}
	handoffs, err := s.Handoff.List(ctx, teamName, "pending", "")
	if err != nil {
		return nil, err
	}
	sort.Slice(members, func(i, j int) bool {
		if members[i].Leader != members[j].Leader {
			return members[i].Leader
		}
		return members[i].AgentName < members[j].AgentName
	})
	runtimeRecords, runtimeSummary, err := s.Runtime.Materialize(ctx, teamName, members, workers)
	if err != nil {
		return nil, err
	}
	officeState, err := s.Office.Materialize(ctx, teamName, tc, members, runtimeRecords, runtimeSummary)
	if err != nil {
		return nil, err
	}

	unreadDirect := 0
	unreadBroadcasts := 0
	for _, member := range members {
		msgs, err := s.Inbox.ReadInbox(ctx, teamName, member.AgentName, true)
		if err != nil {
			return nil, err
		}
		for _, msg := range msgs {
			if msg.Broadcast {
				unreadBroadcasts++
				continue
			}
			unreadDirect++
		}
	}

	return &Snapshot{
		TeamName:         teamName,
		Context:          tc,
		Board:            board,
		BoardSummary:     summarizeBoard(board),
		Members:          members,
		Agency:           buildAgencySnapshot(tc, runtimeSummary),
		Office:           officeState,
		RuntimeRecords:   runtimeRecords,
		RuntimeSummary:   runtimeSummary,
		PendingHandoffs:  len(handoffs),
		UnreadDirect:     unreadDirect,
		UnreadBroadcasts: unreadBroadcasts,
		Workers:          workers,
	}, nil
}

func buildAgencySnapshot(tc *TeamContext, summary RuntimeSummary) *AgencySnapshot {
	if tc == nil {
		return nil
	}
	return &AgencySnapshot{
		Constitution:          tc.Constitution,
		RuntimeMode:           tc.RuntimeMode,
		SharedTruth:           tc.SharedTruth,
		WorkspaceMode:         tc.WorkingAgreement.WorkspaceMode,
		PublicationMode:       tc.WorkingAgreement.PublicationMode,
		ConsensusMode:         tc.WorkingAgreement.ConsensusMode,
		ActiveRuntimeRecords:  summary.Active,
		LocalRuntimeRecords:   summary.Local,
		CommittedRuntimeCount: summary.Committed,
	}
}

func summarizeBoard(board *TaskBoard) map[string]int {
	summary := map[string]int{}
	if board == nil {
		return summary
	}
	for column, tasks := range board.Columns {
		summary[column] = len(tasks)
	}
	return summary
}
