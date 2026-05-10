#!/usr/bin/env python3
"""Deterministic local GIST kernel.

This is intentionally small, local, and replayable. It does not claim full
Pearl do-calculus; it preserves the GIST symmetry contract Agency needs first:
canonical 64-slot lattice, sparse activation, dyad/triad closures,
contradiction clusters, intervention/counterfactual branches, and a trace.
"""

from __future__ import annotations

import hashlib
import json
import re
import sys
from typing import Any, Dict, Iterable, List, Tuple


TEMPORAL = ("past", "present", "future", "meta")
ABSTRACTION = ("micro", "meso", "macro", "system")
EVIDENCE = ("empirical", "theoretical", "computational", "synthetic")

NEGATIVE_MARKERS = (
    "do not",
    "don't",
    "block",
    "blocked",
    "fail",
    "failed",
    "failing",
    "unsafe",
    "risk",
    "risky",
    "cannot",
    "must not",
    "not ready",
    "until tests",
)
POSITIVE_MARKERS = (
    "go",
    "ship",
    "publish",
    "deploy",
    "ready",
    "pass",
    "passed",
    "safe",
    "approve",
    "dispatch",
)
ACTION_MARKERS = ("publish", "deploy", "delete", "write", "run", "ship", "dispatch", "commit", "push")
TOOL_HINTS = {
    "test": "test-runner",
    "tests": "test-runner",
    "build": "build-runner",
    "deploy": "deployment",
    "publish": "release",
    "review": "review",
    "code": "filesystem",
}


def stable_hash(*parts: Any, length: int = 16) -> str:
    payload = "\0".join(json.dumps(part, sort_keys=True, separators=(",", ":")) for part in parts)
    return hashlib.sha256(payload.encode("utf-8")).hexdigest()[:length]


def norm(text: str) -> str:
    return re.sub(r"\s+", " ", text.strip().lower())


def atom_id(atom: Dict[str, Any], index: int) -> str:
    existing = str(atom.get("id") or "").strip()
    if existing:
        return existing
    return "atom_" + stable_hash(atom.get("kind", ""), atom.get("content", ""), index, length=16)


def canonical_slots() -> List[Dict[str, Any]]:
    slots: List[Dict[str, Any]] = []
    for temporal_idx, temporal in enumerate(TEMPORAL):
        for abstraction_idx, abstraction in enumerate(ABSTRACTION):
            for evidence_idx, evidence in enumerate(EVIDENCE):
                index = temporal_idx * 16 + abstraction_idx * 4 + evidence_idx
                bits = f"{index:06b}"
                slots.append(
                    {
                        "id": f"L{index:02d}",
                        "index": index,
                        "temporal": temporal,
                        "abstraction": abstraction,
                        "evidence": evidence,
                        "bits": bits,
                        "active": False,
                        "atomRefs": [],
                        "weight": 0.0,
                        "metrics": {"activation": 0.0, "contradiction": 0.0, "support": 0.0},
                    }
                )
    return slots


def slot_index_for(atom: Dict[str, Any]) -> int:
    meta = atom.get("meta") or {}
    content = norm(str(atom.get("content", "")))
    kind = norm(str(atom.get("kind", "")))
    temporal_idx = 1
    if any(word in content for word in ("previous", "past", "history", "ledger")):
        temporal_idx = 0
    elif any(word in content for word in ("next", "future", "later", "would")):
        temporal_idx = 2
    elif str(meta.get("scale", "")) in ("office", "system") or kind in ("lattice_state", "agent_lattice_state"):
        temporal_idx = 3

    abstraction_idx = {"event": 0, "agent": 1, "office": 2, "system": 3}.get(str(meta.get("scale", "")), 1)
    if kind in ("ledger_sequence", "lattice_state", "agent_lattice_state"):
        abstraction_idx = 3
    elif kind in ("pending_task", "directive"):
        abstraction_idx = 2

    evidence_idx = stable_hash(kind, content, length=2)
    evidence_idx = int(evidence_idx, 16) % 4
    if kind in ("signal", "signal_payload"):
        evidence_idx = 0
    elif kind in ("directive", "pending_task"):
        evidence_idx = 2
    elif kind in ("lattice_state", "agent_lattice_state"):
        evidence_idx = 3

    return temporal_idx * 16 + abstraction_idx * 4 + evidence_idx


def activate_slots(slots: List[Dict[str, Any]], atoms: List[Dict[str, Any]]) -> List[str]:
    active: List[str] = []
    seen = set()
    for index, atom in enumerate(atoms):
        aid = atom_id(atom, index)
        idx = slot_index_for(atom)
        slot = slots[idx]
        if aid not in slot["atomRefs"]:
            slot["atomRefs"].append(aid)
        weight = float(atom.get("weight") or 0.0)
        slot["active"] = True
        slot["weight"] = round(float(slot["weight"]) + weight, 4)
        slot["metrics"]["activation"] = round(min(1.0, slot["weight"] / 3.0), 4)
        if idx not in seen:
            active.append(slot["id"])
            seen.add(idx)
    return active


def polarity(atom: Dict[str, Any]) -> str:
    content = norm(str(atom.get("content", "")))
    neg = any(marker in content for marker in NEGATIVE_MARKERS)
    pos = any(marker in content for marker in POSITIVE_MARKERS)
    if neg and not pos:
        return "negative"
    if pos and not neg:
        return "positive"
    if pos and neg:
        return "mixed"
    return "neutral"


def find_contradictions(atoms: List[Dict[str, Any]], atom_slots: Dict[str, str]) -> List[Dict[str, Any]]:
    positives: List[Tuple[str, Dict[str, Any]]] = []
    negatives: List[Tuple[str, Dict[str, Any]]] = []
    for index, atom in enumerate(atoms):
        aid = atom_id(atom, index)
        p = polarity(atom)
        if p in ("positive", "mixed"):
            positives.append((aid, atom))
        if p in ("negative", "mixed"):
            negatives.append((aid, atom))

    contradictions: List[Dict[str, Any]] = []
    if positives and negatives:
        pos_ids = [aid for aid, _ in positives[:3]]
        neg_ids = [aid for aid, _ in negatives[:3]]
        ids = pos_ids + [aid for aid in neg_ids if aid not in pos_ids]
        slots = sorted({atom_slots.get(aid, "") for aid in ids if atom_slots.get(aid, "")})
        contradictions.append(
            {
                "id": "contra_" + stable_hash(ids, length=12),
                "summary": "Action pressure conflicts with blocking or cautionary evidence.",
                "severity": "high" if any("publish" in norm(str(a.get("content", ""))) or "deploy" in norm(str(a.get("content", ""))) for _, a in positives) else "medium",
                "kind": "action_constraint_conflict",
                "status": "open",
                "atoms": ids,
                "atomRefs": ids,
                "slotIds": slots,
                "evidenceNeeded": ["passing test evidence", "explicit approval for consequential action"],
                "blocking": True,
            }
        )
    return contradictions


def infer_required_tools(atoms: Iterable[Dict[str, Any]]) -> List[str]:
    tools = set()
    for atom in atoms:
        content = norm(str(atom.get("content", "")))
        for marker, tool in TOOL_HINTS.items():
            if marker in content:
                tools.add(tool)
    return sorted(tools)


def extract_action(atoms: Iterable[Dict[str, Any]]) -> str:
    combined = " ".join(norm(str(atom.get("content", ""))) for atom in atoms)
    for marker in ACTION_MARKERS:
        if marker in combined:
            return marker
    return "act"


def build_counterfactuals(atoms: List[Dict[str, Any]], contradictions: List[Dict[str, Any]]) -> Tuple[List[Dict[str, Any]], List[Dict[str, Any]]]:
    action = extract_action(atoms)
    risky = action in ("publish", "deploy", "delete", "push", "ship") or bool(contradictions)
    tests = ["run release smoke", "verify ledger replay"] if risky else ["verify local outcome"]
    interventions = [
        {
            "id": "do_" + stable_hash(action, length=10),
            "label": action,
            "do": f"do({action})",
            "actionAtomRef": next((atom_id(atom, idx) for idx, atom in enumerate(atoms) if action in norm(str(atom.get("content", "")))), ""),
            "assumptions": ["current evidence packet is complete enough for one-step branch comparison"],
            "expectedEffects": ["progresses requested work"],
            "risks": ["may amplify unresolved contradiction"] if contradictions else [],
            "confidence": 0.55 if contradictions else 0.75,
        },
        {
            "id": "not_do_" + stable_hash(action, length=10),
            "label": f"not_{action}",
            "do": f"do(not_{action})",
            "assumptions": ["deferral is allowed by user intent"],
            "expectedEffects": ["preserves current state"],
            "risks": ["delays delivery"],
            "confidence": 0.7,
        },
    ]
    counterfactuals = [
        {
            "id": "cf_" + stable_hash("do", action, length=10),
            "interventionId": interventions[0]["id"],
            "branchKind": "do",
            "if": f"do({action})",
            "then": "execution can proceed only if blocking contradictions are repaired first" if contradictions else "expected path is low-risk continuation",
            "risk": "high" if contradictions else "low",
            "riskLevel": "high" if contradictions else "low",
            "expectedUtility": 0.45 if contradictions else 0.78,
            "unknowns": ["whether tests pass"] if contradictions else [],
            "evidenceNeeded": ["release proof"] if contradictions else [],
            "tests": tests,
        },
        {
            "id": "cf_" + stable_hash("not", action, length=10),
            "interventionId": interventions[1]["id"],
            "branchKind": "not_do",
            "if": f"do(not_{action})",
            "then": "state remains stable but user intent is not fulfilled",
            "risk": "medium",
            "riskLevel": "medium",
            "expectedUtility": 0.35,
            "unknowns": ["whether delay is acceptable"],
            "evidenceNeeded": ["user confirmation for deferral"],
            "tests": ["confirm deferral is intentional"],
        },
        {
            "id": "cf_" + stable_hash("review", action, length=10),
            "branchKind": "instead",
            "if": "do(request_review)",
            "then": "contradiction can be adjudicated before consequential action",
            "risk": "low",
            "riskLevel": "low",
            "expectedUtility": 0.72,
            "unknowns": [],
            "evidenceNeeded": ["review result"],
            "tests": ["review contradiction cluster", "rerun GIST replay"],
        },
    ]
    return interventions, counterfactuals


def closures(active_slots: List[str], atoms: List[Dict[str, Any]], atom_slots: Dict[str, str]) -> Tuple[List[Dict[str, Any]], List[Dict[str, Any]]]:
    atom_refs = [atom_id(atom, idx) for idx, atom in enumerate(atoms)]
    triads: List[Dict[str, Any]] = []
    dyads: List[Dict[str, Any]] = []
    for i in range(0, min(len(active_slots), 9), 3):
        group = active_slots[i : i + 3]
        if len(group) == 3:
            triads.append(
                {
                    "id": "triad_" + stable_hash(group, length=10),
                    "kind": "triad",
                    "arity": 3,
                    "relation": "explains",
                    "slotIds": group,
                    "inputSlotIds": group,
                    "atomRefs": [ref for ref in atom_refs if atom_slots.get(ref) in group],
                    "inputAtomRefs": [ref for ref in atom_refs if atom_slots.get(ref) in group],
                    "outputSlotId": group[-1],
                    "summary": "triadic closure over active causal slots",
                    "weight": round(len(group) / 3.0, 4),
                    "score": round(len(group) / 3.0, 4),
                    "selected": True,
                }
            )
    for i in range(0, min(len(active_slots), 8), 2):
        group = active_slots[i : i + 2]
        if len(group) == 2:
            dyads.append(
                {
                    "id": "dyad_" + stable_hash(group, length=10),
                    "kind": "dyad",
                    "arity": 2,
                    "relation": "contradicts" if len(group) == 2 else "supports",
                    "slotIds": group,
                    "inputSlotIds": group,
                    "atomRefs": [ref for ref in atom_refs if atom_slots.get(ref) in group],
                    "inputAtomRefs": [ref for ref in atom_refs if atom_slots.get(ref) in group],
                    "outputSlotId": group[-1],
                    "summary": "dyadic compression between adjacent active slots",
                    "weight": 1.0,
                    "score": 1.0,
                    "selected": True,
                }
            )
    return triads, dyads


def main() -> int:
    raw = sys.stdin.read()
    payload = json.loads(raw or "{}")
    atoms = payload.get("atoms") or []
    agent_id = payload.get("agentId") or "unknown"
    org_id = payload.get("organizationId") or ""
    if not org_id:
        for atom in atoms:
            org_id = (atom.get("meta") or {}).get("organizationId") or org_id
    scope = payload.get("scope") or {
        "kind": "agent_local",
        "organizationId": org_id,
        "agentId": agent_id,
        "parentKind": "office_fractal",
        "parentId": f"office:{org_id or 'default'}",
    }

    for idx, atom in enumerate(atoms):
        atom["id"] = atom_id(atom, idx)

    slots = canonical_slots()
    active_slots = activate_slots(slots, atoms)
    atom_slots = {atom_id(atom, idx): slots[slot_index_for(atom)]["id"] for idx, atom in enumerate(atoms)}
    contradictions = find_contradictions(atoms, atom_slots)
    interventions, counterfactuals = build_counterfactuals(atoms, contradictions)
    triad_closures, dyad_closures = closures(active_slots, atoms, atom_slots)

    for contradiction in contradictions:
        for slot_id in contradiction.get("slotIds", []):
            idx = int(slot_id[1:])
            slots[idx]["metrics"]["contradiction"] = 1.0
            slots[idx].setdefault("contradictionIds", []).append(contradiction["id"])
    for slot in slots:
        if slot["atomRefs"]:
            slot["metrics"]["support"] = round(min(1.0, len(slot["atomRefs"]) / 3.0), 4)

    lattice = {
        "version": "gist-lattice-v1",
        "scope": {
            "kind": "office_fractal",
            "organizationId": org_id,
            "parentKind": "agent_local",
            "parentId": agent_id,
        },
        "canonicalSlots": 64,
        "slots": slots,
        "activeSlots": active_slots,
        "updatedAt": 0,
    }
    input_hash = stable_hash(atoms, payload.get("budget") or {}, length=16)
    lattice_hash = stable_hash(lattice, length=16)
    trace_id = "gist_" + stable_hash(agent_id, input_hash, lattice_hash, length=16)
    lattice["lastTraceId"] = trace_id

    contradiction_ids = [item["id"] for item in contradictions]
    intervention_ids = [item["id"] for item in interventions]
    counterfactual_ids = [item["id"] for item in counterfactuals]
    lattice_diff = {
        "activatedSlots": active_slots,
        "updatedSlots": [
            {
                "slotId": slot_id,
                "addedAtomRefs": slots[int(slot_id[1:])]["atomRefs"],
                "weightDelta": slots[int(slot_id[1:])]["weight"],
                "metricDelta": slots[int(slot_id[1:])]["metrics"],
            }
            for slot_id in active_slots
        ],
    }
    trace = {
        "id": trace_id,
        "agentId": agent_id,
        "organizationId": org_id,
        "scope": scope,
        "atoms": atoms,
        "atomCount": len(atoms),
        "inputHash": input_hash,
        "nextLatticeHash": lattice_hash,
        "latticeHash": lattice_hash,
        "activeSlots": active_slots,
        "triadClosures": triad_closures,
        "dyadClosures": dyad_closures,
        "contradictionIds": contradiction_ids,
        "interventionIds": intervention_ids,
        "counterfactualIds": counterfactual_ids,
        "latticeDiff": lattice_diff,
        "selectedChain": [atom_id(atom, idx) for idx, atom in enumerate(sorted(atoms, key=lambda a: float(a.get("weight") or 0.0), reverse=True)[:5])],
        "selectedVerdict": "causal_review_required" if contradictions else "causal_path_clear",
        "replayHandle": f"{trace_id}:{input_hash}:{lattice_hash}",
        "createdAt": 0,
    }

    required_tools = infer_required_tools(atoms)
    risk_level = "high" if any(c.get("severity") == "high" for c in contradictions) else ("medium" if contradictions else "low")
    confidence = 0.78
    if contradictions:
        confidence = 0.48 if risk_level == "high" else 0.58
    if len(active_slots) <= 1:
        confidence = min(confidence, 0.4)

    verdict = "causal_review_required" if contradictions else "causal_path_clear"
    execution_intent = "causal_review" if contradictions else "low_risk_dispatch"
    confidence_breakdown = {
        "coverage": round(min(1.0, len(active_slots) / 8.0), 4),
        "contradictionPenalty": 0.35 if contradictions else 0.0,
        "replayability": 1.0,
        "symmetry": 1.0 if len(slots) == 64 else 0.0,
    }
    trace["confidenceBreakdown"] = confidence_breakdown
    proof = {
        "version": "gist-proof-v1",
        "traceId": trace_id,
        "verdict": verdict,
        "confidence": confidence,
        "inputHash": input_hash,
        "nextLatticeHash": lattice_hash,
        "latticeDiff": lattice_diff,
        "contradictionIds": contradiction_ids,
        "interventionIds": intervention_ids,
        "counterfactualIds": counterfactual_ids,
        "confidenceBreakdown": confidence_breakdown,
    }
    intent = {
        "taskType": execution_intent,
        "complexity": confidence,
        "latencyBudgetMs": 5000,
        "privacyLevel": "local",
        "requiredTools": required_tools,
    }
    open_questions = []
    if contradictions:
        open_questions.append("Resolve contradiction cluster before consequential action.")
    if not required_tools:
        open_questions.append("No concrete tool requirement was causally identified.")

    output = {
        "verdict": verdict,
        "confidence": confidence,
        "causalChain": trace["selectedChain"],
        "openQuestions": open_questions,
        "executionIntent": execution_intent,
        "intent": intent,
        "riskLevel": risk_level,
        "requiredTools": required_tools,
        "lattice": lattice,
        "trace": trace,
        "proof": proof,
        "contradictions": contradictions,
        "interventions": interventions,
        "counterfactuals": counterfactuals,
        "confidenceBreakdown": confidence_breakdown,
        "latticeJson": json.dumps(lattice, sort_keys=True, separators=(",", ":")),
    }
    sys.stdout.write(json.dumps(output, sort_keys=True, separators=(",", ":")))
    return 0


if __name__ == "__main__":
    raise SystemExit(main())
