package agency

import (
	"bytes"
	"context"
	"crypto/sha256"
	"encoding/json"
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"time"
)

// gistAtom keeps existing internal call sites compact while the public
// subprocess contract exposes GISTAtom.
type gistAtom = GISTAtom

// gistSubprocessInput is the JSON envelope sent to the Python subprocess on stdin.
type gistSubprocessInput struct {
	AgentID        string          `json:"agentId"`
	OrganizationID string          `json:"organizationId"`
	Scope          GISTScopeRef    `json:"scope"`
	Atoms          []gistAtom      `json:"atoms"`
	PriorLattice   json.RawMessage `json:"priorLattice,omitempty"`
	Budget         ElasticBudget   `json:"budget"`
}

// gistSubprocessOutput is the JSON envelope returned by the subprocess on stdout.
type gistSubprocessOutput struct {
	Verdict             string               `json:"verdict"`
	Confidence          float64              `json:"confidence"`
	CausalChain         []string             `json:"causalChain,omitempty"`
	CausalGraph         *CausalGraph         `json:"causalGraph,omitempty"`
	OpenQuestions       []string             `json:"openQuestions,omitempty"`
	ExecutionIntent     string               `json:"executionIntent"`
	Intent              *ActionIntent        `json:"intent,omitempty"`
	RiskLevel           string               `json:"riskLevel,omitempty"`
	RequiredTools       []string             `json:"requiredTools,omitempty"`
	Lattice             *GISTLattice         `json:"lattice,omitempty"`
	Trace               *GISTTrace           `json:"trace,omitempty"`
	Proof               *GISTProofPacket     `json:"proof,omitempty"`
	Contradictions      []GISTContradiction  `json:"contradictions,omitempty"`
	Interventions       []GISTIntervention   `json:"interventions,omitempty"`
	Counterfactuals     []GISTCounterfactual `json:"counterfactuals,omitempty"`
	ConfidenceBreakdown map[string]float64   `json:"confidenceBreakdown,omitempty"`
	LatticeJSON         string               `json:"latticeJson,omitempty"`
	Error               string               `json:"error,omitempty"`
}

// GISTAgentCore manages the per-agent GIST Python subprocess.
// It provides causal compression of observations into GISTVerdict values
// that are used to prefix LLM calls.
//
// If the subprocess is unavailable the core degrades gracefully, returning a
// low-confidence verdict so the actor can still act.
type GISTAgentCore struct {
	agentID    string
	scriptPath string
	budget     ElasticBudget

	// latticeJSON is the persisted lattice state from the last wake cycle.
	latticeJSON string
}

// NewGISTAgentCore creates a new core for the given agent.
// scriptPath should point to the gist Python entry-point
// (e.g. scripts/gist_subprocess.py). If the script does not exist the
// core will return degraded verdicts on every call.
func NewGISTAgentCore(agentID, scriptPath string, budget ElasticBudget) *GISTAgentCore {
	return &GISTAgentCore{
		agentID:    agentID,
		scriptPath: scriptPath,
		budget:     budget,
	}
}

// DefaultGISTBudget reads elastic budget settings from environment variables.
func DefaultGISTBudget() ElasticBudget {
	threshold := 0.3
	maxTTL := int64(30000)
	if v := os.Getenv("GIST_ELASTIC_RECALL_THRESHOLD"); v != "" {
		var f float64
		if _, err := fmt.Sscanf(v, "%f", &f); err == nil && f > 0 {
			threshold = f
		}
	}
	if v := os.Getenv("GIST_ELASTIC_MAX_TTL_MS"); v != "" {
		var n int64
		if _, err := fmt.Sscanf(v, "%d", &n); err == nil && n > 0 {
			maxTTL = n
		}
	}
	return ElasticBudget{
		RecallThreshold: threshold,
		MaxTTLMs:        maxTTL,
		StretchFactor:   1.0,
	}
}

// BuildAtoms converts an observation snapshot and wake signal into GIST atoms.
func (g *GISTAgentCore) BuildAtoms(obs ObservationSnapshot, signal WakeSignal) []gistAtom {
	atoms := []gistAtom{
		{
			ID:        stableAtomID("actor_identity", obs.Actor.ID),
			Kind:      "actor_identity",
			Content:   fmt.Sprintf("role=%s id=%s", obs.Actor.Role, obs.Actor.ID),
			Scope:     "agent",
			SubjectID: obs.Actor.ID,
			Predicate: "has_role",
			Value:     obs.Actor.Role,
			Weight:    1.0,
			Meta: map[string]string{
				"agentId":        obs.Actor.ID,
				"organizationId": obs.OrganizationID,
				"scale":          "agent",
			},
		},
		{
			ID:        stableAtomID("signal", signal.ID),
			Kind:      "signal",
			Content:   fmt.Sprintf("kind=%s channel=%s id=%s", signal.Kind, signal.Channel, signal.ID),
			Scope:     "event",
			SubjectID: signal.ID,
			Predicate: "received_on",
			Value:     signal.Channel,
			Weight:    0.9,
			Meta: map[string]string{
				"signalId":       signal.ID,
				"organizationId": signal.OrganizationID,
				"scale":          "event",
			},
		},
		{
			ID:        stableAtomID("ledger_sequence", fmt.Sprintf("%d", obs.LedgerSequence)),
			Kind:      "ledger_sequence",
			Content:   fmt.Sprintf("%d", obs.LedgerSequence),
			Scope:     "office",
			SubjectID: obs.OrganizationID,
			Predicate: "ledger_sequence",
			Value:     fmt.Sprintf("%d", obs.LedgerSequence),
			Weight:    0.5,
			Meta: map[string]string{
				"organizationId": obs.OrganizationID,
				"scale":          "office",
			},
		},
	}

	for _, task := range obs.PendingTasks {
		atoms = append(atoms, gistAtom{
			ID:        stableAtomID("pending_task", task.ID),
			Kind:      "pending_task",
			Content:   fmt.Sprintf("id=%s status=%s", task.ID, task.Status),
			Scope:     "office",
			SubjectID: task.ID,
			Predicate: "has_status",
			Value:     task.Status,
			Weight:    0.8,
			Meta: map[string]string{
				"taskId":         task.ID,
				"organizationId": obs.OrganizationID,
				"scale":          "office",
			},
		})
	}

	for k, v := range signal.Payload {
		atoms = append(atoms, gistAtom{
			ID:        stableAtomID("signal_payload", signal.ID, k, v),
			Kind:      "signal_payload",
			Content:   fmt.Sprintf("%s=%s", k, v),
			Scope:     "event",
			SubjectID: signal.ID,
			Predicate: k,
			Value:     v,
			Weight:    0.7,
			Meta: map[string]string{
				"signalId":       signal.ID,
				"payloadKey":     k,
				"organizationId": signal.OrganizationID,
				"scale":          "event",
			},
		})
	}

	return atoms
}

// Compress sends atoms to the GIST Python subprocess and returns a verdict.
// Degrades gracefully if the subprocess is unavailable or errors.
func (g *GISTAgentCore) Compress(ctx context.Context, atoms []gistAtom) (GISTVerdict, string, error) {
	if !g.subprocessAvailable() {
		return g.defaultVerdict("subprocess unavailable"), "{}", nil
	}

	input := gistSubprocessInput{
		AgentID:        g.agentID,
		OrganizationID: organizationIDFromAtoms(atoms),
		Scope:          gistScopeFromAtoms(g.agentID, atoms),
		Atoms:          atoms,
		PriorLattice:   rawLatticeJSON(g.latticeJSON),
		Budget:         g.budget,
	}
	payload, err := json.Marshal(input)
	if err != nil {
		return g.defaultVerdict("marshal error"), "{}", err
	}

	python := findPython()
	if python == "" {
		return g.defaultVerdict("python not found"), "{}", nil
	}

	runCtx, cancel := context.WithTimeout(ctx, 10*time.Second)
	defer cancel()

	cmd := exec.CommandContext(runCtx, python, g.scriptPath)
	cmd.Stdin = bytes.NewReader(payload)
	var stdout, stderr bytes.Buffer
	cmd.Stdout = &stdout
	cmd.Stderr = &stderr

	if err := cmd.Run(); err != nil {
		return g.defaultVerdict(fmt.Sprintf("subprocess error: %s", strings.TrimSpace(stderr.String()))), "{}", nil
	}

	var out gistSubprocessOutput
	if err := json.Unmarshal(stdout.Bytes(), &out); err != nil {
		return g.defaultVerdict("invalid subprocess output"), "{}", nil
	}
	if out.Error != "" {
		return g.defaultVerdict(out.Error), "{}", nil
	}

	lattice := out.LatticeJSON
	if lattice == "" {
		lattice = "{}"
	}

	verdict := GISTVerdict{
		Verdict:             out.Verdict,
		Confidence:          out.Confidence,
		CausalChain:         out.CausalChain,
		CausalGraph:         out.CausalGraph,
		OpenQuestions:       out.OpenQuestions,
		ExecutionIntent:     out.ExecutionIntent,
		Intent:              out.Intent,
		RiskLevel:           out.RiskLevel,
		RequiredTools:       out.RequiredTools,
		Lattice:             out.Lattice,
		Trace:               out.Trace,
		Proof:               out.Proof,
		Contradictions:      out.Contradictions,
		Interventions:       out.Interventions,
		Counterfactuals:     out.Counterfactuals,
		ConfidenceBreakdown: out.ConfidenceBreakdown,
	}
	// Reconcile the typed graph and the legacy []string view. If the
	// subprocess emitted a typed CausalGraph (protocolVersion >= 1)
	// we project it onto CausalChain so legacy consumers stay
	// deterministic; otherwise we hydrate the graph from the chain so
	// Pearl-aware consumers always have a typed view.
	verdict.SyncCausalChain()
	return verdict, lattice, nil
}

// ElasticStretch adjusts verdict confidence based on the elastic budget TTL.
// If the previous lattice is fresh enough, it boosts confidence; otherwise
// it returns the verdict unchanged.
func (g *GISTAgentCore) ElasticStretch(verdict GISTVerdict, lastWakeMs int64) GISTVerdict {
	if g.budget.MaxTTLMs <= 0 {
		return verdict
	}
	ageMs := time.Now().UnixMilli() - lastWakeMs
	if ageMs < 0 {
		ageMs = 0
	}
	freshnessFactor := 1.0 - (float64(ageMs) / float64(g.budget.MaxTTLMs))
	if freshnessFactor < 0 {
		freshnessFactor = 0
	}
	stretchFactor := g.budget.StretchFactor
	if stretchFactor <= 0 {
		stretchFactor = 1.0
	}
	verdict.Confidence = verdict.Confidence + (freshnessFactor * stretchFactor * (1.0 - verdict.Confidence))
	if verdict.Confidence > 1.0 {
		verdict.Confidence = 1.0
	}
	return verdict
}

// SetLattice updates the persisted lattice state (loaded from DB before each wake).
func (g *GISTAgentCore) SetLattice(latticeJSON string) {
	if latticeJSON == "" {
		latticeJSON = "{}"
	}
	g.latticeJSON = latticeJSON
}

// LatticeJSON returns the current lattice JSON (to be persisted to DB after wake).
func (g *GISTAgentCore) LatticeJSON() string {
	if g.latticeJSON == "" {
		return "{}"
	}
	return g.latticeJSON
}

func (g *GISTAgentCore) subprocessAvailable() bool {
	if strings.TrimSpace(g.scriptPath) == "" {
		return false
	}
	if _, err := os.Stat(g.scriptPath); err != nil {
		return false
	}
	return true
}

func (g *GISTAgentCore) defaultVerdict(reason string) GISTVerdict {
	return GISTVerdict{
		Verdict:         "degraded_gist_unavailable",
		Confidence:      0.1,
		CausalChain:     []string{reason},
		OpenQuestions:   []string{"GIST subprocess is unavailable; causal lattice was not updated."},
		ExecutionIntent: "default_low_confidence",
		RiskLevel:       "unknown",
		Degraded:        true,
		DegradedReason:  reason,
		Trace: &GISTTrace{
			ID:            stableAtomID("degraded", g.agentID, reason),
			AgentID:       g.agentID,
			Scope:         GISTScopeRef{Kind: "degraded", AgentID: g.agentID},
			SelectedChain: []string{reason},
			ReplayHandle:  "degraded:" + reason,
			CreatedAt:     time.Now().UnixMilli(),
		},
	}
}

// GISTScriptPath returns the conventional path to the GIST subprocess script.
func GISTScriptPath(baseDir string) string {
	if envPath := strings.TrimSpace(os.Getenv("AGENCY_GIST_SCRIPT_PATH")); envPath != "" {
		return envPath
	}
	candidates := []string{
		filepath.Join("scripts", "gist_subprocess.py"),
		filepath.Join(baseDir, "scripts", "gist_subprocess.py"),
	}
	if cwd, err := os.Getwd(); err == nil {
		candidates = append([]string{filepath.Join(cwd, "scripts", "gist_subprocess.py")}, candidates...)
		for dir := cwd; dir != filepath.Dir(dir); dir = filepath.Dir(dir) {
			candidates = append(candidates, filepath.Join(dir, "scripts", "gist_subprocess.py"))
		}
	}
	for _, candidate := range candidates {
		if _, err := os.Stat(candidate); err == nil {
			return candidate
		}
	}
	return filepath.Join(baseDir, "scripts", "gist_subprocess.py")
}

func stableAtomID(parts ...string) string {
	h := sha256.New()
	for _, part := range parts {
		_, _ = h.Write([]byte(part))
		_, _ = h.Write([]byte{0})
	}
	return fmt.Sprintf("atom_%x", h.Sum(nil))[:21]
}

func officeGISTLatticeKey(orgID string) string {
	orgID = strings.TrimSpace(orgID)
	if orgID == "" {
		orgID = "default"
	}
	return "office:" + orgID
}

func organizationIDFromAtoms(atoms []gistAtom) string {
	for _, atom := range atoms {
		if atom.Meta != nil && strings.TrimSpace(atom.Meta["organizationId"]) != "" {
			return atom.Meta["organizationId"]
		}
	}
	return ""
}

func gistScopeFromAtoms(agentID string, atoms []gistAtom) GISTScopeRef {
	orgID := organizationIDFromAtoms(atoms)
	return GISTScopeRef{
		Kind:           "agent_local",
		OrganizationID: orgID,
		AgentID:        agentID,
		ParentKind:     "office_fractal",
		ParentID:       officeGISTLatticeKey(orgID),
	}
}

func rawLatticeJSON(latticeJSON string) json.RawMessage {
	latticeJSON = strings.TrimSpace(latticeJSON)
	if latticeJSON == "" || latticeJSON == "{}" {
		return nil
	}
	if !json.Valid([]byte(latticeJSON)) {
		return nil
	}
	return json.RawMessage(latticeJSON)
}

// LatticeStore is the persistence interface for per-agent GIST lattice state.
// Implementations may be DB-backed or no-op.
type LatticeStore interface {
	GetLattice(ctx context.Context, agentID string) (string, error)
	SetLattice(ctx context.Context, agentID, latticeJSON string) error
}
