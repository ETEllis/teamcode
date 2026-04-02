package agency

import (
	"context"
	"testing"
	"time"

	"github.com/stretchr/testify/require"
)

func TestSchedulerEmitsWakeSignal(t *testing.T) {
	t.Parallel()

	bus := NewMemoryEventBus()
	scheduler := NewScheduler(bus)
	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()
	defer scheduler.Stop()

	signals, err := bus.Subscribe(ctx, ActorChannel("actor-1"))
	require.NoError(t, err)

	err = scheduler.Register(ctx, AgentSchedule{
		ID:                "sched-1",
		ActorID:           "actor-1",
		Expression:        "@every 50ms",
		Enabled:           true,
		DefaultSignalKind: SignalSchedule,
	}, WakeSignal{
		OrganizationID: "org-1",
	})
	require.NoError(t, err)

	select {
	case sig := <-signals:
		require.Equal(t, "actor-1", sig.ActorID)
		require.Equal(t, SignalSchedule, sig.Kind)
	case <-time.After(500 * time.Millisecond):
		t.Fatal("expected wake signal")
	}
}
