package orchestration

import (
	"context"
	"testing"

	"github.com/ETEllis/teamcode/internal/pubsub"
	"github.com/ETEllis/teamcode/internal/session"
	"github.com/stretchr/testify/require"
)

type fakeSessionService struct{}

func (fakeSessionService) Subscribe(context.Context) <-chan pubsub.Event[session.Session] {
	ch := make(chan pubsub.Event[session.Session])
	close(ch)
	return ch
}

func (fakeSessionService) Create(context.Context, string) (session.Session, error) {
	return session.Session{}, nil
}

func (fakeSessionService) CreateTitleSession(context.Context, string) (session.Session, error) {
	return session.Session{}, nil
}

func (fakeSessionService) CreateTaskSession(_ context.Context, toolCallID, parentSessionID, title string) (session.Session, error) {
	return session.Session{ID: toolCallID, ParentSessionID: parentSessionID, Title: title}, nil
}

func (fakeSessionService) Get(context.Context, string) (session.Session, error) {
	return session.Session{}, nil
}

func (fakeSessionService) List(context.Context) ([]session.Session, error) {
	return nil, nil
}

func (fakeSessionService) Save(_ context.Context, sess session.Session) (session.Session, error) {
	return sess, nil
}

func (fakeSessionService) Delete(context.Context, string) error {
	return nil
}

func TestManagerSpawnAndWait(t *testing.T) {
	manager := NewManager(fakeSessionService{})
	manager.SetRunner(func(ctx context.Context, sessionID string, request RunRequest) (<-chan RunResult, error) {
		ch := make(chan RunResult, 1)
		ch <- RunResult{Content: sessionID + ":" + request.Prompt}
		close(ch)
		return ch, nil
	})

	worker, err := manager.Spawn(context.Background(), SpawnParams{
		ParentSessionID: "parent",
		Name:            "builder",
		Kind:            WorkerKindSubagent,
		Prompt:          "check files",
	})
	require.NoError(t, err)
	require.Equal(t, WorkerStatusQueued, worker.Status)
	require.Equal(t, WorkerStateScopeLocal, worker.StateScope)
	require.Equal(t, WorkerCommitmentStateLocal, worker.CommitmentState)
	require.Equal(t, WorkerWakeStateWaiting, worker.WakeState)
	require.Len(t, worker.Lineage, 1)
	require.Equal(t, worker.ID, worker.RootWorkerID)

	done, err := manager.Wait(context.Background(), worker.ID)
	require.NoError(t, err)
	require.Equal(t, WorkerStatusCompleted, done.Status)
	require.Equal(t, done.SessionID+":check files", done.Result)
	require.Equal(t, WorkerWakeStateQuiescent, done.WakeState)
	require.NotZero(t, done.StartedAt)
	require.NotZero(t, done.LastHeartbeatAt)
}

func TestManagerTracksNestedWorkerLineageAndCommitmentUpdates(t *testing.T) {
	manager := NewManager(fakeSessionService{})
	manager.SetRunner(func(ctx context.Context, sessionID string, request RunRequest) (<-chan RunResult, error) {
		ch := make(chan RunResult, 1)
		ch <- RunResult{Content: "ok"}
		close(ch)
		return ch, nil
	})

	parent, err := manager.Spawn(context.Background(), SpawnParams{
		ParentSessionID: "root-session",
		Name:            "lead",
		Kind:            WorkerKindTeammate,
		Prompt:          "lead",
	})
	require.NoError(t, err)

	child, err := manager.Spawn(context.Background(), SpawnParams{
		ParentSessionID: "child-session",
		ParentWorkerID:  parent.ID,
		Name:            "builder",
		Kind:            WorkerKindSubagent,
		Prompt:          "build",
		WorkspaceMode:   "sandbox",
	})
	require.NoError(t, err)
	require.Equal(t, parent.ID, child.ParentWorkerID)
	require.Equal(t, parent.ID, child.RootWorkerID)
	require.Len(t, child.Lineage, 2)
	require.Equal(t, parent.ID, child.Lineage[0])
	require.Equal(t, child.ID, child.Lineage[1])
	require.Equal(t, 1, child.LineageDepth)
	require.Equal(t, "sandbox", child.WorkspaceMode)

	manager.SetCommitment(child.ID, WorkerStateScopeCommitted, WorkerCommitmentStateCommitted)
	manager.RecordSignal(child.ID, WorkerWakeStateActive)
	manager.MarkHeartbeat(child.ID)

	updated, ok := manager.Get(child.ID)
	require.True(t, ok)
	require.Equal(t, WorkerStateScopeCommitted, updated.StateScope)
	require.Equal(t, WorkerCommitmentStateCommitted, updated.CommitmentState)
	require.Equal(t, WorkerWakeStateActive, updated.WakeState)
	require.NotZero(t, updated.LastSignalAt)
	require.NotZero(t, updated.LastHeartbeatAt)

	byRoot := manager.ListByRoot(parent.ID)
	require.Len(t, byRoot, 2)

	all := manager.List()
	require.Len(t, all, 2)
}
