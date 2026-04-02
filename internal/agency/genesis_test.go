package agency

import (
	"testing"

	"github.com/ETEllis/teamcode/internal/config"
	"github.com/stretchr/testify/require"
)

func TestManufactureGenesisBuildsRoleBundlesAndSignals(t *testing.T) {
	t.Parallel()

	plan := ManufactureGenesis(GenesisManufacturingInput{
		Intent:           "Ship the distributed office runtime",
		Domain:           "software delivery",
		TimeHorizon:      "this sprint",
		WorkingStyle:     "parallel and review-heavy",
		Governance:       "hierarchical",
		GoalShape:        "release-ready runtime",
		ConstitutionName: "coding-office",
		Constitution: config.AgencyConstitution{
			Name:         "coding-office",
			Blueprint:    "software-team",
			TeamTemplate: "leader-led",
			Governance:   "hierarchical",
			Policies: config.AgencyConstitutionPolicies{
				WakeMode:      "event-reactive",
				ConsensusMode: "role-quorum",
			},
		},
		Template: config.TeamTemplate{
			Name:           "leader-led",
			LeadershipMode: "leader-led",
			Orientation:    "software delivery",
			Roles: []config.TeamRoleTemplate{
				{Name: "lead", Responsible: "Coordinate the office.", Profile: "coder", CanSpawnSubagents: boolPointer(true)},
				{Name: "implementer", Responsible: "Build the changes.", Profile: "coder", ReportsTo: "lead", CanSpawnSubagents: boolPointer(true)},
				{Name: "reviewer", Responsible: "Gate quality.", Profile: "coder", ReportsTo: "lead", CanSpawnSubagents: boolPointer(false)},
			},
		},
		DefaultCadence:  "weekday-office-hours",
		Timezone:        "America/Detroit",
		SharedWorkplace: "/shared_workplace",
		RedisAddress:    "redis:6379",
		LedgerPath:      "/ledger",
	})

	require.Equal(t, "hierarchical", plan.Topology)
	require.Len(t, plan.RoleBundles, 3)
	require.NotEmpty(t, plan.SocialThread)
	require.Len(t, plan.ManufacturingSignals, 3)

	lead := plan.RoleBundles[0]
	require.Equal(t, "lead", lead.RoleName)
	require.NotEmpty(t, lead.SystemPrompt)
	require.Contains(t, lead.Tools, "task-board")
	require.Contains(t, lead.Skills, "planning")
	require.Contains(t, lead.AllowedActions, ActionSpawnAgent)
	require.Equal(t, "weekday-office-hours", lead.RecommendedSchedule.Expression)
}
