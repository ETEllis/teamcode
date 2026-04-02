package team

import (
	"context"
	"testing"

	"github.com/ETEllis/teamcode/internal/orchestration"
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
	require.True(t, inbox[0].Broadcast)

	handoff, err := svc.Handoff.Create(ctx, "alpha", "task-1", "bea", "alex", "Ready for review", []string{"main.go"}, nil)
	require.NoError(t, err)
	require.Equal(t, "pending", handoff.Status)

	accepted, err := svc.Handoff.Accept(ctx, "alpha", handoff.ID, "alex")
	require.NoError(t, err)
	require.Equal(t, "accepted", accepted.Status)

	snapshot, err := svc.Snapshot(ctx, "alpha", nil)
	require.NoError(t, err)
	require.NotNil(t, snapshot)
	require.Equal(t, "alpha", snapshot.TeamName)
	require.Equal(t, 1, snapshot.UnreadBroadcasts)
	require.Equal(t, 0, snapshot.UnreadDirect)
	require.NotNil(t, snapshot.Agency)
	require.Equal(t, "coding-office", snapshot.Agency.Constitution)
	require.NotNil(t, snapshot.Office)
	require.Equal(t, "active", snapshot.Office.ContinuityStatus)
}

func TestFindBySessionIDReturnsTeamMember(t *testing.T) {
	svc := NewServiceWithBaseDir(t.TempDir())
	ctx := context.Background()

	_, err := svc.Context.CreateContext(ctx, "alpha", "Ship the feature", nil)
	require.NoError(t, err)
	_, err = svc.Members.Upsert(ctx, "alpha", Member{
		AgentName:         "lead",
		SessionID:         "session-1",
		RoleName:          "lead",
		CanSpawnSubagents: true,
	})
	require.NoError(t, err)

	teamName, member, err := svc.Members.FindBySessionID(ctx, "session-1")
	require.NoError(t, err)
	require.Equal(t, "alpha", teamName)
	require.NotNil(t, member)
	require.Equal(t, "lead", member.AgentName)
}

func TestRuntimeProjectionBridgesMembersAndWorkersIntoAgencySnapshot(t *testing.T) {
	svc := NewServiceWithBaseDir(t.TempDir())
	ctx := context.Background()

	_, err := svc.Context.CreateContext(ctx, "agency", "Run the office", nil)
	require.NoError(t, err)
	_, err = svc.Context.UpdateWorkingAgreement(ctx, "agency", WorkingAgreement{
		WorkspaceMode:   "sandboxed",
		PublicationMode: "ledgered",
		ConsensusMode:   "quorum",
		WakePolicy:      "scheduled",
	})
	require.NoError(t, err)

	_, err = svc.Members.Upsert(ctx, "agency", Member{
		AgentName:       "alex",
		RoleName:        "lead",
		SessionID:       "session-1",
		WorkerID:        "worker-1",
		StateScope:      "local",
		CommitmentState: "pending",
		WorkspaceMode:   "shared",
		WakeState:       "active",
	})
	require.NoError(t, err)
	_, err = svc.Runtime.Upsert(ctx, "agency", RuntimeRecord{
		AgentName:       "archivist",
		SessionID:       "session-archive",
		Status:          "idle",
		StateScope:      "committed",
		CommitmentState: "committed",
		WorkspaceMode:   "shared",
	})
	require.NoError(t, err)

	workers := []orchestration.Worker{{
		ID:              "worker-1",
		SessionID:       "session-1",
		ParentSessionID: "parent-session",
		Name:            "alex",
		TeamName:        "agency",
		RoleName:        "lead",
		Kind:            orchestration.WorkerKindTeammate,
		Status:          orchestration.WorkerStatusRunning,
		StateScope:      orchestration.WorkerStateScopeLocal,
		CommitmentState: orchestration.WorkerCommitmentStatePending,
		WorkspaceMode:   "sandbox",
		WakeState:       orchestration.WorkerWakeStateActive,
		RootWorkerID:    "worker-1",
		Lineage:         []string{"worker-1"},
		CreatedAt:       10,
		UpdatedAt:       20,
	}}

	snapshot, err := svc.Snapshot(ctx, "agency", workers)
	require.NoError(t, err)
	require.NotNil(t, snapshot)
	require.Len(t, snapshot.RuntimeRecords, 2)
	require.Equal(t, 2, snapshot.RuntimeSummary.Total)
	require.Equal(t, 1, snapshot.RuntimeSummary.Active)
	require.Equal(t, 1, snapshot.RuntimeSummary.Committed)
	require.Equal(t, 1, snapshot.RuntimeSummary.Pending)
	require.NotNil(t, snapshot.Agency)
	require.Equal(t, "sandboxed", snapshot.Agency.WorkspaceMode)
	require.Equal(t, "ledgered", snapshot.Agency.PublicationMode)
	require.Equal(t, "quorum", snapshot.Agency.ConsensusMode)
	require.NotNil(t, snapshot.Office)
	require.Equal(t, "active", snapshot.Office.ContinuityStatus)
	require.Len(t, snapshot.Office.Roster, 2)

	var workerRecord RuntimeRecord
	for _, record := range snapshot.RuntimeRecords {
		if record.AgentName == "alex" {
			workerRecord = record
			break
		}
	}
	require.Equal(t, "worker-1", workerRecord.WorkerID)
	require.Equal(t, "pending", workerRecord.CommitmentState)
	require.Equal(t, "sandbox", workerRecord.WorkspaceMode)

	persistedOffice, err := svc.Office.Read(ctx, "agency")
	require.NoError(t, err)
	require.NotNil(t, persistedOffice)
	require.Equal(t, "active", persistedOffice.ContinuityStatus)

	persistedRuntime, err := svc.Runtime.List(ctx, "agency")
	require.NoError(t, err)
	require.Len(t, persistedRuntime, 2)
}
