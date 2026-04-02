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

type WorkerStateScope string

const (
	WorkerStateScopeLocal     WorkerStateScope = "local"
	WorkerStateScopeCommitted WorkerStateScope = "committed"
)

type WorkerCommitmentState string

const (
	WorkerCommitmentStateLocal     WorkerCommitmentState = "local"
	WorkerCommitmentStatePending   WorkerCommitmentState = "pending"
	WorkerCommitmentStateCommitted WorkerCommitmentState = "committed"
	WorkerCommitmentStateRejected  WorkerCommitmentState = "rejected"
)

type WorkerWakeState string

const (
	WorkerWakeStateWaiting   WorkerWakeState = "waiting"
	WorkerWakeStateActive    WorkerWakeState = "active"
	WorkerWakeStateQuiescent WorkerWakeState = "quiescent"
)

type Worker struct {
	ID              string                `json:"id"`
	SessionID       string                `json:"sessionId"`
	ParentSessionID string                `json:"parentSessionId"`
	ParentWorkerID  string                `json:"parentWorkerId,omitempty"`
	RootWorkerID    string                `json:"rootWorkerId,omitempty"`
	Lineage         []string              `json:"lineage,omitempty"`
	LineageDepth    int                   `json:"lineageDepth,omitempty"`
	Name            string                `json:"name,omitempty"`
	TeamName        string                `json:"teamName,omitempty"`
	RoleName        string                `json:"roleName,omitempty"`
	Kind            WorkerKind            `json:"kind"`
	Profile         string                `json:"profile,omitempty"`
	Status          WorkerStatus          `json:"status"`
	StateScope      WorkerStateScope      `json:"stateScope,omitempty"`
	CommitmentState WorkerCommitmentState `json:"commitmentState,omitempty"`
	WorkspaceMode   string                `json:"workspaceMode,omitempty"`
	WakeState       WorkerWakeState       `json:"wakeState,omitempty"`
	Prompt          string                `json:"prompt"`
	Result          string                `json:"result,omitempty"`
	Error           string                `json:"error,omitempty"`
	StartedAt       int64                 `json:"startedAt,omitempty"`
	LastSignalAt    int64                 `json:"lastSignalAt,omitempty"`
	LastHeartbeatAt int64                 `json:"lastHeartbeatAt,omitempty"`
	CreatedAt       int64                 `json:"createdAt"`
	UpdatedAt       int64                 `json:"updatedAt"`
	CompletedAt     int64                 `json:"completedAt,omitempty"`
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
	ParentWorkerID  string
	Name            string
	TeamName        string
	RoleName        string
	Kind            WorkerKind
	Profile         string
	Prompt          string
	Title           string
	WorkspaceMode   string
	StateScope      WorkerStateScope
	CommitmentState WorkerCommitmentState
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
	if params.WorkspaceMode == "" {
		params.WorkspaceMode = "shared"
	}
	if params.StateScope == "" {
		params.StateScope = WorkerStateScopeLocal
	}
	if params.CommitmentState == "" {
		params.CommitmentState = WorkerCommitmentStateLocal
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
	rootWorkerID, lineage := m.lineageForSpawn(params.ParentWorkerID, workerID)
	state := &workerState{
		worker: Worker{
			ID:              workerID,
			SessionID:       sess.ID,
			ParentSessionID: params.ParentSessionID,
			ParentWorkerID:  params.ParentWorkerID,
			RootWorkerID:    rootWorkerID,
			Lineage:         lineage,
			LineageDepth:    len(lineage) - 1,
			Name:            params.Name,
			TeamName:        params.TeamName,
			RoleName:        params.RoleName,
			Kind:            params.Kind,
			Profile:         params.Profile,
			Status:          WorkerStatusQueued,
			StateScope:      params.StateScope,
			CommitmentState: params.CommitmentState,
			WorkspaceMode:   params.WorkspaceMode,
			WakeState:       WorkerWakeStateWaiting,
			Prompt:          params.Prompt,
			LastSignalAt:    now,
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
		now := time.Now().UnixMilli()
		worker.Status = WorkerStatusRunning
		worker.WakeState = WorkerWakeStateActive
		worker.StartedAt = now
		worker.LastSignalAt = now
		worker.LastHeartbeatAt = now
		worker.UpdatedAt = now
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
		now := time.Now().UnixMilli()
		worker.Status = status
		worker.Result = content
		if err != nil {
			worker.Error = err.Error()
		} else {
			worker.Error = ""
		}
		worker.WakeState = WorkerWakeStateQuiescent
		worker.LastHeartbeatAt = now
		worker.CompletedAt = now
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

func (m *Manager) lineageForSpawn(parentWorkerID, workerID string) (string, []string) {
	if parentWorkerID == "" {
		return workerID, []string{workerID}
	}

	m.mu.RLock()
	defer m.mu.RUnlock()

	state := m.workers[parentWorkerID]
	if state == nil {
		return workerID, []string{workerID}
	}

	rootWorkerID := state.worker.RootWorkerID
	if rootWorkerID == "" {
		rootWorkerID = state.worker.ID
	}
	lineage := append([]string{}, state.worker.Lineage...)
	if len(lineage) == 0 {
		lineage = []string{state.worker.ID}
	}
	lineage = append(lineage, workerID)
	return rootWorkerID, lineage
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

func (m *Manager) MarkHeartbeat(workerID string) {
	m.update(workerID, func(worker *Worker) {
		now := time.Now().UnixMilli()
		worker.LastHeartbeatAt = now
		worker.UpdatedAt = now
	})
}

func (m *Manager) RecordSignal(workerID string, wakeState WorkerWakeState) {
	m.update(workerID, func(worker *Worker) {
		now := time.Now().UnixMilli()
		worker.WakeState = wakeState
		worker.LastSignalAt = now
		worker.UpdatedAt = now
	})
}

func (m *Manager) SetCommitment(workerID string, scope WorkerStateScope, commitment WorkerCommitmentState) {
	m.update(workerID, func(worker *Worker) {
		if scope != "" {
			worker.StateScope = scope
		}
		if commitment != "" {
			worker.CommitmentState = commitment
		}
		worker.UpdatedAt = time.Now().UnixMilli()
	})
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

func (m *Manager) List() []Worker {
	m.mu.RLock()
	defer m.mu.RUnlock()

	workers := make([]Worker, 0, len(m.workers))
	for _, state := range m.workers {
		workers = append(workers, state.worker)
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

func (m *Manager) ListByRoot(rootWorkerID string) []Worker {
	m.mu.RLock()
	defer m.mu.RUnlock()

	workers := make([]Worker, 0)
	for _, state := range m.workers {
		rootID := state.worker.RootWorkerID
		if rootID == "" {
			rootID = state.worker.ID
		}
		if rootID == rootWorkerID {
			workers = append(workers, state.worker)
		}
	}
	return workers
}
