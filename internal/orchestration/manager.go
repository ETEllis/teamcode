package orchestration

import (
	"context"
	"fmt"
	"sync"
	"time"

	"github.com/google/uuid"

	"github.com/ETEllis/teamcode/internal/session"
)

type WorkerKind string

const (
	WorkerKindSubagent WorkerKind = "subagent"
	WorkerKindTeammate WorkerKind = "teammate"
)

type WorkerStatus string

const (
	WorkerStatusQueued    WorkerStatus = "queued"
	WorkerStatusRunning   WorkerStatus = "running"
	WorkerStatusCompleted WorkerStatus = "completed"
	WorkerStatusFailed    WorkerStatus = "failed"
	WorkerStatusCanceled  WorkerStatus = "canceled"
)

type Worker struct {
	ID              string       `json:"id"`
	SessionID       string       `json:"sessionId"`
	ParentSessionID string       `json:"parentSessionId"`
	Name            string       `json:"name,omitempty"`
	TeamName        string       `json:"teamName,omitempty"`
	RoleName        string       `json:"roleName,omitempty"`
	Kind            WorkerKind   `json:"kind"`
	Profile         string       `json:"profile,omitempty"`
	Status          WorkerStatus `json:"status"`
	Prompt          string       `json:"prompt"`
	Result          string       `json:"result,omitempty"`
	Error           string       `json:"error,omitempty"`
	CreatedAt       int64        `json:"createdAt"`
	UpdatedAt       int64        `json:"updatedAt"`
	CompletedAt     int64        `json:"completedAt,omitempty"`
}

type RunRequest struct {
	Prompt  string
	Profile string
}

type RunResult struct {
	Content string
	Error   error
}

type Runner func(ctx context.Context, sessionID string, request RunRequest) (<-chan RunResult, error)

type SpawnParams struct {
	ParentSessionID string
	Name            string
	TeamName        string
	RoleName        string
	Kind            WorkerKind
	Profile         string
	Prompt          string
	Title           string
}

type workerState struct {
	worker Worker
	done   chan struct{}
	cancel context.CancelFunc
}

type Manager struct {
	mu       sync.RWMutex
	sessions session.Service
	runner   Runner
	workers  map[string]*workerState
}

func NewManager(sessions session.Service) *Manager {
	return &Manager{
		sessions: sessions,
		workers:  make(map[string]*workerState),
	}
}

func (m *Manager) SetRunner(runner Runner) {
	m.mu.Lock()
	defer m.mu.Unlock()
	m.runner = runner
}

func (m *Manager) Spawn(ctx context.Context, params SpawnParams) (Worker, error) {
	m.mu.RLock()
	runner := m.runner
	m.mu.RUnlock()
	if runner == nil {
		return Worker{}, fmt.Errorf("worker runner is not configured")
	}

	if params.ParentSessionID == "" {
		return Worker{}, fmt.Errorf("parent session id is required")
	}
	if params.Prompt == "" {
		return Worker{}, fmt.Errorf("prompt is required")
	}
	if params.Kind == "" {
		params.Kind = WorkerKindSubagent
	}
	if params.Profile == "" {
		params.Profile = "coder"
	}

	workerID := uuid.NewString()
	title := params.Title
	if title == "" {
		switch params.Kind {
		case WorkerKindTeammate:
			title = "Teammate: " + params.Name
		default:
			title = "Subagent: " + params.Name
			if params.Name == "" {
				title = "Subagent Session"
			}
		}
	}

	sess, err := m.sessions.CreateTaskSession(ctx, workerID, params.ParentSessionID, title)
	if err != nil {
		return Worker{}, err
	}

	now := time.Now().UnixMilli()
	state := &workerState{
		worker: Worker{
			ID:              workerID,
			SessionID:       sess.ID,
			ParentSessionID: params.ParentSessionID,
			Name:            params.Name,
			TeamName:        params.TeamName,
			RoleName:        params.RoleName,
			Kind:            params.Kind,
			Profile:         params.Profile,
			Status:          WorkerStatusQueued,
			Prompt:          params.Prompt,
			CreatedAt:       now,
			UpdatedAt:       now,
		},
		done: make(chan struct{}),
	}

	runCtx, cancel := context.WithCancel(context.Background())
	state.cancel = cancel

	m.mu.Lock()
	m.workers[state.worker.ID] = state
	m.mu.Unlock()

	go m.runWorker(runCtx, runner, state)

	return state.worker, nil
}

func (m *Manager) runWorker(ctx context.Context, runner Runner, state *workerState) {
	m.update(state.worker.ID, func(worker *Worker) {
		worker.Status = WorkerStatusRunning
		worker.UpdatedAt = time.Now().UnixMilli()
	})

	resultCh, err := runner(ctx, state.worker.SessionID, RunRequest{
		Prompt:  state.worker.Prompt,
		Profile: state.worker.Profile,
	})
	if err != nil {
		m.finish(state.worker.ID, WorkerStatusFailed, "", err)
		return
	}

	result, ok := <-resultCh
	if !ok {
		m.finish(state.worker.ID, WorkerStatusFailed, "", fmt.Errorf("worker did not return a result"))
		return
	}
	if result.Error != nil {
		status := WorkerStatusFailed
		if ctx.Err() != nil {
			status = WorkerStatusCanceled
		}
		m.finish(state.worker.ID, status, result.Content, result.Error)
		return
	}

	m.finish(state.worker.ID, WorkerStatusCompleted, result.Content, nil)
}

func (m *Manager) finish(workerID string, status WorkerStatus, content string, err error) {
	m.update(workerID, func(worker *Worker) {
		worker.Status = status
		worker.Result = content
		if err != nil {
			worker.Error = err.Error()
		} else {
			worker.Error = ""
		}
		worker.CompletedAt = time.Now().UnixMilli()
		worker.UpdatedAt = worker.CompletedAt
	})

	m.mu.RLock()
	state := m.workers[workerID]
	m.mu.RUnlock()
	if state != nil {
		close(state.done)
	}
}

func (m *Manager) update(workerID string, fn func(*Worker)) {
	m.mu.Lock()
	defer m.mu.Unlock()
	state := m.workers[workerID]
	if state == nil {
		return
	}
	fn(&state.worker)
}

func (m *Manager) Get(workerID string) (Worker, bool) {
	m.mu.RLock()
	defer m.mu.RUnlock()
	state, ok := m.workers[workerID]
	if !ok {
		return Worker{}, false
	}
	return state.worker, true
}

func (m *Manager) Wait(ctx context.Context, workerID string) (Worker, error) {
	m.mu.RLock()
	state := m.workers[workerID]
	m.mu.RUnlock()
	if state == nil {
		return Worker{}, fmt.Errorf("worker not found: %s", workerID)
	}

	select {
	case <-ctx.Done():
		return Worker{}, ctx.Err()
	case <-state.done:
	}

	worker, ok := m.Get(workerID)
	if !ok {
		return Worker{}, fmt.Errorf("worker not found: %s", workerID)
	}
	return worker, nil
}

func (m *Manager) Cancel(workerID string) error {
	m.mu.RLock()
	state := m.workers[workerID]
	m.mu.RUnlock()
	if state == nil {
		return fmt.Errorf("worker not found: %s", workerID)
	}
	if state.cancel != nil {
		state.cancel()
	}
	return nil
}

func (m *Manager) ListByParent(parentSessionID string) []Worker {
	m.mu.RLock()
	defer m.mu.RUnlock()

	workers := make([]Worker, 0)
	for _, state := range m.workers {
		if state.worker.ParentSessionID == parentSessionID {
			workers = append(workers, state.worker)
		}
	}
	return workers
}

func (m *Manager) ListByTeam(teamName string) []Worker {
	m.mu.RLock()
	defer m.mu.RUnlock()

	workers := make([]Worker, 0)
	for _, state := range m.workers {
		if state.worker.TeamName == teamName {
			workers = append(workers, state.worker)
		}
	}
	return workers
}
