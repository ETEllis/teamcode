package config

import (
	"fmt"
	"path/filepath"
	"strings"
)

type AgencyConfig struct {
	Enabled             *bool                         `json:"enabled,omitempty"`
	ProductName         string                        `json:"productName,omitempty"`
	CurrentConstitution string                        `json:"currentConstitution,omitempty"`
	SoloConstitution    string                        `json:"soloConstitution,omitempty"`
	Office              OfficeRuntimeConfig           `json:"office,omitempty"`
	Docker              DockerRuntimeConfig           `json:"docker,omitempty"`
	Redis               RedisRuntimeConfig            `json:"redis,omitempty"`
	Ledger              LedgerRuntimeConfig           `json:"ledger,omitempty"`
	Voice               VoiceRuntimeConfig            `json:"voice,omitempty"`
	Schedules           ScheduleDefaultsConfig        `json:"schedules,omitempty"`
	Genesis             GenesisRuntimeConfig          `json:"genesis,omitempty"`
	Constitutions       map[string]AgencyConstitution `json:"constitutions,omitempty"`
}

type OfficeRuntimeConfig struct {
	Enabled              *bool  `json:"enabled,omitempty"`
	Mode                 string `json:"mode,omitempty"`
	AutoBoot             *bool  `json:"autoBoot,omitempty"`
	SharedWorkplace      string `json:"sharedWorkplace,omitempty"`
	StateFile            string `json:"stateFile,omitempty"`
	DefaultWorkspaceMode string `json:"defaultWorkspaceMode,omitempty"`
	AllowSoloFallback    *bool  `json:"allowSoloFallback,omitempty"`
}

type DockerRuntimeConfig struct {
	Enabled        *bool  `json:"enabled,omitempty"`
	ComposeProject string `json:"composeProject,omitempty"`
	ComposeFile    string `json:"composeFile,omitempty"`
	Image          string `json:"image,omitempty"`
	SharedVolume   string `json:"sharedVolume,omitempty"`
	Network        string `json:"network,omitempty"`
}

type RedisRuntimeConfig struct {
	Enabled       *bool  `json:"enabled,omitempty"`
	Address       string `json:"address,omitempty"`
	DB            int    `json:"db,omitempty"`
	ChannelPrefix string `json:"channelPrefix,omitempty"`
}

type LedgerRuntimeConfig struct {
	Backend        string `json:"backend,omitempty"`
	Path           string `json:"path,omitempty"`
	SnapshotPath   string `json:"snapshotPath,omitempty"`
	ConsensusMode  string `json:"consensusMode,omitempty"`
	DefaultQuorum  int    `json:"defaultQuorum,omitempty"`
	ProjectionFile string `json:"projectionFile,omitempty"`
}

type ScheduleDefaultsConfig struct {
	Timezone            string           `json:"timezone,omitempty"`
	DefaultCadence      string           `json:"defaultCadence,omitempty"`
	WakeOnOfficeOpen    *bool            `json:"wakeOnOfficeOpen,omitempty"`
	RequireShiftHandoff *bool            `json:"requireShiftHandoff,omitempty"`
	Windows             []ScheduleWindow `json:"windows,omitempty"`
}

type ScheduleWindow struct {
	Name  string   `json:"name,omitempty"`
	Days  []string `json:"days,omitempty"`
	Start string   `json:"start,omitempty"`
	End   string   `json:"end,omitempty"`
}

type GenesisRuntimeConfig struct {
	ConversationDriven *bool  `json:"conversationDriven,omitempty"`
	AutoResearch       *bool  `json:"autoResearch,omitempty"`
	AutoSkills         *bool  `json:"autoSkills,omitempty"`
	AutoToolBinding    *bool  `json:"autoToolBinding,omitempty"`
	SequentialSpawn    *bool  `json:"sequentialSpawn,omitempty"`
	DefaultTopology    string `json:"defaultTopology,omitempty"`
}

type VoiceRuntimeConfig struct {
	Enabled              *bool                 `json:"enabled,omitempty"`
	Provider             string                `json:"provider,omitempty"`
	GatewayState         string                `json:"gatewayState,omitempty"`
	AssetDir             string                `json:"assetDir,omitempty"`
	ControlChannel       string                `json:"controlChannel,omitempty"`
	SynthesisChannel     string                `json:"synthesisChannel,omitempty"`
	MeetingTranscriptDir string                `json:"meetingTranscriptDir,omitempty"`
	Projection           VoiceProjectionConfig `json:"projection,omitempty"`
	STT                  VoiceEngineConfig     `json:"stt,omitempty"`
	TTS                  VoiceEngineConfig     `json:"tts,omitempty"`
}

type VoiceProjectionConfig struct {
	DefaultRoom           string `json:"defaultRoom,omitempty"`
	TranscriptProjection  *bool  `json:"transcriptProjection,omitempty"`
	AudioProjection       *bool  `json:"audioProjection,omitempty"`
	AutoProjectTranscript *bool  `json:"autoProjectTranscript,omitempty"`
	AutoProjectSynthesis  *bool  `json:"autoProjectSynthesis,omitempty"`
}

type VoiceEngineConfig struct {
	Enabled     *bool    `json:"enabled,omitempty"`
	Command     string   `json:"command,omitempty"`
	Args        []string `json:"args,omitempty"`
	InputMode   string   `json:"inputMode,omitempty"`
	OutputMode  string   `json:"outputMode,omitempty"`
	Language    string   `json:"language,omitempty"`
	Voice       string   `json:"voice,omitempty"`
	AudioFormat string   `json:"audioFormat,omitempty"`
	Timeout     string   `json:"timeout,omitempty"`
}

type AgencyConstitution struct {
	Name            string                     `json:"name,omitempty"`
	Description     string                     `json:"description,omitempty"`
	Blueprint       string                     `json:"blueprint,omitempty"`
	TeamTemplate    string                     `json:"teamTemplate,omitempty"`
	Governance      string                     `json:"governance,omitempty"`
	RuntimeMode     string                     `json:"runtimeMode,omitempty"`
	EntryMode       string                     `json:"entryMode,omitempty"`
	DefaultSchedule string                     `json:"defaultSchedule,omitempty"`
	Policies        AgencyConstitutionPolicies `json:"policies,omitempty"`
}

type AgencyConstitutionPolicies struct {
	WakeMode          string `json:"wakeMode,omitempty"`
	ConsensusMode     string `json:"consensusMode,omitempty"`
	PublicationPolicy string `json:"publicationPolicy,omitempty"`
	SpawnMode         string `json:"spawnMode,omitempty"`
	DefaultQuorum     int    `json:"defaultQuorum,omitempty"`
}

func DefaultAgencyConfig(teamCfg TeamConfig, dataDir string) AgencyConfig {
	constitutions := map[string]AgencyConstitution{
		"solo": {
			Name:            "solo",
			Description:     "Preserved solo coding constitution using the existing TeamCode/OpenCode-derived flow.",
			Blueprint:       "solo",
			TeamTemplate:    "solo",
			Governance:      "solo",
			RuntimeMode:     "interactive-session",
			EntryMode:       "solo",
			DefaultSchedule: "always-on",
			Policies: AgencyConstitutionPolicies{
				WakeMode:          "interactive",
				ConsensusMode:     "single-actor",
				PublicationPolicy: "self-attested",
				SpawnMode:         "delegated",
				DefaultQuorum:     1,
			},
		},
		"coding-office": {
			Name:            "coding-office",
			Description:     "Shared software delivery office backed by the Agency runtime and the existing engineering blueprint.",
			Blueprint:       "software-team",
			TeamTemplate:    "leader-led",
			Governance:      "hierarchical",
			RuntimeMode:     "distributed-office",
			EntryMode:       "office",
			DefaultSchedule: "weekday-office-hours",
			Policies: AgencyConstitutionPolicies{
				WakeMode:          "event-reactive",
				ConsensusMode:     "role-quorum",
				PublicationPolicy: "review-gated",
				SpawnMode:         "delegated",
				DefaultQuorum:     2,
			},
		},
		"full-agency": {
			Name:            "full-agency",
			Description:     "General persistent organization constitution for multi-role, long-running work.",
			Blueprint:       "freeform",
			TeamTemplate:    "freeform",
			Governance:      "hybrid",
			RuntimeMode:     "distributed-office",
			EntryMode:       "office",
			DefaultSchedule: "weekday-office-hours",
			Policies: AgencyConstitutionPolicies{
				WakeMode:          "event-reactive",
				ConsensusMode:     "distributed-consensus",
				PublicationPolicy: "quorum-gated",
				SpawnMode:         "federated",
				DefaultQuorum:     2,
			},
		},
	}

	for name, blueprint := range teamCfg.Blueprints {
		if _, exists := constitutions[name]; exists {
			continue
		}
		templateName := name
		if _, ok := teamCfg.Templates[templateName]; !ok {
			templateName = teamCfg.DefaultTemplate
		}
		constitutions[name] = AgencyConstitution{
			Name:            name,
			Description:     blueprint.Description,
			Blueprint:       name,
			TeamTemplate:    templateName,
			Governance:      governanceFromLeadershipMode(blueprint.LeadershipMode),
			RuntimeMode:     "distributed-office",
			EntryMode:       "office",
			DefaultSchedule: "weekday-office-hours",
			Policies: AgencyConstitutionPolicies{
				WakeMode:          "event-reactive",
				ConsensusMode:     consensusFromLeadershipMode(blueprint.LeadershipMode),
				PublicationPolicy: publicationPolicyForTemplate(blueprint),
				SpawnMode:         spawnModeForTemplate(blueprint),
				DefaultQuorum:     quorumFromTemplate(blueprint),
			},
		}
	}

	return AgencyConfig{
		Enabled:             boolPtr(false),
		ProductName:         "The Agency",
		CurrentConstitution: "coding-office",
		SoloConstitution:    "solo",
		Office: OfficeRuntimeConfig{
			Enabled:              boolPtr(false),
			Mode:                 "distributed-office",
			AutoBoot:             boolPtr(false),
			SharedWorkplace:      filepath.Join(dataDir, "agency", "shared_workplace"),
			StateFile:            filepath.Join(dataDir, "agency", "office-state.json"),
			DefaultWorkspaceMode: "shared",
			AllowSoloFallback:    boolPtr(true),
		},
		Docker: DockerRuntimeConfig{
			Enabled:        boolPtr(true),
			ComposeProject: "the-agency",
			ComposeFile:    "docker-compose.agency.yml",
			Image:          "ghcr.io/etellis/the-agency:latest",
			SharedVolume:   "agency_shared_workplace",
			Network:        "agency_office",
		},
		Redis: RedisRuntimeConfig{
			Enabled:       boolPtr(true),
			Address:       "127.0.0.1:6379",
			DB:            7,
			ChannelPrefix: "agency",
		},
		Ledger: LedgerRuntimeConfig{
			Backend:        "append-only-log",
			Path:           filepath.Join(dataDir, "agency", "ledger.log"),
			SnapshotPath:   filepath.Join(dataDir, "agency", "snapshots"),
			ConsensusMode:  "distributed-consensus",
			DefaultQuorum:  2,
			ProjectionFile: filepath.Join(dataDir, "agency", "context-snapshot.json"),
		},
		Voice: VoiceRuntimeConfig{
			Enabled:              boolPtr(false),
			Provider:             "local",
			GatewayState:         filepath.Join(dataDir, "agency", "voice-gateway.json"),
			AssetDir:             filepath.Join(dataDir, "agency", "voice"),
			ControlChannel:       "agency.voice.control",
			SynthesisChannel:     "agency.voice.synthesis",
			MeetingTranscriptDir: filepath.Join(dataDir, "agency", "voice", "transcripts"),
			Projection: VoiceProjectionConfig{
				DefaultRoom:           "strategy-room",
				TranscriptProjection:  boolPtr(true),
				AudioProjection:       boolPtr(false),
				AutoProjectTranscript: boolPtr(true),
				AutoProjectSynthesis:  boolPtr(false),
			},
			STT: VoiceEngineConfig{
				Enabled:     boolPtr(false),
				InputMode:   "file",
				OutputMode:  "stdout",
				Language:    "en",
				AudioFormat: "wav",
				Timeout:     "2m",
			},
			TTS: VoiceEngineConfig{
				Enabled:     boolPtr(false),
				Command:     "",
				Args:        []string{"--voice", "{voice}", "--output", "{output}", "--text", "{text}"},
				InputMode:   "text",
				OutputMode:  "file",
				Language:    "en",
				Voice:       "af_heart",
				AudioFormat: "wav",
				Timeout:     "30s",
			},
		},
		Schedules: ScheduleDefaultsConfig{
			Timezone:            "local",
			DefaultCadence:      "weekday-office-hours",
			WakeOnOfficeOpen:    boolPtr(true),
			RequireShiftHandoff: boolPtr(true),
			Windows: []ScheduleWindow{
				{
					Name:  "weekday-office-hours",
					Days:  []string{"mon", "tue", "wed", "thu", "fri"},
					Start: "09:00",
					End:   "17:00",
				},
			},
		},
		Genesis: GenesisRuntimeConfig{
			ConversationDriven: boolPtr(true),
			AutoResearch:       boolPtr(true),
			AutoSkills:         boolPtr(true),
			AutoToolBinding:    boolPtr(true),
			SequentialSpawn:    boolPtr(true),
			DefaultTopology:    "hierarchical",
		},
		Constitutions: constitutions,
	}
}

func normalizeAgencyConfig(cfg AgencyConfig, teamCfg TeamConfig, dataDir string) AgencyConfig {
	defaults := DefaultAgencyConfig(teamCfg, dataDir)

	if cfg.Enabled != nil {
		defaults.Enabled = cfg.Enabled
	}
	if cfg.ProductName != "" {
		defaults.ProductName = cfg.ProductName
	}
	if cfg.CurrentConstitution != "" {
		defaults.CurrentConstitution = cfg.CurrentConstitution
	}
	if cfg.SoloConstitution != "" {
		defaults.SoloConstitution = cfg.SoloConstitution
	}

	defaults.Office = mergeOfficeRuntimeConfig(defaults.Office, cfg.Office, dataDir)
	defaults.Docker = mergeDockerRuntimeConfig(defaults.Docker, cfg.Docker)
	defaults.Redis = mergeRedisRuntimeConfig(defaults.Redis, cfg.Redis)
	defaults.Ledger = mergeLedgerRuntimeConfig(defaults.Ledger, cfg.Ledger, dataDir)
	defaults.Voice = mergeVoiceRuntimeConfig(defaults.Voice, cfg.Voice, dataDir)
	defaults.Schedules = mergeScheduleDefaultsConfig(defaults.Schedules, cfg.Schedules)
	defaults.Genesis = mergeGenesisRuntimeConfig(defaults.Genesis, cfg.Genesis)

	if defaults.Constitutions == nil {
		defaults.Constitutions = map[string]AgencyConstitution{}
	}
	for name, constitution := range cfg.Constitutions {
		base, ok := defaults.Constitutions[name]
		if ok {
			defaults.Constitutions[name] = mergeAgencyConstitution(base, constitution)
			continue
		}
		defaults.Constitutions[name] = normalizeAgencyConstitution(name, constitution, defaults.Ledger.DefaultQuorum)
	}

	if defaults.CurrentConstitution == "" {
		defaults.CurrentConstitution = defaults.SoloConstitution
	}
	if defaults.SoloConstitution == "" {
		defaults.SoloConstitution = "solo"
	}
	if _, ok := defaults.Constitutions[defaults.CurrentConstitution]; !ok {
		defaults.CurrentConstitution = defaults.SoloConstitution
	}
	if _, ok := defaults.Constitutions[defaults.SoloConstitution]; !ok {
		defaults.SoloConstitution = defaults.CurrentConstitution
	}

	return defaults
}

func ValidateAgencyConfig(cfg AgencyConfig) error {
	if cfg.CurrentConstitution == "" {
		return fmt.Errorf("agency.currentConstitution must not be empty")
	}
	if len(cfg.Constitutions) == 0 {
		return fmt.Errorf("agency.constitutions must define at least one constitution")
	}
	if _, ok := cfg.Constitutions[cfg.CurrentConstitution]; !ok {
		return fmt.Errorf("agency.currentConstitution %q is not defined", cfg.CurrentConstitution)
	}
	if cfg.SoloConstitution != "" {
		if _, ok := cfg.Constitutions[cfg.SoloConstitution]; !ok {
			return fmt.Errorf("agency.soloConstitution %q is not defined", cfg.SoloConstitution)
		}
	}
	if strings.TrimSpace(cfg.Office.SharedWorkplace) == "" {
		return fmt.Errorf("agency.office.sharedWorkplace must not be empty")
	}
	if strings.TrimSpace(cfg.Office.StateFile) == "" {
		return fmt.Errorf("agency.office.stateFile must not be empty")
	}
	if cfg.Redis.DB < 0 {
		return fmt.Errorf("agency.redis.db must be greater than or equal to 0")
	}
	if cfg.Ledger.DefaultQuorum < 1 {
		return fmt.Errorf("agency.ledger.defaultQuorum must be at least 1")
	}
	if cfg.Voice.Enabled != nil && *cfg.Voice.Enabled {
		if strings.TrimSpace(cfg.Voice.GatewayState) == "" {
			return fmt.Errorf("agency.voice.gatewayState must not be empty when voice is enabled")
		}
		if strings.TrimSpace(cfg.Voice.AssetDir) == "" {
			return fmt.Errorf("agency.voice.assetDir must not be empty when voice is enabled")
		}
	}
	return nil
}

func AgencyEnabled(cfg *Config) bool {
	if cfg == nil || cfg.Agency.Enabled == nil {
		return false
	}
	return *cfg.Agency.Enabled
}

func ActiveConstitution(cfg *Config) AgencyConstitution {
	if cfg == nil {
		return AgencyConstitution{}
	}
	if constitution, ok := cfg.Agency.Constitutions[cfg.Agency.CurrentConstitution]; ok {
		return constitution
	}
	if constitution, ok := cfg.Agency.Constitutions[cfg.Agency.SoloConstitution]; ok {
		return constitution
	}
	return AgencyConstitution{}
}

func UpdateAgencyConstitution(name string) error {
	if cfg == nil {
		return fmt.Errorf("config not loaded")
	}
	if _, ok := cfg.Agency.Constitutions[name]; !ok {
		return fmt.Errorf("agency constitution %q not found", name)
	}

	cfg.Agency.CurrentConstitution = name
	if cfg.Agency.Enabled == nil {
		cfg.Agency.Enabled = boolPtr(true)
	} else {
		*cfg.Agency.Enabled = true
	}

	return updateCfgFile(func(config *Config) {
		if config == nil {
			return
		}
		config.Agency.CurrentConstitution = name
		if config.Agency.Enabled == nil {
			config.Agency.Enabled = boolPtr(true)
		} else {
			*config.Agency.Enabled = true
		}
	})
}

func mergeOfficeRuntimeConfig(base, override OfficeRuntimeConfig, dataDir string) OfficeRuntimeConfig {
	if override.Enabled != nil {
		base.Enabled = override.Enabled
	}
	if override.Mode != "" {
		base.Mode = override.Mode
	}
	if override.AutoBoot != nil {
		base.AutoBoot = override.AutoBoot
	}
	if override.SharedWorkplace != "" {
		base.SharedWorkplace = override.SharedWorkplace
	}
	if override.StateFile != "" {
		base.StateFile = override.StateFile
	}
	if override.DefaultWorkspaceMode != "" {
		base.DefaultWorkspaceMode = override.DefaultWorkspaceMode
	}
	if override.AllowSoloFallback != nil {
		base.AllowSoloFallback = override.AllowSoloFallback
	}
	if base.SharedWorkplace == "" {
		base.SharedWorkplace = filepath.Join(dataDir, "agency", "shared_workplace")
	}
	if base.StateFile == "" {
		base.StateFile = filepath.Join(dataDir, "agency", "office-state.json")
	}
	if base.Mode == "" {
		base.Mode = "distributed-office"
	}
	if base.DefaultWorkspaceMode == "" {
		base.DefaultWorkspaceMode = "shared"
	}
	if base.Enabled == nil {
		base.Enabled = boolPtr(false)
	}
	if base.AutoBoot == nil {
		base.AutoBoot = boolPtr(false)
	}
	if base.AllowSoloFallback == nil {
		base.AllowSoloFallback = boolPtr(true)
	}
	return base
}

func mergeDockerRuntimeConfig(base, override DockerRuntimeConfig) DockerRuntimeConfig {
	if override.Enabled != nil {
		base.Enabled = override.Enabled
	}
	if override.ComposeProject != "" {
		base.ComposeProject = override.ComposeProject
	}
	if override.ComposeFile != "" {
		base.ComposeFile = override.ComposeFile
	}
	if override.Image != "" {
		base.Image = override.Image
	}
	if override.SharedVolume != "" {
		base.SharedVolume = override.SharedVolume
	}
	if override.Network != "" {
		base.Network = override.Network
	}
	if base.Enabled == nil {
		base.Enabled = boolPtr(true)
	}
	return base
}

func mergeRedisRuntimeConfig(base, override RedisRuntimeConfig) RedisRuntimeConfig {
	if override.Enabled != nil {
		base.Enabled = override.Enabled
	}
	if override.Address != "" {
		base.Address = override.Address
	}
	if override.DB != 0 {
		base.DB = override.DB
	}
	if override.ChannelPrefix != "" {
		base.ChannelPrefix = override.ChannelPrefix
	}
	if base.Enabled == nil {
		base.Enabled = boolPtr(true)
	}
	if base.Address == "" {
		base.Address = "127.0.0.1:6379"
	}
	if base.ChannelPrefix == "" {
		base.ChannelPrefix = "agency"
	}
	return base
}

func mergeLedgerRuntimeConfig(base, override LedgerRuntimeConfig, dataDir string) LedgerRuntimeConfig {
	if override.Backend != "" {
		base.Backend = override.Backend
	}
	if override.Path != "" {
		base.Path = override.Path
	}
	if override.SnapshotPath != "" {
		base.SnapshotPath = override.SnapshotPath
	}
	if override.ConsensusMode != "" {
		base.ConsensusMode = override.ConsensusMode
	}
	if override.DefaultQuorum != 0 {
		base.DefaultQuorum = override.DefaultQuorum
	}
	if override.ProjectionFile != "" {
		base.ProjectionFile = override.ProjectionFile
	}
	if base.Backend == "" {
		base.Backend = "append-only-log"
	}
	if base.Path == "" {
		base.Path = filepath.Join(dataDir, "agency", "ledger.log")
	}
	if base.SnapshotPath == "" {
		base.SnapshotPath = filepath.Join(dataDir, "agency", "snapshots")
	}
	if base.ProjectionFile == "" {
		base.ProjectionFile = filepath.Join(dataDir, "agency", "context-snapshot.json")
	}
	if base.ConsensusMode == "" {
		base.ConsensusMode = "distributed-consensus"
	}
	if base.DefaultQuorum == 0 {
		base.DefaultQuorum = 2
	}
	return base
}

func mergeVoiceRuntimeConfig(base, override VoiceRuntimeConfig, dataDir string) VoiceRuntimeConfig {
	if override.Enabled != nil {
		base.Enabled = override.Enabled
	}
	if override.Provider != "" {
		base.Provider = override.Provider
	}
	if override.GatewayState != "" {
		base.GatewayState = override.GatewayState
	}
	if override.AssetDir != "" {
		base.AssetDir = override.AssetDir
	}
	if override.ControlChannel != "" {
		base.ControlChannel = override.ControlChannel
	}
	if override.SynthesisChannel != "" {
		base.SynthesisChannel = override.SynthesisChannel
	}
	if override.MeetingTranscriptDir != "" {
		base.MeetingTranscriptDir = override.MeetingTranscriptDir
	}
	base.Projection = mergeVoiceProjectionConfig(base.Projection, override.Projection)
	base.STT = mergeVoiceEngineConfig(base.STT, override.STT)
	base.TTS = mergeVoiceEngineConfig(base.TTS, override.TTS)
	if base.Provider == "" {
		base.Provider = "local"
	}
	if base.GatewayState == "" {
		base.GatewayState = filepath.Join(dataDir, "agency", "voice-gateway.json")
	}
	if base.AssetDir == "" {
		base.AssetDir = filepath.Join(dataDir, "agency", "voice")
	}
	if base.ControlChannel == "" {
		base.ControlChannel = "agency.voice.control"
	}
	if base.SynthesisChannel == "" {
		base.SynthesisChannel = "agency.voice.synthesis"
	}
	if base.MeetingTranscriptDir == "" {
		base.MeetingTranscriptDir = filepath.Join(dataDir, "agency", "voice", "transcripts")
	}
	if base.Enabled == nil {
		base.Enabled = boolPtr(false)
	}
	return base
}

func mergeVoiceProjectionConfig(base, override VoiceProjectionConfig) VoiceProjectionConfig {
	if override.DefaultRoom != "" {
		base.DefaultRoom = override.DefaultRoom
	}
	if override.TranscriptProjection != nil {
		base.TranscriptProjection = override.TranscriptProjection
	}
	if override.AudioProjection != nil {
		base.AudioProjection = override.AudioProjection
	}
	if override.AutoProjectTranscript != nil {
		base.AutoProjectTranscript = override.AutoProjectTranscript
	}
	if override.AutoProjectSynthesis != nil {
		base.AutoProjectSynthesis = override.AutoProjectSynthesis
	}
	if base.DefaultRoom == "" {
		base.DefaultRoom = "strategy-room"
	}
	if base.TranscriptProjection == nil {
		base.TranscriptProjection = boolPtr(true)
	}
	if base.AudioProjection == nil {
		base.AudioProjection = boolPtr(false)
	}
	if base.AutoProjectTranscript == nil {
		base.AutoProjectTranscript = boolPtr(true)
	}
	if base.AutoProjectSynthesis == nil {
		base.AutoProjectSynthesis = boolPtr(false)
	}
	return base
}

func mergeVoiceEngineConfig(base, override VoiceEngineConfig) VoiceEngineConfig {
	if override.Enabled != nil {
		base.Enabled = override.Enabled
	}
	if override.Command != "" {
		base.Command = override.Command
	}
	if len(override.Args) > 0 {
		base.Args = append([]string(nil), override.Args...)
	}
	if override.InputMode != "" {
		base.InputMode = override.InputMode
	}
	if override.OutputMode != "" {
		base.OutputMode = override.OutputMode
	}
	if override.Language != "" {
		base.Language = override.Language
	}
	if override.Voice != "" {
		base.Voice = override.Voice
	}
	if override.AudioFormat != "" {
		base.AudioFormat = override.AudioFormat
	}
	if override.Timeout != "" {
		base.Timeout = override.Timeout
	}
	if base.Enabled == nil {
		base.Enabled = boolPtr(false)
	}
	if base.InputMode == "" {
		base.InputMode = "file"
	}
	if base.OutputMode == "" {
		base.OutputMode = "stdout"
	}
	if base.AudioFormat == "" {
		base.AudioFormat = "wav"
	}
	if base.Timeout == "" {
		base.Timeout = "30s"
	}
	return base
}

func mergeScheduleDefaultsConfig(base, override ScheduleDefaultsConfig) ScheduleDefaultsConfig {
	if override.Timezone != "" {
		base.Timezone = override.Timezone
	}
	if override.DefaultCadence != "" {
		base.DefaultCadence = override.DefaultCadence
	}
	if override.WakeOnOfficeOpen != nil {
		base.WakeOnOfficeOpen = override.WakeOnOfficeOpen
	}
	if override.RequireShiftHandoff != nil {
		base.RequireShiftHandoff = override.RequireShiftHandoff
	}
	if len(override.Windows) > 0 {
		base.Windows = override.Windows
	}
	if base.Timezone == "" {
		base.Timezone = "local"
	}
	if base.DefaultCadence == "" {
		base.DefaultCadence = "weekday-office-hours"
	}
	if base.WakeOnOfficeOpen == nil {
		base.WakeOnOfficeOpen = boolPtr(true)
	}
	if base.RequireShiftHandoff == nil {
		base.RequireShiftHandoff = boolPtr(true)
	}
	return base
}

func mergeGenesisRuntimeConfig(base, override GenesisRuntimeConfig) GenesisRuntimeConfig {
	if override.ConversationDriven != nil {
		base.ConversationDriven = override.ConversationDriven
	}
	if override.AutoResearch != nil {
		base.AutoResearch = override.AutoResearch
	}
	if override.AutoSkills != nil {
		base.AutoSkills = override.AutoSkills
	}
	if override.AutoToolBinding != nil {
		base.AutoToolBinding = override.AutoToolBinding
	}
	if override.SequentialSpawn != nil {
		base.SequentialSpawn = override.SequentialSpawn
	}
	if override.DefaultTopology != "" {
		base.DefaultTopology = override.DefaultTopology
	}
	if base.ConversationDriven == nil {
		base.ConversationDriven = boolPtr(true)
	}
	if base.AutoResearch == nil {
		base.AutoResearch = boolPtr(true)
	}
	if base.AutoSkills == nil {
		base.AutoSkills = boolPtr(true)
	}
	if base.AutoToolBinding == nil {
		base.AutoToolBinding = boolPtr(true)
	}
	if base.SequentialSpawn == nil {
		base.SequentialSpawn = boolPtr(true)
	}
	if base.DefaultTopology == "" {
		base.DefaultTopology = "hierarchical"
	}
	return base
}

func mergeAgencyConstitution(base, override AgencyConstitution) AgencyConstitution {
	if override.Name != "" {
		base.Name = override.Name
	}
	if override.Description != "" {
		base.Description = override.Description
	}
	if override.Blueprint != "" {
		base.Blueprint = override.Blueprint
	}
	if override.TeamTemplate != "" {
		base.TeamTemplate = override.TeamTemplate
	}
	if override.Governance != "" {
		base.Governance = override.Governance
	}
	if override.RuntimeMode != "" {
		base.RuntimeMode = override.RuntimeMode
	}
	if override.EntryMode != "" {
		base.EntryMode = override.EntryMode
	}
	if override.DefaultSchedule != "" {
		base.DefaultSchedule = override.DefaultSchedule
	}
	base.Policies = mergeAgencyConstitutionPolicies(base.Policies, override.Policies)
	return normalizeAgencyConstitution(base.Name, base, base.Policies.DefaultQuorum)
}

func mergeAgencyConstitutionPolicies(base, override AgencyConstitutionPolicies) AgencyConstitutionPolicies {
	if override.WakeMode != "" {
		base.WakeMode = override.WakeMode
	}
	if override.ConsensusMode != "" {
		base.ConsensusMode = override.ConsensusMode
	}
	if override.PublicationPolicy != "" {
		base.PublicationPolicy = override.PublicationPolicy
	}
	if override.SpawnMode != "" {
		base.SpawnMode = override.SpawnMode
	}
	if override.DefaultQuorum != 0 {
		base.DefaultQuorum = override.DefaultQuorum
	}
	return base
}

func normalizeAgencyConstitution(name string, constitution AgencyConstitution, defaultQuorum int) AgencyConstitution {
	if constitution.Name == "" {
		constitution.Name = name
	}
	if constitution.Governance == "" {
		constitution.Governance = "hybrid"
	}
	if constitution.RuntimeMode == "" {
		constitution.RuntimeMode = "distributed-office"
	}
	if constitution.EntryMode == "" {
		constitution.EntryMode = "office"
	}
	if constitution.DefaultSchedule == "" {
		constitution.DefaultSchedule = "weekday-office-hours"
	}
	if constitution.Policies.WakeMode == "" {
		constitution.Policies.WakeMode = "event-reactive"
	}
	if constitution.Policies.ConsensusMode == "" {
		constitution.Policies.ConsensusMode = "distributed-consensus"
	}
	if constitution.Policies.PublicationPolicy == "" {
		constitution.Policies.PublicationPolicy = "quorum-gated"
	}
	if constitution.Policies.SpawnMode == "" {
		constitution.Policies.SpawnMode = "delegated"
	}
	if constitution.Policies.DefaultQuorum == 0 {
		if defaultQuorum > 0 {
			constitution.Policies.DefaultQuorum = defaultQuorum
		} else {
			constitution.Policies.DefaultQuorum = 2
		}
	}
	return constitution
}

func governanceFromLeadershipMode(mode string) string {
	switch mode {
	case "solo":
		return "solo"
	case "peer":
		return "flat"
	case "leader-led":
		return "hierarchical"
	default:
		return "hybrid"
	}
}

func consensusFromLeadershipMode(mode string) string {
	switch mode {
	case "solo":
		return "single-actor"
	case "peer":
		return "peer-quorum"
	default:
		return "role-quorum"
	}
}

func publicationPolicyForTemplate(template TeamTemplate) string {
	if template.Policies.ReviewRequired != nil && *template.Policies.ReviewRequired {
		return "review-gated"
	}
	return "self-attested"
}

func spawnModeForTemplate(template TeamTemplate) string {
	if template.Policies.AllowsSubagents != nil && *template.Policies.AllowsSubagents {
		return "delegated"
	}
	return "bounded"
}

func quorumFromTemplate(template TeamTemplate) int {
	if template.Policies.ConcurrencyBudget != nil && *template.Policies.ConcurrencyBudget > 1 {
		return 2
	}
	return 1
}
