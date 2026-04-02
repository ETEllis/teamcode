package agency

import (
	"context"
	"fmt"
	"os"
	"path/filepath"
	"sync"
	"time"
)

type Config struct {
	BaseDir         string
	SharedWorkplace string
	WorkingDir      string
	RuntimeMode     RuntimeMode
	ActorBinaryPath string
	Redis           *RedisConfig
	Voice           *VoiceGatewayConfig
}

type Service struct {
	cfg       Config
	baseDir   string
	mu        sync.RWMutex
	started   bool
	orgID     string
	officeCtx context.Context

	Ledger    *LedgerService
	Bus       EventBus
	Kernel    *Kernel
	Scheduler *Scheduler
	Runtime   *RuntimeManager
	Voice     *VoiceGateway
}

type OfficeStatus struct {
	BaseDir         string              `json:"baseDir"`
	SharedWorkplace string              `json:"sharedWorkplace"`
	RuntimeMode     RuntimeMode         `json:"runtimeMode"`
	OrganizationID  string              `json:"organizationId"`
	LedgerSequence  int64               `json:"ledgerSequence"`
	ActorCount      int                 `json:"actorCount"`
	ScheduleCount   int                 `json:"scheduleCount"`
	Running         bool                `json:"running"`
	BusBackend      string              `json:"busBackend"`
	LastSignal      *WakeSignal         `json:"lastSignal,omitempty"`
	Actors          []AgentIdentity     `json:"actors,omitempty"`
	Schedules       []AgentSchedule     `json:"schedules,omitempty"`
	Constitution    AgencyConstitution  `json:"constitution"`
	Voice           *VoiceGatewayStatus `json:"voice,omitempty"`
}

func NewService(ctx context.Context, cfg Config) (*Service, error) {
	_ = ctx
	baseDir := cfg.BaseDir
	if baseDir == "" {
		cwd, err := os.Getwd()
		if err != nil {
			baseDir = filepath.Join(os.TempDir(), "the-agency")
		} else {
			baseDir = filepath.Join(cwd, ".agency")
		}
	}
	ledger, err := NewLedgerService(filepath.Join(baseDir, "ledger"))
	if err != nil {
		return nil, err
	}

	var bus EventBus = NewMemoryEventBus()
	if cfg.Redis != nil && cfg.Redis.Addr != "" {
		bus = NewRedisEventBus(*cfg.Redis)
	}

	kernel := NewKernel()
	scheduler := NewScheduler(bus)
	var spawner ProcessSpawner
	if cfg.RuntimeMode == RuntimeModeDaemonized && cfg.ActorBinaryPath != "" {
		spawner = &ExecProcessSpawner{
			BinaryPath:       cfg.ActorBinaryPath,
			WorkingDirectory: cfg.WorkingDir,
			BaseDir:          baseDir,
			SharedWorkplace:  cfg.SharedWorkplace,
			Redis:            cfg.Redis,
		}
	}
	runtime := NewRuntimeManager(RuntimeConfig{
		BaseDir:         baseDir,
		SharedWorkplace: cfg.SharedWorkplace,
		Ledger:          ledger,
		Bus:             bus,
		Kernel:          kernel,
		Mode:            cfg.RuntimeMode,
		Spawner:         spawner,
	})
	svc := &Service{
		cfg:       cfg,
		baseDir:   baseDir,
		officeCtx: context.Background(),
		Ledger:    ledger,
		Bus:       bus,
		Kernel:    kernel,
		Scheduler: scheduler,
		Runtime:   runtime,
	}
	if cfg.Voice != nil {
		svc.Voice = NewVoiceGateway(*cfg.Voice, ledger, bus)
	}
	scheduler.SetPublisher(func(ctx context.Context, schedule AgentSchedule, signal WakeSignal) error {
		if _, err := svc.Ledger.AppendSignal(ctx, signal); err != nil {
			return err
		}
		return svc.Bus.Publish(ctx, signal)
	})

	return svc, nil
}

func (s *Service) StartOffice(ctx context.Context, constitution AgencyConstitution) error {
	if err := s.StartCoordinator(ctx, constitution); err != nil {
		return err
	}
	if err := s.StartRuntime(ctx, constitution); err != nil {
		return err
	}
	return s.StartScheduler(ctx, constitution.OrganizationID)
}

func (s *Service) StartCoordinator(ctx context.Context, constitution AgencyConstitution) error {
	if s == nil {
		return fmt.Errorf("service is nil")
	}
	if constitution.OrganizationID == "" {
		return fmt.Errorf("constitution organization id is required")
	}

	s.mu.Lock()
	s.orgID = constitution.OrganizationID
	s.officeCtx = ctx
	s.Runtime.SetConstitution(constitution)
	s.started = true
	s.mu.Unlock()

	if err := s.ensureSharedWorkplace(); err != nil {
		return err
	}

	_, err := s.Ledger.AppendSnapshot(ctx, ContextSnapshot{
		OrganizationID: constitution.OrganizationID,
		Actors:         s.Runtime.Actors(),
		OpenSchedules:  s.Scheduler.Schedules(),
		UpdatedAt:      time.Now().UnixMilli(),
		Metadata: map[string]string{
			"constitutionId": constitution.ID,
			"constitution":   constitution.Name,
			"governanceMode": string(constitution.GovernanceMode),
			"entrySource":    "office.start",
		},
	})
	return err
}

func (s *Service) StartRuntime(ctx context.Context, constitution AgencyConstitution) error {
	if s == nil {
		return fmt.Errorf("service is nil")
	}
	if constitution.OrganizationID == "" {
		constitution.OrganizationID = s.organizationID()
	}
	if constitution.OrganizationID == "" {
		return fmt.Errorf("constitution organization id is required")
	}

	s.mu.Lock()
	s.orgID = constitution.OrganizationID
	s.officeCtx = ctx
	s.Runtime.SetConstitution(constitution)
	s.started = true
	s.mu.Unlock()

	if daemonized := s.cfg.RuntimeMode == RuntimeModeDaemonized; daemonized {
		if spawner, ok := s.Runtime.cfg.Spawner.(*ExecProcessSpawner); ok {
			spawner.ConstitutionName = constitution.Name
		}
	}
	if err := s.ensureSharedWorkplace(); err != nil {
		return err
	}
	if err := s.Runtime.Start(ctx); err != nil {
		return err
	}

	_, err := s.Ledger.AppendSnapshot(ctx, ContextSnapshot{
		OrganizationID: constitution.OrganizationID,
		Actors:         s.Runtime.Actors(),
		OpenSchedules:  s.Scheduler.Schedules(),
		UpdatedAt:      time.Now().UnixMilli(),
		Metadata: map[string]string{
			"constitutionId": constitution.ID,
			"constitution":   constitution.Name,
			"governanceMode": string(constitution.GovernanceMode),
			"entrySource":    "runtime.start",
			"runtimeMode":    string(s.cfg.RuntimeMode),
		},
	})
	return err
}

func (s *Service) RegisterActor(ctx context.Context, actor Actor) error {
	if s == nil {
		return fmt.Errorf("service is nil")
	}
	if err := s.Runtime.Register(ctx, actor); err != nil {
		return err
	}

	s.mu.RLock()
	orgID := s.orgID
	s.mu.RUnlock()
	if orgID == "" {
		orgID = actor.Identity().OrganizationID
	}

	_, err := s.Ledger.AppendSnapshot(ctx, ContextSnapshot{
		OrganizationID: orgID,
		Actors:         s.Runtime.Actors(),
		OpenSchedules:  s.Scheduler.Schedules(),
		UpdatedAt:      time.Now().UnixMilli(),
		Metadata: map[string]string{
			"entrySource": "actor.register",
			"actorId":     actor.Identity().ID,
			"role":        actor.Identity().Role,
		},
	})
	return err
}

func (s *Service) RegisterSchedule(ctx context.Context, schedule AgentSchedule) error {
	if s == nil {
		return fmt.Errorf("service is nil")
	}
	signal := WakeSignal{
		ID:             schedule.ID,
		OrganizationID: s.organizationID(),
		ActorID:        schedule.ActorID,
		Channel:        ActorChannel(schedule.ActorID),
		Kind:           schedule.DefaultSignalKind,
		Payload: map[string]string{
			"scheduleId":  schedule.ID,
			"expression":  schedule.Expression,
			"timezone":    schedule.Timezone,
			"entrySource": "scheduler.register",
		},
		CreatedAt: time.Now().UnixMilli(),
	}
	if _, err := s.Ledger.AppendSchedule(ctx, schedule, signal); err != nil {
		return err
	}
	if err := s.Scheduler.Register(ctx, schedule, signal); err != nil {
		return err
	}
	_, err := s.Ledger.AppendSnapshot(ctx, ContextSnapshot{
		OrganizationID: signal.OrganizationID,
		Actors:         s.Runtime.Actors(),
		OpenSchedules:  s.Scheduler.Schedules(),
		UpdatedAt:      time.Now().UnixMilli(),
		Metadata: map[string]string{
			"entrySource": "scheduler.snapshot",
		},
	})
	return err
}

func (s *Service) StartScheduler(ctx context.Context, organizationID string) error {
	if s == nil {
		return fmt.Errorf("service is nil")
	}
	if organizationID == "" {
		organizationID = s.organizationID()
	}
	if organizationID == "" {
		return fmt.Errorf("organization id is required")
	}

	snapshot, err := s.Ledger.LatestSnapshot(ctx, organizationID)
	if err != nil {
		return err
	}
	for _, schedule := range snapshot.OpenSchedules {
		base := WakeSignal{
			ID:             schedule.ID,
			OrganizationID: organizationID,
			ActorID:        schedule.ActorID,
			Channel:        ActorChannel(schedule.ActorID),
			Kind:           schedule.DefaultSignalKind,
			Payload: map[string]string{
				"scheduleId":  schedule.ID,
				"expression":  schedule.Expression,
				"timezone":    schedule.Timezone,
				"entrySource": "scheduler.restore",
			},
		}
		if err := s.Scheduler.Register(ctx, schedule, base); err != nil {
			return err
		}
	}
	_, err = s.Ledger.AppendSnapshot(ctx, ContextSnapshot{
		OrganizationID: organizationID,
		Actors:         s.Runtime.Actors(),
		OpenSchedules:  s.Scheduler.Schedules(),
		UpdatedAt:      time.Now().UnixMilli(),
		Metadata: map[string]string{
			"entrySource": "scheduler.start",
		},
	})
	return err
}

func (s *Service) PublishSignal(ctx context.Context, signal WakeSignal) error {
	if s == nil {
		return fmt.Errorf("service is nil")
	}
	if signal.OrganizationID == "" {
		signal.OrganizationID = s.organizationID()
	}
	if signal.CreatedAt == 0 {
		signal.CreatedAt = time.Now().UnixMilli()
	}
	if signal.Channel == "" && signal.ActorID != "" {
		signal.Channel = ActorChannel(signal.ActorID)
	}
	if _, err := s.Ledger.AppendSignal(ctx, signal); err != nil {
		return err
	}
	return s.Bus.Publish(ctx, signal)
}

func (s *Service) Snapshot(ctx context.Context, organizationID string) (*ContextSnapshot, error) {
	if organizationID == "" {
		organizationID = s.organizationID()
	}
	return s.Ledger.LatestSnapshot(ctx, organizationID)
}

func (s *Service) Status(ctx context.Context, organizationID string) (OfficeStatus, error) {
	if organizationID == "" {
		organizationID = s.organizationID()
	}
	snapshot, err := s.Snapshot(ctx, organizationID)
	if err != nil {
		return OfficeStatus{}, err
	}
	s.mu.RLock()
	constitution := s.Runtime.constitution
	started := s.started
	s.mu.RUnlock()
	status := OfficeStatus{
		BaseDir:         s.baseDir,
		SharedWorkplace: s.cfg.SharedWorkplace,
		RuntimeMode:     s.cfg.RuntimeMode,
		OrganizationID:  organizationID,
		LedgerSequence:  snapshot.LedgerSequence,
		ActorCount:      len(s.Runtime.Actors()),
		ScheduleCount:   len(s.Scheduler.Schedules()),
		Running:         started && s.Runtime.Running(),
		BusBackend:      busBackendName(s.Bus),
		LastSignal:      snapshot.LastSignal,
		Actors:          snapshot.Actors,
		Schedules:       snapshot.OpenSchedules,
		Constitution:    constitution,
	}
	if s.Voice != nil {
		voiceStatus, err := s.Voice.Status(ctx, organizationID)
		if err == nil {
			status.Voice = &voiceStatus
		}
	}
	return status, nil
}

func (s *Service) organizationID() string {
	s.mu.RLock()
	defer s.mu.RUnlock()
	return s.orgID
}

func (s *Service) ensureSharedWorkplace() error {
	if s.cfg.SharedWorkplace == "" {
		return nil
	}
	return os.MkdirAll(s.cfg.SharedWorkplace, 0o755)
}

func busBackendName(bus EventBus) string {
	switch bus.(type) {
	case *RedisEventBus:
		return "redis"
	case *MemoryEventBus:
		return "memory"
	default:
		return "unknown"
	}
}

func (s *Service) Shutdown(ctx context.Context) error {
	if s == nil {
		return nil
	}
	if s.Scheduler != nil {
		s.Scheduler.Stop()
	}
	if s.Runtime != nil {
		s.Runtime.Stop()
	}
	if s.Bus != nil {
		return s.Bus.Close(ctx)
	}
	return nil
}
