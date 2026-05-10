package agency

import (
	"crypto/sha256"
	"encoding/hex"
	"sort"
	"strconv"
	"strings"
	"time"
)

// MerkleAttestProtocolVersion is the wire version for MerkleAttestation
// payloads. Bump if the leaf preimage or tree shape changes; persisted
// attestations from older versions must be rehydrated, not silently
// reinterpreted, since identical graphs can produce different roots
// across versions.
const MerkleAttestProtocolVersion = 1

// merkleLeafTag and merkleInternalTag domain-separate leaves from
// internal nodes so an attacker cannot produce a leaf preimage that
// collides with an internal-node preimage. This is the standard
// defense against second-preimage attacks on flat Merkle trees.
const (
	merkleLeafTag     = "node\x00"
	merkleInternalTag = "internal\x00"
)

// MerkleAttestation is a content-addressed, ordering-invariant summary
// of a CausalGraph. Two attestations share a Root iff, after
// canonicalisation, the underlying graphs have identical typed causal
// structure (nodes, roles, weights to 6 decimal places, parents,
// summaries modulo whitespace/case). Root equality is therefore a
// mechanism-faithful equivalence relation, not a textual one.
//
// LeafHashes is optional but recommended: it is what allows
// MerkleConverge to localise divergence, pointing reviewers at the
// specific leaves a minority of peers disagree about rather than just
// emitting "graphs differ".
type MerkleAttestation struct {
	ProtocolVersion int       `json:"protocolVersion"`
	Root            string    `json:"root"`
	LeafCount       int       `json:"leafCount"`
	LeafHashes      []string  `json:"leafHashes,omitempty"`
	GraphSize       int       `json:"graphSize"`
	AttestedAt      time.Time `json:"attestedAt"`
}

// MerkleAttestConfig tunes optional knobs on the leaf canonicalisation.
// The zero value is a sensible default: lowercase + whitespace-collapse
// summary normalisation, 6-decimal-place weight rounding, no leaf
// hashes hidden.
type MerkleAttestConfig struct {
	// IncludeLeafHashes controls whether the attestation carries the
	// per-leaf hashes. Defaults to true via NewMerkleAttestConfig.
	IncludeLeafHashes bool
	// AttestedAt overrides the timestamp; zero means time.Now().UTC().
	AttestedAt time.Time
}

// NewMerkleAttestConfig returns the default configuration: leaf
// hashes included, timestamp set at attest time.
func NewMerkleAttestConfig() MerkleAttestConfig {
	return MerkleAttestConfig{IncludeLeafHashes: true}
}

// MerkleAttest computes the Merkle attestation for a CausalGraph using
// default config. A nil or empty graph produces an attestation with
// LeafCount=0 and a fixed sentinel Root so the empty case is still
// distinguishable from "never attested".
func MerkleAttest(graph *CausalGraph) MerkleAttestation {
	return MerkleAttestWithConfig(graph, NewMerkleAttestConfig())
}

// MerkleAttestWithConfig is the configurable form of MerkleAttest.
func MerkleAttestWithConfig(graph *CausalGraph, cfg MerkleAttestConfig) MerkleAttestation {
	now := cfg.AttestedAt
	if now.IsZero() {
		now = time.Now().UTC()
	}
	if graph == nil || len(graph.Nodes) == 0 {
		return MerkleAttestation{
			ProtocolVersion: MerkleAttestProtocolVersion,
			Root:            emptyMerkleRoot(),
			LeafCount:       0,
			GraphSize:       0,
			AttestedAt:      now,
		}
	}
	leaves := make([]string, 0, len(graph.Nodes))
	for _, node := range graph.Nodes {
		leaves = append(leaves, canonicalLeafHash(node))
	}
	// Lexicographic sort: graphs differing only in node listing order
	// must produce identical roots.
	sort.Strings(leaves)
	root := merkleRootFromSortedLeaves(leaves)
	att := MerkleAttestation{
		ProtocolVersion: MerkleAttestProtocolVersion,
		Root:            root,
		LeafCount:       len(leaves),
		GraphSize:       len(graph.Nodes),
		AttestedAt:      now,
	}
	if cfg.IncludeLeafHashes {
		att.LeafHashes = append([]string(nil), leaves...)
	}
	return att
}

// canonicalLeafHash produces the per-node leaf hash. The exact preimage
// shape is part of the wire protocol — see MerkleAttestProtocolVersion.
func canonicalLeafHash(node CausalNode) string {
	h := sha256.New()
	h.Write([]byte(merkleLeafTag))
	writeNUL(h, string(node.ID))
	writeNUL(h, string(node.Role))
	writeNUL(h, normalizeSummary(node.Summary))
	writeNUL(h, canonicalWeight(node.Weight))
	writeNUL(h, sortedJoin(stringifyNodeIDs(node.Parents)))
	writeNUL(h, sortedJoin(node.AtomRefs))
	writeNUL(h, sortedMetaJoin(node.Meta))
	return hex.EncodeToString(h.Sum(nil))
}

// writeNUL writes s followed by a NUL terminator. NUL terminators
// prevent length-extension ambiguity between adjacent fields (e.g. a
// summary "ab" + role "cd" must not collide with summary "a" + role
// "bcd").
func writeNUL(h interface {
	Write(p []byte) (int, error)
}, s string) {
	h.Write([]byte(s))
	h.Write([]byte{0})
}

// normalizeSummary is the floor-level summary canonicalisation:
// trim, collapse whitespace, lowercase. Open Q1 in the design memo:
// stricter normalisation (punctuation strip, stemming) buys more
// cross-agent agreement at the cost of fusing summaries that
// reviewers wrote differently on purpose.
func normalizeSummary(s string) string {
	s = strings.ToLower(strings.TrimSpace(s))
	if s == "" {
		return ""
	}
	out := make([]byte, 0, len(s))
	prevSpace := false
	for i := 0; i < len(s); i++ {
		c := s[i]
		if c == ' ' || c == '\t' || c == '\n' || c == '\r' || c == '\v' || c == '\f' {
			if prevSpace {
				continue
			}
			out = append(out, ' ')
			prevSpace = true
			continue
		}
		out = append(out, c)
		prevSpace = false
	}
	return string(out)
}

// canonicalWeight serialises a float64 weight to a locale-free, 6-decimal
// fixed-point string. Two weights within 1e-6 of each other will produce
// the same canonical form. Trailing zeros are kept so the string length
// is stable. NaN and Inf are mapped to fixed sentinels so they don't
// blow up the hash.
func canonicalWeight(w float64) string {
	switch {
	case w != w:
		return "NaN"
	case w > 1e308:
		return "+Inf"
	case w < -1e308:
		return "-Inf"
	}
	return strconv.FormatFloat(roundTo6(w), 'f', 6, 64)
}

func roundTo6(w float64) float64 {
	scaled := w * 1e6
	if scaled >= 0 {
		scaled = float64(int64(scaled + 0.5))
	} else {
		scaled = float64(int64(scaled - 0.5))
	}
	return scaled / 1e6
}

func stringifyNodeIDs(ids []NodeID) []string {
	out := make([]string, len(ids))
	for i, id := range ids {
		out[i] = string(id)
	}
	return out
}

func sortedJoin(xs []string) string {
	if len(xs) == 0 {
		return ""
	}
	cp := append([]string(nil), xs...)
	sort.Strings(cp)
	return strings.Join(cp, "\x00")
}

func sortedMetaJoin(m map[string]string) string {
	if len(m) == 0 {
		return ""
	}
	keys := make([]string, 0, len(m))
	for k := range m {
		keys = append(keys, k)
	}
	sort.Strings(keys)
	parts := make([]string, 0, len(keys))
	for _, k := range keys {
		parts = append(parts, k+"="+m[k])
	}
	return strings.Join(parts, "\x00")
}

// merkleRootFromSortedLeaves builds a binary Merkle tree over a sorted
// leaf-hash slice and returns the root as hex. Odd-leaf-out is
// duplicated (Bitcoin convention) so the tree shape is deterministic
// for any leaf count.
func merkleRootFromSortedLeaves(leaves []string) string {
	if len(leaves) == 0 {
		return emptyMerkleRoot()
	}
	level := append([]string(nil), leaves...)
	for len(level) > 1 {
		next := make([]string, 0, (len(level)+1)/2)
		for i := 0; i < len(level); i += 2 {
			left := level[i]
			right := left
			if i+1 < len(level) {
				right = level[i+1]
			}
			next = append(next, merkleInternalHash(left, right))
		}
		level = next
	}
	return level[0]
}

func merkleInternalHash(left, right string) string {
	h := sha256.New()
	h.Write([]byte(merkleInternalTag))
	leftBytes, _ := hex.DecodeString(left)
	rightBytes, _ := hex.DecodeString(right)
	h.Write(leftBytes)
	h.Write([]byte{0})
	h.Write(rightBytes)
	return hex.EncodeToString(h.Sum(nil))
}

// emptyMerkleRoot is the sentinel Root for a zero-leaf attestation.
// Distinguishable from any real graph (which has at least one leaf).
func emptyMerkleRoot() string {
	h := sha256.New()
	h.Write([]byte(merkleLeafTag))
	h.Write([]byte("empty\x00"))
	return hex.EncodeToString(h.Sum(nil))
}

// LeafSet returns the leaf hashes as a set for divergence comparison.
// Returns nil for attestations that didn't carry leaf hashes.
func (a MerkleAttestation) LeafSet() map[string]struct{} {
	if len(a.LeafHashes) == 0 {
		return nil
	}
	out := make(map[string]struct{}, len(a.LeafHashes))
	for _, h := range a.LeafHashes {
		out[h] = struct{}{}
	}
	return out
}
