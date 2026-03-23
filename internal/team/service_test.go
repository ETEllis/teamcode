package team

import (
	"context"
	"testing"

	"github.com/stretchr/testify/require"
)

func TestTeamServiceSupportsBoardMessagingAndHandoffs(t *testing.T) {
	svc := NewServiceWithBaseDir(t.TempDir())
	ctx := context.Background()

	_, err := svc.Context.CreateContext(ctx, "alpha", "Ship the feature", map[string]Role{
		"lead": {Name: "lead", Responsible: "Coordinate"},
	})
	require.NoError(t, err)

	_, err = svc.Context.AssignAgentToRole(ctx, "alpha", "lead", "alex")
	require.NoError(t, err)

	_, err = svc.Members.Upsert(ctx, "alpha", Member{AgentName: "alex", RoleName: "lead", Status: "running"})
	require.NoError(t, err)
	_, err = svc.Members.Upsert(ctx, "alpha", Member{AgentName: "bea", RoleName: "builder", Status: "idle"})
	require.NoError(t, err)

	board, err := svc.Board.AddTaskToColumn(ctx, "alpha", "task-1", "ready")
	require.NoError(t, err)
	require.Contains(t, board.Columns["ready"], "task-1")

	board, err = svc.Board.MoveTask(ctx, "alpha", "task-1", "ready", "in_progress", "bea")
	require.NoError(t, err)
	require.Contains(t, board.Columns["in_progress"], "task-1")
	require.Equal(t, "bea", board.Assignments["task-1"])

	err = svc.Inbox.Broadcast(ctx, "alpha", "alex", "Heads up", "status")
	require.NoError(t, err)

	inbox, err := svc.Inbox.ReadInbox(ctx, "alpha", "bea", false)
	require.NoError(t, err)
	require.Len(t, inbox, 1)
	require.Equal(t, "alex", inbox[0].From_)

	handoff, err := svc.Handoff.Create(ctx, "alpha", "task-1", "bea", "alex", "Ready for review", []string{"main.go"}, nil)
	require.NoError(t, err)
	require.Equal(t, "pending", handoff.Status)

	accepted, err := svc.Handoff.Accept(ctx, "alpha", handoff.ID, "alex")
	require.NoError(t, err)
	require.Equal(t, "accepted", accepted.Status)
}
