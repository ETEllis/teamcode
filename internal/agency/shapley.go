package agency

import (
	"math"
	"math/bits"
	"math/rand"
	"sort"
)

// NodeAttribution is the per-node Shapley/PSE value answering "how much did
// this node contribute to the verdict's confidence?". Phi is the Shapley
// value over the same value function used by the Pearl loop:
//
//	v(S) = sigmoid( sum_{n in S} roleWeight(n.Role) * n.Weight )
//
// Positive Phi means the node pushed confidence up; negative means it
// pulled confidence down (typical of confounders).
type NodeAttribution struct {
	NodeID      NodeID   `json:"nodeId"`
	Role        NodeRole `json:"role"`
	Phi         float64  `json:"phi"`
	Rank        int      `json:"rank"`
	Approximate bool     `json:"approximate,omitempty"`
}

// exactShapleyMaxPlayers is the cutoff above which we switch from exact
// 2^n enumeration to Monte Carlo sampling. 14 players = 16384 subsets,
// which evaluates in well under a millisecond on any modern CPU.
const exactShapleyMaxPlayers = 14

// sampledShapleyPermutations is the number of random permutations used by
// the Monte Carlo estimator. 256 gives Phi within ~0.01 of the true value
// for the value functions we use here.
const sampledShapleyPermutations = 256

// shapleyMaxPlayers caps the bitmask representation at uint64 width.
// Graphs larger than this are silently truncated (highest-weight first)
// before attribution; in practice GIST graphs stay well under 32 nodes.
const shapleyMaxPlayers = 64

// AttributeNecessity computes per-node Shapley values over the typed
// CausalGraph and returns them ranked by absolute contribution. The
// outcome node is excluded from the player set since it is the target,
// not a contributor.
//
// For graphs up to exactShapleyMaxPlayers nodes the result is exact; for
// larger graphs the result is a Monte Carlo estimate flagged via
// NodeAttribution.Approximate = true.
func AttributeNecessity(graph *CausalGraph) []NodeAttribution {
	if graph == nil {
		return nil
	}
	players := make([]CausalNode, 0, len(graph.Nodes))
	for _, n := range graph.Nodes {
		if n.Role == NodeRoleOutcome {
			continue
		}
		players = append(players, n)
	}
	if len(players) == 0 {
		return nil
	}
	if len(players) > shapleyMaxPlayers {
		// Keep the heaviest players to fit in the bitmask. Stable
		// truncation: largest weight first, ID tiebreak.
		sort.SliceStable(players, func(i, j int) bool {
			if players[i].Weight != players[j].Weight {
				return players[i].Weight > players[j].Weight
			}
			return players[i].ID < players[j].ID
		})
		players = players[:shapleyMaxPlayers]
	}

	var phi []float64
	approx := false
	if len(players) <= exactShapleyMaxPlayers {
		phi = exactShapley(players)
	} else {
		phi = sampledShapley(players, sampledShapleyPermutations)
		approx = true
	}

	attributions := make([]NodeAttribution, len(players))
	for i, p := range players {
		attributions[i] = NodeAttribution{
			NodeID:      p.ID,
			Role:        p.Role,
			Phi:         phi[i],
			Approximate: approx,
		}
	}
	// Rank by |Phi| descending; ID tiebreak so identical-Phi nodes get a
	// stable order across runs.
	sort.SliceStable(attributions, func(i, j int) bool {
		ai := math.Abs(attributions[i].Phi)
		aj := math.Abs(attributions[j].Phi)
		if ai != aj {
			return ai > aj
		}
		return attributions[i].NodeID < attributions[j].NodeID
	})
	for i := range attributions {
		attributions[i].Rank = i + 1
	}
	return attributions
}

// shapleyValue evaluates v(S) for the subset encoded by mask. The value
// function is shared with the Pearl loop's score so action recommendations
// and Shapley attributions are consistent: a node with high positive Phi is
// also a node whose presence pushes the recommended action toward
// confidence.
func shapleyValue(players []CausalNode, mask uint64) float64 {
	score := 0.0
	for i, p := range players {
		if mask&(uint64(1)<<uint(i)) == 0 {
			continue
		}
		score += roleWeight(p.Role) * p.Weight
	}
	return sigmoid(score)
}

// roleWeight maps a NodeRole onto its directional contribution. Confounders
// pull verdict confidence down twice as hard as evidence pulls it up,
// reflecting Pearl's principle that a known common cause demands stronger
// disconfirmation than a typical observation supports.
func roleWeight(r NodeRole) float64 {
	switch r {
	case NodeRoleEvidence:
		return 1.0
	case NodeRoleIntervention:
		return 0.5
	case NodeRoleConfounder:
		return -2.0
	case NodeRoleUnknown:
		return 0.5
	default:
		return 0
	}
}

// exactShapley enumerates all 2^n subsets and accumulates the closed-form
// Shapley sum. O(n * 2^n) work; bounded by exactShapleyMaxPlayers.
func exactShapley(players []CausalNode) []float64 {
	n := len(players)
	phi := make([]float64, n)
	factorial := make([]float64, n+1)
	factorial[0] = 1
	for i := 1; i <= n; i++ {
		factorial[i] = factorial[i-1] * float64(i)
	}
	nFact := factorial[n]

	upper := uint64(1) << uint(n)
	for mask := uint64(0); mask < upper; mask++ {
		sSize := bits.OnesCount64(mask)
		for i := 0; i < n; i++ {
			bit := uint64(1) << uint(i)
			if mask&bit != 0 {
				continue
			}
			vWithout := shapleyValue(players, mask)
			vWith := shapleyValue(players, mask|bit)
			weight := factorial[sSize] * factorial[n-sSize-1] / nFact
			phi[i] += weight * (vWith - vWithout)
		}
	}
	return phi
}

// sampledShapley approximates Shapley values by averaging marginal
// contributions over random permutations. The seed is derived from the
// player IDs so repeated calls with the same graph return the same result.
func sampledShapley(players []CausalNode, perms int) []float64 {
	n := len(players)
	phi := make([]float64, n)
	rng := rand.New(rand.NewSource(int64(stableSeed(players))))
	perm := make([]int, n)
	for i := range perm {
		perm[i] = i
	}
	for k := 0; k < perms; k++ {
		rng.Shuffle(n, func(i, j int) { perm[i], perm[j] = perm[j], perm[i] })
		var mask uint64
		prev := shapleyValue(players, mask)
		for _, idx := range perm {
			mask |= uint64(1) << uint(idx)
			curr := shapleyValue(players, mask)
			phi[idx] += curr - prev
			prev = curr
		}
	}
	scale := 1.0 / float64(perms)
	for i := range phi {
		phi[i] *= scale
	}
	return phi
}

// stableSeed derives a deterministic seed from player IDs so sampled
// Shapley results are reproducible (and CI-stable).
func stableSeed(players []CausalNode) uint64 {
	const fnvOffset = 14695981039346656037
	const fnvPrime = 1099511628211
	s := uint64(fnvOffset)
	for _, p := range players {
		s = s*fnvPrime ^ uint64(len(p.ID))
		for _, c := range p.ID {
			s = s*fnvPrime ^ uint64(c)
		}
	}
	return s
}
