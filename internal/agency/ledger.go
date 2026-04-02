package agency

import (
	"bufio"
	"context"
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
	"sync"
	"time"

	"github.com/google/uuid"
)

type LedgerService struct {
	mu          sync.Mutex
	baseDir     string
	entriesPath string
	pendingPath string
}

func NewLedgerService(baseDir string) (*LedgerService, error) {
	if baseDir == "" {
		return nil, fmt.Errorf("ledger base directory is required")
	}
	if err := os.MkdirAll(baseDir, 0o755); err != nil {
		return nil, err
	}

	entriesPath := filepath.Join(baseDir, "entries.jsonl")
	if _, err := os.Stat(entriesPath); os.IsNotExist(err) {
		if err := os.WriteFile(entriesPath, nil, 0o644); err != nil {
			return nil, err
		}
	}

	pendingPath := filepath.Join(baseDir, "pending.json")
	if _, err := os.Stat(pendingPath); os.IsNotExist(err) {
		if err := os.WriteFile(pendingPath, []byte("[]"), 0o644); err != nil {
			return nil, err
		}
	}

	return &LedgerService{
		baseDir:     baseDir,
		entriesPath: entriesPath,
		pendingPath: pendingPath,
	}, nil
}

func (l *LedgerService) Append(ctx context.Context, entry LedgerEntry) (CommitCertificate, error) {
	_ = ctx

	l.mu.Lock()
	defer l.mu.Unlock()

	entries, err := l.replayLocked()
	if err != nil {
		return CommitCertificate{}, err
	}
	return l.appendLocked(int64(len(entries)+1), entry)
}

func (l *LedgerService) appendLocked(sequence int64, entry LedgerEntry) (CommitCertificate, error) {
	now := time.Now().UnixMilli()
	entry.Sequence = sequence
	if entry.ID == "" {
		entry.ID = uuid.NewString()
	}
	if entry.ProposedAt == 0 {
		entry.ProposedAt = now
	}
	if entry.CommittedAt == 0 {
		entry.CommittedAt = now
	}
	if entry.Status == "" {
		entry.Status = deriveEntryStatus(entry)
	}

	cert := CommitCertificate{
		EntryID:     entry.ID,
		Sequence:    entry.Sequence,
		Hash:        entryHash(entry),
		CommittedAt: entry.CommittedAt,
		QuorumSize:  len(entry.Votes),
		Status:      entry.Status,
		Approvals:   approvalCount(entry.Votes),
		Rejections:  rejectionCount(entry.Votes),
	}
	entry.Certificate = &cert

	file, err := os.OpenFile(l.entriesPath, os.O_APPEND|os.O_WRONLY, 0o644)
	if err != nil {
		return CommitCertificate{}, err
	}
	defer file.Close()

	data, err := json.Marshal(entry)
	if err != nil {
		return CommitCertificate{}, err
	}
	if _, err := file.Write(append(data, '\n')); err != nil {
		return CommitCertificate{}, err
	}
	return cert, nil
}

func (l *LedgerService) AppendSignal(ctx context.Context, signal WakeSignal) (CommitCertificate, error) {
	return l.Append(ctx, LedgerEntry{
		ID:             signal.ID,
		OrganizationID: signal.OrganizationID,
		Kind:           LedgerEntrySignal,
		ActorID:        signal.ActorID,
		Signal:         &signal,
		CommittedAt:    signal.CreatedAt,
	})
}

func (l *LedgerService) AppendSchedule(ctx context.Context, schedule AgentSchedule, signal WakeSignal) (CommitCertificate, error) {
	snapshot, err := l.LatestSnapshot(ctx, signal.OrganizationID)
	if err != nil {
		return CommitCertificate{}, err
	}
	snapshot.OpenSchedules = upsertSchedule(snapshot.OpenSchedules, schedule)
	snapshot.LastSignal = &signal
	snapshot.UpdatedAt = maxInt64(snapshot.UpdatedAt, signal.CreatedAt)

	return l.Append(ctx, LedgerEntry{
		ID:             uuid.NewString(),
		OrganizationID: signal.OrganizationID,
		Kind:           LedgerEntrySchedule,
		ActorID:        schedule.ActorID,
		Signal:         &signal,
		Snapshot:       snapshot,
		CommittedAt:    maxInt64(signal.CreatedAt, time.Now().UnixMilli()),
	})
}

func (l *LedgerService) AppendSnapshot(ctx context.Context, snapshot ContextSnapshot) (CommitCertificate, error) {
	return l.Append(ctx, LedgerEntry{
		ID:             uuid.NewString(),
		OrganizationID: snapshot.OrganizationID,
		Kind:           LedgerEntrySnapshot,
		Snapshot:       &snapshot,
		CommittedAt:    maxInt64(snapshot.UpdatedAt, time.Now().UnixMilli()),
	})
}

func (l *LedgerService) AppendVoiceEvent(ctx context.Context, event VoiceEvent) (CommitCertificate, error) {
	snapshot, err := l.LatestSnapshot(ctx, event.OrganizationID)
	if err != nil {
		return CommitCertificate{}, err
	}
	applyVoiceEvent(snapshot, event)
	return l.Append(ctx, LedgerEntry{
		ID:             event.ID,
		OrganizationID: event.OrganizationID,
		Kind:           LedgerEntryVoice,
		ActorID:        event.ActorID,
		Voice:          &event,
		Snapshot:       snapshot,
		CommittedAt:    maxInt64(event.CreatedAt, time.Now().UnixMilli()),
	})
}

func (l *LedgerService) Replay(ctx context.Context) ([]LedgerEntry, error) {
	_ = ctx

	l.mu.Lock()
	defer l.mu.Unlock()
	return l.replayLocked()
}

func (l *LedgerService) Propose(ctx context.Context, entry LedgerEntry) (LedgerEntry, error) {
	_ = ctx

	l.mu.Lock()
	defer l.mu.Unlock()

	pending, err := l.readPendingLocked()
	if err != nil {
		return LedgerEntry{}, err
	}

	if entry.ID == "" {
		entry.ID = uuid.NewString()
	}
	if entry.ProposedAt == 0 {
		entry.ProposedAt = time.Now().UnixMilli()
	}
	entry.Status = LedgerEntryStatusProposed
	if entry.Consensus != nil {
		entry.Quorum = &QuorumState{
			QuorumKey:         entry.Consensus.QuorumKey,
			RequiredApprovals: entry.Consensus.RequiredApprovals,
			EligibleVoters:    append([]string(nil), entry.Consensus.EligibleVoters...),
			MissingApprovals:  entry.Consensus.RequiredApprovals,
		}
	}

	pending = append(pending, entry)
	if err := l.writePendingLocked(pending); err != nil {
		return LedgerEntry{}, err
	}
	return entry, nil
}

func (l *LedgerService) Pending(ctx context.Context, organizationID string) ([]LedgerEntry, error) {
	_ = ctx

	l.mu.Lock()
	defer l.mu.Unlock()

	pending, err := l.readPendingLocked()
	if err != nil {
		return nil, err
	}
	if organizationID == "" {
		return pending, nil
	}
	filtered := make([]LedgerEntry, 0, len(pending))
	for _, entry := range pending {
		if entry.OrganizationID == organizationID {
			filtered = append(filtered, entry)
		}
	}
	return filtered, nil
}

func (l *LedgerService) Vote(ctx context.Context, entryID string, vote ConsensusVote) (QuorumState, error) {
	_ = ctx

	l.mu.Lock()
	defer l.mu.Unlock()

	pending, err := l.readPendingLocked()
	if err != nil {
		return QuorumState{}, err
	}
	for i := range pending {
		if pending[i].ID != entryID {
			continue
		}
		for _, existing := range pending[i].Votes {
			if existing.VoterID == vote.VoterID {
				return QuorumState{}, fmt.Errorf("duplicate vote from %s", vote.VoterID)
			}
		}
		if vote.VotedAt == 0 {
			vote.VotedAt = time.Now().UnixMilli()
		}
		vote.EntryID = entryID
		pending[i].Votes = append(pending[i].Votes, vote)
		quorum := buildQuorumState(pending[i])
		pending[i].Quorum = &quorum
		if pending[i].Consensus != nil && pending[i].Consensus.AutoFinalize && quorum.Finalizable {
			if _, err := l.finalizePendingEntryLocked(&pending[i]); err != nil {
				return QuorumState{}, err
			}
			pending = append(pending[:i], pending[i+1:]...)
		}
		if err := l.writePendingLocked(pending); err != nil {
			return QuorumState{}, err
		}
		return quorum, nil
	}
	return QuorumState{}, fmt.Errorf("pending entry %q not found", entryID)
}

func (l *LedgerService) Finalize(ctx context.Context, entryID string) (CommitCertificate, error) {
	_ = ctx

	l.mu.Lock()
	defer l.mu.Unlock()

	pending, err := l.readPendingLocked()
	if err != nil {
		return CommitCertificate{}, err
	}
	for i := range pending {
		if pending[i].ID != entryID {
			continue
		}
		quorum := buildQuorumState(pending[i])
		pending[i].Quorum = &quorum
		if !quorum.Finalizable {
			return CommitCertificate{}, fmt.Errorf("entry %q is not finalizable", entryID)
		}
		cert, err := l.finalizePendingEntryLocked(&pending[i])
		if err != nil {
			return CommitCertificate{}, err
		}
		pending = append(pending[:i], pending[i+1:]...)
		if err := l.writePendingLocked(pending); err != nil {
			return CommitCertificate{}, err
		}
		return cert, nil
	}
	return CommitCertificate{}, fmt.Errorf("pending entry %q not found", entryID)
}

func (l *LedgerService) LatestSnapshot(ctx context.Context, organizationID string) (*ContextSnapshot, error) {
	_ = ctx

	l.mu.Lock()
	defer l.mu.Unlock()

	entries, err := l.replayLocked()
	if err != nil {
		return nil, err
	}
	snapshot := &ContextSnapshot{
		OrganizationID: organizationID,
		Metadata:       map[string]string{},
	}
	for _, entry := range entries {
		if entry.OrganizationID != organizationID {
			continue
		}
		applyLedgerEntry(snapshot, entry)
		snapshot.LedgerSequence = entry.Sequence
	}
	if snapshot.UpdatedAt == 0 {
		snapshot.UpdatedAt = time.Now().UnixMilli()
	}
	return snapshot, nil
}

func (l *LedgerService) replayLocked() ([]LedgerEntry, error) {
	file, err := os.Open(l.entriesPath)
	if err != nil {
		return nil, err
	}
	defer file.Close()

	entries := []LedgerEntry{}
	scanner := bufio.NewScanner(file)
	for scanner.Scan() {
		line := scanner.Bytes()
		if len(line) == 0 {
			continue
		}
		var entry LedgerEntry
		if err := json.Unmarshal(line, &entry); err != nil {
			return nil, err
		}
		entries = append(entries, entry)
	}
	if err := scanner.Err(); err != nil {
		return nil, err
	}
	return entries, nil
}

func (l *LedgerService) readPendingLocked() ([]LedgerEntry, error) {
	data, err := os.ReadFile(l.pendingPath)
	if err != nil {
		return nil, err
	}
	if len(data) == 0 {
		return []LedgerEntry{}, nil
	}
	var pending []LedgerEntry
	if err := json.Unmarshal(data, &pending); err != nil {
		return nil, err
	}
	return pending, nil
}

func (l *LedgerService) writePendingLocked(entries []LedgerEntry) error {
	data, err := json.MarshalIndent(entries, "", "  ")
	if err != nil {
		return err
	}
	return os.WriteFile(l.pendingPath, data, 0o644)
}

func applyLedgerEntry(snapshot *ContextSnapshot, entry LedgerEntry) {
	if entry.Snapshot != nil {
		mergeSnapshot(snapshot, entry.Snapshot)
	}
	if entry.Signal != nil {
		sig := *entry.Signal
		snapshot.LastSignal = &sig
	}
	if entry.Publication != nil {
		snapshot.Publications = upsertPublication(snapshot.Publications, *entry.Publication)
	}
	if entry.Voice != nil {
		voice := *entry.Voice
		snapshot.LastVoiceEvent = &voice
		applyVoiceEvent(snapshot, voice)
	}
	if snapshot.UpdatedAt < entry.CommittedAt {
		snapshot.UpdatedAt = entry.CommittedAt
	}
}

func mergeSnapshot(dst, src *ContextSnapshot) {
	if dst == nil || src == nil {
		return
	}
	if src.OrganizationID != "" {
		dst.OrganizationID = src.OrganizationID
	}
	if len(src.Actors) > 0 {
		dst.Actors = append([]AgentIdentity(nil), src.Actors...)
	}
	if len(src.Publications) > 0 {
		dst.Publications = append([]PublicationRecord(nil), src.Publications...)
	}
	if src.LastSignal != nil {
		sig := *src.LastSignal
		dst.LastSignal = &sig
	}
	if len(src.VoiceRooms) > 0 {
		dst.VoiceRooms = append([]VoiceRoomState(nil), src.VoiceRooms...)
	}
	if src.LastVoiceEvent != nil {
		voice := *src.LastVoiceEvent
		dst.LastVoiceEvent = &voice
	}
	if len(src.OpenSchedules) > 0 {
		dst.OpenSchedules = append([]AgentSchedule(nil), src.OpenSchedules...)
	}
	if src.UpdatedAt > 0 {
		dst.UpdatedAt = src.UpdatedAt
	}
	if len(src.Metadata) > 0 {
		if dst.Metadata == nil {
			dst.Metadata = map[string]string{}
		}
		for key, value := range src.Metadata {
			dst.Metadata[key] = value
		}
	}
}

func applyVoiceEvent(snapshot *ContextSnapshot, event VoiceEvent) {
	if snapshot == nil {
		return
	}
	roomID := event.RoomID
	if roomID == "" && event.Projection != nil {
		roomID = event.Projection.RoomID
	}
	room := VoiceRoomState{
		RoomID:    roomID,
		UpdatedAt: event.CreatedAt,
	}
	if roomID != "" {
		for _, existing := range snapshot.VoiceRooms {
			if existing.RoomID == roomID {
				room = existing
				break
			}
		}
	}
	if event.Projection != nil {
		room = *event.Projection
	}
	switch event.Kind {
	case VoiceEventTranscript:
		room.LastTranscriptID = event.ID
		room.UpdatedAt = event.CreatedAt
	case VoiceEventSynthesis:
		room.LastSynthesisID = event.ID
		room.UpdatedAt = event.CreatedAt
	case VoiceEventProjection:
		if room.UpdatedAt == 0 {
			room.UpdatedAt = event.CreatedAt
		}
	}
	if room.RoomID != "" {
		snapshot.VoiceRooms = upsertVoiceRoom(snapshot.VoiceRooms, room)
	}
	voice := event
	snapshot.LastVoiceEvent = &voice
}

func upsertPublication(publications []PublicationRecord, publication PublicationRecord) []PublicationRecord {
	for i := range publications {
		if publications[i].ID == publication.ID {
			publications[i] = publication
			return publications
		}
	}
	return append(publications, publication)
}

func upsertVoiceRoom(rooms []VoiceRoomState, room VoiceRoomState) []VoiceRoomState {
	for i := range rooms {
		if rooms[i].RoomID == room.RoomID {
			rooms[i] = room
			return rooms
		}
	}
	return append(rooms, room)
}

func upsertSchedule(schedules []AgentSchedule, schedule AgentSchedule) []AgentSchedule {
	for i := range schedules {
		if schedules[i].ID == schedule.ID {
			schedules[i] = schedule
			return schedules
		}
	}
	return append(schedules, schedule)
}

func deriveEntryStatus(entry LedgerEntry) LedgerEntryStatus {
	if entry.Decision != nil && !entry.Decision.Accepted {
		return LedgerEntryStatusRejected
	}
	if entry.Consensus != nil && entry.Consensus.Strategy == ConsensusStrategyQuorum {
		return LedgerEntryStatusPending
	}
	return LedgerEntryStatusFinalized
}

func buildQuorumState(entry LedgerEntry) QuorumState {
	quorum := QuorumState{}
	if entry.Quorum != nil {
		quorum = *entry.Quorum
	}
	if entry.Consensus != nil {
		quorum.QuorumKey = entry.Consensus.QuorumKey
		quorum.RequiredApprovals = entry.Consensus.RequiredApprovals
		quorum.EligibleVoters = append([]string(nil), entry.Consensus.EligibleVoters...)
	}
	quorum.Approvals = approvalCount(entry.Votes)
	quorum.Rejections = rejectionCount(entry.Votes)
	quorum.MissingApprovals = quorum.RequiredApprovals - quorum.Approvals
	if quorum.MissingApprovals < 0 {
		quorum.MissingApprovals = 0
	}
	if len(entry.Votes) > 0 {
		quorum.LastVoteAt = entry.Votes[len(entry.Votes)-1].VotedAt
	}
	remainingVoters := len(quorum.EligibleVoters) - len(entry.Votes)
	if remainingVoters < 0 {
		remainingVoters = 0
	}
	quorum.Rejected = quorum.Rejections > 0 && quorum.Approvals+remainingVoters < quorum.RequiredApprovals
	quorum.Finalizable = quorum.Approvals >= quorum.RequiredApprovals || quorum.Rejected
	quorum.Finalized = entry.Status == LedgerEntryStatusFinalized || entry.Status == LedgerEntryStatusRejected
	return quorum
}

func (l *LedgerService) finalizePendingEntryLocked(entry *LedgerEntry) (CommitCertificate, error) {
	if entry == nil {
		return CommitCertificate{}, fmt.Errorf("pending entry is nil")
	}
	quorum := buildQuorumState(*entry)
	entry.Quorum = &quorum
	entry.FinalizedAt = time.Now().UnixMilli()
	if quorum.Rejected {
		entry.Status = LedgerEntryStatusRejected
		entry.RejectedAt = entry.FinalizedAt
		return CommitCertificate{
			EntryID:     entry.ID,
			Hash:        entryHash(*entry),
			CommittedAt: entry.FinalizedAt,
			QuorumSize:  len(entry.Votes),
			Status:      LedgerEntryStatusRejected,
			Approvals:   quorum.Approvals,
			Rejections:  quorum.Rejections,
		}, nil
	}
	entry.Status = LedgerEntryStatusFinalized
	entry.CommittedAt = entry.FinalizedAt
	entries, err := l.replayLocked()
	if err != nil {
		return CommitCertificate{}, err
	}
	return l.appendLocked(int64(len(entries)+1), *entry)
}

func entryHash(entry LedgerEntry) string {
	data, _ := json.Marshal(entry)
	sum := sha256.Sum256(data)
	return hex.EncodeToString(sum[:])
}

func approvalCount(votes []ConsensusVote) int {
	count := 0
	for _, vote := range votes {
		if vote.Approved {
			count++
		}
	}
	return count
}

func rejectionCount(votes []ConsensusVote) int {
	count := 0
	for _, vote := range votes {
		if !vote.Approved {
			count++
		}
	}
	return count
}

func maxInt64(values ...int64) int64 {
	var max int64
	for i, value := range values {
		if i == 0 || value > max {
			max = value
		}
	}
	return max
}
