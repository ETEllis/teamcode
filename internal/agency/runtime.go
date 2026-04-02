package agency

import (
	"context"
	"fmt"
	"sync"
	"time"
)

type Actor interface {
	Identity() AgentIdentity
	Capabilities() CapabilityPack
	Handle(context.Context, ObservationSnapshot, WakeSignal) ([]ActionProposal, error)
}

type RuntimeConfig struct {
	BaseDir         string
	SharedWorkplace string
	Ledger          *LedgerService
	Bus             EventBus
	Kernel          *Kernel
	Mode            RuntimeMode
	Spawner         ProcessSpawner
}

type RuntimeManager struct {
	cfg          RuntimeConfig
	mu           sync.RWMutex
	constitution AgencyConstitution
	actors       map[string]Actor
	specs        map[string]ActorRuntimeSpec
	cancels      map[string]context.CancelFunc
	processes    map[string]ProcessHandle
	running      bool
}

func NewRuntimeManager(cfg RuntimeConfig) *RuntimeManager {
	if cfg.Mode == "" {
		cfg.Mode = RuntimeModeEmbedded
	}
	return &RuntimeManager{
		cfg:       cfg,
		actors:    make(map[string]Actor),
		specs:     make(map[string]ActorRuntimeSpec),
		cancels:   make(map[string]context.CancelFunc),
		processes: make(map[string]ProcessHandle),
	}
}

func (r *RuntimeManager) SetConstitution(constitution AgencyConstitution) {
	r.mu.Lock()
	defer r.mu.Unlock()
	r.constitution = constitution
}

func (r *RuntimeManager) Register(ctx context.Context, actor Actor) error {
	r.mu.Lock()
	defer r.mu.Unlock()
	id := actor.Identity().ID
	if id == "" {
		return fmt.Errorf("actor id is required")
	}
	if _, exists := r.actors[id]; exists {
		return fmt.Errorf("actor already registered: %s", id)
	}
	r.actors[id] = actor
	spec := ActorRuntimeSpec{
		Identity:        actor.Identity(),
		Capabilities:    actor.Capabilities(),
		SharedWorkplace: r.cfg.SharedWorkplace,
		OrganizationID:  actor.Identity().OrganizationID,
		RegisteredAt:    time.Now().UnixMilli(),
		RuntimeMode:     r.cfg.Mode,
	}
	r.specs[id] = spec
	if err := writeActorSpec(r.cfg.BaseDir, spec); err != nil {
		return err
	}
	if r.running {
		return r.startActorLocked(ctx, actor)
	}
	return nil
}

func (r *RuntimeManager) RegisterSpec(ctx context.Context, spec ActorRuntimeSpec) error {
	r.mu.Lock()
	defer r.mu.Unlock()

	if spec.Identity.ID == "" {
		return fmt.Errorf("actor id is required")
	}
	if spec.OrganizationID == "" {
		spec.OrganizationID = spec.Identity.OrganizationID
	}
	if spec.SharedWorkplace == "" {
		spec.SharedWorkplace = r.cfg.SharedWorkplace
	}
	if spec.RegisteredAt == 0 {
		spec.RegisteredAt = time.Now().UnixMilli()
	}
	if spec.RuntimeMode == "" {
		spec.RuntimeMode = r.cfg.Mode
	}

	r.specs[spec.Identity.ID] = spec
	if err := writeActorSpec(r.cfg.BaseDir, spec); err != nil {
		return err
	}
	if r.running && r.cfg.Mode == RuntimeModeDaemonized {
		return r.startDaemonLocked(ctx, spec)
	}
	return nil
}

func (r *RuntimeManager) Start(ctx context.Context) error {
	r.mu.Lock()
	defer r.mu.Unlock()
	if r.running {
		return nil
	}
	if err := r.loadPersistedSpecsLocked(); err != nil {
		return err
	}
	r.running = true
	for _, actor := range r.actors {
		if err := r.startActorLocked(ctx, actor); err != nil {
			r.stopLocked()
			return err
		}
	}
	if r.cfg.Mode == RuntimeModeDaemonized {
		for id, spec := range r.specs {
			if _, inProcess := r.actors[id]; inProcess {
				continue
			}
			if err := r.startDaemonLocked(ctx, spec); err != nil {
				r.stopLocked()
				return err
			}
		}
	}
	return nil
}

func (r *RuntimeManager) Stop() {
	r.mu.Lock()
	defer r.mu.Unlock()
	r.stopLocked()
}

func (r *RuntimeManager) stopLocked() {
	for _, cancel := range r.cancels {
		cancel()
	}
	r.cancels = map[string]context.CancelFunc{}
	for id, handle := range r.processes {
		_ = handle.Stop(context.Background())
		delete(r.processes, id)
	}
	r.running = false
}

func (r *RuntimeManager) Actors() []AgentIdentity {
	r.mu.RLock()
	defer r.mu.RUnlock()
	out := make([]AgentIdentity, 0, len(r.specs))
	for _, spec := range r.specs {
		out = append(out, spec.Identity)
	}
	return out
}

func (r *RuntimeManager) Specs() []ActorRuntimeSpec {
	r.mu.RLock()
	defer r.mu.RUnlock()
	out := make([]ActorRuntimeSpec, 0, len(r.specs))
	for _, spec := range r.specs {
		out = append(out, spec)
	}
	return out
}

func (r *RuntimeManager) Running() bool {
	r.mu.RLock()
	defer r.mu.RUnlock()
	return r.running
}

func (r *RuntimeManager) startActorLocked(parent context.Context, actor Actor) error {
	if r.cfg.Mode == RuntimeModeDaemonized {
		spec, ok := r.specs[actor.Identity().ID]
		if !ok {
			spec = ActorRuntimeSpec{
				Identity:        actor.Identity(),
				Capabilities:    actor.Capabilities(),
				SharedWorkplace: r.cfg.SharedWorkplace,
				OrganizationID:  actor.Identity().OrganizationID,
				RegisteredAt:    time.Now().UnixMilli(),
				RuntimeMode:     r.cfg.Mode,
			}
			r.specs[actor.Identity().ID] = spec
		}
		return r.startDaemonLocked(parent, spec)
	}
	id := actor.Identity().ID
	runCtx, cancel := context.WithCancel(parent)
	r.cancels[id] = cancel
	actorCh, err := r.cfg.Bus.Subscribe(runCtx, ActorChannel(id))
	if err != nil {
		cancel()
		delete(r.cancels, id)
		return err
	}
	orgCh, err := r.cfg.Bus.Subscribe(runCtx, OrganizationChannel(actor.Identity().OrganizationID))
	if err != nil {
		cancel()
		delete(r.cancels, id)
		return err
	}
	go r.loop(runCtx, actor, actorCh)
	go r.loop(runCtx, actor, orgCh)
	return nil
}

func (r *RuntimeManager) startDaemonLocked(parent context.Context, spec ActorRuntimeSpec) error {
	if r.cfg.Spawner == nil {
		return fmt.Errorf("daemonized runtime mode requires a process spawner")
	}
	if existing, ok := r.processes[spec.Identity.ID]; ok && existing != nil && existing.PID() > 0 {
		return nil
	}
	handle, err := r.cfg.Spawner.Spawn(parent, spec)
	if err != nil {
		return err
	}
	r.processes[spec.Identity.ID] = handle
	go r.waitForProcess(spec.Identity.ID, handle)
	return nil
}

func (r *RuntimeManager) waitForProcess(actorID string, handle ProcessHandle) {
	_ = handle.Wait()
	r.mu.Lock()
	defer r.mu.Unlock()
	delete(r.processes, actorID)
}

func (r *RuntimeManager) loop(ctx context.Context, actor Actor, ch <-chan WakeSignal) {
	for {
		select {
		case <-ctx.Done():
			return
		case signal, ok := <-ch:
			if !ok {
				return
			}
			r.handleSignal(ctx, actor, signal)
		}
	}
}

func (r *RuntimeManager) handleSignal(ctx context.Context, actor Actor, signal WakeSignal) {
	r.mu.RLock()
	constitution := r.constitution
	r.mu.RUnlock()
	processActorSignal(ctx, r.cfg, constitution, actor, signal)
}

func ActorChannel(actorID string) string {
	return "agency.actor." + actorID
}

func OrganizationChannel(organizationID string) string {
	return "agency.org." + organizationID
}

func ApprovalChannel(organizationID string) string {
	return "agency.approval." + organizationID
}

func (r *RuntimeManager) loadPersistedSpecsLocked() error {
	specs, err := loadActorSpecs(r.cfg.BaseDir)
	if err != nil {
		return err
	}
	for _, spec := range specs {
		if _, exists := r.specs[spec.Identity.ID]; !exists {
			r.specs[spec.Identity.ID] = spec
		}
	}
	return nil
}
