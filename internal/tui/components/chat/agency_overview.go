package chat

import (
	"context"
	"fmt"
	"sort"
	"strings"

	"github.com/ETEllis/teamcode/internal/app"
	"github.com/ETEllis/teamcode/internal/config"
	"github.com/ETEllis/teamcode/internal/orchestration"
	"github.com/ETEllis/teamcode/internal/team"
)

type AgencyOverview struct {
	ProductName         string
	CurrentConstitution string
	SoloConstitution    string
	OfficeMode          string
	ConsensusMode       string
	SharedWorkplace     string
	RedisAddress        string
	LedgerPath          string
	LastEvent           string
	Blueprint           string
	TeamTemplate        string
	Governance          string
	Topology            string
	WorkspaceMode       string
	RequiredGates       []string
	GenesisSummary      string
	ManufacturedRoles   int
	ThreadSource        string
	Running             bool
	DefaultQuorum       int
	TeamName            string
	Leader              string
	UnreadDirect        int
	UnreadBroadcasts    int
	PendingHandoffs     int
	MemberCount         int
	WorkerCount         int
	ActiveWorkerCount   int
	BoardSummary        map[string]int
	Thread              []string
	Constitutions       []string
	HasOfficeState      bool
	HasTeamSnapshot     bool
}

func InspectAgencyOverview(appState *app.App) AgencyOverview {
	overview := AgencyOverview{
		ProductName:      "The Agency",
		BoardSummary:     map[string]int{},
		RequiredGates:    []string{},
		Constitutions:    []string{},
		HasOfficeState:   false,
		HasTeamSnapshot:  false,
		UnreadDirect:     0,
		UnreadBroadcasts: 0,
		PendingHandoffs:  0,
	}

	cfg := config.Get()
	if cfg != nil {
		overview.ProductName = cfg.Agency.ProductName
		overview.CurrentConstitution = cfg.Agency.CurrentConstitution
		overview.SoloConstitution = cfg.Agency.SoloConstitution
		active := config.ActiveConstitution(cfg)
		if overview.CurrentConstitution == "" {
			overview.CurrentConstitution = active.Name
		}
		if overview.SoloConstitution == "" {
			overview.SoloConstitution = active.Name
		}
		overview.OfficeMode = cfg.Agency.Office.Mode
		overview.ConsensusMode = cfg.Agency.Ledger.ConsensusMode
		overview.SharedWorkplace = cfg.Agency.Office.SharedWorkplace
		overview.RedisAddress = cfg.Agency.Redis.Address
		overview.LedgerPath = cfg.Agency.Ledger.Path
		overview.DefaultQuorum = cfg.Agency.Ledger.DefaultQuorum
	}

	if appState != nil && appState.Agency != nil {
		office, err := appState.Agency.InspectOffice()
		if err == nil {
			overview.HasOfficeState = office.ProductName != "" || office.Constitution != "" || office.SharedWorkplace != "" || office.Running
			if office.ProductName != "" {
				overview.ProductName = office.ProductName
			}
			if office.Constitution != "" {
				overview.CurrentConstitution = office.Constitution
			}
			if office.Mode != "" {
				overview.OfficeMode = office.Mode
			}
			if office.SharedWorkplace != "" {
				overview.SharedWorkplace = office.SharedWorkplace
			}
			if office.RedisAddress != "" {
				overview.RedisAddress = office.RedisAddress
			}
			if office.LedgerPath != "" {
				overview.LedgerPath = office.LedgerPath
			}
			if office.ConsensusMode != "" {
				overview.ConsensusMode = office.ConsensusMode
			}
			if office.DefaultQuorum > 0 {
				overview.DefaultQuorum = office.DefaultQuorum
			}
			overview.Running = office.Running
			overview.LastEvent = office.LastEvent
		}

		org, err := appState.Agency.InspectOrganization()
		if err == nil {
			if org.ProductName != "" {
				overview.ProductName = org.ProductName
			}
			if org.CurrentConstitution != "" {
				overview.CurrentConstitution = org.CurrentConstitution
			}
			if org.SoloConstitution != "" {
				overview.SoloConstitution = org.SoloConstitution
			}
			overview.Blueprint = org.Blueprint
			overview.TeamTemplate = org.TeamTemplate
			overview.Governance = org.Governance
			overview.Topology = org.Governance
			overview.WorkspaceMode = org.WorkspaceMode
			overview.RequiredGates = append([]string(nil), org.RequiredGates...)
		}

		genesis, err := appState.Agency.LatestGenesis()
		if err == nil && genesis != nil {
			overview.GenesisSummary = genesis.Summary
			overview.ManufacturedRoles = len(genesis.RoleBundles)
			if overview.Topology == "" {
				overview.Topology = genesis.Topology
			}
			if !overview.HasTeamSnapshot && len(genesis.SocialThread) > 0 {
				overview.Thread = append([]string(nil), genesis.SocialThread...)
				overview.ThreadSource = "genesis"
			}
		}

		constitutions := appState.Agency.ListConstitutions()
		names := make([]string, 0, len(constitutions))
		for _, constitution := range constitutions {
			if strings.TrimSpace(constitution.Name) != "" {
				names = append(names, constitution.Name)
			}
		}
		sort.Strings(names)
		overview.Constitutions = names
	}

	snapshot := latestTeamSnapshot(appState)
	if snapshot != nil {
		overview.HasTeamSnapshot = true
		overview.TeamName = snapshot.TeamName
		if snapshot.Context != nil {
			overview.Leader = snapshot.Context.Leader
		}
		overview.UnreadDirect = snapshot.UnreadDirect
		overview.UnreadBroadcasts = snapshot.UnreadBroadcasts
		overview.PendingHandoffs = snapshot.PendingHandoffs
		overview.MemberCount = len(snapshot.Members)
		overview.WorkerCount = len(snapshot.Workers)
		overview.ActiveWorkerCount = activeWorkerCount(snapshot)
		overview.BoardSummary = snapshot.BoardSummary
		overview.Thread = buildAgencyThread(snapshot)
		overview.ThreadSource = "runtime"
		if snapshot.Office != nil && overview.Topology == "" {
			overview.Topology = snapshot.Office.ConsensusMode
		}
	}

	return overview
}

func latestTeamSnapshot(appState *app.App) *team.Snapshot {
	if appState == nil || appState.Team == nil {
		return nil
	}

	cfg := config.Get()
	teamName := ""
	if cfg != nil {
		teamName = strings.TrimSpace(cfg.Team.ActiveTeam)
	}

	ctx := context.Background()
	if teamName == "" {
		latest, err := appState.Team.MostRecentTeamName(ctx)
		if err == nil {
			teamName = latest
		}
	}
	if teamName == "" {
		return nil
	}

	var workers []orchestration.Worker
	if appState.Workers != nil {
		workers = appState.Workers.ListByTeam(teamName)
	}

	snapshot, err := appState.Team.Snapshot(ctx, teamName, workers)
	if err != nil {
		return nil
	}
	return snapshot
}

func activeWorkerCount(snapshot *team.Snapshot) int {
	if snapshot == nil {
		return 0
	}
	active := 0
	for _, worker := range snapshot.Workers {
		if worker.Status == orchestration.WorkerStatusQueued || worker.Status == orchestration.WorkerStatusRunning {
			active++
		}
	}
	return active
}

func buildAgencyThread(snapshot *team.Snapshot) []string {
	if snapshot == nil {
		return nil
	}

	lines := make([]string, 0, 5)
	if snapshot.Office != nil {
		lines = append(lines, describeOfficePulse(snapshot))
		for _, line := range describeRuntimeRoster(snapshot) {
			if len(lines) >= 5 {
				break
			}
			lines = append(lines, line)
		}
	}

	if len(lines) == 0 && snapshot.Context != nil && strings.TrimSpace(snapshot.Context.Leader) != "" {
		lines = append(lines, fmt.Sprintf("%s is coordinating the office", snapshot.Context.Leader))
	}

	if len(lines) < 5 {
		for _, member := range snapshot.Members {
			if len(lines) >= 5 {
				break
			}
			status := emptyFallback(member.Status, "waiting")
			line := member.AgentName
			if member.RoleName != "" {
				line += " (" + member.RoleName + ")"
			}
			line += " is " + status
			if member.LastResult != "" {
				line += ": " + compactSummary(member.LastResult)
			}
			lines = append(lines, line)
		}
	}

	if len(lines) < 5 && snapshot.UnreadBroadcasts > 0 {
		lines = append(lines, fmt.Sprintf("%d broadcast update(s) are unread", snapshot.UnreadBroadcasts))
	}
	if len(lines) < 5 && snapshot.PendingHandoffs > 0 {
		lines = append(lines, fmt.Sprintf("%d handoff(s) need acknowledgement", snapshot.PendingHandoffs))
	}
	return lines
}

func describeOfficePulse(snapshot *team.Snapshot) string {
	if snapshot == nil || snapshot.Office == nil {
		return ""
	}
	continuity := emptyFallback(snapshot.Office.ContinuityStatus, "standing-by")
	return fmt.Sprintf(
		"%s office is %s: %d active, %d pending, %d committed",
		emptyFallback(snapshot.TeamName, emptyFallback(snapshot.Office.Constitution, "agency")),
		continuity,
		snapshot.RuntimeSummary.Active,
		snapshot.RuntimeSummary.Pending,
		snapshot.RuntimeSummary.Committed,
	)
}

func describeRuntimeRoster(snapshot *team.Snapshot) []string {
	if snapshot == nil || snapshot.Office == nil {
		return nil
	}

	rows := append([]team.OfficeRosterRow(nil), snapshot.Office.Roster...)
	sort.SliceStable(rows, func(i, j int) bool {
		score := func(row team.OfficeRosterRow) int {
			score := 0
			if row.Leader {
				score += 100
			}
			switch strings.ToLower(row.Status) {
			case "running", "active":
				score += 50
			case "queued", "pending":
				score += 35
			case "idle":
				score += 10
			}
			switch strings.ToLower(row.CommitmentState) {
			case "pending":
				score += 20
			case "committed":
				score += 10
			}
			if row.Present {
				score += 5
			}
			return score
		}
		left, right := score(rows[i]), score(rows[j])
		if left != right {
			return left > right
		}
		if rows[i].RoleName != rows[j].RoleName {
			return rows[i].RoleName < rows[j].RoleName
		}
		return rows[i].AgentName < rows[j].AgentName
	})

	lines := make([]string, 0, 3)
	for _, row := range rows {
		if len(lines) >= 3 {
			break
		}
		lines = append(lines, describeRosterRow(row))
	}
	return lines
}

func describeRosterRow(row team.OfficeRosterRow) string {
	name := emptyFallback(row.AgentName, displayRoleFallback(row.RoleName))
	status := emptyFallback(strings.ToLower(row.Status), emptyFallback(strings.ToLower(row.WakeState), "waiting"))
	line := fmt.Sprintf("%s is %s", name, status)
	qualifiers := make([]string, 0, 3)
	if row.RoleName != "" {
		qualifiers = append(qualifiers, row.RoleName)
	}
	if row.WorkspaceMode != "" {
		qualifiers = append(qualifiers, row.WorkspaceMode)
	}
	if row.CommitmentState != "" {
		qualifiers = append(qualifiers, row.CommitmentState)
	}
	if len(qualifiers) > 0 {
		line += " [" + strings.Join(qualifiers, " / ") + "]"
	}
	return line
}

func displayRoleFallback(value string) string {
	value = strings.TrimSpace(strings.NewReplacer("-", " ", "_", " ").Replace(value))
	if value == "" {
		return "office role"
	}
	return value
}

func compactSummary(value string) string {
	value = strings.TrimSpace(strings.ReplaceAll(value, "\n", " "))
	value = strings.Join(strings.Fields(value), " ")
	if value == "" {
		return ""
	}
	if len(value) > 44 {
		return value[:41] + "..."
	}
	return value
}

func emptyFallback(value, fallback string) string {
	if strings.TrimSpace(value) == "" {
		return fallback
	}
	return value
}
