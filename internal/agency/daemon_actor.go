package agency

import (
	"context"
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
	"time"
)

// RoutingLog persists model routing decisions for audit.
// If nil, routing decisions are only logged to stderr.
type RoutingLog interface {
	LogDecision(ctx context.Context, agentID, orgID string, result InferenceResult, decision RoutingDecision, intent ActionIntent) error
}

type ActorDaemonConfig struct {
	BaseDir         string
	SharedWorkplace string
	Redis           *RedisConfig
	Constitution    AgencyConstitution
	SpecPath        string
	// LatticeStore persists GIST lattice state across wake cycles.
	// If nil, lattice state is not persisted (in-memory only).
	LatticeStore LatticeStore
	// RoutingLog persists model routing decisions. Optional.
	RoutingLog RoutingLog
	// ExecutionPolicy constrains model selection. Zero value = permissive.
	ExecutionPolicy ExecutionPolicy
}

func RunActorDaemon(ctx context.Context, cfg ActorDaemonConfig) error {
	spec, err := loadActorSpecFromPath(cfg.SpecPath)
	if err != nil {
		return err
	}

	ledger, err := NewLedgerService(filepath.Join(cfg.BaseDir, "ledger"))
	if err != nil {
		return err
	}

	var bus EventBus = NewMemoryEventBus()
	if cfg.Redis != nil && cfg.Redis.Addr != "" {
		bus = NewRedisEventBus(*cfg.Redis)
	}
	defer bus.Close(context.Background())

	kernel := NewKernel()
	if err := appendActorLifecycle(ctx, ledger, spec, "actor.daemon.start", nil); err != nil {
		return err
	}
	defer func() {
		_ = appendActorLifecycle(context.Background(), ledger, spec, "actor.daemon.stop", nil)
	}()

	actorCh, err := bus.Subscribe(ctx, ActorChannel(spec.Identity.ID))
	if err != nil {
		return err
	}
	orgCh, err := bus.Subscribe(ctx, OrganizationChannel(spec.Identity.OrganizationID))
	if err != nil {
		return err
	}

	handle := func(signal WakeSignal) error {
		snapshot, err := ledger.LatestSnapshot(ctx, spec.Identity.OrganizationID)
		if err != nil {
			return err
		}
		observation := ObservationSnapshot{
			OrganizationID: spec.Identity.OrganizationID,
			Actor:          spec.Identity,
			LedgerSequence: snapshot.LedgerSequence,
			RecentSignals:  []WakeSignal{signal},
			Resources: ResourceState{
				SharedWorkplace: firstBootstrapValue(cfg.SharedWorkplace, spec.SharedWorkplace),
				AvailableTools:  spec.Capabilities.Tools,
			},
			CurrentTime: time.Now(),
			Metadata: map[string]string{
				"scope":       "default",
				"entrySource": "actor.daemon",
			},
		}
		constitution := cfg.Constitution
		if _, ok := constitution.Roles[spec.Identity.Role]; !ok {
			if constitution.Roles == nil {
				constitution.Roles = map[string]RoleSpec{}
			}
			constitution.Roles[spec.Identity.Role] = RoleSpec{
				Name:              spec.Identity.Role,
				Mission:           spec.Identity.Role,
				AllowedActions:    spec.Capabilities.ActionConstraints,
				ObservationScopes: []string{"default"},
				ToolBindings:      spec.Capabilities.Tools,
				CanSpawnAgents:    false,
			}
		}
		decision := kernel.ValidateObservation(constitution, observation)
		metadata := map[string]string{
			"entrySource":   "actor.daemon.signal",
			"actorId":       spec.Identity.ID,
			"signalKind":    string(signal.Kind),
			"signalChannel": signal.Channel,
			"accepted":      fmt.Sprintf("%t", decision.Accepted),
			"reason":        decision.Reason,
		}

		if decision.Accepted {
			roleSpec := constitution.Roles[spec.Identity.Role]

			// GIST causal compression: build atoms → compress → verdict.
			gistCore := NewGISTAgentCore(
				spec.Identity.ID,
				GISTScriptPath(cfg.BaseDir),
				DefaultGISTBudget(),
			)
			if cfg.LatticeStore != nil {
				if lattice, err := cfg.LatticeStore.GetLattice(ctx, spec.Identity.ID); err == nil {
					gistCore.SetLattice(lattice)
				}
			}
			atoms := gistCore.BuildAtoms(observation, signal)
			// Inject prompt_injection directive as high-weight GIST atom (weight 1.5).
			if injection := signal.Payload["prompt_injection"]; injection != "" {
				atoms = append(atoms, gistAtom{
					Kind:    "directive",
					Content: injection,
					Weight:  1.5,
				})
			}
			verdict, newLattice, _ := gistCore.Compress(ctx, atoms)
			verdict = gistCore.ElasticStretch(verdict, signal.CreatedAt)

			// Persist updated lattice state.
			if cfg.LatticeStore != nil {
				_ = cfg.LatticeStore.SetLattice(ctx, spec.Identity.ID, newLattice)
			} else {
				gistCore.SetLattice(newLattice)
			}

			metadata["gistVerdict"] = verdict.Verdict
			metadata["gistConfidence"] = fmt.Sprintf("%.2f", verdict.Confidence)

			// Build inference request from GIST verdict + role context.
			prompter := NewLLMActorProposer(DefaultActorLLMConfig())
			prompter.SetGISTContext(verdict)
			systemPrompt := prompter.BuildSystemPrompt(observation, roleSpec)
			userMessage := prompter.BuildUserMessage(observation, signal)
			intent := ActionIntent{
				TaskType:        verdict.ExecutionIntent,
				Complexity:      verdict.Confidence,
				LatencyBudgetMs: 5000,
				PrivacyLevel:    cfg.ExecutionPolicy.PrivacyLevel,
			}
			inferReq := InferenceRequest{
				System:      systemPrompt,
				UserMessage: userMessage,
				Intent:      intent,
				AgentID:     spec.Identity.ID,
				OrgID:       spec.Identity.OrganizationID,
			}

			// Route → adapter → execute.
			router := newDefaultRouter(cfg.ExecutionPolicy)
			adapter, routeDecision, routeErr := router.Route(ctx, inferReq)
			var inferResult InferenceResult
			if routeErr == nil && adapter != nil {
				inferResult, routeErr = adapter.Execute(ctx, inferReq)
			}
			if routeErr != nil {
				// Graceful degrade: use prompter's default (no external call).
				inferResult = InferenceResult{
					Text:     "actor ready",
					Provider: "default",
					ModelID:  "none",
				}
				routeDecision.GateReason = routeErr.Error()
			}
			metadata["routingProvider"] = routeDecision.SelectedProvider
			metadata["routingModel"] = routeDecision.SelectedModel
			if cfg.RoutingLog != nil {
				_ = cfg.RoutingLog.LogDecision(ctx, spec.Identity.ID, spec.Identity.OrganizationID, inferResult, routeDecision, intent)
			}

			// Publish performance record to bulletin channel.
			directive := signal.Payload["prompt_injection"]
			if directive == "" {
				directive = fmt.Sprintf("%s:%s", signal.Kind, signal.ID)
			}
			_ = PublishPerformance(ctx, bus, PerformanceRecord{
				OrganizationID: spec.Identity.OrganizationID,
				ActorID:        spec.Identity.ID,
				Directive:      directive,
				Output:         inferResult.Text,
				Score:          verdict.Confidence,
				SignalID:       signal.ID,
				Provider:       inferResult.Provider,
				ModelID:        inferResult.ModelID,
			})

			// Build proposals from inference result.
			proposals := []ActionProposal{
				{
					OrganizationID: observation.OrganizationID,
					ActorID:        spec.Identity.ID,
					Type:           ActionBroadcast,
					ProposedAt:     time.Now().UnixMilli(),
					Payload: map[string]any{
						"message":         inferResult.Text,
						"signalKind":      string(signal.Kind),
						"signalID":        signal.ID,
						"actorRole":       spec.Identity.Role,
						"provider":        inferResult.Provider,
						"model":           inferResult.ModelID,
						"executionIntent": verdict.ExecutionIntent,
						"entrySource":     "actor.daemon.routed",
					},
				},
			}
			propErr := error(nil)
			_ = propErr
			if true {
				for i := range proposals {
					if proposals[i].ProposedAt == 0 {
						proposals[i].ProposedAt = time.Now().UnixMilli()
					}
					entry := LedgerEntry{
						OrganizationID: proposals[i].OrganizationID,
						Kind:           LedgerEntryAction,
						ActorID:        spec.Identity.ID,
						Action:         &proposals[i],
						CommittedAt:    time.Now().UnixMilli(),
					}
					_, _ = ledger.Append(ctx, entry)

					// Publish ActionBroadcast proposals to the org channel so the TUI can display them.
					if proposals[i].Type == ActionBroadcast {
						msg, _ := proposals[i].Payload["message"].(string)
						orgSignal := WakeSignal{
							ID:             proposals[i].ID,
							OrganizationID: proposals[i].OrganizationID,
							ActorID:        proposals[i].ActorID,
							Channel:        OrganizationChannel(proposals[i].OrganizationID),
							Kind:           SignalBroadcast,
							Payload: map[string]string{
								"message":     msg,
								"actorId":     proposals[i].ActorID,
								"proposalId":  proposals[i].ID,
								"entrySource": "actor.daemon.broadcast",
							},
							CreatedAt: proposals[i].ProposedAt,
						}
						_ = bus.Publish(ctx, orgSignal)
						}

						// Publish all proposals to the approval channel so the TUI can display pending actions.
						approvalSignal := WakeSignal{
							ID:             proposals[i].ID,
							OrganizationID: proposals[i].OrganizationID,
							ActorID:        proposals[i].ActorID,
							Channel:        ApprovalChannel(proposals[i].OrganizationID),
							Kind:           SignalReview,
							Payload: map[string]string{
								"proposalId":  proposals[i].ID,
								"actorId":     proposals[i].ActorID,
								"actionType":  string(proposals[i].Type),
								"target":      proposals[i].Target,
								"entrySource": "actor.daemon.approval",
							},
							CreatedAt: proposals[i].ProposedAt,
						}
						_ = bus.Publish(ctx, approvalSignal)
					}
				metadata["proposalCount"] = fmt.Sprintf("%d", len(proposals))
			}
		}

		return appendActorLifecycle(ctx, ledger, spec, "actor.daemon.signal", metadata)
	}

	for {
		select {
		case <-ctx.Done():
			return ctx.Err()
		case signal, ok := <-actorCh:
			if !ok {
				return nil
			}
			if err := handle(signal); err != nil {
				return err
			}
		case signal, ok := <-orgCh:
			if !ok {
				return nil
			}
			if err := handle(signal); err != nil {
				return err
			}
		}
	}
}

func loadActorSpecFromPath(path string) (ActorRuntimeSpec, error) {
	if path == "" {
		return ActorRuntimeSpec{}, fmt.Errorf("actor spec path is required")
	}
	data, err := os.ReadFile(path)
	if err != nil {
		return ActorRuntimeSpec{}, err
	}
	var spec ActorRuntimeSpec
	if err := json.Unmarshal(data, &spec); err != nil {
		return ActorRuntimeSpec{}, err
	}
	if spec.Identity.ID == "" {
		return ActorRuntimeSpec{}, fmt.Errorf("actor spec id is required")
	}
	return spec, nil
}

// newDefaultRouter builds a ModelRouter with all known provider adapters.
func newDefaultRouter(policy ExecutionPolicy) *ModelRouter {
	broker := NewCredentialBroker()
	adapters := defaultProviderAdapters()
	return NewModelRouter(adapters, broker, policy)
}

func appendActorLifecycle(ctx context.Context, ledger *LedgerService, spec ActorRuntimeSpec, source string, metadata map[string]string) error {
	if metadata == nil {
		metadata = map[string]string{}
	}
	metadata["entrySource"] = source
	metadata["actorId"] = spec.Identity.ID
	_, err := ledger.AppendSnapshot(ctx, ContextSnapshot{
		OrganizationID: spec.Identity.OrganizationID,
		Actors:         []AgentIdentity{spec.Identity},
		UpdatedAt:      time.Now().UnixMilli(),
		Metadata:       metadata,
	})
	return err
}
