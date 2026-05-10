package agency

import (
	"testing"
)

func dyadVerdict(id string, mutate func(g *CausalGraph)) LabeledVerdict {
	g := merkleSampleGraph()
	if mutate != nil {
		mutate(g)
	}
	v := GISTVerdict{Verdict: "test", Confidence: 0.8, CausalGraph: g}
	v.SyncCausalChain()
	return LabeledVerdict{ID: id, Verdict: v}
}

func TestDyadCompress_Empty(t *testing.T) {
	out, report := DyadCompress(nil)
	if len(out) != 0 || report.SlotsBefore != 0 || report.SlotsAfter != 0 {
		t.Errorf("empty input should produce empty output, got %#v %#v", out, report)
	}
}

func TestDyadCompress_SingleVerdict_Unpaired(t *testing.T) {
	v := dyadVerdict("solo", nil)
	out, report := DyadCompress([]LabeledVerdict{v})
	if len(out) != 1 || report.SlotsAfter != 1 {
		t.Errorf("single verdict should be unpaired, got SlotsAfter=%d", report.SlotsAfter)
	}
	if len(report.UnpairedIDs) != 1 || report.UnpairedIDs[0] != "solo" {
		t.Errorf("expected solo in UnpairedIDs, got %v", report.UnpairedIDs)
	}
}

func TestDyadCompress_TwoIdentical_NotPaired(t *testing.T) {
	// Identical verdicts have Hamming distance 0, not 1 — they should NOT
	// be paired (compression of duplicates is a different mechanism).
	a := dyadVerdict("a", nil)
	b := dyadVerdict("b", nil)
	_, report := DyadCompress([]LabeledVerdict{a, b})
	if report.SlotsAfter != 2 || len(report.Deltas) != 0 {
		t.Errorf("identical verdicts should not be paired (Hamming-0), got SlotsAfter=%d deltas=%d",
			report.SlotsAfter, len(report.Deltas))
	}
}

func TestDyadCompress_MutationPair_SlotCountHalves(t *testing.T) {
	a := dyadVerdict("a", nil)
	b := dyadVerdict("b", func(g *CausalGraph) {
		// Flip one node's weight — Hamming-1 mutation.
		g.Nodes[0].Weight = 0.5
	})
	out, report := DyadCompress([]LabeledVerdict{a, b})
	if report.SlotsAfter != 1 {
		t.Errorf("expected 1 slot after compression, got %d", report.SlotsAfter)
	}
	if len(report.Deltas) != 1 {
		t.Fatalf("expected 1 delta, got %d", len(report.Deltas))
	}
	d := report.Deltas[0]
	if d.NodeMutation == nil || d.NodeMutation.NodeID != "ev1" {
		t.Errorf("expected mutation on ev1, got %#v", d.NodeMutation)
	}
	if d.NodeMutation.OldWeight == nil || d.NodeMutation.NewWeight == nil {
		t.Errorf("expected weight pointers populated for weight mutation")
	}
	if len(out) != 1 {
		t.Errorf("expected 1 output base, got %d", len(out))
	}
}

func TestDyadCompress_AddedNodePair(t *testing.T) {
	a := dyadVerdict("a", nil)
	b := dyadVerdict("b", func(g *CausalGraph) {
		g.Nodes = append(g.Nodes, CausalNode{ID: "extra", Role: NodeRoleEvidence, Summary: "extra atom", Weight: 0.3})
	})
	_, report := DyadCompress([]LabeledVerdict{a, b})
	if len(report.Deltas) != 1 {
		t.Fatalf("expected 1 delta, got %d", len(report.Deltas))
	}
	d := report.Deltas[0]
	if d.NodeAdded == nil || d.NodeAdded.ID != "extra" {
		t.Errorf("expected NodeAdded with id 'extra', got %#v", d.NodeAdded)
	}
}

func TestDyadCompress_RemovedNodePair(t *testing.T) {
	a := dyadVerdict("a", nil)
	b := dyadVerdict("b", func(g *CausalGraph) {
		g.Nodes = g.Nodes[:len(g.Nodes)-1]
	})
	_, report := DyadCompress([]LabeledVerdict{a, b})
	if len(report.Deltas) != 1 {
		t.Fatalf("expected 1 delta, got %d", len(report.Deltas))
	}
	d := report.Deltas[0]
	if d.NodeRemoved == nil || *d.NodeRemoved != "out1" {
		t.Errorf("expected NodeRemoved 'out1', got %#v", d.NodeRemoved)
	}
}

func TestDyadCompress_Hamming2_NotPaired(t *testing.T) {
	a := dyadVerdict("a", nil)
	b := dyadVerdict("b", func(g *CausalGraph) {
		g.Nodes[0].Weight = 0.5  // mutation 1
		g.Nodes[1].Role = NodeRoleEvidence // mutation 2
	})
	_, report := DyadCompress([]LabeledVerdict{a, b})
	if len(report.Deltas) != 0 {
		t.Errorf("Hamming-2 verdicts should not be paired, got %d deltas", len(report.Deltas))
	}
	if report.SlotsAfter != 2 {
		t.Errorf("Hamming-2 verdicts should retain both slots, got %d", report.SlotsAfter)
	}
}

func TestDyadCompress_GreedyOneDyadPerVerdict(t *testing.T) {
	// Three verdicts, each pair-wise Hamming-1 with the next one.
	// Greedy pairing should produce 1 dyad and 1 unpaired.
	a := dyadVerdict("a", nil)
	b := dyadVerdict("b", func(g *CausalGraph) { g.Nodes[0].Weight = 0.5 })
	c := dyadVerdict("c", func(g *CausalGraph) { g.Nodes[0].Weight = 0.5; g.Nodes[1].Role = NodeRoleEvidence })
	out, report := DyadCompress([]LabeledVerdict{a, b, c})
	if report.SlotsAfter != 2 {
		t.Errorf("greedy 3-verdict run should produce 2 slots (1 dyad + 1 solo), got %d", report.SlotsAfter)
	}
	if len(report.Deltas) != 1 {
		t.Errorf("greedy run should produce exactly 1 delta, got %d", len(report.Deltas))
	}
	if len(out) != 2 {
		t.Errorf("expected 2 base verdicts, got %d", len(out))
	}
}

func TestDyadHydrate_RoundTripsMutation(t *testing.T) {
	a := dyadVerdict("a", nil)
	b := dyadVerdict("b", func(g *CausalGraph) {
		g.Nodes[0].Weight = 0.42
		g.Nodes[0].Summary = "renamed summary"
	})
	_, report := DyadCompress([]LabeledVerdict{a, b})
	if len(report.Deltas) != 1 {
		t.Fatalf("expected 1 delta")
	}
	rehydrated, err := DyadHydrate(a, report.Deltas[0])
	if err != nil {
		t.Fatalf("hydrate error: %v", err)
	}
	if MerkleAttest(rehydrated.Verdict.CausalGraph).Root != MerkleAttest(b.Verdict.CausalGraph).Root {
		t.Errorf("rehydrated sibling did not match original by Merkle root")
	}
	if rehydrated.ID != "b" {
		t.Errorf("rehydrated ID = %q, want b", rehydrated.ID)
	}
}

func TestDyadHydrate_RoundTripsAddition(t *testing.T) {
	a := dyadVerdict("a", nil)
	b := dyadVerdict("b", func(g *CausalGraph) {
		g.Nodes = append(g.Nodes, CausalNode{ID: "newone", Role: NodeRoleEvidence, Summary: "added atom", Weight: 0.4})
	})
	_, report := DyadCompress([]LabeledVerdict{a, b})
	if len(report.Deltas) != 1 {
		t.Fatalf("expected 1 delta")
	}
	rehydrated, err := DyadHydrate(a, report.Deltas[0])
	if err != nil {
		t.Fatalf("hydrate error: %v", err)
	}
	if MerkleAttest(rehydrated.Verdict.CausalGraph).Root != MerkleAttest(b.Verdict.CausalGraph).Root {
		t.Errorf("rehydrated sibling did not match original by Merkle root")
	}
}

func TestDyadHydrate_RoundTripsRemoval(t *testing.T) {
	a := dyadVerdict("a", nil)
	b := dyadVerdict("b", func(g *CausalGraph) {
		g.Nodes = g.Nodes[:len(g.Nodes)-1]
	})
	_, report := DyadCompress([]LabeledVerdict{a, b})
	if len(report.Deltas) != 1 {
		t.Fatalf("expected 1 delta")
	}
	rehydrated, err := DyadHydrate(a, report.Deltas[0])
	if err != nil {
		t.Fatalf("hydrate error: %v", err)
	}
	if MerkleAttest(rehydrated.Verdict.CausalGraph).Root != MerkleAttest(b.Verdict.CausalGraph).Root {
		t.Errorf("rehydrated sibling did not match original by Merkle root")
	}
}

func TestDyadHydrate_NilBase_Errors(t *testing.T) {
	_, err := DyadHydrate(LabeledVerdict{ID: "x"}, DyadDelta{})
	if err == nil {
		t.Errorf("expected error for nil base graph")
	}
}

func TestDyadCompress_DoesNotMutateOriginalGraphs(t *testing.T) {
	a := dyadVerdict("a", nil)
	b := dyadVerdict("b", func(g *CausalGraph) { g.Nodes[0].Weight = 0.5 })
	beforeA := MerkleAttest(a.Verdict.CausalGraph).Root
	beforeB := MerkleAttest(b.Verdict.CausalGraph).Root
	_, _ = DyadCompress([]LabeledVerdict{a, b})
	if MerkleAttest(a.Verdict.CausalGraph).Root != beforeA {
		t.Errorf("compression mutated A's graph")
	}
	if MerkleAttest(b.Verdict.CausalGraph).Root != beforeB {
		t.Errorf("compression mutated B's graph")
	}
}
