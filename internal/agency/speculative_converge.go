package agency

import (
	"sort"
)

// ConvergenceProtocolVersion is the wire version for ConvergenceReport.
// Bump on any non-additive change so downstream consumers can refuse
// to interpret reports they don't understand.
const ConvergenceProtocolVersion = 1

// ConvergenceStatus is the three-tier ladder for cohort agreement.
//
//	converged - quorum >= threshold on a single root; downstream stages may run
//	partial   - plurality root exists but below threshold; downstream stages run with caveats
//	divergent - no plurality OR no agents; downstream stages MUST refuse
//
// Hard tie at the top (multiple roots tied for plurality) collapses to
// divergent because there's no defensible consensus to propagate.
type ConvergenceStatus string

const (
	ConvergenceStatusConverged ConvergenceStatus = "converged"
	ConvergenceStatusPartial   ConvergenceStatus = "partial"
	ConvergenceStatusDivergent ConvergenceStatus = "divergent"
)

// DefaultConvergenceThreshold is the default quorum required for the
// 'converged' tier. 0.66 means at least two-thirds of agents must
// share the same Merkle root.
const DefaultConvergenceThreshold = 0.66

// PeerAttestation pairs a per-agent attestation with the agent's ID
// so divergence loci can name agents.
type PeerAttestation struct {
	AgentID     string            `json:"agentId"`
	Attestation MerkleAttestation `json:"attestation"`
}

// DivergenceLocus reports a leaf that the cohort disagrees about,
// naming the agents on each side. Severity 'extra' means the leaf is
// in the inclusion set but not the consensus; 'missing' means the
// reverse. This lets a reviewer point at the specific node IDs
// (via leaf hash) the minority is wrong about — without ever shipping
// the full graphs.
type DivergenceLocus struct {
	LeafHash  string   `json:"leafHash"`
	Severity  string   `json:"severity"`
	Inclusion []string `json:"inclusion,omitempty"`
	Exclusion []string `json:"exclusion,omitempty"`
}

// ConvergenceReport is the full output of MerkleConverge. It is the
// gate signal: status determines whether downstream stages may run,
// ConsensusRoot identifies the consensus bucket, RootHistogram exposes
// the full distribution for diagnostic UIs, and DivergenceLoci names
// the leaves that broke unanimity.
type ConvergenceReport struct {
	ProtocolVersion int                `json:"protocolVersion"`
	Status          ConvergenceStatus  `json:"status"`
	ConsensusRoot   string             `json:"consensusRoot,omitempty"`
	Threshold       float64            `json:"threshold"`
	Quorum          float64            `json:"quorum"`
	AgentCount      int                `json:"agentCount"`
	RootHistogram   map[string]int     `json:"rootHistogram"`
	BucketAgents    map[string][]string `json:"bucketAgents,omitempty"`
	DivergenceLoci  []DivergenceLocus  `json:"divergenceLoci,omitempty"`
	Notes           []string           `json:"notes,omitempty"`
}

// MerkleConverge runs the gate over a slice of peer attestations using
// the default threshold. Empty input is an explicit divergence — there
// is no honest 'converged' answer over zero agents.
func MerkleConverge(peers []PeerAttestation) ConvergenceReport {
	return MerkleConvergeWithThreshold(peers, DefaultConvergenceThreshold)
}

// MerkleConvergeWithThreshold lets the caller override the quorum
// threshold (e.g. require unanimity by passing 1.0). Threshold values
// outside (0,1] fall back to the default.
func MerkleConvergeWithThreshold(peers []PeerAttestation, threshold float64) ConvergenceReport {
	if threshold <= 0 || threshold > 1 {
		threshold = DefaultConvergenceThreshold
	}
	report := ConvergenceReport{
		ProtocolVersion: ConvergenceProtocolVersion,
		Threshold:       threshold,
		AgentCount:      len(peers),
		RootHistogram:   map[string]int{},
		BucketAgents:    map[string][]string{},
	}
	if len(peers) == 0 {
		report.Status = ConvergenceStatusDivergent
		report.Notes = append(report.Notes, "no peer attestations supplied")
		return report
	}
	for _, p := range peers {
		report.RootHistogram[p.Attestation.Root]++
		report.BucketAgents[p.Attestation.Root] = append(report.BucketAgents[p.Attestation.Root], p.AgentID)
	}
	// Sort bucket agents so report is deterministic.
	for k := range report.BucketAgents {
		sort.Strings(report.BucketAgents[k])
	}
	plurality, pluralityCount, tied := pluralityRoot(report.RootHistogram)
	report.Quorum = float64(pluralityCount) / float64(len(peers))
	switch {
	case tied:
		report.Status = ConvergenceStatusDivergent
		report.Notes = append(report.Notes, "multiple roots tied for plurality; no defensible consensus")
	case report.Quorum >= threshold:
		report.Status = ConvergenceStatusConverged
		report.ConsensusRoot = plurality
	default:
		report.Status = ConvergenceStatusPartial
		report.ConsensusRoot = plurality
		report.Notes = append(report.Notes, "plurality root below threshold; downstream stages should run with caveats")
	}
	if report.ConsensusRoot != "" {
		report.DivergenceLoci = computeDivergenceLoci(peers, report.ConsensusRoot)
	}
	return report
}

// pluralityRoot returns the most-common root, its count, and a flag
// indicating whether the top is tied (which forces divergent status).
func pluralityRoot(hist map[string]int) (string, int, bool) {
	best := ""
	bestCount := 0
	tied := false
	// Sort keys for determinism so ties resolve identically across runs.
	keys := make([]string, 0, len(hist))
	for k := range hist {
		keys = append(keys, k)
	}
	sort.Strings(keys)
	for _, k := range keys {
		c := hist[k]
		switch {
		case c > bestCount:
			best, bestCount, tied = k, c, false
		case c == bestCount && k != best:
			tied = true
		}
	}
	return best, bestCount, tied
}

// computeDivergenceLoci diff the leaf sets of every peer against the
// consensus bucket's leaf set. Peers in the consensus bucket
// contribute their leaves as the reference; peers outside contribute
// their differences.
func computeDivergenceLoci(peers []PeerAttestation, consensusRoot string) []DivergenceLocus {
	consensusLeaves := map[string]struct{}{}
	for _, p := range peers {
		if p.Attestation.Root == consensusRoot {
			for _, l := range p.Attestation.LeafHashes {
				consensusLeaves[l] = struct{}{}
			}
			break // one peer in the bucket is enough; all bucket peers share a leaf set by definition
		}
	}
	if len(consensusLeaves) == 0 {
		return nil
	}
	// Per-leaf inclusion/exclusion across the cohort.
	type leafState struct {
		inclusion map[string]struct{}
		exclusion map[string]struct{}
	}
	state := map[string]*leafState{}
	getState := func(leaf string) *leafState {
		s, ok := state[leaf]
		if !ok {
			s = &leafState{
				inclusion: map[string]struct{}{},
				exclusion: map[string]struct{}{},
			}
			state[leaf] = s
		}
		return s
	}
	for _, p := range peers {
		if p.Attestation.Root == consensusRoot {
			continue
		}
		peerSet := p.Attestation.LeafSet()
		for leaf := range consensusLeaves {
			if _, ok := peerSet[leaf]; !ok {
				getState(leaf).exclusion[p.AgentID] = struct{}{}
			}
		}
		for leaf := range peerSet {
			if _, ok := consensusLeaves[leaf]; !ok {
				getState(leaf).inclusion[p.AgentID] = struct{}{}
			}
		}
	}
	leaves := make([]string, 0, len(state))
	for k := range state {
		leaves = append(leaves, k)
	}
	sort.Strings(leaves)
	out := make([]DivergenceLocus, 0, len(leaves))
	for _, l := range leaves {
		s := state[l]
		_, inConsensus := consensusLeaves[l]
		severity := "extra"
		if inConsensus {
			severity = "missing"
		}
		locus := DivergenceLocus{
			LeafHash:  l,
			Severity:  severity,
			Inclusion: setToSortedSlice(s.inclusion),
			Exclusion: setToSortedSlice(s.exclusion),
		}
		out = append(out, locus)
	}
	return out
}

func setToSortedSlice(set map[string]struct{}) []string {
	if len(set) == 0 {
		return nil
	}
	out := make([]string, 0, len(set))
	for k := range set {
		out = append(out, k)
	}
	sort.Strings(out)
	return out
}

// IsGateOpen reports whether downstream speculative-tier stages may
// proceed. Converged: yes. Partial: yes with caveats. Divergent: no.
func (r ConvergenceReport) IsGateOpen() bool {
	return r.Status == ConvergenceStatusConverged || r.Status == ConvergenceStatusPartial
}

// ConsensusBucketAgents returns the agent IDs in the consensus root's
// bucket — the cohort whose verdicts the downstream stages may
// reconcile or compress over. Returns nil when there is no consensus.
func (r ConvergenceReport) ConsensusBucketAgents() []string {
	if r.ConsensusRoot == "" {
		return nil
	}
	return append([]string(nil), r.BucketAgents[r.ConsensusRoot]...)
}
