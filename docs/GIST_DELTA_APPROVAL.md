# GIST Delta Approval Packet

Date: 2026-05-10

Repo head inspected: `c828e92df8f7acdb27a5271d8175bc60ea3dd841`

## Inspected Agency GIST Before This Pass

At repo head `c828e92df8f7acdb27a5271d8175bc60ea3dd841`, Agency shipped a
Go-side `GISTAgentCore` wrapper that:

- builds weighted atoms from actor identity, wake signal, ledger sequence, pending tasks, signal payload, and prior lattice JSON;
- calls an expected Python subprocess at `scripts/gist_subprocess.py`;
- receives `verdict`, `confidence`, `causalChain`, `openQuestions`, `executionIntent`, and `latticeJson`;
- boosts confidence through a simple freshness-based `ElasticStretch`;
- prefixes the actor LLM prompt and model-routing intent with the resulting verdict.

Important gap at inspection time: no `gist_subprocess.py` was present in the
pushed repo, so the default path degraded to `proceed_with_caution` and a
low-confidence causal chain of `subprocess unavailable`.

## Current Implementation Status

This pass lands the first symmetry-faithful GIST kernel for Agency:

- bundled deterministic `scripts/gist_subprocess.py`;
- typed `GISTLattice`, `GISTSlot`, `GISTTrace`, `GISTClosure`,
  `GISTContradiction`, `GISTIntervention`, and `GISTCounterfactual` contracts;
- canonical 4x4x4 / 64-slot lattice with sparse runtime activation;
- per-agent local GIST plus office-macro lattice persistence through the same
  `LatticeStore` grammar;
- contradiction clusters that remain first-class instead of being averaged into
  confidence;
- `do(action)` intervention records and counterfactual branches;
- replay handles, input hashes, lattice hashes, confidence breakdown, risk
  level, and required tool hints;
- degraded fallback explicitly marked as degraded when the engine is unavailable;
- GIST prompt context expanded beyond a one-line verdict into trace,
  causal-chain, contradiction, counterfactual, and open-question context.

## Mobius Target

Mobius positions GIST as the owner of causal interpretation: a standalone causal
reasoning product and organ that turns events into atoms, contradictions,
counterfactuals, lattices, and verdicts. In full Mobius, GIST is also the
elastic causal adjudicator for cross-scale disagreement and, at OneMind scale,
the causal attribution engine for contribution necessity and anti-gaming.

## Delta

Agency has:

- atom collection,
- lattice persistence hook,
- a verdict envelope,
- prompt injection weighting,
- model-routing integration,
- ledger-adjacent audit metadata.

Agency still lacks:

- exported public causal atom schemas beyond the internal subprocess envelope,
- abduction-action-prediction cycles,
- richer causal graph update rules beyond the V1 lattice slot activation,
- confounder/evidence distinction,
- causal necessity attribution,
- dispute/adjudication semantics,
- persisted trace/proof table separate from lattice JSON,
- inspector-visible reasoning traces in the TUI.

## Approval List

1. Implement a real local GIST subprocess first. **Done in this pass.**
   - Deterministic Python or Go engine, no API key required.
   - Reads current atom envelope and returns real structured verdicts.

2. Replace freeform atoms with typed causal atoms.
   - `event`, `actor`, `action`, `claim`, `evidence`, `constraint`, `outcome`, `contradiction`, `intervention`, `counterfactual`.

3. Add a causal graph/lattice model.
   - Nodes are causal atoms.
   - Edges encode `causes`, `enables`, `blocks`, `requires`, `contradicts`, `supports`, `updates`, `explains`.

4. Add contradiction maps. **V1 done in this pass.**
   - Detect conflicting claims/actions/outcomes.
   - Return contradiction clusters and severity in `GISTVerdict`.

5. Add Pearl-style intervention support. **V1 intervention records done in this pass.**
   - Represent `do(action)` as a first-class atom.
   - Estimate expected downstream effects from local ledger history and current constraints.

6. Add counterfactual branch simulation. **V1 branch emission done in this pass.**
   - For each candidate action: `if do(A)`, `if not do(A)`, `if do(B instead)`.
   - Return branch risks, expected utility, unknowns, and needed evidence.

7. Add abduction-action-prediction loop.
   - Abduction: infer likely hidden causes of current state.
   - Action: propose intervention candidates.
   - Prediction: forecast likely consequences and observation tests.

8. Add causal necessity attribution.
   - Score whether an atom was necessary for a downstream outcome.
   - Keep local version simple now; preserve path to OneMind/global attribution later.

9. Add GIST proof packets. **V1 `GISTTrace` done in this pass; durable trace table remains next.**
   - Persist replayable `GISTTrace` entries: input atoms, graph diff, contradictions, counterfactuals, selected verdict, confidence decomposition.

10. Wire GIST into Director policy and Agency approvals.
   - Director high/unknown risk should ask GIST: "what causal unknowns make this unsafe?"
   - Approval lane should show GIST reason, not just action type.

11. Add fixture-driven tests. **First anti-theater suite done in this pass.**
   - Conflicting instructions.
   - Risky publish action.
   - Failed test after code change.
   - Two alternative actions with different downstream risks.
   - Provenance/necessity replay.

12. Update README claims after implementation. **Done in this pass.**
   - Either call current GIST a scaffold honestly, or ship the engine and promote the claim.

## Recommended First Build Slice

Build `scripts/gist_subprocess.py` plus Go schema expansion and tests. Keep it
local, deterministic, and small. The first useful version should not try to be
omniscient; it should identify contradictions, model one-step interventions,
generate counterfactual branches, and emit replayable traces.
