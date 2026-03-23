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

	done, err := manager.Wait(context.Background(), worker.ID)
	require.NoError(t, err)
	require.Equal(t, WorkerStatusCompleted, done.Status)
	require.Equal(t, done.SessionID+":check files", done.Result)
}
