package agency

import "time"

type ActionType string

const (
	ActionWriteCode       ActionType = "write_code"
	ActionRunTest         ActionType = "run_test"
	ActionPingPeer        ActionType = "ping_peer"
	ActionUpdateTask      ActionType = "update_task"
	ActionRequestReview   ActionType = "request_review"
	ActionBroadcast       ActionType = "broadcast"
	ActionSpawnAgent      ActionType = "spawn_agent"
	ActionPublishArtifact ActionType = "publish_artifact"
	ActionHandoffShift    ActionType = "handoff_shift"
)

type GovernanceMode string

const (
	GovernanceHierarchical GovernanceMode = "hierarchical"
	GovernancePeer         GovernanceMode = "peer"
	GovernanceFederated    GovernanceMode = "federated"
	GovernanceFlat         GovernanceMode = "flat"
	GovernanceHybrid       GovernanceMode = "hybrid"
)

type LedgerEntryKind string

const (
	LedgerEntryAction      LedgerEntryKind = "action"
	LedgerEntrySignal      LedgerEntryKind = "signal"
	LedgerEntrySnapshot    LedgerEntryKind = "snapshot"
	LedgerEntrySchedule    LedgerEntryKind = "schedule"
	LedgerEntryPublication LedgerEntryKind = "publication"
	LedgerEntryVoice       LedgerEntryKind = "voice"
)

type LedgerEntryStatus string

const (
	LedgerEntryStatusProposed  LedgerEntryStatus = "proposed"
	LedgerEntryStatusPending   LedgerEntryStatus = "pending"
	LedgerEntryStatusFinalized LedgerEntryStatus = "finalized"
	LedgerEntryStatusRejected  LedgerEntryStatus = "rejected"
)

type ConsensusStrategy string

const (
	ConsensusStrategyDirect ConsensusStrategy = "direct"
	ConsensusStrategyQuorum ConsensusStrategy = "quorum"
)

type SignalKind string

const (
	SignalTick        SignalKind = "tick"
	SignalSchedule    SignalKind = "schedule"
	SignalPeerMessage SignalKind = "peer_message"
	SignalTaskChange  SignalKind = "task_change"
	SignalReview      SignalKind = "review"
	SignalCorrection  SignalKind = "correction"
	SignalBroadcast   SignalKind = "broadcast"
	SignalVoice       SignalKind = "voice"
	SignalProjection  SignalKind = "projection"
)

type VoiceEventKind string

const (
	VoiceEventTranscript VoiceEventKind = "transcript"
	VoiceEventSynthesis  VoiceEventKind = "synthesis"
	VoiceEventProjection VoiceEventKind = "projection"
)

type AgencyConstitution struct {
	ID             string              `json:"id"`
	Name           string              `json:"name"`
	Description    string              `json:"description,omitempty"`
	OrganizationID string              `json:"organizationId"`
	GovernanceMode GovernanceMode      `json:"governanceMode"`
	Roles          map[string]RoleSpec `json:"roles"`
	Metadata       map[string]string   `json:"metadata,omitempty"`
}

type OrgIntent struct {
	ID           string            `json:"id"`
	Domain       string            `json:"domain"`
	TimeHorizon  string            `json:"timeHorizon"`
	WorkingStyle string            `json:"workingStyle"`
	GoalShape    string            `json:"goalShape"`
	Governance   GovernanceMode    `json:"governance"`
	Summary      string            `json:"summary"`
	Requirements []string          `json:"requirements,omitempty"`
	Metadata     map[string]string `json:"metadata,omitempty"`
}

type RoleSpec struct {
	Name              string            `json:"name"`
	Mission           string            `json:"mission"`
	Personality       string            `json:"personality,omitempty"`
	WorkingPosture    string            `json:"workingPosture,omitempty"`
	SystemPrompt      string            `json:"systemPrompt,omitempty"`
	AllowedActions    []ActionType      `json:"allowedActions,omitempty"`
	ObservationScopes []string          `json:"observationScopes,omitempty"`
	ToolBindings      []string          `json:"toolBindings,omitempty"`
	PeerRouting       map[string]string `json:"peerRouting,omitempty"`
	CanSpawnAgents    bool              `json:"canSpawnAgents"`
}

type CapabilityPack struct {
	Skills            []string          `json:"skills,omitempty"`
	Tools             []string          `json:"tools,omitempty"`
	ActionConstraints []ActionType      `json:"actionConstraints,omitempty"`
	ContextScopes     []string          `json:"contextScopes,omitempty"`
	Metadata          map[string]string `json:"metadata,omitempty"`
}

type AgentIdentity struct {
	ID             string            `json:"id"`
	Name           string            `json:"name"`
	Role           string            `json:"role"`
	OrganizationID string            `json:"organizationId"`
	ParentID       string            `json:"parentId,omitempty"`
	AvatarPrompt   string            `json:"avatarPrompt,omitempty"`
	Metadata       map[string]string `json:"metadata,omitempty"`
}

type AgentSchedule struct {
	ID                string            `json:"id"`
	ActorID           string            `json:"actorId"`
	Expression        string            `json:"expression"`
	Timezone          string            `json:"timezone,omitempty"`
	Enabled           bool              `json:"enabled"`
	DefaultSignalKind SignalKind        `json:"defaultSignalKind"`
	Metadata          map[string]string `json:"metadata,omitempty"`
}

type VoiceRoomState struct {
	RoomID               string            `json:"roomId"`
	ProjectionEnabled    bool              `json:"projectionEnabled"`
	TranscriptProjection bool              `json:"transcriptProjection"`
	AudioProjection      bool              `json:"audioProjection"`
	LastTranscriptID     string            `json:"lastTranscriptId,omitempty"`
	LastSynthesisID      string            `json:"lastSynthesisId,omitempty"`
	Metadata             map[string]string `json:"metadata,omitempty"`
	UpdatedAt            int64             `json:"updatedAt"`
}

type VoiceEvent struct {
	ID             string            `json:"id"`
	OrganizationID string            `json:"organizationId"`
	ActorID        string            `json:"actorId,omitempty"`
	RoomID         string            `json:"roomId,omitempty"`
	Kind           VoiceEventKind    `json:"kind"`
	CanonicalText  string            `json:"canonicalText,omitempty"`
	AudioInputRef  string            `json:"audioInputRef,omitempty"`
	AudioOutputRef string            `json:"audioOutputRef,omitempty"`
	Engine         string            `json:"engine,omitempty"`
	Projection     *VoiceRoomState   `json:"projection,omitempty"`
	Metadata       map[string]string `json:"metadata,omitempty"`
	CreatedAt      int64             `json:"createdAt"`
}

type TaskSummary struct {
	ID       string `json:"id"`
	Title    string `json:"title"`
	Status   string `json:"status"`
	Assigned string `json:"assigned,omitempty"`
}

type ResourceState struct {
	SharedWorkplace string            `json:"sharedWorkplace"`
	AvailableTools  []string          `json:"availableTools,omitempty"`
	Metadata        map[string]string `json:"metadata,omitempty"`
}

type ObservationSnapshot struct {
	OrganizationID string            `json:"organizationId"`
	Actor          AgentIdentity     `json:"actor"`
	LedgerSequence int64             `json:"ledgerSequence"`
	PendingTasks   []TaskSummary     `json:"pendingTasks,omitempty"`
	RecentSignals  []WakeSignal      `json:"recentSignals,omitempty"`
	Resources      ResourceState     `json:"resources"`
	CurrentTime    time.Time         `json:"currentTime"`
	Metadata       map[string]string `json:"metadata,omitempty"`
}

type ActionProposal struct {
	ID             string         `json:"id"`
	ActorID        string         `json:"actorId"`
	OrganizationID string         `json:"organizationId"`
	Type           ActionType     `json:"type"`
	Target         string         `json:"target,omitempty"`
	Payload        map[string]any `json:"payload,omitempty"`
	ObservedAt     int64          `json:"observedAt,omitempty"`
	ProposedAt     int64          `json:"proposedAt"`
}

type KernelDecision struct {
	Accepted      bool               `json:"accepted"`
	Reason        string             `json:"reason,omitempty"`
	Corrections   []CorrectionSignal `json:"corrections,omitempty"`
	ValidatedAt   int64              `json:"validatedAt"`
	CommitAllowed bool               `json:"commitAllowed"`
}

type ConsensusRequirement struct {
	Strategy          ConsensusStrategy `json:"strategy,omitempty"`
	QuorumKey         string            `json:"quorumKey,omitempty"`
	RequiredApprovals int               `json:"requiredApprovals,omitempty"`
	EligibleVoters    []string          `json:"eligibleVoters,omitempty"`
	AutoFinalize      bool              `json:"autoFinalize,omitempty"`
}

type QuorumState struct {
	QuorumKey         string   `json:"quorumKey,omitempty"`
	RequiredApprovals int      `json:"requiredApprovals"`
	EligibleVoters    []string `json:"eligibleVoters,omitempty"`
	Approvals         int      `json:"approvals"`
	Rejections        int      `json:"rejections"`
	MissingApprovals  int      `json:"missingApprovals,omitempty"`
	LastVoteAt        int64    `json:"lastVoteAt,omitempty"`
	Finalizable       bool     `json:"finalizable"`
	Finalized         bool     `json:"finalized"`
	Rejected          bool     `json:"rejected"`
}

type LedgerEntry struct {
	Sequence       int64                 `json:"sequence"`
	ID             string                `json:"id"`
	OrganizationID string                `json:"organizationId"`
	Kind           LedgerEntryKind       `json:"kind"`
	Status         LedgerEntryStatus     `json:"status,omitempty"`
	ActorID        string                `json:"actorId,omitempty"`
	Action         *ActionProposal       `json:"action,omitempty"`
	Decision       *KernelDecision       `json:"decision,omitempty"`
	Snapshot       *ContextSnapshot      `json:"snapshot,omitempty"`
	Signal         *WakeSignal           `json:"signal,omitempty"`
	Publication    *PublicationRecord    `json:"publication,omitempty"`
	Voice          *VoiceEvent           `json:"voice,omitempty"`
	Consensus      *ConsensusRequirement `json:"consensus,omitempty"`
	Quorum         *QuorumState          `json:"quorum,omitempty"`
	Certificate    *CommitCertificate    `json:"certificate,omitempty"`
	Votes          []ConsensusVote       `json:"votes,omitempty"`
	ProposedAt     int64                 `json:"proposedAt,omitempty"`
	CommittedAt    int64                 `json:"committedAt"`
	FinalizedAt    int64                 `json:"finalizedAt,omitempty"`
	RejectedAt     int64                 `json:"rejectedAt,omitempty"`
}

type CommitCertificate struct {
	EntryID     string            `json:"entryId"`
	Sequence    int64             `json:"sequence"`
	Hash        string            `json:"hash"`
	CommittedAt int64             `json:"committedAt"`
	QuorumSize  int               `json:"quorumSize"`
	Status      LedgerEntryStatus `json:"status,omitempty"`
	Approvals   int               `json:"approvals,omitempty"`
	Rejections  int               `json:"rejections,omitempty"`
}

type ConsensusVote struct {
	VoterID  string `json:"voterId"`
	EntryID  string `json:"entryId"`
	Approved bool   `json:"approved"`
	Reason   string `json:"reason,omitempty"`
	VotedAt  int64  `json:"votedAt"`
}

type ContextSnapshot struct {
	OrganizationID string              `json:"organizationId"`
	LedgerSequence int64               `json:"ledgerSequence"`
	Actors         []AgentIdentity     `json:"actors,omitempty"`
	Publications   []PublicationRecord `json:"publications,omitempty"`
	LastSignal     *WakeSignal         `json:"lastSignal,omitempty"`
	VoiceRooms     []VoiceRoomState    `json:"voiceRooms,omitempty"`
	LastVoiceEvent *VoiceEvent         `json:"lastVoiceEvent,omitempty"`
	OpenSchedules  []AgentSchedule     `json:"openSchedules,omitempty"`
	UpdatedAt      int64               `json:"updatedAt"`
	Metadata       map[string]string   `json:"metadata,omitempty"`
}

type WakeSignal struct {
	ID             string            `json:"id"`
	OrganizationID string            `json:"organizationId"`
	ActorID        string            `json:"actorId,omitempty"`
	Channel        string            `json:"channel"`
	Kind           SignalKind        `json:"kind"`
	Payload        map[string]string `json:"payload,omitempty"`
	CreatedAt      int64             `json:"createdAt"`
}

type ShiftHandoff struct {
	ID          string   `json:"id"`
	FromActorID string   `json:"fromActorId"`
	ToActorID   string   `json:"toActorId"`
	Summary     string   `json:"summary"`
	OpenTasks   []string `json:"openTasks,omitempty"`
	CreatedAt   int64    `json:"createdAt"`
}

type SpawnLineage struct {
	ParentActorID string `json:"parentActorId"`
	ChildActorID  string `json:"childActorId"`
	RootActorID   string `json:"rootActorId"`
	Depth         int    `json:"depth"`
	SpawnedAt     int64  `json:"spawnedAt"`
}

type PublicationRecord struct {
	ID           string   `json:"id"`
	ActorID      string   `json:"actorId"`
	Target       string   `json:"target"`
	Status       string   `json:"status"`
	Summary      string   `json:"summary,omitempty"`
	ArtifactRefs []string `json:"artifactRefs,omitempty"`
	CreatedAt    int64    `json:"createdAt"`
}

type CorrectionSignal struct {
	Code          string `json:"code"`
	Message       string `json:"message"`
	TargetActorID string `json:"targetActorId,omitempty"`
	CreatedAt     int64  `json:"createdAt"`
}

// ScheduleNode is a single node in the nested temporal schedule tree.
// Each node can carry a prompt_injection directive that is added as a
// high-weight GIST atom when the schedule fires.
type ScheduleNode struct {
	ID              string            `json:"id"`
	OrganizationID  string            `json:"organizationId"`
	ActorID         string            `json:"actorId"`
	ParentID        string            `json:"parentId,omitempty"`
	Expression      string            `json:"expression"`
	PromptInjection string            `json:"promptInjection,omitempty"`
	Layer           int               `json:"layer"`
	Enabled         bool              `json:"enabled"`
	Metadata        map[string]string `json:"metadata,omitempty"`
	CreatedAt       int64             `json:"createdAt"`
	UpdatedAt       int64             `json:"updatedAt"`
}

// ScheduleLayerConfig configures defaults for a depth level in the schedule tree.
type ScheduleLayerConfig struct {
	LayerDepth              int    `json:"layerDepth"`
	MaxNodes                int    `json:"maxNodes"`
	DefaultExpression       string `json:"defaultExpression"`
	InheritParentInjection  bool   `json:"inheritParentInjection"`
}

type RuntimeMode string

const (
	RuntimeModeEmbedded   RuntimeMode = "embedded"
	RuntimeModeDaemonized RuntimeMode = "daemonized"
)

type ServiceRole string

const (
	ServiceRoleOfficeCoordinator ServiceRole = "office-coordinator"
	ServiceRoleRuntimeDaemon     ServiceRole = "runtime-daemon"
	ServiceRoleSchedulerDaemon   ServiceRole = "scheduler-daemon"
	ServiceRoleActorDaemon       ServiceRole = "actor-daemon"
)

type ActorRuntimeSpec struct {
	Identity        AgentIdentity  `json:"identity"`
	Capabilities    CapabilityPack `json:"capabilities"`
	SharedWorkplace string         `json:"sharedWorkplace,omitempty"`
	OrganizationID  string         `json:"organizationId,omitempty"`
	RegisteredAt    int64          `json:"registeredAt"`
	RuntimeMode     RuntimeMode    `json:"runtimeMode,omitempty"`
	Metadata        map[string]any `json:"metadata,omitempty"`
}

// ElasticBudget controls how much GIST elastic stretch is allowed per wake cycle.
type ElasticBudget struct {
	RecallThreshold float64 `json:"recallThreshold"`
	MaxTTLMs        int64   `json:"maxTtlMs"`
	StretchFactor   float64 `json:"stretchFactor,omitempty"`
}

// GISTVerdict is the output of the GIST causal compression step.
type GISTVerdict struct {
	Verdict         string   `json:"verdict"`
	Confidence      float64  `json:"confidence"`
	CausalChain     []string `json:"causalChain,omitempty"`
	OpenQuestions   []string `json:"openQuestions,omitempty"`
	ExecutionIntent string   `json:"executionIntent"`
}

// ActionIntent carries model routing requirements derived from a GISTVerdict.
type ActionIntent struct {
	TaskType        string   `json:"taskType"`
	Complexity      float64  `json:"complexity"`
	LatencyBudgetMs int64    `json:"latencyBudgetMs"`
	PrivacyLevel    string   `json:"privacyLevel"`
	CostCeilingUsd  float64  `json:"costCeilingUsd"`
	RequiredTools   []string `json:"requiredTools,omitempty"`
}

// CredentialHandle represents a validated provider credential.
type CredentialHandle struct {
	Provider string `json:"provider"`
	KeyRef   string `json:"keyRef"`  // env var name
	Status   string `json:"status"`  // "valid", "missing", "expired"
	ModelID  string `json:"modelId"` // default model for this credential
}

// InferenceRequest is the input to the ModelRouter.
type InferenceRequest struct {
	System      string       `json:"system"`
	UserMessage string       `json:"userMessage"`
	Intent      ActionIntent `json:"intent"`
	AgentID     string       `json:"agentId"`
	OrgID       string       `json:"orgId"`
}

// InferenceResult is the output from a ProviderAdapter.
type InferenceResult struct {
	Text       string `json:"text"`
	Provider   string `json:"provider"`
	ModelID    string `json:"modelId"`
	LatencyMs  int64  `json:"latencyMs"`
	TokensUsed int    `json:"tokensUsed,omitempty"`
}

// ExecutionPolicy constrains which providers the ModelRouter may select.
type ExecutionPolicy struct {
	AllowedProviders []string `json:"allowedProviders,omitempty"`
	PreferLocal      bool     `json:"preferLocal"`
	MaxCostUsd       float64  `json:"maxCostUsd,omitempty"`
	MaxLatencyMs     int64    `json:"maxLatencyMs,omitempty"`
	PrivacyLevel     string   `json:"privacyLevel,omitempty"` // "local", "cloud", "any"
}
