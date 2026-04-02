package agency

import (
	"context"
	"fmt"
	"strconv"
	"strings"
	"sync"
	"time"

	"github.com/google/uuid"
)

type Scheduler struct {
	bus       EventBus
	mu        sync.Mutex
	cancel    map[string]context.CancelFunc
	schedules map[string]AgentSchedule
	publish   func(context.Context, AgentSchedule, WakeSignal) error
}

func NewScheduler(bus EventBus) *Scheduler {
	s := &Scheduler{
		bus:       bus,
		cancel:    make(map[string]context.CancelFunc),
		schedules: make(map[string]AgentSchedule),
	}
	s.publish = func(ctx context.Context, schedule AgentSchedule, signal WakeSignal) error {
		return s.bus.Publish(ctx, signal)
	}
	return s
}

func (s *Scheduler) Register(ctx context.Context, schedule AgentSchedule, base WakeSignal) error {
	if schedule.ID == "" {
		return fmt.Errorf("schedule id is required")
	}
	if !schedule.Enabled {
		return nil
	}
	interval, err := parseSchedule(schedule.Expression)
	if err != nil {
		return err
	}

	s.mu.Lock()
	defer s.mu.Unlock()
	if cancel, ok := s.cancel[schedule.ID]; ok {
		cancel()
	}
	runCtx, cancel := context.WithCancel(ctx)
	s.cancel[schedule.ID] = cancel
	s.schedules[schedule.ID] = schedule
	go s.run(runCtx, schedule, base, interval)
	return nil
}

func (s *Scheduler) SetPublisher(fn func(context.Context, AgentSchedule, WakeSignal) error) {
	s.mu.Lock()
	defer s.mu.Unlock()
	if fn == nil {
		s.publish = func(ctx context.Context, schedule AgentSchedule, signal WakeSignal) error {
			return s.bus.Publish(ctx, signal)
		}
		return
	}
	s.publish = fn
}

func (s *Scheduler) Schedules() []AgentSchedule {
	s.mu.Lock()
	defer s.mu.Unlock()
	out := make([]AgentSchedule, 0, len(s.schedules))
	for _, schedule := range s.schedules {
		out = append(out, schedule)
	}
	return out
}

func (s *Scheduler) Stop() {
	s.mu.Lock()
	defer s.mu.Unlock()
	for _, cancel := range s.cancel {
		cancel()
	}
	s.cancel = make(map[string]context.CancelFunc)
	s.schedules = make(map[string]AgentSchedule)
}

func (s *Scheduler) run(ctx context.Context, schedule AgentSchedule, base WakeSignal, interval time.Duration) {
	ticker := time.NewTicker(interval)
	defer ticker.Stop()

	for {
		select {
		case <-ctx.Done():
			return
		case <-ticker.C:
			signal := base
			if signal.ID == "" {
				signal.ID = uuid.NewString()
			}
			signal.ActorID = schedule.ActorID
			signal.Kind = schedule.DefaultSignalKind
			signal.CreatedAt = time.Now().UnixMilli()
			if signal.Channel == "" {
				signal.Channel = ActorChannel(schedule.ActorID)
			}
			s.mu.Lock()
			publish := s.publish
			s.mu.Unlock()
			_ = publish(ctx, schedule, signal)
		}
	}
}

// ParseScheduleInterval parses a schedule expression and returns its firing interval.
func ParseScheduleInterval(expr string) (time.Duration, error) {
	return parseSchedule(expr)
}

func parseSchedule(expr string) (time.Duration, error) {
	expr = strings.TrimSpace(expr)
	switch expr {
	case "@hourly":
		return time.Hour, nil
	case "@daily":
		return 24 * time.Hour, nil
	case "@weekly":
		return 7 * 24 * time.Hour, nil
	}
	if after, ok := strings.CutPrefix(expr, "@every "); ok {
		return time.ParseDuration(strings.TrimSpace(after))
	}
	parts := strings.Fields(expr)
	if len(parts) == 5 && strings.HasPrefix(parts[0], "*/") {
		n, err := strconv.Atoi(strings.TrimPrefix(parts[0], "*/"))
		if err != nil || n <= 0 {
			return 0, fmt.Errorf("invalid schedule expression: %s", expr)
		}
		return time.Duration(n) * time.Minute, nil
	}
	return 0, fmt.Errorf("unsupported schedule expression: %s", expr)
}
