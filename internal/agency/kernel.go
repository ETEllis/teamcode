package agency

import (
	"slices"
	"time"
)

type Kernel struct{}

func NewKernel() *Kernel {
	return &Kernel{}
}

func (k *Kernel) ValidateObservation(constitution AgencyConstitution, observation ObservationSnapshot) KernelDecision {
	now := time.Now().UnixMilli()
	if observation.OrganizationID == "" {
		return rejectedDecision("observation organization is required", observation.Actor.ID, now)
	}
	if observation.Actor.ID == "" {
		return rejectedDecision("observation actor is required", observation.Actor.ID, now)
	}
	if observation.Actor.OrganizationID == "" || observation.Actor.OrganizationID != observation.OrganizationID {
		return rejectedDecision("observation organization mismatch", observation.Actor.ID, now)
	}
	if observation.CurrentTime.IsZero() {
		return rejectedDecision("observation current time is required", observation.Actor.ID, now)
	}
	role, ok := constitution.Roles[observation.Actor.Role]
	if !ok {
		return rejectedDecision("unknown role", observation.Actor.ID, now)
	}
	if len(role.ObservationScopes) > 0 && len(observation.Metadata) > 0 {
		scope := observation.Metadata["scope"]
		if scope != "" && !slices.Contains(role.ObservationScopes, scope) {
			return rejectedDecision("observation scope not allowed for role", observation.Actor.ID, now)
		}
	}
	for _, signal := range observation.RecentSignals {
		if signal.OrganizationID != "" && signal.OrganizationID != observation.OrganizationID {
			return rejectedDecision("observation signal organization mismatch", observation.Actor.ID, now)
		}
	}

	return KernelDecision{
		Accepted:      true,
		ValidatedAt:   now,
		CommitAllowed: true,
		Reason:        "validated",
	}
}

func (k *Kernel) ValidateAction(constitution AgencyConstitution, actor AgentIdentity, proposal ActionProposal) KernelDecision {
	now := time.Now().UnixMilli()

	if proposal.ActorID == "" || actor.ID == "" || proposal.ActorID != actor.ID {
		return rejectedDecision("actor identity mismatch", actor.ID, now)
	}
	if proposal.OrganizationID == "" || actor.OrganizationID == "" || proposal.OrganizationID != actor.OrganizationID {
		return rejectedDecision("organization mismatch", actor.ID, now)
	}
	if proposal.Type == "" {
		return rejectedDecision("action type is required", actor.ID, now)
	}

	role, ok := constitution.Roles[actor.Role]
	if !ok {
		return rejectedDecision("unknown role", actor.ID, now)
	}
	if len(role.AllowedActions) > 0 && !slices.Contains(role.AllowedActions, proposal.Type) {
		return rejectedDecision("action not allowed for role", actor.ID, now)
	}
	if proposal.Type == ActionSpawnAgent && !role.CanSpawnAgents {
		return rejectedDecision("role cannot spawn agents", actor.ID, now)
	}
	if requiresTarget(proposal.Type) && proposal.Target == "" {
		return rejectedDecision("target is required", actor.ID, now)
	}

	return KernelDecision{
		Accepted:      true,
		ValidatedAt:   now,
		CommitAllowed: true,
		Reason:        "validated",
	}
}

func requiresTarget(action ActionType) bool {
	switch action {
	case ActionWriteCode, ActionRunTest, ActionPingPeer, ActionUpdateTask, ActionRequestReview, ActionPublishArtifact, ActionHandoffShift:
		return true
	default:
		return false
	}
}

func rejectedDecision(reason, actorID string, validatedAt int64) KernelDecision {
	return KernelDecision{
		Accepted:      false,
		Reason:        reason,
		ValidatedAt:   validatedAt,
		CommitAllowed: false,
		Corrections: []CorrectionSignal{
			{
				Code:          "kernel_reject",
				Message:       reason,
				TargetActorID: actorID,
				CreatedAt:     validatedAt,
			},
		},
	}
}
