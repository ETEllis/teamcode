package team

import (
	"context"
	"sort"
	"time"

	"github.com/ETEllis/teamcode/internal/orchestration"
)

type RuntimeRecord struct {
	RecordID        string   `json:"recordId"`
	TeamName        string   `json:"teamName,omitempty"`
	AgentName       string   `json:"agentName,omitempty"`
	RoleName        string   `json:"roleName,omitempty"`
	SessionID       string   `json:"sessionId,omitempty"`
	WorkerID        string   `json:"workerId,omitempty"`
	ParentWorkerID  string   `json:"parentWorkerId,omitempty"`
	RootWorkerID    string   `json:"rootWorkerId,omitempty"`
	ParentAgentName string   `json:"parentAgentName,omitempty"`
	Lineage         []string `json:"lineage,omitempty"`
	LineageDepth    int      `json:"lineageDepth,omitempty"`
	Kind            string   `json:"kind,omitempty"`
	Profile         string   `json:"profile,omitempty"`
	Status          string   `json:"status,omitempty"`
	WakeState       string   `json:"wakeState,omitempty"`
	StateScope      string   `json:"stateScope,omitempty"`
	CommitmentState string   `json:"commitmentState,omitempty"`
	WorkspaceMode   string   `json:"workspaceMode,omitempty"`
	Result          string   `json:"result,omitempty"`
	Error           string   `json:"error,omitempty"`
	Source          string   `json:"source,omitempty"`
	LastSignalAt    int64    `json:"lastSignalAt,omitempty"`
	LastHeartbeatAt int64    `json:"lastHeartbeatAt,omitempty"`
	StartedAt       int64    `json:"startedAt,omitempty"`
	CompletedAt     int64    `json:"completedAt,omitempty"`
	UpdatedAt       int64    `json:"updatedAt"`
	CreatedAt       int64    `json:"createdAt"`
}

type RuntimeSummary struct {
	Total             int            `json:"total"`
	Active            int            `json:"active"`
	Local             int            `json:"local"`
	Committed         int            `json:"committed"`
	Pending           int            `json:"pending"`
	MaxLineageDepth   int            `json:"maxLineageDepth"`
	ByStatus          map[string]int `json:"byStatus,omitempty"`
	ByWakeState       map[string]int `json:"byWakeState,omitempty"`
	ByCommitmentState map[string]int `json:"byCommitmentState,omitempty"`
	ByWorkspaceMode   map[string]int `json:"byWorkspaceMode,omitempty"`
}

type RuntimeService struct {
	store *store
}

func NewRuntimeService(sharedStore *store) *RuntimeService {
	return &RuntimeService{store: sharedStore}
}

func (s *RuntimeService) List(ctx context.Context, teamName string) ([]RuntimeRecord, error) {
	_ = ctx
	var records []RuntimeRecord
	if err := s.store.readJSON(teamName, "runtime_records.json", &records); err != nil {
		return nil, err
	}
	return records, nil
}

func (s *RuntimeService) Upsert(ctx context.Context, teamName string, record RuntimeRecord) (*RuntimeRecord, error) {
	_ = ctx
	records, err := s.List(context.Background(), teamName)
	if err != nil {
		return nil, err
	}

	now := time.Now().UnixMilli()
	record.TeamName = teamName
	record.UpdatedAt = now
	if record.CreatedAt == 0 {
		record.CreatedAt = now
	}
	if record.RecordID == "" {
		record.RecordID = runtimeRecordKey(record)
	}

	updated := false
	for i := range records {
		if records[i].RecordID != record.RecordID {
			continue
		}
		record.CreatedAt = records[i].CreatedAt
		records[i] = mergeRuntimeRecord(records[i], record)
		updated = true
		break
	}
	if !updated {
		records = append(records, record)
	}

	sortRuntimeRecords(records)
	if err := s.store.writeJSON(teamName, "runtime_records.json", records); err != nil {
		return nil, err
	}
	if updated {
		for i := range records {
			if records[i].RecordID == record.RecordID {
				copy := records[i]
				return &copy, nil
			}
		}
	}
	return &record, nil
}

func (s *RuntimeService) FindBySessionID(ctx context.Context, sessionID string) (string, *RuntimeRecord, error) {
	_ = ctx
	teamNames, err := s.store.listTeamNames()
	if err != nil {
		return "", nil, err
	}
	for _, teamName := range teamNames {
		records, err := s.List(context.Background(), teamName)
		if err != nil {
			return "", nil, err
		}
		for _, record := range records {
			if record.SessionID == sessionID {
				copy := record
				return teamName, &copy, nil
			}
		}
	}
	return "", nil, nil
}

func (s *RuntimeService) Materialize(ctx context.Context, teamName string, members []Member, workers []orchestration.Worker) ([]RuntimeRecord, RuntimeSummary, error) {
	projected, summary, err := s.Project(ctx, teamName, members, workers)
	if err != nil {
		return nil, RuntimeSummary{}, err
	}
	if err := s.store.writeJSON(teamName, "runtime_records.json", projected); err != nil {
		return nil, RuntimeSummary{}, err
	}
	return projected, summary, nil
}

func (s *RuntimeService) Project(ctx context.Context, teamName string, members []Member, workers []orchestration.Worker) ([]RuntimeRecord, RuntimeSummary, error) {
	_ = ctx
	records, err := s.List(context.Background(), teamName)
	if err != nil {
		return nil, RuntimeSummary{}, err
	}

	recordMap := make(map[string]RuntimeRecord, len(records)+len(members)+len(workers))
	for _, record := range records {
		if record.RecordID == "" {
			record.RecordID = runtimeRecordKey(record)
		}
		recordMap[record.RecordID] = record
	}

	membersBySession := make(map[string]Member, len(members))
	membersByName := make(map[string]Member, len(members))
	for _, member := range members {
		if member.AgentName != "" {
			membersByName[member.AgentName] = member
		}
		if member.SessionID != "" {
			membersBySession[member.SessionID] = member
		}
		record := runtimeRecordFromMember(teamName, member)
		key := runtimeRecordKey(record)
		record.RecordID = key
		if existing, ok := recordMap[key]; ok {
			recordMap[key] = mergeRuntimeRecord(existing, record)
			continue
		}
		recordMap[key] = record
	}

	for _, worker := range workers {
		member, ok := membersBySession[worker.SessionID]
		if !ok && worker.Name != "" {
			member, ok = membersByName[worker.Name]
		}
		record := runtimeRecordFromWorker(teamName, worker, ok, member)
		key := runtimeRecordKey(record)
		record.RecordID = key
		if existing, exists := recordMap[key]; exists {
			recordMap[key] = mergeRuntimeRecord(existing, record)
			continue
		}
		recordMap[key] = record
	}

	projected := make([]RuntimeRecord, 0, len(recordMap))
	for _, record := range recordMap {
		if record.RecordID == "" {
			record.RecordID = runtimeRecordKey(record)
		}
		record.TeamName = teamName
		projected = append(projected, record)
	}
	sortRuntimeRecords(projected)
	return projected, summarizeRuntimeRecords(projected), nil
}

func runtimeRecordFromMember(teamName string, member Member) RuntimeRecord {
	return RuntimeRecord{
		TeamName:        teamName,
		AgentName:       member.AgentName,
		RoleName:        member.RoleName,
		SessionID:       member.SessionID,
		WorkerID:        member.WorkerID,
		ParentWorkerID:  member.ParentWorkerID,
		RootWorkerID:    member.RootWorkerID,
		ParentAgentName: member.ReportsTo,
		Lineage:         append([]string{}, member.Lineage...),
		LineageDepth:    member.LineageDepth,
		Kind:            member.Kind,
		Profile:         member.Profile,
		Status:          member.Status,
		WakeState:       member.WakeState,
		StateScope:      member.StateScope,
		CommitmentState: member.CommitmentState,
		WorkspaceMode:   member.WorkspaceMode,
		Result:          member.LastResult,
		Source:          "member",
		LastSignalAt:    member.LastSignalAt,
		LastHeartbeatAt: member.LastHeartbeatAt,
		UpdatedAt:       member.UpdatedAt,
		CreatedAt:       member.CreatedAt,
	}
}

func runtimeRecordFromWorker(teamName string, worker orchestration.Worker, hasMember bool, member Member) RuntimeRecord {
	record := RuntimeRecord{
		TeamName:        teamName,
		AgentName:       worker.Name,
		RoleName:        worker.RoleName,
		SessionID:       worker.SessionID,
		WorkerID:        worker.ID,
		ParentWorkerID:  worker.ParentWorkerID,
		RootWorkerID:    worker.RootWorkerID,
		Lineage:         append([]string{}, worker.Lineage...),
		LineageDepth:    worker.LineageDepth,
		Kind:            string(worker.Kind),
		Profile:         worker.Profile,
		Status:          string(worker.Status),
		WakeState:       string(worker.WakeState),
		StateScope:      string(worker.StateScope),
		CommitmentState: string(worker.CommitmentState),
		WorkspaceMode:   worker.WorkspaceMode,
		Result:          worker.Result,
		Error:           worker.Error,
		Source:          "worker",
		LastSignalAt:    worker.LastSignalAt,
		LastHeartbeatAt: worker.LastHeartbeatAt,
		StartedAt:       worker.StartedAt,
		CompletedAt:     worker.CompletedAt,
		UpdatedAt:       worker.UpdatedAt,
		CreatedAt:       worker.CreatedAt,
	}
	if hasMember {
		record.ParentAgentName = member.ReportsTo
		if record.AgentName == "" {
			record.AgentName = member.AgentName
		}
		if record.RoleName == "" {
			record.RoleName = member.RoleName
		}
		if record.Profile == "" {
			record.Profile = member.Profile
		}
		if record.WorkspaceMode == "" {
			record.WorkspaceMode = member.WorkspaceMode
		}
	}
	if record.RootWorkerID == "" {
		record.RootWorkerID = record.WorkerID
	}
	if len(record.Lineage) == 0 && record.WorkerID != "" {
		record.Lineage = []string{record.WorkerID}
	}
	if record.StateScope == "" {
		record.StateScope = "local"
	}
	if record.CommitmentState == "" {
		record.CommitmentState = "local"
	}
	if record.WorkspaceMode == "" {
		record.WorkspaceMode = "shared"
	}
	if record.WakeState == "" {
		record.WakeState = deriveWakeState(record.Status)
	}
	return record
}

func mergeRuntimeRecord(base, override RuntimeRecord) RuntimeRecord {
	merged := base
	if override.TeamName != "" {
		merged.TeamName = override.TeamName
	}
	if override.AgentName != "" {
		merged.AgentName = override.AgentName
	}
	if override.RoleName != "" {
		merged.RoleName = override.RoleName
	}
	if override.SessionID != "" {
		merged.SessionID = override.SessionID
	}
	if override.WorkerID != "" {
		merged.WorkerID = override.WorkerID
	}
	if override.ParentWorkerID != "" {
		merged.ParentWorkerID = override.ParentWorkerID
	}
	if override.RootWorkerID != "" {
		merged.RootWorkerID = override.RootWorkerID
	}
	if override.ParentAgentName != "" {
		merged.ParentAgentName = override.ParentAgentName
	}
	if len(override.Lineage) > 0 {
		merged.Lineage = append([]string{}, override.Lineage...)
	}
	if override.LineageDepth > merged.LineageDepth {
		merged.LineageDepth = override.LineageDepth
	}
	if override.Kind != "" {
		merged.Kind = override.Kind
	}
	if override.Profile != "" {
		merged.Profile = override.Profile
	}
	if override.Status != "" {
		merged.Status = override.Status
	}
	if override.WakeState != "" {
		merged.WakeState = override.WakeState
	}
	if override.StateScope != "" {
		merged.StateScope = override.StateScope
	}
	if override.CommitmentState != "" {
		merged.CommitmentState = override.CommitmentState
	}
	if override.WorkspaceMode != "" {
		merged.WorkspaceMode = override.WorkspaceMode
	}
	if override.Result != "" {
		merged.Result = override.Result
	}
	if override.Error != "" {
		merged.Error = override.Error
	}
	if override.Source != "" {
		merged.Source = override.Source
	}
	if override.LastSignalAt > 0 {
		merged.LastSignalAt = override.LastSignalAt
	}
	if override.LastHeartbeatAt > 0 {
		merged.LastHeartbeatAt = override.LastHeartbeatAt
	}
	if override.StartedAt > 0 {
		merged.StartedAt = override.StartedAt
	}
	if override.CompletedAt > 0 {
		merged.CompletedAt = override.CompletedAt
	}
	if override.RecordID != "" {
		merged.RecordID = override.RecordID
	}
	if override.CreatedAt > 0 {
		merged.CreatedAt = minNonZero(merged.CreatedAt, override.CreatedAt)
	}
	if override.UpdatedAt > 0 {
		merged.UpdatedAt = override.UpdatedAt
	}
	return merged
}

func summarizeRuntimeRecords(records []RuntimeRecord) RuntimeSummary {
	summary := RuntimeSummary{
		Total:             len(records),
		ByStatus:          map[string]int{},
		ByWakeState:       map[string]int{},
		ByCommitmentState: map[string]int{},
		ByWorkspaceMode:   map[string]int{},
	}
	for _, record := range records {
		if record.Status != "" {
			summary.ByStatus[record.Status]++
			if record.Status == string(orchestration.WorkerStatusQueued) || record.Status == string(orchestration.WorkerStatusRunning) {
				summary.Active++
			}
		}
		if record.WakeState != "" {
			summary.ByWakeState[record.WakeState]++
		}
		switch record.CommitmentState {
		case "committed":
			summary.Committed++
		case "pending":
			summary.Pending++
		default:
			summary.Local++
		}
		if record.CommitmentState != "" {
			summary.ByCommitmentState[record.CommitmentState]++
		}
		if record.WorkspaceMode != "" {
			summary.ByWorkspaceMode[record.WorkspaceMode]++
		}
		if record.LineageDepth > summary.MaxLineageDepth {
			summary.MaxLineageDepth = record.LineageDepth
		}
	}
	return summary
}

func runtimeRecordKey(record RuntimeRecord) string {
	switch {
	case record.WorkerID != "":
		return "worker:" + record.WorkerID
	case record.SessionID != "":
		return "session:" + record.SessionID
	case record.AgentName != "":
		return "agent:" + normalizeName(record.AgentName)
	default:
		return "record:unknown"
	}
}

func sortRuntimeRecords(records []RuntimeRecord) {
	sort.Slice(records, func(i, j int) bool {
		if records[i].AgentName != records[j].AgentName {
			return records[i].AgentName < records[j].AgentName
		}
		if records[i].LineageDepth != records[j].LineageDepth {
			return records[i].LineageDepth < records[j].LineageDepth
		}
		if records[i].WorkerID != records[j].WorkerID {
			return records[i].WorkerID < records[j].WorkerID
		}
		return records[i].SessionID < records[j].SessionID
	})
}

func deriveWakeState(status string) string {
	switch status {
	case string(orchestration.WorkerStatusRunning):
		return string(orchestration.WorkerWakeStateActive)
	case string(orchestration.WorkerStatusQueued):
		return string(orchestration.WorkerWakeStateWaiting)
	default:
		return string(orchestration.WorkerWakeStateQuiescent)
	}
}

func minNonZero(values ...int64) int64 {
	var min int64
	for _, value := range values {
		if value == 0 {
			continue
		}
		if min == 0 || value < min {
			min = value
		}
	}
	return min
}
