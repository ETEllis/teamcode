package main

import (
	"context"
	"encoding/json"
	"log"
	"os"
	"os/signal"
	"syscall"
	"time"

	"github.com/google/uuid"

	"github.com/ETEllis/teamcode/internal/agency"
	"github.com/ETEllis/teamcode/internal/db"

	_ "github.com/ncruces/go-sqlite3/driver"
	_ "github.com/ncruces/go-sqlite3/embed"
)

func main() {
	ctx, stop := signal.NotifyContext(context.Background(), syscall.SIGINT, syscall.SIGTERM)
	defer stop()

	cwd, _ := os.Getwd()
	bootstrap, err := agency.LoadBootstrap(cwd, os.Getenv("AGENCY_CONSTITUTION_NAME"), agency.RuntimeModeEmbedded, "")
	if err != nil {
		log.Fatal(err)
	}

	cfg := agency.ActorDaemonConfig{
		BaseDir:         bootstrap.Config.BaseDir,
		SharedWorkplace: bootstrap.Config.SharedWorkplace,
		Redis:           bootstrap.Config.Redis,
		Constitution:    bootstrap.Constitution,
		SpecPath:        os.Getenv("AGENCY_ACTOR_SPEC_PATH"),
	}

	// Wire DB-backed stores when available.
	if conn, dbErr := db.Connect(); dbErr != nil {
		log.Printf("actor-daemon: DB unavailable, lattice/routing state will not be persisted: %v", dbErr)
	} else {
		q := db.New(conn)
		cfg.LatticeStore = &dbLatticeStore{q: q}
		cfg.RoutingLog = &dbRoutingLog{q: q}
		cfg.GISTTraceStore = &dbGISTTraceStore{q: q}
	}

	if err := agency.RunActorDaemon(ctx, cfg); err != nil && err != context.Canceled {
		log.Fatal(err)
	}
}

type dbGISTTraceStore struct {
	q *db.Queries
}

func (s *dbGISTTraceStore) StoreTrace(ctx context.Context, organizationID, agentID, latticeJSON string, verdict agency.GISTVerdict) error {
	if verdict.Trace == nil || verdict.Trace.ID == "" {
		return nil
	}
	traceJSON, err := json.Marshal(verdict.Trace)
	if err != nil {
		return err
	}
	proofJSON := []byte("{}")
	if verdict.Proof != nil {
		proofJSON, err = json.Marshal(verdict.Proof)
		if err != nil {
			return err
		}
	}
	createdAt := verdict.Trace.CreatedAt
	if createdAt == 0 {
		createdAt = time.Now().UnixMilli()
	}
	// Build the lattice-inspector envelope alongside the legacy blobs so
	// the Director portal can render /lattice/<trace_id> without having
	// to re-run the kernel. A marshal failure here MUST NOT lose the
	// rest of the trace - we degrade to "{}" and log, then continue.
	inspectorJSON, inspErr := agency.MarshalInspectorBundle(verdict)
	if inspErr != nil {
		log.Printf("actor-daemon: inspector bundle marshal failed for trace %s: %v",
			verdict.Trace.ID, inspErr)
		inspectorJSON = "{}"
	}
	return s.q.InsertAgencyGistTrace(ctx, db.InsertAgencyGistTraceParams{
		ID:              verdict.Trace.ID,
		OfficeID:        organizationID,
		AgentID:         agentID,
		Verdict:         verdict.Verdict,
		RiskLevel:       verdict.RiskLevel,
		Confidence:      verdict.Confidence,
		TraceJSON:       string(traceJSON),
		ProofJSON:       string(proofJSON),
		LatticeJSON:     latticeJSON,
		InspectorJSON:   inspectorJSON,
		InputHash:       verdict.Trace.InputHash,
		NextLatticeHash: firstNonEmpty(verdict.Trace.NextLatticeHash, verdict.Trace.LatticeHash),
		CreatedAt:       createdAt,
	})
}

func firstNonEmpty(values ...string) string {
	for _, value := range values {
		if value != "" {
			return value
		}
	}
	return ""
}

// dbLatticeStore is a DB-backed implementation of agency.LatticeStore.
type dbLatticeStore struct {
	q *db.Queries
}

func (s *dbLatticeStore) GetLattice(ctx context.Context, agentID string) (string, error) {
	return s.q.GetAgencyGistLattice(ctx, agentID)
}

func (s *dbLatticeStore) SetLattice(ctx context.Context, agentID, latticeJSON string) error {
	return s.q.UpsertAgencyGistLattice(ctx, agentID, latticeJSON)
}

// dbRoutingLog is a DB-backed implementation of agency.RoutingLog.
type dbRoutingLog struct {
	q *db.Queries
}

func (r *dbRoutingLog) LogDecision(ctx context.Context, agentID, orgID string,
	result agency.InferenceResult, decision agency.RoutingDecision, intent agency.ActionIntent,
) error {
	return r.q.InsertAgencyRoutingLog(ctx, db.InsertRoutingLogParams{
		ID:              uuid.NewString(),
		AgentID:         agentID,
		OrgID:           orgID,
		Provider:        result.Provider,
		ModelID:         result.ModelID,
		ExecutionIntent: intent.TaskType,
		LatencyMs:       result.LatencyMs,
		TokensUsed:      result.TokensUsed,
		GateReason:      decision.GateReason,
	})
}
