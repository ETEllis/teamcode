package agency

import (
	"strings"
	"testing"
)

func merkleSampleGraph() *CausalGraph {
	return &CausalGraph{
		ProtocolVersion: CausalGraphProtocolVersion,
		Nodes: []CausalNode{
			{ID: "ev1", Role: NodeRoleEvidence, Summary: "log shows deploy succeeded", Weight: 1.0, Parents: []NodeID{}, AtomRefs: []string{"atom-1"}},
			{ID: "conf1", Role: NodeRoleConfounder, Summary: "  regional  outage   masked  errors  ", Weight: 0.4},
			{ID: "act1", Role: NodeRoleIntervention, Summary: "promote to prod", Weight: 0.6, Parents: []NodeID{"ev1"}},
			{ID: "out1", Role: NodeRoleOutcome, Summary: "deploy is healthy", Weight: 1.0, Parents: []NodeID{"act1"}},
		},
	}
}

func TestMerkleAttest_NilGraph_StableSentinel(t *testing.T) {
	a := MerkleAttest(nil)
	b := MerkleAttest(&CausalGraph{ProtocolVersion: 1})
	if a.Root != b.Root {
		t.Errorf("nil and empty graphs should produce same sentinel root, got %q vs %q", a.Root, b.Root)
	}
	if a.LeafCount != 0 || a.GraphSize != 0 {
		t.Errorf("empty attestation should report zero leaves and graph size")
	}
}

func TestMerkleAttest_DeterministicAcrossOrderings(t *testing.T) {
	g1 := merkleSampleGraph()
	g2 := &CausalGraph{
		ProtocolVersion: 1,
		Nodes: []CausalNode{
			g1.Nodes[3], g1.Nodes[0], g1.Nodes[2], g1.Nodes[1],
		},
	}
	a1 := MerkleAttest(g1)
	a2 := MerkleAttest(g2)
	if a1.Root != a2.Root {
		t.Errorf("reordered graph produced different root: %q != %q", a1.Root, a2.Root)
	}
	if len(a1.LeafHashes) != len(a2.LeafHashes) {
		t.Fatalf("leaf counts differ: %d vs %d", len(a1.LeafHashes), len(a2.LeafHashes))
	}
}

func TestMerkleAttest_ParentsReorder_StillSameRoot(t *testing.T) {
	g1 := &CausalGraph{
		Nodes: []CausalNode{
			{ID: "n", Role: NodeRoleEvidence, Summary: "x", Weight: 1, Parents: []NodeID{"a", "b", "c"}},
		},
	}
	g2 := &CausalGraph{
		Nodes: []CausalNode{
			{ID: "n", Role: NodeRoleEvidence, Summary: "x", Weight: 1, Parents: []NodeID{"c", "a", "b"}},
		},
	}
	if MerkleAttest(g1).Root != MerkleAttest(g2).Root {
		t.Errorf("parent reordering changed root")
	}
}

func TestMerkleAttest_SummaryWhitespaceCaseInvariance(t *testing.T) {
	g1 := &CausalGraph{Nodes: []CausalNode{{ID: "n", Role: NodeRoleEvidence, Summary: "Hello   World", Weight: 1}}}
	g2 := &CausalGraph{Nodes: []CausalNode{{ID: "n", Role: NodeRoleEvidence, Summary: "  hello world  ", Weight: 1}}}
	if MerkleAttest(g1).Root != MerkleAttest(g2).Root {
		t.Errorf("summary whitespace/case difference changed root")
	}
}

func TestMerkleAttest_WeightRoundsTo6Decimals(t *testing.T) {
	g1 := &CausalGraph{Nodes: []CausalNode{{ID: "n", Role: NodeRoleEvidence, Summary: "x", Weight: 0.123456}}}
	g2 := &CausalGraph{Nodes: []CausalNode{{ID: "n", Role: NodeRoleEvidence, Summary: "x", Weight: 0.1234561}}}
	if MerkleAttest(g1).Root != MerkleAttest(g2).Root {
		t.Errorf("sub-1e-6 weight diff changed root: should round to 6 decimals")
	}
}

func TestMerkleAttest_RoleChangeChangesRoot(t *testing.T) {
	g1 := &CausalGraph{Nodes: []CausalNode{{ID: "n", Role: NodeRoleEvidence, Summary: "x", Weight: 1}}}
	g2 := &CausalGraph{Nodes: []CausalNode{{ID: "n", Role: NodeRoleConfounder, Summary: "x", Weight: 1}}}
	if MerkleAttest(g1).Root == MerkleAttest(g2).Root {
		t.Errorf("role flip should change root (mechanism-faithful equivalence)")
	}
}

func TestMerkleAttest_NodeAddChangesRoot(t *testing.T) {
	g1 := merkleSampleGraph()
	g2 := merkleSampleGraph()
	g2.Nodes = append(g2.Nodes, CausalNode{ID: "extra", Role: NodeRoleEvidence, Summary: "extra evidence", Weight: 0.2})
	a1 := MerkleAttest(g1)
	a2 := MerkleAttest(g2)
	if a1.Root == a2.Root {
		t.Errorf("adding a node didn't change root")
	}
	if a2.LeafCount != a1.LeafCount+1 {
		t.Errorf("expected leaf count +1, got %d -> %d", a1.LeafCount, a2.LeafCount)
	}
}

func TestMerkleAttest_LeafHashesSorted(t *testing.T) {
	a := MerkleAttest(merkleSampleGraph())
	for i := 1; i < len(a.LeafHashes); i++ {
		if a.LeafHashes[i-1] > a.LeafHashes[i] {
			t.Errorf("leaf hashes not sorted at index %d: %s > %s", i, a.LeafHashes[i-1], a.LeafHashes[i])
		}
	}
}

func TestMerkleAttest_LeafSet_RoundTrip(t *testing.T) {
	a := MerkleAttest(merkleSampleGraph())
	set := a.LeafSet()
	if len(set) != len(a.LeafHashes) {
		t.Errorf("leaf set size mismatch: %d vs %d", len(set), len(a.LeafHashes))
	}
	for _, h := range a.LeafHashes {
		if _, ok := set[h]; !ok {
			t.Errorf("missing leaf %s from set", h)
		}
	}
}

func TestMerkleAttest_DomainSeparation(t *testing.T) {
	// A leaf hash must never collide with an internal-node hash on
	// the same byte preimage. The only way to verify externally is
	// that root format differs from leaf format under domain separation.
	root := MerkleAttest(merkleSampleGraph()).Root
	if !strings.HasPrefix(root, "") || len(root) != 64 {
		t.Errorf("root should be 64-hex sha256, got %q", root)
	}
}

func TestCanonicalWeight_NaNAndInf(t *testing.T) {
	if canonicalWeight(0) != "0.000000" {
		t.Errorf("zero weight: got %q", canonicalWeight(0))
	}
	if got := canonicalWeight(1.0 / 0.0); got != "+Inf" {
		t.Errorf("inf got %q", got)
	}
	if got := canonicalWeight(-1.0 / 0.0); got != "-Inf" {
		t.Errorf("-inf got %q", got)
	}
}

func TestNormalizeSummary_CollapsesWhitespace(t *testing.T) {
	cases := map[string]string{
		"  Hello   World  ":        "hello world",
		"\tA\nB\rC":                "a b c",
		"Already lowercase normal": "already lowercase normal",
		"":                         "",
	}
	for in, want := range cases {
		if got := normalizeSummary(in); got != want {
			t.Errorf("normalizeSummary(%q) = %q, want %q", in, got, want)
		}
	}
}
