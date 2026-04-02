package agency

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"time"
)

// gistAtom is a single causal fact fed into the GIST subprocess.
type gistAtom struct {
	Kind    string            `json:"kind"`
	Content string            `json:"content"`
	Weight  float64           `json:"weight,omitempty"`
	Meta    map[string]string `json:"meta,omitempty"`
}

// gistSubprocessInput is the JSON envelope sent to the Python subprocess on stdin.
type gistSubprocessInput struct {
	AgentID string      `json:"agentId"`
	Atoms   []gistAtom  `json:"atoms"`
	Budget  ElasticBudget `json:"budget"`
}

// gistSubprocessOutput is the JSON envelope returned by the subprocess on stdout.
type gistSubprocessOutput struct {
	Verdict     string   `json:"verdict"`
	Confidence  float64  `json:"confidence"`
	CausalChain []string `json:"causalChain,omitempty"`
	OpenQuestions []string `json:"openQuestions,omitempty"`
	ExecutionIntent string `json:"executionIntent"`
	LatticeJSON string   `json:"latticeJson,omitempty"`
	Error       string   `json:"error,omitempty"`
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
			Kind:    "actor_identity",
			Content: fmt.Sprintf("role=%s id=%s", obs.Actor.Role, obs.Actor.ID),
			Weight:  1.0,
		},
		{
			Kind:    "signal",
			Content: fmt.Sprintf("kind=%s channel=%s id=%s", signal.Kind, signal.Channel, signal.ID),
			Weight:  0.9,
		},
		{
			Kind:    "ledger_sequence",
			Content: fmt.Sprintf("%d", obs.LedgerSequence),
			Weight:  0.5,
		},
	}

	for _, task := range obs.PendingTasks {
		atoms = append(atoms, gistAtom{
			Kind:    "pending_task",
			Content: fmt.Sprintf("id=%s status=%s", task.ID, task.Status),
			Weight:  0.8,
		})
	}

	for k, v := range signal.Payload {
		atoms = append(atoms, gistAtom{
			Kind:    "signal_payload",
			Content: fmt.Sprintf("%s=%s", k, v),
			Weight:  0.7,
		})
	}

	if g.latticeJSON != "" && g.latticeJSON != "{}" {
		atoms = append(atoms, gistAtom{
			Kind:    "lattice_state",
			Content: g.latticeJSON,
			Weight:  0.6,
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
		AgentID: g.agentID,
		Atoms:   atoms,
		Budget:  g.budget,
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
		Verdict:         out.Verdict,
		Confidence:      out.Confidence,
		CausalChain:     out.CausalChain,
		OpenQuestions:   out.OpenQuestions,
		ExecutionIntent: out.ExecutionIntent,
	}
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
		Verdict:         "proceed_with_caution",
		Confidence:      0.1,
		CausalChain:     []string{reason},
		OpenQuestions:   nil,
		ExecutionIntent: "default_low_confidence",
	}
}

// GISTScriptPath returns the conventional path to the GIST subprocess script.
func GISTScriptPath(baseDir string) string {
	return filepath.Join(baseDir, "scripts", "gist_subprocess.py")
}

// LatticeStore is the persistence interface for per-agent GIST lattice state.
// Implementations may be DB-backed or no-op.
type LatticeStore interface {
	GetLattice(ctx context.Context, agentID string) (string, error)
	SetLattice(ctx context.Context, agentID, latticeJSON string) error
}
