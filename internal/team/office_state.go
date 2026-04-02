package team

import (
	"context"
	"sort"
	"time"
)

type OfficeState struct {
	TeamName         string            `json:"teamName"`
	Leader           string            `json:"leader,omitempty"`
	Constitution     string            `json:"constitution,omitempty"`
	RuntimeMode      string            `json:"runtimeMode,omitempty"`
	SharedTruth      string            `json:"sharedTruth,omitempty"`
	WorkspaceMode    string            `json:"workspaceMode,omitempty"`
	PublicationMode  string            `json:"publicationMode,omitempty"`
	ConsensusMode    string            `json:"consensusMode,omitempty"`
	WakePolicy       string            `json:"wakePolicy,omitempty"`
	MaterializedAt   int64             `json:"materializedAt"`
	LastActiveAt     int64             `json:"lastActiveAt,omitempty"`
	LastWakeAt       int64             `json:"lastWakeAt,omitempty"`
	Roster           []OfficeRosterRow `json:"roster,omitempty"`
	RuntimeSummary   RuntimeSummary    `json:"runtimeSummary"`
	ContinuityStatus string            `json:"continuityStatus,omitempty"`
}

type OfficeRosterRow struct {
	AgentName        string `json:"agentName,omitempty"`
	RoleName         string `json:"roleName,omitempty"`
	Responsible      string `json:"responsible,omitempty"`
	SessionID        string `json:"sessionId,omitempty"`
	WorkerID         string `json:"workerId,omitempty"`
	ReportsTo        string `json:"reportsTo,omitempty"`
	Leader           bool   `json:"leader,omitempty"`
	Kind             string `json:"kind,omitempty"`
	Profile          string `json:"profile,omitempty"`
	Status           string `json:"status,omitempty"`
	WakeState        string `json:"wakeState,omitempty"`
	StateScope       string `json:"stateScope,omitempty"`
	CommitmentState  string `json:"commitmentState,omitempty"`
	WorkspaceMode    string `json:"workspaceMode,omitempty"`
	Present          bool   `json:"present,omitempty"`
	LastActiveAt     int64  `json:"lastActiveAt,omitempty"`
	LastHeartbeatAt  int64  `json:"lastHeartbeatAt,omitempty"`
	LineageDepth     int    `json:"lineageDepth,omitempty"`
	CanSpawnChildren bool   `json:"canSpawnChildren,omitempty"`
}

type OfficeService struct {
	store *store
}

func NewOfficeService(sharedStore *store) *OfficeService {
	return &OfficeService{store: sharedStore}
}

func (s *OfficeService) Read(ctx context.Context, teamName string) (*OfficeState, error) {
	_ = ctx
	var office OfficeState
	if err := s.store.readJSON(teamName, "office_state.json", &office); err != nil {
		return nil, err
	}
	if office.TeamName == "" {
		return nil, nil
	}
	return &office, nil
}

func (s *OfficeService) Materialize(ctx context.Context, teamName string, tc *TeamContext, members []Member, runtimeRecords []RuntimeRecord, summary RuntimeSummary) (*OfficeState, error) {
	_ = ctx
	office := buildOfficeState(teamName, tc, members, runtimeRecords, summary)
	if err := s.store.writeJSON(teamName, "office_state.json", office); err != nil {
		return nil, err
	}
	return office, nil
}

func buildOfficeState(teamName string, tc *TeamContext, members []Member, runtimeRecords []RuntimeRecord, summary RuntimeSummary) *OfficeState {
	office := &OfficeState{
		TeamName:       teamName,
		RuntimeSummary: summary,
		MaterializedAt: time.Now().UnixMilli(),
	}
	if tc != nil {
		office.Leader = tc.Leader
		office.Constitution = tc.Constitution
		office.RuntimeMode = tc.RuntimeMode
		office.SharedTruth = tc.SharedTruth
		office.WorkspaceMode = tc.WorkingAgreement.WorkspaceMode
		office.PublicationMode = tc.WorkingAgreement.PublicationMode
		office.ConsensusMode = tc.WorkingAgreement.ConsensusMode
		office.WakePolicy = tc.WorkingAgreement.WakePolicy
	}

	roster := make(map[string]OfficeRosterRow)
	rosterKey := func(agentName, roleName string) string {
		if agentName != "" {
			return "agent:" + normalizeName(agentName)
		}
		if roleName != "" {
			return "role:" + normalizeName(roleName)
		}
		return "office:unknown"
	}

	if tc != nil {
		for roleName, role := range tc.Roles {
			row := OfficeRosterRow{
				AgentName:        role.Agent,
				RoleName:         roleName,
				Responsible:      role.Responsible,
				ReportsTo:        role.ReportsTo,
				Profile:          role.Profile,
				CanSpawnChildren: role.CanSpawnSubagents,
			}
			roster[rosterKey(role.Agent, roleName)] = row
		}
	}

	for _, member := range members {
		key := rosterKey(member.AgentName, member.RoleName)
		row := roster[key]
		row.AgentName = coalesceString(member.AgentName, row.AgentName)
		row.RoleName = coalesceString(member.RoleName, row.RoleName)
		row.SessionID = coalesceString(member.SessionID, row.SessionID)
		row.WorkerID = coalesceString(member.WorkerID, row.WorkerID)
		row.ReportsTo = coalesceString(member.ReportsTo, row.ReportsTo)
		row.Kind = coalesceString(member.Kind, row.Kind)
		row.Profile = coalesceString(member.Profile, row.Profile)
		row.Status = coalesceString(member.Status, row.Status)
		row.WakeState = coalesceString(member.WakeState, row.WakeState)
		row.StateScope = coalesceString(member.StateScope, row.StateScope)
		row.CommitmentState = coalesceString(member.CommitmentState, row.CommitmentState)
		row.WorkspaceMode = coalesceString(member.WorkspaceMode, row.WorkspaceMode)
		row.Leader = row.Leader || member.Leader
		row.Present = row.Present || member.SessionID != ""
		row.LastHeartbeatAt = maxInt64(row.LastHeartbeatAt, member.LastHeartbeatAt)
		row.LastActiveAt = maxInt64(row.LastActiveAt, member.LastSignalAt, member.LastHeartbeatAt, member.UpdatedAt)
		if member.LineageDepth > row.LineageDepth {
			row.LineageDepth = member.LineageDepth
		}
		row.CanSpawnChildren = row.CanSpawnChildren || member.CanSpawnSubagents
		roster[key] = row
	}

	for _, record := range runtimeRecords {
		key := rosterKey(record.AgentName, record.RoleName)
		row := roster[key]
		row.AgentName = coalesceString(record.AgentName, row.AgentName)
		row.RoleName = coalesceString(record.RoleName, row.RoleName)
		row.SessionID = coalesceString(record.SessionID, row.SessionID)
		row.WorkerID = coalesceString(record.WorkerID, row.WorkerID)
		row.Kind = coalesceString(record.Kind, row.Kind)
		row.Profile = coalesceString(record.Profile, row.Profile)
		row.Status = coalesceString(record.Status, row.Status)
		row.WakeState = coalesceString(record.WakeState, row.WakeState)
		row.StateScope = coalesceString(record.StateScope, row.StateScope)
		row.CommitmentState = coalesceString(record.CommitmentState, row.CommitmentState)
		row.WorkspaceMode = coalesceString(record.WorkspaceMode, row.WorkspaceMode)
		row.Present = row.Present || record.SessionID != "" || record.WorkerID != ""
		row.LastHeartbeatAt = maxInt64(row.LastHeartbeatAt, record.LastHeartbeatAt)
		row.LastActiveAt = maxInt64(row.LastActiveAt, record.LastSignalAt, record.LastHeartbeatAt, record.StartedAt, record.UpdatedAt, record.CompletedAt)
		if record.LineageDepth > row.LineageDepth {
			row.LineageDepth = record.LineageDepth
		}
		roster[key] = row
	}

	office.Roster = make([]OfficeRosterRow, 0, len(roster))
	for _, row := range roster {
		office.Roster = append(office.Roster, row)
		office.LastActiveAt = maxInt64(office.LastActiveAt, row.LastActiveAt)
		office.LastWakeAt = maxInt64(office.LastWakeAt, row.LastHeartbeatAt)
	}
	sort.Slice(office.Roster, func(i, j int) bool {
		if office.Roster[i].Leader != office.Roster[j].Leader {
			return office.Roster[i].Leader
		}
		if office.Roster[i].RoleName != office.Roster[j].RoleName {
			return office.Roster[i].RoleName < office.Roster[j].RoleName
		}
		return office.Roster[i].AgentName < office.Roster[j].AgentName
	})

	switch {
	case summary.Active > 0:
		office.ContinuityStatus = "active"
	case len(office.Roster) > 0:
		office.ContinuityStatus = "standing-by"
	default:
		office.ContinuityStatus = "empty"
	}

	return office
}

func coalesceString(values ...string) string {
	for _, value := range values {
		if value != "" {
			return value
		}
	}
	return ""
}

func maxInt64(base int64, values ...int64) int64 {
	maxValue := base
	for _, value := range values {
		if value > maxValue {
			maxValue = value
		}
	}
	return maxValue
}
