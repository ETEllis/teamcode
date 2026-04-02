package agency

import (
	"context"
	"testing"
	"time"

	"github.com/stretchr/testify/require"
)

type stubActor struct {
	identity     AgentIdentity
	capabilities CapabilityPack
	proposals    []ActionProposal
}

func (a stubActor) Identity() AgentIdentity {
	return a.identity
}

func (a stubActor) Capabilities() CapabilityPack {
	return a.capabilities
}

func (a stubActor) Handle(_ context.Context, _ ObservationSnapshot, _ WakeSignal) ([]ActionProposal, error) {
	return a.proposals, nil
}

func TestAgentDaemonProcessesSignalsFromBus(t *testing.T) {
	t.Parallel()

	ledger, err := NewLedgerService(t.TempDir())
	require.NoError(t, err)

	bus := NewMemoryEventBus()
	kernel := NewKernel()
	constitution := AgencyConstitution{
		OrganizationID: "org-1",
		Roles: map[string]RoleSpec{
			"lead": {
				Name:           "lead",
				AllowedActions: []ActionType{ActionBroadcast},
			},
		},
	}
	actor := stubActor{
		identity: AgentIdentity{
			ID:             "actor-1",
			Role:           "lead",
			OrganizationID: "org-1",
		},
		capabilities: CapabilityPack{Tools: []string{"shell"}},
		proposals: []ActionProposal{{
			ID:     "proposal-1",
			Type:   ActionBroadcast,
			Target: "team",
			Payload: map[string]any{
				"message": "hello office",
			},
		}},
	}

	daemon := NewAgentDaemon(RuntimeConfig{
		SharedWorkplace: t.TempDir(),
		Ledger:          ledger,
		Bus:             bus,
		Kernel:          kernel,
	}, constitution, actor)

	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()
	go func() {
		_ = daemon.Run(ctx)
	}()
	time.Sleep(50 * time.Millisecond)

	require.NoError(t, bus.Publish(ctx, WakeSignal{
		ID:             "signal-1",
		OrganizationID: "org-1",
		ActorID:        "actor-1",
		Channel:        ActorChannel("actor-1"),
		Kind:           SignalTick,
		CreatedAt:      time.Now().UnixMilli(),
	}))

	require.Eventually(t, func() bool {
		entries, err := ledger.Replay(context.Background())
		require.NoError(t, err)
		for _, entry := range entries {
			if entry.Action != nil && entry.Action.ID == "proposal-1" {
				return true
			}
		}
		return false
	}, 3*time.Second, 20*time.Millisecond)
}
