package agency

import (
	"context"
	"testing"
	"time"

	"github.com/stretchr/testify/require"
)

func TestLedgerAppendAndReplay(t *testing.T) {
	t.Parallel()

	ledger, err := NewLedgerService(t.TempDir())
	require.NoError(t, err)

	_, err = ledger.Append(context.Background(), LedgerEntry{
		OrganizationID: "org-1",
		Kind:           LedgerEntrySignal,
		Signal: &WakeSignal{
			ID:             "sig-1",
			OrganizationID: "org-1",
			Channel:        "agency.org.org-1",
			Kind:           SignalSchedule,
			CreatedAt:      time.Now().UnixMilli(),
		},
	})
	require.NoError(t, err)

	_, err = ledger.Append(context.Background(), LedgerEntry{
		OrganizationID: "org-1",
		Kind:           LedgerEntryPublication,
		Publication: &PublicationRecord{
			ID:        "pub-1",
			ActorID:   "actor-1",
			Target:    "artifact",
			Status:    "published",
			CreatedAt: time.Now().UnixMilli(),
		},
	})
	require.NoError(t, err)

	entries, err := ledger.Replay(context.Background())
	require.NoError(t, err)
	require.Len(t, entries, 2)
	require.Equal(t, int64(1), entries[0].Sequence)
	require.Equal(t, int64(2), entries[1].Sequence)
	require.Equal(t, LedgerEntryStatusFinalized, entries[0].Status)
	require.Equal(t, LedgerEntryStatusFinalized, entries[1].Status)

	snapshot, err := ledger.LatestSnapshot(context.Background(), "org-1")
	require.NoError(t, err)
	require.Equal(t, int64(2), snapshot.LedgerSequence)
	require.Len(t, snapshot.Publications, 1)
}

func TestLedgerPendingEntriesRequireExplicitFinalization(t *testing.T) {
	t.Parallel()

	ledger, err := NewLedgerService(t.TempDir())
	require.NoError(t, err)

	entry, err := ledger.Propose(context.Background(), LedgerEntry{
		OrganizationID: "org-1",
		Kind:           LedgerEntryAction,
		ActorID:        "actor-1",
		Action: &ActionProposal{
			ID:             "proposal-1",
			ActorID:        "actor-1",
			OrganizationID: "org-1",
			Type:           ActionWriteCode,
			Target:         "main.go",
			ProposedAt:     time.Now().UnixMilli(),
		},
		Consensus: &ConsensusRequirement{
			Strategy:          ConsensusStrategyQuorum,
			RequiredApprovals: 2,
			EligibleVoters:    []string{"lead", "reviewer"},
			AutoFinalize:      false,
		},
	})
	require.NoError(t, err)
	require.Equal(t, LedgerEntryStatusProposed, entry.Status)

	replay, err := ledger.Replay(context.Background())
	require.NoError(t, err)
	require.Empty(t, replay)

	pending, err := ledger.Pending(context.Background(), "org-1")
	require.NoError(t, err)
	require.Len(t, pending, 1)
	require.Equal(t, LedgerEntryStatusProposed, pending[0].Status)

	quorum, err := ledger.Vote(context.Background(), entry.ID, ConsensusVote{
		VoterID:  "lead",
		Approved: true,
	})
	require.NoError(t, err)
	require.Equal(t, 1, quorum.Approvals)
	require.False(t, quorum.Finalizable)

	quorum, err = ledger.Vote(context.Background(), entry.ID, ConsensusVote{
		VoterID:  "reviewer",
		Approved: true,
	})
	require.NoError(t, err)
	require.Equal(t, 2, quorum.Approvals)
	require.True(t, quorum.Finalizable)

	replay, err = ledger.Replay(context.Background())
	require.NoError(t, err)
	require.Empty(t, replay)

	cert, err := ledger.Finalize(context.Background(), entry.ID)
	require.NoError(t, err)
	require.Equal(t, LedgerEntryStatusFinalized, cert.Status)
	require.Equal(t, 2, cert.QuorumSize)
	require.Equal(t, 2, cert.Approvals)

	replay, err = ledger.Replay(context.Background())
	require.NoError(t, err)
	require.Len(t, replay, 1)
	require.Equal(t, LedgerEntryStatusFinalized, replay[0].Status)
	require.Len(t, replay[0].Votes, 2)

	pending, err = ledger.Pending(context.Background(), "org-1")
	require.NoError(t, err)
	require.Empty(t, pending)
}

func TestLedgerCanFinalizeRejectedProposalWhenQuorumBecomesImpossible(t *testing.T) {
	t.Parallel()

	ledger, err := NewLedgerService(t.TempDir())
	require.NoError(t, err)

	entry, err := ledger.Propose(context.Background(), LedgerEntry{
		OrganizationID: "org-1",
		Kind:           LedgerEntryPublication,
		ActorID:        "actor-1",
		Publication: &PublicationRecord{
			ID:        "pub-1",
			ActorID:   "actor-1",
			Target:    "artifact",
			Status:    "draft",
			CreatedAt: time.Now().UnixMilli(),
		},
		Consensus: &ConsensusRequirement{
			Strategy:          ConsensusStrategyQuorum,
			RequiredApprovals: 2,
			EligibleVoters:    []string{"lead", "reviewer"},
			AutoFinalize:      false,
		},
	})
	require.NoError(t, err)

	quorum, err := ledger.Vote(context.Background(), entry.ID, ConsensusVote{
		VoterID:  "lead",
		Approved: false,
		Reason:   "not ready",
	})
	require.NoError(t, err)
	require.True(t, quorum.Rejected)
	require.True(t, quorum.Finalizable)

	cert, err := ledger.Finalize(context.Background(), entry.ID)
	require.NoError(t, err)
	require.Equal(t, LedgerEntryStatusRejected, cert.Status)
	require.Equal(t, 1, cert.Rejections)

	replay, err := ledger.Replay(context.Background())
	require.NoError(t, err)
	require.Empty(t, replay)

	pending, err := ledger.Pending(context.Background(), "org-1")
	require.NoError(t, err)
	require.Empty(t, pending)
}

func TestLedgerRejectsDuplicateVotes(t *testing.T) {
	t.Parallel()

	ledger, err := NewLedgerService(t.TempDir())
	require.NoError(t, err)

	entry, err := ledger.Propose(context.Background(), LedgerEntry{
		OrganizationID: "org-1",
		Kind:           LedgerEntryAction,
		ActorID:        "actor-1",
		Action: &ActionProposal{
			ActorID:        "actor-1",
			OrganizationID: "org-1",
			Type:           ActionRunTest,
			Target:         "go test ./...",
			ProposedAt:     time.Now().UnixMilli(),
		},
		Consensus: &ConsensusRequirement{
			Strategy:          ConsensusStrategyQuorum,
			RequiredApprovals: 2,
			EligibleVoters:    []string{"lead", "reviewer"},
			AutoFinalize:      false,
		},
	})
	require.NoError(t, err)

	_, err = ledger.Vote(context.Background(), entry.ID, ConsensusVote{
		VoterID:  "lead",
		Approved: true,
	})
	require.NoError(t, err)

	_, err = ledger.Vote(context.Background(), entry.ID, ConsensusVote{
		VoterID:  "lead",
		Approved: true,
	})
	require.Error(t, err)
}
