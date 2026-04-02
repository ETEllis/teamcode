package agency

import (
	"testing"
	"time"

	"github.com/stretchr/testify/require"
)

func TestKernelValidateActionHonorsRolePermissions(t *testing.T) {
	t.Parallel()

	kernel := NewKernel()
	constitution := AgencyConstitution{
		OrganizationID: "org-1",
		Roles: map[string]RoleSpec{
			"engineer": {
				Name:           "engineer",
				AllowedActions: []ActionType{ActionWriteCode, ActionRunTest},
			},
		},
	}
	actor := AgentIdentity{
		ID:             "actor-1",
		Role:           "engineer",
		OrganizationID: "org-1",
	}

	accepted := kernel.ValidateAction(constitution, actor, ActionProposal{
		ActorID:        actor.ID,
		OrganizationID: actor.OrganizationID,
		Type:           ActionWriteCode,
		Target:         "main.go",
	})
	require.True(t, accepted.Accepted)
	require.True(t, accepted.CommitAllowed)

	rejected := kernel.ValidateAction(constitution, actor, ActionProposal{
		ActorID:        actor.ID,
		OrganizationID: actor.OrganizationID,
		Type:           ActionSpawnAgent,
		Target:         "child",
	})
	require.False(t, rejected.Accepted)
	require.False(t, rejected.CommitAllowed)
	require.NotEmpty(t, rejected.Corrections)
}

func TestKernelValidateObservationRejectsOrgMismatch(t *testing.T) {
	t.Parallel()

	kernel := NewKernel()
	constitution := AgencyConstitution{
		OrganizationID: "org-1",
		Roles: map[string]RoleSpec{
			"engineer": {
				Name: "engineer",
			},
		},
	}

	decision := kernel.ValidateObservation(constitution, ObservationSnapshot{
		OrganizationID: "org-1",
		Actor: AgentIdentity{
			ID:             "actor-1",
			Role:           "engineer",
			OrganizationID: "org-2",
		},
		CurrentTime: time.Now(),
	})

	require.False(t, decision.Accepted)
	require.False(t, decision.CommitAllowed)
	require.NotEmpty(t, decision.Corrections)
}
