package agency

import (
	"slices"
	"sort"
	"time"
)

func normalizeConsensusRequirement(entry LedgerEntry) ConsensusRequirement {
	if entry.Consensus != nil {
		req := *entry.Consensus
		if req.RequiredApprovals <= 0 {
			req.RequiredApprovals = 1
		}
		if req.Strategy == "" {
			if req.RequiredApprovals > 1 || len(req.EligibleVoters) > 1 {
				req.Strategy = ConsensusStrategyQuorum
			} else {
				req.Strategy = ConsensusStrategyDirect
			}
		}
		if req.QuorumKey == "" {
			req.QuorumKey = entry.ID
		}
		return req
	}

	quorumKey := entry.ID
	if quorumKey == "" {
		quorumKey = entry.OrganizationID + ":" + string(entry.Kind)
	}
	return ConsensusRequirement{
		Strategy:          ConsensusStrategyDirect,
		QuorumKey:         quorumKey,
		RequiredApprovals: 1,
		AutoFinalize:      true,
	}
}

func computeQuorumState(req ConsensusRequirement, votes []ConsensusVote) QuorumState {
	state := QuorumState{
		QuorumKey:         req.QuorumKey,
		RequiredApprovals: req.RequiredApprovals,
		EligibleVoters:    append([]string{}, req.EligibleVoters...),
	}

	seen := make(map[string]struct{}, len(votes))
	for _, vote := range votes {
		if vote.VoterID == "" {
			continue
		}
		if _, exists := seen[vote.VoterID]; exists {
			continue
		}
		seen[vote.VoterID] = struct{}{}
		if vote.Approved {
			state.Approvals++
		} else {
			state.Rejections++
		}
		if vote.VotedAt > state.LastVoteAt {
			state.LastVoteAt = vote.VotedAt
		}
	}

	state.MissingApprovals = maxInt(req.RequiredApprovals-state.Approvals, 0)
	if state.Approvals >= req.RequiredApprovals {
		state.Finalizable = true
		return state
	}

	if len(req.EligibleVoters) > 0 {
		maxPossibleApprovals := len(req.EligibleVoters) - state.Rejections
		if maxPossibleApprovals < req.RequiredApprovals {
			state.Rejected = true
			state.Finalizable = true
		}
	}

	return state
}

func sortConsensusVotes(votes []ConsensusVote) []ConsensusVote {
	out := append([]ConsensusVote{}, votes...)
	sort.Slice(out, func(i, j int) bool {
		if out[i].VotedAt != out[j].VotedAt {
			return out[i].VotedAt < out[j].VotedAt
		}
		return out[i].VoterID < out[j].VoterID
	})
	return out
}

func allowsVoter(req ConsensusRequirement, voterID string) bool {
	if voterID == "" {
		return false
	}
	if len(req.EligibleVoters) == 0 {
		return true
	}
	return slices.Contains(req.EligibleVoters, voterID)
}

func implicitConsensusVote(entry LedgerEntry) (ConsensusVote, bool) {
	voterID := entry.ActorID
	if voterID == "" {
		voterID = "system"
	}
	if voterID == "" {
		return ConsensusVote{}, false
	}
	return ConsensusVote{
		VoterID:  voterID,
		EntryID:  entry.ID,
		Approved: true,
		VotedAt:  time.Now().UnixMilli(),
		Reason:   "compatibility append",
	}, true
}

func maxInt(values ...int) int {
	maxValue := 0
	for _, value := range values {
		if value > maxValue {
			maxValue = value
		}
	}
	return maxValue
}
