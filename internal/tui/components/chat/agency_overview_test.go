package chat

import (
	"testing"

	"github.com/ETEllis/teamcode/internal/team"
	"github.com/stretchr/testify/require"
)

func TestBuildAgencyThreadUsesRuntimeState(t *testing.T) {
	t.Parallel()

	snapshot := &team.Snapshot{
		TeamName: "launch-office",
		Office: &team.OfficeState{
			ContinuityStatus: "active",
			Roster: []team.OfficeRosterRow{
				{AgentName: "Avery", RoleName: "lead", Leader: true, Status: "running", WorkspaceMode: "shared", CommitmentState: "pending"},
				{AgentName: "Rin", RoleName: "reviewer", Status: "queued", WorkspaceMode: "sandbox", CommitmentState: "committed"},
			},
		},
		RuntimeSummary: team.RuntimeSummary{
			Active:    1,
			Pending:   1,
			Committed: 1,
		},
		UnreadBroadcasts: 1,
		PendingHandoffs:  1,
	}

	lines := buildAgencyThread(snapshot)
	require.NotEmpty(t, lines)
	require.Contains(t, lines[0], "launch-office office is active")
	require.Contains(t, lines[1], "Avery is running")
}
