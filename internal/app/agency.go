package app

import (
	"context"
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
	"sort"
	"strings"
	"sync"
	"time"

	agencypkg "github.com/ETEllis/teamcode/internal/agency"
	"github.com/ETEllis/teamcode/internal/config"
)

type AgencyService struct {
	mu        sync.RWMutex
	cfg       *config.Config
	stateFile string
}

type AgencyOfficeStatus struct {
	ProductName     string     `json:"productName"`
	Mode            string     `json:"mode"`
	Running         bool       `json:"running"`
	Constitution    string     `json:"constitution"`
	SharedWorkplace string     `json:"sharedWorkplace"`
	RedisAddress    string     `json:"redisAddress"`
	LedgerPath      string     `json:"ledgerPath"`
	ConsensusMode   string     `json:"consensusMode"`
	DefaultQuorum   int        `json:"defaultQuorum"`
	StartedAt       *time.Time `json:"startedAt,omitempty"`
	StoppedAt       *time.Time `json:"stoppedAt,omitempty"`
	LastEvent       string     `json:"lastEvent,omitempty"`
	LastUpdatedAt   time.Time  `json:"lastUpdatedAt"`
}

type AgencyOrganizationView struct {
	ProductName         string                    `json:"productName"`
	CurrentConstitution string                    `json:"currentConstitution"`
	SoloConstitution    string                    `json:"soloConstitution"`
	Constitution        config.AgencyConstitution `json:"constitution"`
	Blueprint           string                    `json:"blueprint"`
	TeamTemplate        string                    `json:"teamTemplate"`
	Governance          string                    `json:"governance"`
	Roles               []config.TeamRoleTemplate `json:"roles"`
	LoopStrategy        string                    `json:"loopStrategy"`
	WorkspaceMode       string                    `json:"workspaceMode"`
	RequiredGates       []string                  `json:"requiredGates"`
}

type AgencyConstitutionView struct {
	Name          string                    `json:"name"`
	Description   string                    `json:"description"`
	Current       bool                      `json:"current"`
	Solo          bool                      `json:"solo"`
	Constitution  config.AgencyConstitution `json:"constitution"`
	Blueprint     string                    `json:"blueprint"`
	TeamTemplate  string                    `json:"teamTemplate"`
	Roles         []config.TeamRoleTemplate `json:"roles"`
	LoopStrategy  string                    `json:"loopStrategy"`
	WorkspaceMode string                    `json:"workspaceMode"`
	RequiredGates []string                  `json:"requiredGates"`
}

type AgencyScheduleView struct {
	Timezone            string                  `json:"timezone"`
	DefaultCadence      string                  `json:"defaultCadence"`
	WakeOnOfficeOpen    bool                    `json:"wakeOnOfficeOpen"`
	RequireShiftHandoff bool                    `json:"requireShiftHandoff"`
	Windows             []config.ScheduleWindow `json:"windows"`
}

type AgencyRuntimeServicesView struct {
	Docker AgencyDockerServiceView `json:"docker"`
	Redis  AgencyRedisServiceView  `json:"redis"`
	Ledger AgencyLedgerServiceView `json:"ledger"`
}

type AgencyDockerServiceView struct {
	Enabled        bool   `json:"enabled"`
	ComposeProject string `json:"composeProject"`
	ComposeFile    string `json:"composeFile"`
	Image          string `json:"image"`
	SharedVolume   string `json:"sharedVolume"`
	Network        string `json:"network"`
}

type AgencyRedisServiceView struct {
	Enabled       bool   `json:"enabled"`
	Address       string `json:"address"`
	DB            int    `json:"db"`
	ChannelPrefix string `json:"channelPrefix"`
}

type AgencyLedgerServiceView struct {
	Backend        string `json:"backend"`
	Path           string `json:"path"`
	SnapshotPath   string `json:"snapshotPath"`
	ConsensusMode  string `json:"consensusMode"`
	DefaultQuorum  int    `json:"defaultQuorum"`
	ProjectionFile string `json:"projectionFile"`
}

type AgencyGenesisRequest struct {
	Intent       string   `json:"intent"`
	Domain       string   `json:"domain,omitempty"`
	TimeHorizon  string   `json:"timeHorizon,omitempty"`
	WorkingStyle string   `json:"workingStyle,omitempty"`
	Governance   string   `json:"governance,omitempty"`
	GoalShape    string   `json:"goalShape,omitempty"`
	Constitution string   `json:"constitution,omitempty"`
	Roles        []string `json:"roles,omitempty"`
}

type AgencyGenesisResult struct {
	Intent               string                        `json:"intent"`
	Summary              string                        `json:"summary"`
	ConstitutionName     string                        `json:"constitutionName"`
	Constitution         config.AgencyConstitution     `json:"constitution"`
	Topology             string                        `json:"topology,omitempty"`
	OrgIntent            agencypkg.OrgIntent           `json:"orgIntent"`
	Roles                []config.TeamRoleTemplate     `json:"roles"`
	RoleBundles          []agencypkg.GenesisRoleBundle `json:"roleBundles,omitempty"`
	SocialThread         []string                      `json:"socialThread,omitempty"`
	ManufacturingSignals []string                      `json:"manufacturingSignals,omitempty"`
	Tooling              map[string]string             `json:"tooling"`
	CreatedAt            time.Time                     `json:"createdAt"`
}

type agencyState struct {
	Office      AgencyOfficeStatus   `json:"office"`
	LastGenesis *AgencyGenesisResult `json:"lastGenesis,omitempty"`
}

func NewAgencyService(cfg *config.Config) *AgencyService {
	stateFile := ""
	if cfg != nil {
		stateFile = cfg.Agency.Office.StateFile
	}
	return &AgencyService{
		cfg:       cfg,
		stateFile: stateFile,
	}
}

func (s *AgencyService) MaybeBootOnStartup(ctx context.Context) error {
	s.mu.RLock()
	cfg := s.cfg
	s.mu.RUnlock()

	if cfg == nil {
		return nil
	}
	if cfg.Agency.Office.AutoBoot == nil || !*cfg.Agency.Office.AutoBoot {
		return nil
	}

	_, err := s.BootOffice(ctx, "")
	return err
}

func (s *AgencyService) BootOffice(_ context.Context, constitutionOverride string) (AgencyOfficeStatus, error) {
	s.mu.Lock()
	defer s.mu.Unlock()

	if err := s.ensureOfficeFilesystemLocked(); err != nil {
		return AgencyOfficeStatus{}, err
	}

	state, err := s.readStateLocked()
	if err != nil {
		return AgencyOfficeStatus{}, err
	}

	cfg := s.cfg
	if cfg == nil {
		return AgencyOfficeStatus{}, fmt.Errorf("config not loaded")
	}

	constitutionName, constitution, err := s.resolveConstitutionLocked(constitutionOverride)
	if err != nil {
		return AgencyOfficeStatus{}, err
	}
	now := time.Now()

	state.Office = s.defaultOfficeStatusLocked(constitutionName, constitution)
	state.Office.Running = true
	state.Office.StartedAt = timestampPtr(now)
	state.Office.StoppedAt = nil
	state.Office.LastEvent = "office.boot"
	state.Office.LastUpdatedAt = now

	if existing, err := s.readStateLocked(); err == nil && existing.Office.StartedAt != nil {
		state.Office.StartedAt = existing.Office.StartedAt
		if !existing.Office.Running {
			state.Office.StartedAt = timestampPtr(now)
		}
	}

	return state.Office, s.writeStateLocked(state)
}

func (s *AgencyService) StopOffice() (AgencyOfficeStatus, error) {
	s.mu.Lock()
	defer s.mu.Unlock()

	state, err := s.readStateLocked()
	if err != nil {
		return AgencyOfficeStatus{}, err
	}

	constitutionName, constitution, err := s.resolveConstitutionLocked(state.Office.Constitution)
	if err != nil {
		return AgencyOfficeStatus{}, err
	}
	if state.Office.ProductName == "" {
		state.Office = s.defaultOfficeStatusLocked(constitutionName, constitution)
	}

	now := time.Now()
	state.Office.Running = false
	state.Office.LastEvent = "office.stop"
	state.Office.StoppedAt = timestampPtr(now)
	state.Office.LastUpdatedAt = now

	return state.Office, s.writeStateLocked(state)
}

func (s *AgencyService) InspectOffice() (AgencyOfficeStatus, error) {
	s.mu.RLock()
	defer s.mu.RUnlock()

	state, err := s.readStateLocked()
	if err != nil {
		return AgencyOfficeStatus{}, err
	}
	if s.cfg != nil {
		constitutionName, constitution, resolveErr := s.resolveConstitutionLocked("")
		if resolveErr != nil {
			return AgencyOfficeStatus{}, resolveErr
		}
		defaults := s.defaultOfficeStatusLocked(constitutionName, constitution)
		if state.Office.ProductName == "" {
			state.Office = defaults
		} else {
			state.Office.ProductName = defaults.ProductName
			state.Office.Mode = defaults.Mode
			state.Office.Constitution = defaults.Constitution
			state.Office.SharedWorkplace = defaults.SharedWorkplace
			state.Office.RedisAddress = defaults.RedisAddress
			state.Office.LedgerPath = defaults.LedgerPath
			state.Office.ConsensusMode = defaults.ConsensusMode
			state.Office.DefaultQuorum = defaults.DefaultQuorum
		}
	}
	return state.Office, nil
}

func (s *AgencyService) InspectOrganization() (AgencyOrganizationView, error) {
	s.mu.RLock()
	defer s.mu.RUnlock()

	if s.cfg == nil {
		return AgencyOrganizationView{}, fmt.Errorf("config not loaded")
	}

	constitution := config.ActiveConstitution(s.cfg)
	blueprintName := constitution.Blueprint
	if blueprintName == "" {
		blueprintName = constitution.TeamTemplate
	}
	template := s.cfg.Team.Blueprints[blueprintName]
	if template.Name == "" {
		template = s.cfg.Team.Templates[constitution.TeamTemplate]
	}

	return AgencyOrganizationView{
		ProductName:         s.cfg.Agency.ProductName,
		CurrentConstitution: s.cfg.Agency.CurrentConstitution,
		SoloConstitution:    s.cfg.Agency.SoloConstitution,
		Constitution:        constitution,
		Blueprint:           blueprintName,
		TeamTemplate:        constitution.TeamTemplate,
		Governance:          constitution.Governance,
		Roles:               template.Roles,
		LoopStrategy:        template.Policies.LoopStrategy,
		WorkspaceMode:       template.Policies.WorkspaceModeDefault,
		RequiredGates:       append([]string(nil), template.Policies.RequiredGates...),
	}, nil
}

func (s *AgencyService) InspectSchedules() AgencyScheduleView {
	s.mu.RLock()
	defer s.mu.RUnlock()

	cfg := s.cfg
	if cfg == nil {
		return AgencyScheduleView{}
	}

	return AgencyScheduleView{
		Timezone:            cfg.Agency.Schedules.Timezone,
		DefaultCadence:      cfg.Agency.Schedules.DefaultCadence,
		WakeOnOfficeOpen:    boolValue(cfg.Agency.Schedules.WakeOnOfficeOpen),
		RequireShiftHandoff: boolValue(cfg.Agency.Schedules.RequireShiftHandoff),
		Windows:             append([]config.ScheduleWindow(nil), cfg.Agency.Schedules.Windows...),
	}
}

func (s *AgencyService) InspectRuntimeServices() AgencyRuntimeServicesView {
	s.mu.RLock()
	defer s.mu.RUnlock()

	cfg := s.cfg
	if cfg == nil {
		return AgencyRuntimeServicesView{}
	}

	return AgencyRuntimeServicesView{
		Docker: AgencyDockerServiceView{
			Enabled:        boolValue(cfg.Agency.Docker.Enabled),
			ComposeProject: cfg.Agency.Docker.ComposeProject,
			ComposeFile:    cfg.Agency.Docker.ComposeFile,
			Image:          cfg.Agency.Docker.Image,
			SharedVolume:   cfg.Agency.Docker.SharedVolume,
			Network:        cfg.Agency.Docker.Network,
		},
		Redis: AgencyRedisServiceView{
			Enabled:       boolValue(cfg.Agency.Redis.Enabled),
			Address:       cfg.Agency.Redis.Address,
			DB:            cfg.Agency.Redis.DB,
			ChannelPrefix: cfg.Agency.Redis.ChannelPrefix,
		},
		Ledger: AgencyLedgerServiceView{
			Backend:        cfg.Agency.Ledger.Backend,
			Path:           cfg.Agency.Ledger.Path,
			SnapshotPath:   cfg.Agency.Ledger.SnapshotPath,
			ConsensusMode:  cfg.Agency.Ledger.ConsensusMode,
			DefaultQuorum:  cfg.Agency.Ledger.DefaultQuorum,
			ProjectionFile: cfg.Agency.Ledger.ProjectionFile,
		},
	}
}

func (s *AgencyService) ListConstitutions() []config.AgencyConstitution {
	s.mu.RLock()
	defer s.mu.RUnlock()

	if s.cfg == nil {
		return nil
	}

	names := make([]string, 0, len(s.cfg.Agency.Constitutions))
	for name := range s.cfg.Agency.Constitutions {
		names = append(names, name)
	}
	sort.Strings(names)

	out := make([]config.AgencyConstitution, 0, len(names))
	for _, name := range names {
		constitution := s.cfg.Agency.Constitutions[name]
		out = append(out, constitution)
	}
	return out
}

func (s *AgencyService) InspectConstitution(name string) (AgencyConstitutionView, error) {
	s.mu.RLock()
	defer s.mu.RUnlock()

	if s.cfg == nil {
		return AgencyConstitutionView{}, fmt.Errorf("config not loaded")
	}

	constitution, ok := s.cfg.Agency.Constitutions[name]
	if !ok {
		return AgencyConstitutionView{}, fmt.Errorf("agency constitution %q not found", name)
	}

	blueprintName := constitution.Blueprint
	if blueprintName == "" {
		blueprintName = constitution.TeamTemplate
	}
	template := s.cfg.Team.Blueprints[blueprintName]
	if template.Name == "" {
		template = s.cfg.Team.Templates[constitution.TeamTemplate]
	}

	return AgencyConstitutionView{
		Name:          constitution.Name,
		Description:   constitution.Description,
		Current:       name == s.cfg.Agency.CurrentConstitution,
		Solo:          name == s.cfg.Agency.SoloConstitution,
		Constitution:  constitution,
		Blueprint:     blueprintName,
		TeamTemplate:  constitution.TeamTemplate,
		Roles:         append([]config.TeamRoleTemplate(nil), template.Roles...),
		LoopStrategy:  template.Policies.LoopStrategy,
		WorkspaceMode: template.Policies.WorkspaceModeDefault,
		RequiredGates: append([]string(nil), template.Policies.RequiredGates...),
	}, nil
}

func (s *AgencyService) SwitchConstitution(name string) (config.AgencyConstitution, error) {
	s.mu.Lock()
	defer s.mu.Unlock()

	if s.cfg == nil {
		return config.AgencyConstitution{}, fmt.Errorf("config not loaded")
	}
	if err := config.UpdateAgencyConstitution(name); err != nil {
		return config.AgencyConstitution{}, err
	}
	s.cfg = config.Get()

	constitution := s.cfg.Agency.Constitutions[name]

	state, err := s.readStateLocked()
	if err == nil {
		state.Office.Constitution = name
		state.Office.LastEvent = "agency.switch_constitution"
		state.Office.LastUpdatedAt = time.Now()
		_ = s.writeStateLocked(state)
	}

	return constitution, nil
}

func (s *AgencyService) StartGenesis(req AgencyGenesisRequest) (AgencyGenesisResult, error) {
	s.mu.Lock()
	defer s.mu.Unlock()

	if s.cfg == nil {
		return AgencyGenesisResult{}, fmt.Errorf("config not loaded")
	}
	if strings.TrimSpace(req.Intent) == "" {
		return AgencyGenesisResult{}, fmt.Errorf("genesis intent is required")
	}

	if err := s.ensureOfficeFilesystemLocked(); err != nil {
		return AgencyGenesisResult{}, err
	}
	constitutionName, constitution, err := s.resolveConstitutionLocked(req.Constitution)
	if err != nil {
		return AgencyGenesisResult{}, err
	}

	template := s.resolveTemplateLocked(constitution)
	manufactured := agencypkg.ManufactureGenesis(agencypkg.GenesisManufacturingInput{
		Intent:           req.Intent,
		Domain:           req.Domain,
		TimeHorizon:      req.TimeHorizon,
		WorkingStyle:     req.WorkingStyle,
		Governance:       req.Governance,
		GoalShape:        req.GoalShape,
		ConstitutionName: constitutionName,
		Constitution:     constitution,
		Template:         template,
		RequestedRoles:   append([]string(nil), req.Roles...),
		DefaultCadence:   s.cfg.Agency.Schedules.DefaultCadence,
		Timezone:         s.cfg.Agency.Schedules.Timezone,
		SharedWorkplace:  s.cfg.Agency.Office.SharedWorkplace,
		RedisAddress:     s.cfg.Agency.Redis.Address,
		LedgerPath:       s.cfg.Agency.Ledger.Path,
	})

	now := time.Now()
	result := AgencyGenesisResult{
		Intent:               req.Intent,
		Summary:              manufactured.Summary,
		ConstitutionName:     constitutionName,
		Constitution:         constitution,
		Topology:             manufactured.Topology,
		OrgIntent:            manufactured.Intent,
		Roles:                roleTemplatesFromBundles(manufactured.RoleBundles),
		RoleBundles:          append([]agencypkg.GenesisRoleBundle(nil), manufactured.RoleBundles...),
		SocialThread:         append([]string(nil), manufactured.SocialThread...),
		ManufacturingSignals: append([]string(nil), manufactured.ManufacturingSignals...),
		Tooling: map[string]string{
			"eventBus": s.cfg.Agency.Redis.Address,
			"ledger":   s.cfg.Agency.Ledger.Path,
			"office":   s.cfg.Agency.Office.SharedWorkplace,
		},
		CreatedAt: now,
	}

	state, err := s.readStateLocked()
	if err != nil {
		return AgencyGenesisResult{}, err
	}
	state.LastGenesis = &result
	state.Office.LastEvent = "agency.genesis"
	state.Office.LastUpdatedAt = now

	return result, s.writeStateLocked(state)
}

// BroadcastMsg carries an actor broadcast for TUI display.
type BroadcastMsg struct {
	ActorID   string
	Message   string
	CreatedAt int64
}

// ProposalMsg carries a pending action proposal for TUI approval display.
type ProposalMsg struct {
	ProposalID string
	ActorID    string
	ActionType string
	Target     string
	CreatedAt  int64
}

// BulletinRecord carries a directive→output→score performance entry for TUI display.
type BulletinRecord struct {
	ActorID   string
	Directive string
	Output    string
	Score     string
	Provider  string
	ModelID   string
	CreatedAt int64
}

// SubscribeBroadcasts connects to the agency event bus and returns a channel of
// broadcast messages for the given organization. The channel closes when ctx is
// cancelled. Returns nil if Redis is not configured (embedded mode).
func (s *AgencyService) SubscribeBroadcasts(ctx context.Context, orgID string) (<-chan BroadcastMsg, error) {
	s.mu.RLock()
	cfg := s.cfg
	s.mu.RUnlock()

	if cfg == nil || !boolValue(cfg.Agency.Redis.Enabled) || cfg.Agency.Redis.Address == "" {
		return nil, nil
	}

	bus := agencypkg.NewRedisEventBus(agencypkg.RedisConfig{
		Addr: cfg.Agency.Redis.Address,
		DB:   cfg.Agency.Redis.DB,
	})

	rawCh, err := bus.Subscribe(ctx, agencypkg.OrganizationChannel(orgID))
	if err != nil {
		_ = bus.Close(context.Background())
		return nil, err
	}

	out := make(chan BroadcastMsg, 32)
	go func() {
		defer close(out)
		defer bus.Close(context.Background())
		for {
			select {
			case <-ctx.Done():
				return
			case sig, ok := <-rawCh:
				if !ok {
					return
				}
				if sig.Kind != agencypkg.SignalBroadcast {
					continue
				}
				msg := sig.Payload["message"]
				if msg == "" {
					continue
				}
				select {
				case out <- BroadcastMsg{
					ActorID:   sig.ActorID,
					Message:   msg,
					CreatedAt: sig.CreatedAt,
				}:
				default:
				}
			}
		}
	}()
	return out, nil
}

// SubscribeApprovals connects to the approval channel and returns pending action proposals.
// Returns nil if Redis is not configured.
func (s *AgencyService) SubscribeApprovals(ctx context.Context, orgID string) (<-chan ProposalMsg, error) {
	s.mu.RLock()
	cfg := s.cfg
	s.mu.RUnlock()

	if cfg == nil || !boolValue(cfg.Agency.Redis.Enabled) || cfg.Agency.Redis.Address == "" {
		return nil, nil
	}

	bus := agencypkg.NewRedisEventBus(agencypkg.RedisConfig{
		Addr: cfg.Agency.Redis.Address,
		DB:   cfg.Agency.Redis.DB,
	})

	rawCh, err := bus.Subscribe(ctx, agencypkg.ApprovalChannel(orgID))
	if err != nil {
		_ = bus.Close(context.Background())
		return nil, err
	}

	out := make(chan ProposalMsg, 32)
	go func() {
		defer close(out)
		defer bus.Close(context.Background())
		for {
			select {
			case <-ctx.Done():
				return
			case sig, ok := <-rawCh:
				if !ok {
					return
				}
				if sig.Kind != agencypkg.SignalReview {
					continue
				}
				proposalID := sig.Payload["proposalId"]
				if proposalID == "" {
					continue
				}
				select {
				case out <- ProposalMsg{
					ProposalID: proposalID,
					ActorID:    sig.ActorID,
					ActionType: sig.Payload["actionType"],
					Target:     sig.Payload["target"],
					CreatedAt:  sig.CreatedAt,
				}:
				default:
				}
			}
		}
	}()
	return out, nil
}

// SendApprovalVote publishes an approve or reject signal for a pending proposal.
func (s *AgencyService) SendApprovalVote(ctx context.Context, orgID, proposalID string, approved bool) error {
	s.mu.RLock()
	cfg := s.cfg
	s.mu.RUnlock()

	if cfg == nil || !boolValue(cfg.Agency.Redis.Enabled) || cfg.Agency.Redis.Address == "" {
		return nil
	}

	bus := agencypkg.NewRedisEventBus(agencypkg.RedisConfig{
		Addr: cfg.Agency.Redis.Address,
		DB:   cfg.Agency.Redis.DB,
	})
	defer bus.Close(context.Background())

	vote := "reject"
	if approved {
		vote = "approve"
	}

	return bus.Publish(ctx, agencypkg.WakeSignal{
		OrganizationID: orgID,
		Channel:        agencypkg.OrganizationChannel(orgID),
		Kind:           agencypkg.SignalCorrection,
		Payload: map[string]string{
			"proposalId": proposalID,
			"vote":       vote,
			"source":     "tui.approval",
		},
		CreatedAt: 0,
	})
}

// SubscribeBulletin connects to the bulletin channel and returns performance records.
// Returns nil if Redis is not configured.
func (s *AgencyService) SubscribeBulletin(ctx context.Context, orgID string) (<-chan BulletinRecord, error) {
	s.mu.RLock()
	cfg := s.cfg
	s.mu.RUnlock()

	if cfg == nil || !boolValue(cfg.Agency.Redis.Enabled) || cfg.Agency.Redis.Address == "" {
		return nil, nil
	}

	bus := agencypkg.NewRedisEventBus(agencypkg.RedisConfig{
		Addr: cfg.Agency.Redis.Address,
		DB:   cfg.Agency.Redis.DB,
	})

	rawCh, err := bus.Subscribe(ctx, agencypkg.BulletinChannel(orgID))
	if err != nil {
		_ = bus.Close(context.Background())
		return nil, err
	}

	out := make(chan BulletinRecord, 32)
	go func() {
		defer close(out)
		defer bus.Close(context.Background())
		for {
			select {
			case <-ctx.Done():
				return
			case sig, ok := <-rawCh:
				if !ok {
					return
				}
				if sig.Kind != agencypkg.SignalProjection {
					continue
				}
				directive := sig.Payload["directive"]
				output := sig.Payload["output"]
				if directive == "" && output == "" {
					continue
				}
				select {
				case out <- BulletinRecord{
					ActorID:   sig.ActorID,
					Directive: directive,
					Output:    output,
					Score:     sig.Payload["score"],
					Provider:  sig.Payload["provider"],
					ModelID:   sig.Payload["modelId"],
					CreatedAt: sig.CreatedAt,
				}:
				default:
				}
			}
		}
	}()
	return out, nil
}

func (s *AgencyService) LatestGenesis() (*AgencyGenesisResult, error) {
	s.mu.RLock()
	defer s.mu.RUnlock()

	state, err := s.readStateLocked()
	if err != nil {
		return nil, err
	}
	if state.LastGenesis == nil {
		return nil, nil
	}
	copy := *state.LastGenesis
	return &copy, nil
}

func (s *AgencyService) resolveConstitutionLocked(preferred string) (string, config.AgencyConstitution, error) {
	if s.cfg == nil {
		return "", config.AgencyConstitution{}, fmt.Errorf("config not loaded")
	}

	candidates := []string{
		strings.TrimSpace(preferred),
		strings.TrimSpace(s.cfg.Agency.CurrentConstitution),
		strings.TrimSpace(s.cfg.Agency.SoloConstitution),
		"coding-office",
		"solo",
	}
	for _, candidate := range candidates {
		if candidate == "" {
			continue
		}
		if constitution, ok := s.cfg.Agency.Constitutions[candidate]; ok {
			return candidate, constitution, nil
		}
	}
	return "", config.AgencyConstitution{}, fmt.Errorf("no valid agency constitution is configured")
}

func (s *AgencyService) resolveTemplateLocked(constitution config.AgencyConstitution) config.TeamTemplate {
	if s.cfg == nil {
		return config.TeamTemplate{}
	}
	if constitution.Blueprint != "" {
		if template, ok := s.cfg.Team.Blueprints[constitution.Blueprint]; ok && template.Name != "" {
			return template
		}
	}
	if constitution.TeamTemplate != "" {
		if template, ok := s.cfg.Team.Templates[constitution.TeamTemplate]; ok && template.Name != "" {
			return template
		}
	}
	if template, ok := s.cfg.Team.Blueprints[s.cfg.Team.DefaultBlueprint]; ok && template.Name != "" {
		return template
	}
	if template, ok := s.cfg.Team.Templates[s.cfg.Team.DefaultTemplate]; ok && template.Name != "" {
		return template
	}
	return config.TeamTemplate{}
}

func (s *AgencyService) defaultOfficeStatusLocked(constitutionName string, constitution config.AgencyConstitution) AgencyOfficeStatus {
	if s.cfg == nil {
		return AgencyOfficeStatus{}
	}
	consensusMode := constitution.Policies.ConsensusMode
	if consensusMode == "" {
		consensusMode = s.cfg.Agency.Ledger.ConsensusMode
	}
	defaultQuorum := constitution.Policies.DefaultQuorum
	if defaultQuorum == 0 {
		defaultQuorum = s.cfg.Agency.Ledger.DefaultQuorum
	}
	return AgencyOfficeStatus{
		ProductName:     s.cfg.Agency.ProductName,
		Mode:            s.cfg.Agency.Office.Mode,
		Running:         false,
		Constitution:    constitutionName,
		SharedWorkplace: s.cfg.Agency.Office.SharedWorkplace,
		RedisAddress:    s.cfg.Agency.Redis.Address,
		LedgerPath:      s.cfg.Agency.Ledger.Path,
		ConsensusMode:   consensusMode,
		DefaultQuorum:   defaultQuorum,
	}
}

func (s *AgencyService) ensureOfficeFilesystemLocked() error {
	if s.cfg == nil {
		return fmt.Errorf("config not loaded")
	}

	dirs := []string{
		s.cfg.Agency.Office.SharedWorkplace,
		s.cfg.Agency.Ledger.SnapshotPath,
		filepath.Dir(s.stateFile),
	}
	for _, dir := range dirs {
		dir = strings.TrimSpace(dir)
		if dir == "" || dir == "." {
			continue
		}
		if err := os.MkdirAll(dir, 0o755); err != nil {
			return fmt.Errorf("create agency runtime directory %q: %w", dir, err)
		}
	}
	return nil
}

func (s *AgencyService) readStateLocked() (agencyState, error) {
	if s.stateFile == "" {
		return agencyState{}, nil
	}

	data, err := os.ReadFile(s.stateFile)
	if err != nil {
		if os.IsNotExist(err) {
			return agencyState{}, nil
		}
		return agencyState{}, fmt.Errorf("read agency state: %w", err)
	}

	var state agencyState
	if err := json.Unmarshal(data, &state); err != nil {
		return agencyState{}, fmt.Errorf("decode agency state: %w", err)
	}
	return state, nil
}

func (s *AgencyService) writeStateLocked(state agencyState) error {
	if s.stateFile == "" {
		return nil
	}

	if err := os.MkdirAll(filepath.Dir(s.stateFile), 0o755); err != nil {
		return fmt.Errorf("create agency state directory: %w", err)
	}

	data, err := json.MarshalIndent(state, "", "  ")
	if err != nil {
		return fmt.Errorf("encode agency state: %w", err)
	}
	if err := os.WriteFile(s.stateFile, data, 0o644); err != nil {
		return fmt.Errorf("write agency state: %w", err)
	}
	return nil
}

func buildGenesisSummary(req AgencyGenesisRequest, constitution config.AgencyConstitution, roleCount int) string {
	parts := []string{
		fmt.Sprintf("Intent: %s.", strings.TrimSpace(req.Intent)),
		fmt.Sprintf("Constitution: %s.", constitution.Name),
		fmt.Sprintf("Governance: %s.", firstNonEmpty(req.Governance, constitution.Governance)),
		fmt.Sprintf("Planned roles: %d.", roleCount),
	}
	if req.Domain != "" {
		parts = append(parts, fmt.Sprintf("Domain: %s.", req.Domain))
	}
	if req.TimeHorizon != "" {
		parts = append(parts, fmt.Sprintf("Time horizon: %s.", req.TimeHorizon))
	}
	if req.WorkingStyle != "" {
		parts = append(parts, fmt.Sprintf("Working style: %s.", req.WorkingStyle))
	}
	if req.GoalShape != "" {
		parts = append(parts, fmt.Sprintf("Goal shape: %s.", req.GoalShape))
	}
	return strings.Join(parts, " ")
}

func roleTemplatesFromBundles(bundles []agencypkg.GenesisRoleBundle) []config.TeamRoleTemplate {
	roles := make([]config.TeamRoleTemplate, 0, len(bundles))
	for _, bundle := range bundles {
		roles = append(roles, config.TeamRoleTemplate{
			Name:              bundle.RoleName,
			Responsible:       bundle.Mission,
			CurrentFocus:      bundle.WorkingPosture,
			Profile:           bundle.Profile,
			Prompt:            bundle.SystemPrompt,
			ReportsTo:         bundle.ReportsTo,
			CanSpawnSubagents: boolPointer(bundle.CanSpawnAgents),
		})
	}
	return roles
}

func firstNonEmpty(values ...string) string {
	for _, value := range values {
		if strings.TrimSpace(value) != "" {
			return value
		}
	}
	return ""
}

func timestampPtr(value time.Time) *time.Time {
	return &value
}

func boolValue(value *bool) bool {
	return value != nil && *value
}

func boolPointer(value bool) *bool {
	return &value
}
