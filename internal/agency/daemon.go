package agency

import (
	"context"
	"fmt"
	"time"
)

type AgentDaemon struct {
	cfg          RuntimeConfig
	constitution AgencyConstitution
	actor        Actor
}

func NewAgentDaemon(cfg RuntimeConfig, constitution AgencyConstitution, actor Actor) *AgentDaemon {
	return &AgentDaemon{
		cfg:          cfg,
		constitution: constitution,
		actor:        actor,
	}
}

func (d *AgentDaemon) Run(ctx context.Context) error {
	if d == nil {
		return fmt.Errorf("daemon is nil")
	}
	if d.actor == nil {
		return fmt.Errorf("daemon actor is required")
	}
	if d.cfg.Ledger == nil {
		return fmt.Errorf("ledger is required")
	}
	if d.cfg.Bus == nil {
		return fmt.Errorf("event bus is required")
	}
	if d.cfg.Kernel == nil {
		return fmt.Errorf("kernel is required")
	}

	identity := d.actor.Identity()
	if identity.ID == "" {
		return fmt.Errorf("actor id is required")
	}
	if identity.OrganizationID == "" {
		return fmt.Errorf("organization id is required")
	}

	actorCh, err := d.cfg.Bus.Subscribe(ctx, ActorChannel(identity.ID))
	if err != nil {
		return err
	}
	orgCh, err := d.cfg.Bus.Subscribe(ctx, OrganizationChannel(identity.OrganizationID))
	if err != nil {
		return err
	}

	for {
		select {
		case <-ctx.Done():
			return nil
		case signal, ok := <-actorCh:
			if !ok {
				return nil
			}
			d.handleSignal(ctx, signal)
		case signal, ok := <-orgCh:
			if !ok {
				return nil
			}
			d.handleSignal(ctx, signal)
		}
	}
}

func (d *AgentDaemon) handleSignal(ctx context.Context, signal WakeSignal) {
	processActorSignal(ctx, d.cfg, d.constitution, d.actor, signal)
}

func processActorSignal(ctx context.Context, cfg RuntimeConfig, constitution AgencyConstitution, actor Actor, signal WakeSignal) {
	identity := actor.Identity()
	snapshot, err := cfg.Ledger.LatestSnapshot(ctx, identity.OrganizationID)
	if err != nil {
		return
	}

	observation := ObservationSnapshot{
		OrganizationID: snapshot.OrganizationID,
		Actor:          identity,
		LedgerSequence: snapshot.LedgerSequence,
		RecentSignals:  []WakeSignal{signal},
		Resources: ResourceState{
			SharedWorkplace: cfg.SharedWorkplace,
			AvailableTools:  actor.Capabilities().Tools,
		},
		CurrentTime: time.Now(),
		Metadata: map[string]string{
			"scope": "default",
		},
	}

	decision := cfg.Kernel.ValidateObservation(constitution, observation)
	if !decision.Accepted {
		_, _ = cfg.Ledger.Append(ctx, LedgerEntry{
			OrganizationID: identity.OrganizationID,
			Kind:           LedgerEntrySignal,
			ActorID:        identity.ID,
			Signal:         &signal,
			Decision:       &decision,
			CommittedAt:    time.Now().UnixMilli(),
		})
		return
	}

	proposals, err := actor.Handle(ctx, observation, signal)
	if err != nil {
		return
	}

	for _, proposal := range proposals {
		if proposal.ProposedAt == 0 {
			proposal.ProposedAt = time.Now().UnixMilli()
		}
		if proposal.ActorID == "" {
			proposal.ActorID = identity.ID
		}
		if proposal.OrganizationID == "" {
			proposal.OrganizationID = identity.OrganizationID
		}

		decision := cfg.Kernel.ValidateAction(constitution, identity, proposal)
		entry := LedgerEntry{
			OrganizationID: proposal.OrganizationID,
			Kind:           LedgerEntryAction,
			ActorID:        identity.ID,
			Action:         &proposal,
			Decision:       &decision,
			CommittedAt:    time.Now().UnixMilli(),
		}
		if _, err := cfg.Ledger.Append(ctx, entry); err != nil {
			continue
		}
		if decision.Accepted {
			_ = cfg.Bus.Publish(ctx, WakeSignal{
				ID:             signal.ID,
				OrganizationID: proposal.OrganizationID,
				Channel:        OrganizationChannel(proposal.OrganizationID),
				Kind:           SignalBroadcast,
				Payload: map[string]string{
					"entryKind": string(LedgerEntryAction),
					"actorId":   identity.ID,
					"action":    string(proposal.Type),
				},
				CreatedAt: time.Now().UnixMilli(),
			})
		}
	}
}
