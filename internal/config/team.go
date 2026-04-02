package config

type TeamConfig struct {
	ActiveTeam       string                  `json:"activeTeam,omitempty"`
	DefaultTemplate  string                  `json:"defaultTemplate,omitempty"`
	DefaultBlueprint string                  `json:"defaultBlueprint,omitempty"`
	CollaborationHUD *bool                   `json:"collaborationHud,omitempty"`
	Templates        map[string]TeamTemplate `json:"templates,omitempty"`
	Blueprints       map[string]TeamTemplate `json:"blueprints,omitempty"`
}

type TeamTemplate struct {
	Name           string             `json:"name,omitempty"`
	Description    string             `json:"description,omitempty"`
	Category       string             `json:"category,omitempty"`
	Orientation    string             `json:"orientation,omitempty"`
	LeadershipMode string             `json:"leadershipMode,omitempty"`
	SpawnTeammates *bool              `json:"spawnTeammates,omitempty"`
	Roles          []TeamRoleTemplate `json:"roles,omitempty"`
	Policies       TeamPolicies       `json:"policies,omitempty"`
}

type TeamRoleTemplate struct {
	Name              string `json:"name"`
	Responsible       string `json:"responsible,omitempty"`
	CurrentFocus      string `json:"currentFocus,omitempty"`
	Profile           string `json:"profile,omitempty"`
	Prompt            string `json:"prompt,omitempty"`
	ReportsTo         string `json:"reportsTo,omitempty"`
	CanSpawnSubagents *bool  `json:"canSpawnSubagents,omitempty"`
}

type TeamPolicies struct {
	CommitMessageFormat  string   `json:"commitMessageFormat,omitempty"`
	MaxWIP               *int     `json:"maxWip,omitempty"`
	HandoffRequires      []string `json:"handoffRequires,omitempty"`
	ReviewRequired       *bool    `json:"reviewRequired,omitempty"`
	AllowsSubagents      *bool    `json:"allowsSubagents,omitempty"`
	DelegationMode       string   `json:"delegationMode,omitempty"`
	LocalChatDefault     string   `json:"localChatDefault,omitempty"`
	ReviewRouting        string   `json:"reviewRouting,omitempty"`
	SynthesisRouting     string   `json:"synthesisRouting,omitempty"`
	AllowsPeerMessaging  *bool    `json:"allowsPeerMessaging,omitempty"`
	AllowsBroadcasts     *bool    `json:"allowsBroadcasts,omitempty"`
	WorkspaceModeDefault string   `json:"workspaceModeDefault,omitempty"`
	LoopStrategy         string   `json:"loopStrategy,omitempty"`
	ConcurrencyBudget    *int     `json:"concurrencyBudget,omitempty"`
	RequiredGates        []string `json:"requiredGates,omitempty"`
}

func DefaultTeamConfig() TeamConfig {
	templates := map[string]TeamTemplate{
		"solo": {
			Name:           "solo",
			Description:    "Single lead operating without spawned teammates by default.",
			Category:       "engineering",
			Orientation:    "individual execution",
			LeadershipMode: "solo",
			SpawnTeammates: boolPtr(false),
			Roles: []TeamRoleTemplate{
				{
					Name:              "lead",
					Responsible:       "Own the full task, plan the work, and only delegate when it clearly helps.",
					CurrentFocus:      "solo execution",
					Profile:           "coder",
					CanSpawnSubagents: boolPtr(true),
				},
			},
			Policies: TeamPolicies{
				CommitMessageFormat:  "type: subject",
				MaxWIP:               intPtr(1),
				HandoffRequires:      []string{"summary", "artifacts"},
				ReviewRequired:       boolPtr(false),
				AllowsSubagents:      boolPtr(true),
				DelegationMode:       "solo-first",
				LocalChatDefault:     "direct",
				ReviewRouting:        "self",
				SynthesisRouting:     "self",
				AllowsPeerMessaging:  boolPtr(false),
				AllowsBroadcasts:     boolPtr(false),
				WorkspaceModeDefault: "shared",
				LoopStrategy:         "ralph",
				ConcurrencyBudget:    intPtr(1),
				RequiredGates:        []string{"all_items_passed", "required_checks_green"},
			},
		},
		"leader-led": {
			Name:           "leader-led",
			Description:    "Default small software team with a coordinating lead and specialist teammates.",
			Category:       "engineering",
			Orientation:    "software delivery",
			LeadershipMode: "leader-led",
			SpawnTeammates: boolPtr(true),
			Roles: []TeamRoleTemplate{
				{
					Name:              "lead",
					Responsible:       "Own the plan, coordinate the team, assign work, maintain the board, and decide when to delegate.",
					CurrentFocus:      "coordination",
					Profile:           "coder",
					CanSpawnSubagents: boolPtr(true),
				},
				{
					Name:              "implementer",
					Responsible:       "Build the main code changes and keep implementation moving.",
					CurrentFocus:      "implementation",
					Profile:           "coder",
					ReportsTo:         "lead",
					CanSpawnSubagents: boolPtr(true),
				},
				{
					Name:              "reviewer",
					Responsible:       "Review regressions, tests, integration quality, and release readiness.",
					CurrentFocus:      "verification",
					Profile:           "coder",
					ReportsTo:         "lead",
					CanSpawnSubagents: boolPtr(false),
				},
				{
					Name:              "researcher",
					Responsible:       "Trace code paths, compare behavior, and gather evidence for the team.",
					CurrentFocus:      "analysis",
					Profile:           "coder",
					ReportsTo:         "lead",
					CanSpawnSubagents: boolPtr(true),
				},
			},
			Policies: TeamPolicies{
				CommitMessageFormat:  "type: subject",
				MaxWIP:               intPtr(3),
				HandoffRequires:      []string{"summary", "artifacts"},
				ReviewRequired:       boolPtr(true),
				AllowsSubagents:      boolPtr(true),
				DelegationMode:       "leader-controlled",
				LocalChatDefault:     "direct",
				ReviewRouting:        "reviewer",
				SynthesisRouting:     "lead",
				AllowsPeerMessaging:  boolPtr(true),
				AllowsBroadcasts:     boolPtr(true),
				WorkspaceModeDefault: "shared",
				LoopStrategy:         "ralph",
				ConcurrencyBudget:    intPtr(3),
				RequiredGates:        []string{"all_items_passed", "required_checks_green", "required_review_complete"},
			},
		},
		"research-heavy": {
			Name:           "research-heavy",
			Description:    "Bias the team toward codebase exploration and evidence gathering before implementation.",
			Category:       "engineering",
			Orientation:    "research and synthesis",
			LeadershipMode: "leader-led",
			SpawnTeammates: boolPtr(true),
			Roles: []TeamRoleTemplate{
				{
					Name:              "lead",
					Responsible:       "Coordinate the team, define investigation tracks, and synthesize findings into an execution plan.",
					CurrentFocus:      "synthesis",
					Profile:           "coder",
					CanSpawnSubagents: boolPtr(true),
				},
				{
					Name:              "researcher",
					Responsible:       "Map the codebase, compare behavior, and produce high-signal evidence.",
					CurrentFocus:      "evidence gathering",
					Profile:           "coder",
					ReportsTo:         "lead",
					CanSpawnSubagents: boolPtr(true),
				},
				{
					Name:              "analyst",
					Responsible:       "Pressure-test findings, identify risks, and translate ambiguity into concrete options.",
					CurrentFocus:      "risk analysis",
					Profile:           "task",
					ReportsTo:         "lead",
					CanSpawnSubagents: boolPtr(false),
				},
				{
					Name:              "implementer",
					Responsible:       "Build only after the lead clears the path.",
					CurrentFocus:      "targeted execution",
					Profile:           "coder",
					ReportsTo:         "lead",
					CanSpawnSubagents: boolPtr(true),
				},
			},
			Policies: TeamPolicies{
				CommitMessageFormat:  "type: subject",
				MaxWIP:               intPtr(2),
				HandoffRequires:      []string{"summary", "artifacts", "evidence"},
				ReviewRequired:       boolPtr(true),
				AllowsSubagents:      boolPtr(true),
				DelegationMode:       "research-first",
				LocalChatDefault:     "direct",
				ReviewRouting:        "lead",
				SynthesisRouting:     "lead",
				AllowsPeerMessaging:  boolPtr(true),
				AllowsBroadcasts:     boolPtr(true),
				WorkspaceModeDefault: "shared",
				LoopStrategy:         "ralph",
				ConcurrencyBudget:    intPtr(2),
				RequiredGates:        []string{"all_items_passed", "required_synthesis_complete"},
			},
		},
		"review-heavy": {
			Name:           "review-heavy",
			Description:    "Bias the team toward verification, critique, and controlled release quality.",
			Category:       "engineering",
			Orientation:    "verification and release",
			LeadershipMode: "leader-led",
			SpawnTeammates: boolPtr(true),
			Roles: []TeamRoleTemplate{
				{
					Name:              "lead",
					Responsible:       "Own release quality, adjudicate debates, and control merge readiness.",
					CurrentFocus:      "release control",
					Profile:           "coder",
					CanSpawnSubagents: boolPtr(true),
				},
				{
					Name:              "implementer",
					Responsible:       "Build the requested changes and respond to review feedback quickly.",
					CurrentFocus:      "implementation",
					Profile:           "coder",
					ReportsTo:         "lead",
					CanSpawnSubagents: boolPtr(true),
				},
				{
					Name:              "reviewer",
					Responsible:       "Own code review, regression pressure-testing, and release gating.",
					CurrentFocus:      "verification",
					Profile:           "coder",
					ReportsTo:         "lead",
					CanSpawnSubagents: boolPtr(true),
				},
				{
					Name:              "tester",
					Responsible:       "Drive checks, acceptance scenarios, and validation coverage.",
					CurrentFocus:      "tests",
					Profile:           "task",
					ReportsTo:         "reviewer",
					CanSpawnSubagents: boolPtr(false),
				},
			},
			Policies: TeamPolicies{
				CommitMessageFormat:  "type: subject",
				MaxWIP:               intPtr(2),
				HandoffRequires:      []string{"summary", "artifacts", "validation"},
				ReviewRequired:       boolPtr(true),
				AllowsSubagents:      boolPtr(true),
				DelegationMode:       "quality-gated",
				LocalChatDefault:     "direct",
				ReviewRouting:        "reviewer",
				SynthesisRouting:     "lead",
				AllowsPeerMessaging:  boolPtr(true),
				AllowsBroadcasts:     boolPtr(true),
				WorkspaceModeDefault: "shared",
				LoopStrategy:         "ralph",
				ConcurrencyBudget:    intPtr(2),
				RequiredGates:        []string{"all_items_passed", "required_checks_green", "required_review_complete"},
			},
		},
		"freeform": {
			Name:           "freeform",
			Description:    "Minimal starter template intended to be customized by the user.",
			Category:       "general",
			Orientation:    "custom organization",
			LeadershipMode: "custom",
			SpawnTeammates: boolPtr(false),
			Roles: []TeamRoleTemplate{
				{
					Name:              "lead",
					Responsible:       "Customize this template to fit your preferred operating model.",
					CurrentFocus:      "configuration",
					Profile:           "coder",
					CanSpawnSubagents: boolPtr(true),
				},
			},
			Policies: TeamPolicies{
				CommitMessageFormat:  "type: subject",
				MaxWIP:               intPtr(3),
				HandoffRequires:      []string{"summary", "artifacts"},
				ReviewRequired:       boolPtr(true),
				AllowsSubagents:      boolPtr(true),
				DelegationMode:       "custom",
				LocalChatDefault:     "direct",
				ReviewRouting:        "lead",
				SynthesisRouting:     "lead",
				AllowsPeerMessaging:  boolPtr(true),
				AllowsBroadcasts:     boolPtr(true),
				WorkspaceModeDefault: "shared",
				LoopStrategy:         "ralph",
				ConcurrencyBudget:    intPtr(3),
				RequiredGates:        []string{"all_items_passed"},
			},
		},
		"software-team": {
			Name:           "software-team",
			Description:    "Code-first delivery blueprint with planning, implementation, review, and research roles.",
			Category:       "engineering",
			Orientation:    "software delivery",
			LeadershipMode: "leader-led",
			SpawnTeammates: boolPtr(true),
			Roles: []TeamRoleTemplate{
				{Name: "lead", Responsible: "Own planning, decomposition, gate setting, and final synthesis.", CurrentFocus: "coordination", Profile: "coder", CanSpawnSubagents: boolPtr(true)},
				{Name: "implementer", Responsible: "Build the main code changes.", CurrentFocus: "implementation", Profile: "coder", ReportsTo: "lead", CanSpawnSubagents: boolPtr(true)},
				{Name: "reviewer", Responsible: "Own regression and release review.", CurrentFocus: "verification", Profile: "coder", ReportsTo: "lead", CanSpawnSubagents: boolPtr(false)},
				{Name: "researcher", Responsible: "Gather evidence, compare references, and unblock the team.", CurrentFocus: "analysis", Profile: "coder", ReportsTo: "lead", CanSpawnSubagents: boolPtr(true)},
				{Name: "tester", Responsible: "Drive test execution and acceptance verification.", CurrentFocus: "validation", Profile: "task", ReportsTo: "reviewer", CanSpawnSubagents: boolPtr(false)},
			},
			Policies: TeamPolicies{
				CommitMessageFormat:  "type: subject",
				MaxWIP:               intPtr(3),
				HandoffRequires:      []string{"summary", "artifacts", "validation"},
				ReviewRequired:       boolPtr(true),
				AllowsSubagents:      boolPtr(true),
				DelegationMode:       "leader-controlled",
				LocalChatDefault:     "direct",
				ReviewRouting:        "reviewer",
				SynthesisRouting:     "lead",
				AllowsPeerMessaging:  boolPtr(true),
				AllowsBroadcasts:     boolPtr(true),
				WorkspaceModeDefault: "shared",
				LoopStrategy:         "ralph",
				ConcurrencyBudget:    intPtr(3),
				RequiredGates:        []string{"all_items_passed", "required_checks_green", "required_review_complete"},
			},
		},
		"research-lab": {
			Name:           "research-lab",
			Description:    "Investigation-oriented blueprint for exploration, analysis, and synthesis-heavy work.",
			Category:       "analysis",
			Orientation:    "research and synthesis",
			LeadershipMode: "leader-led",
			SpawnTeammates: boolPtr(true),
			Roles: []TeamRoleTemplate{
				{Name: "lead-investigator", Responsible: "Define questions, assign tracks, and synthesize findings.", CurrentFocus: "synthesis", Profile: "coder", CanSpawnSubagents: boolPtr(true)},
				{Name: "researcher", Responsible: "Investigate and gather evidence.", CurrentFocus: "evidence", Profile: "coder", ReportsTo: "lead-investigator", CanSpawnSubagents: boolPtr(true)},
				{Name: "analyst", Responsible: "Pressure-test evidence and articulate conclusions.", CurrentFocus: "analysis", Profile: "task", ReportsTo: "lead-investigator", CanSpawnSubagents: boolPtr(false)},
				{Name: "implementer", Responsible: "Turn the cleared path into changes only after findings stabilize.", CurrentFocus: "execution", Profile: "coder", ReportsTo: "lead-investigator", CanSpawnSubagents: boolPtr(true)},
			},
			Policies: TeamPolicies{
				MaxWIP:               intPtr(2),
				HandoffRequires:      []string{"summary", "artifacts", "evidence"},
				ReviewRequired:       boolPtr(true),
				AllowsSubagents:      boolPtr(true),
				DelegationMode:       "research-first",
				LocalChatDefault:     "direct",
				ReviewRouting:        "lead-investigator",
				SynthesisRouting:     "lead-investigator",
				AllowsPeerMessaging:  boolPtr(true),
				AllowsBroadcasts:     boolPtr(true),
				WorkspaceModeDefault: "shared",
				LoopStrategy:         "ralph",
				ConcurrencyBudget:    intPtr(2),
				RequiredGates:        []string{"all_items_passed", "required_synthesis_complete"},
			},
		},
		"marketing-agency": {
			Name:           "marketing-agency",
			Description:    "Campaign-oriented blueprint for strategy, copy, operations, and analytics.",
			Category:       "business",
			Orientation:    "campaign execution",
			LeadershipMode: "leader-led",
			SpawnTeammates: boolPtr(true),
			Roles: []TeamRoleTemplate{
				{Name: "account-lead", Responsible: "Own client objective, planning, and approvals.", CurrentFocus: "coordination", Profile: "coder", CanSpawnSubagents: boolPtr(true)},
				{Name: "strategist", Responsible: "Define channel strategy and positioning.", CurrentFocus: "strategy", Profile: "coder", ReportsTo: "account-lead", CanSpawnSubagents: boolPtr(true)},
				{Name: "copywriter", Responsible: "Produce campaign language and content variants.", CurrentFocus: "content", Profile: "coder", ReportsTo: "account-lead", CanSpawnSubagents: boolPtr(true)},
				{Name: "campaign-operator", Responsible: "Structure execution tasks and rollout operations.", CurrentFocus: "operations", Profile: "task", ReportsTo: "account-lead", CanSpawnSubagents: boolPtr(false)},
				{Name: "analyst", Responsible: "Track results and summarize learnings.", CurrentFocus: "analysis", Profile: "task", ReportsTo: "account-lead", CanSpawnSubagents: boolPtr(false)},
			},
			Policies: TeamPolicies{
				MaxWIP:               intPtr(3),
				HandoffRequires:      []string{"summary", "artifacts", "brief"},
				ReviewRequired:       boolPtr(true),
				AllowsSubagents:      boolPtr(true),
				DelegationMode:       "campaign-led",
				LocalChatDefault:     "direct",
				ReviewRouting:        "account-lead",
				SynthesisRouting:     "account-lead",
				AllowsPeerMessaging:  boolPtr(true),
				AllowsBroadcasts:     boolPtr(true),
				WorkspaceModeDefault: "shared",
				LoopStrategy:         "ralph",
				ConcurrencyBudget:    intPtr(3),
				RequiredGates:        []string{"all_items_passed", "required_artifacts_produced", "required_approval_recorded"},
			},
		},
		"brand-studio": {
			Name:           "brand-studio",
			Description:    "Creative blueprint for brand strategy, design direction, critique, and signoff.",
			Category:       "creative",
			Orientation:    "brand development",
			LeadershipMode: "leader-led",
			SpawnTeammates: boolPtr(true),
			Roles: []TeamRoleTemplate{
				{Name: "creative-director", Responsible: "Own concept quality and final synthesis.", CurrentFocus: "direction", Profile: "coder", CanSpawnSubagents: boolPtr(true)},
				{Name: "brand-strategist", Responsible: "Define positioning and narrative structure.", CurrentFocus: "strategy", Profile: "coder", ReportsTo: "creative-director", CanSpawnSubagents: boolPtr(true)},
				{Name: "visual-designer", Responsible: "Produce visual system concepts and assets.", CurrentFocus: "design", Profile: "coder", ReportsTo: "creative-director", CanSpawnSubagents: boolPtr(true)},
				{Name: "researcher", Responsible: "Gather references and audience/context insight.", CurrentFocus: "reference gathering", Profile: "task", ReportsTo: "creative-director", CanSpawnSubagents: boolPtr(false)},
				{Name: "reviewer", Responsible: "Critique for consistency, clarity, and polish.", CurrentFocus: "critique", Profile: "task", ReportsTo: "creative-director", CanSpawnSubagents: boolPtr(false)},
			},
			Policies: TeamPolicies{
				MaxWIP:               intPtr(2),
				HandoffRequires:      []string{"summary", "artifacts", "rationale"},
				ReviewRequired:       boolPtr(true),
				AllowsSubagents:      boolPtr(true),
				DelegationMode:       "creative-led",
				LocalChatDefault:     "direct",
				ReviewRouting:        "reviewer",
				SynthesisRouting:     "creative-director",
				AllowsPeerMessaging:  boolPtr(true),
				AllowsBroadcasts:     boolPtr(true),
				WorkspaceModeDefault: "shared",
				LoopStrategy:         "ralph",
				ConcurrencyBudget:    intPtr(2),
				RequiredGates:        []string{"all_items_passed", "required_review_complete", "required_artifacts_produced"},
			},
		},
		"media-production": {
			Name:           "media-production",
			Description:    "Production blueprint for scripting, editing, sequencing, and release management.",
			Category:       "media",
			Orientation:    "production pipeline",
			LeadershipMode: "leader-led",
			SpawnTeammates: boolPtr(true),
			Roles: []TeamRoleTemplate{
				{Name: "producer", Responsible: "Own schedule, sequencing, and final release readiness.", CurrentFocus: "production control", Profile: "coder", CanSpawnSubagents: boolPtr(true)},
				{Name: "writer", Responsible: "Create drafts, scripts, and narrative structure.", CurrentFocus: "drafting", Profile: "coder", ReportsTo: "producer", CanSpawnSubagents: boolPtr(true)},
				{Name: "editor", Responsible: "Refine, sequence, and polish content.", CurrentFocus: "editing", Profile: "coder", ReportsTo: "producer", CanSpawnSubagents: boolPtr(true)},
				{Name: "researcher", Responsible: "Provide source material and factual grounding.", CurrentFocus: "research", Profile: "task", ReportsTo: "producer", CanSpawnSubagents: boolPtr(false)},
				{Name: "release-manager", Responsible: "Package outputs and release notes.", CurrentFocus: "release", Profile: "task", ReportsTo: "producer", CanSpawnSubagents: boolPtr(false)},
			},
			Policies: TeamPolicies{
				MaxWIP:               intPtr(3),
				HandoffRequires:      []string{"summary", "artifacts", "release_notes"},
				ReviewRequired:       boolPtr(true),
				AllowsSubagents:      boolPtr(true),
				DelegationMode:       "pipeline-led",
				LocalChatDefault:     "direct",
				ReviewRouting:        "producer",
				SynthesisRouting:     "producer",
				AllowsPeerMessaging:  boolPtr(true),
				AllowsBroadcasts:     boolPtr(true),
				WorkspaceModeDefault: "shared",
				LoopStrategy:         "ralph",
				ConcurrencyBudget:    intPtr(3),
				RequiredGates:        []string{"all_items_passed", "required_artifacts_produced", "required_review_complete"},
			},
		},
		"corporate-ops": {
			Name:           "corporate-ops",
			Description:    "Operations blueprint for planning, analysis, execution coordination, and communications.",
			Category:       "operations",
			Orientation:    "organizational execution",
			LeadershipMode: "leader-led",
			SpawnTeammates: boolPtr(true),
			Roles: []TeamRoleTemplate{
				{Name: "chief-of-staff", Responsible: "Own execution structure, sequencing, and synthesis.", CurrentFocus: "coordination", Profile: "coder", CanSpawnSubagents: boolPtr(true)},
				{Name: "analyst", Responsible: "Model tradeoffs and summarize evidence.", CurrentFocus: "analysis", Profile: "task", ReportsTo: "chief-of-staff", CanSpawnSubagents: boolPtr(false)},
				{Name: "operator", Responsible: "Carry work across process lanes and task systems.", CurrentFocus: "operations", Profile: "task", ReportsTo: "chief-of-staff", CanSpawnSubagents: boolPtr(false)},
				{Name: "reviewer", Responsible: "Validate readiness and policy adherence.", CurrentFocus: "review", Profile: "task", ReportsTo: "chief-of-staff", CanSpawnSubagents: boolPtr(false)},
				{Name: "communications", Responsible: "Produce summaries, briefings, and stakeholder updates.", CurrentFocus: "communications", Profile: "coder", ReportsTo: "chief-of-staff", CanSpawnSubagents: boolPtr(true)},
			},
			Policies: TeamPolicies{
				MaxWIP:               intPtr(3),
				HandoffRequires:      []string{"summary", "artifacts", "decision_log"},
				ReviewRequired:       boolPtr(true),
				AllowsSubagents:      boolPtr(true),
				DelegationMode:       "operations-led",
				LocalChatDefault:     "direct",
				ReviewRouting:        "reviewer",
				SynthesisRouting:     "chief-of-staff",
				AllowsPeerMessaging:  boolPtr(true),
				AllowsBroadcasts:     boolPtr(true),
				WorkspaceModeDefault: "shared",
				LoopStrategy:         "ralph",
				ConcurrencyBudget:    intPtr(3),
				RequiredGates:        []string{"all_items_passed", "required_review_complete", "required_approval_recorded"},
			},
		},
	}

	templates["software-team-alias"] = templates["software-team"]

	return TeamConfig{
		DefaultTemplate:  "leader-led",
		DefaultBlueprint: "software-team",
		CollaborationHUD: boolPtr(true),
		Templates:        templates,
		Blueprints:       cloneTemplates(templates),
	}
}

func normalizeTeamConfig(cfg TeamConfig) TeamConfig {
	defaults := DefaultTeamConfig()

	if cfg.ActiveTeam != "" {
		defaults.ActiveTeam = cfg.ActiveTeam
	}
	if cfg.DefaultTemplate != "" {
		defaults.DefaultTemplate = cfg.DefaultTemplate
	}
	if cfg.DefaultBlueprint != "" {
		defaults.DefaultBlueprint = cfg.DefaultBlueprint
	}
	if cfg.CollaborationHUD != nil {
		defaults.CollaborationHUD = cfg.CollaborationHUD
	}
	if defaults.Templates == nil {
		defaults.Templates = map[string]TeamTemplate{}
	}
	if defaults.Blueprints == nil {
		defaults.Blueprints = cloneTemplates(defaults.Templates)
	}

	for name, tmpl := range cfg.Templates {
		base, ok := defaults.Templates[name]
		if ok {
			defaults.Templates[name] = mergeTeamTemplate(base, tmpl)
			continue
		}
		defaults.Templates[name] = normalizeTeamTemplate(name, tmpl)
	}
	for name, tmpl := range cfg.Blueprints {
		base, ok := defaults.Templates[name]
		if ok {
			defaults.Templates[name] = mergeTeamTemplate(base, tmpl)
			continue
		}
		defaults.Templates[name] = normalizeTeamTemplate(name, tmpl)
	}
	if defaults.DefaultBlueprint == "" {
		defaults.DefaultBlueprint = defaults.DefaultTemplate
	}
	if defaults.DefaultTemplate == "" {
		defaults.DefaultTemplate = defaults.DefaultBlueprint
	}
	defaults.Blueprints = cloneTemplates(defaults.Templates)

	return defaults
}

func TeamHUDEnabled(cfg *Config) bool {
	if cfg == nil || cfg.Team.CollaborationHUD == nil {
		return true
	}
	return *cfg.Team.CollaborationHUD
}

func mergeTeamTemplate(base, override TeamTemplate) TeamTemplate {
	if override.Name != "" {
		base.Name = override.Name
	}
	if override.Description != "" {
		base.Description = override.Description
	}
	if override.Category != "" {
		base.Category = override.Category
	}
	if override.Orientation != "" {
		base.Orientation = override.Orientation
	}
	if override.LeadershipMode != "" {
		base.LeadershipMode = override.LeadershipMode
	}
	if override.SpawnTeammates != nil {
		base.SpawnTeammates = override.SpawnTeammates
	}
	if len(override.Roles) > 0 {
		base.Roles = override.Roles
	}
	base.Policies = mergeTeamPolicies(base.Policies, override.Policies)
	return normalizeTeamTemplate(base.Name, base)
}

func mergeTeamPolicies(base, override TeamPolicies) TeamPolicies {
	if override.CommitMessageFormat != "" {
		base.CommitMessageFormat = override.CommitMessageFormat
	}
	if override.MaxWIP != nil {
		base.MaxWIP = override.MaxWIP
	}
	if len(override.HandoffRequires) > 0 {
		base.HandoffRequires = override.HandoffRequires
	}
	if override.ReviewRequired != nil {
		base.ReviewRequired = override.ReviewRequired
	}
	if override.AllowsSubagents != nil {
		base.AllowsSubagents = override.AllowsSubagents
	}
	if override.DelegationMode != "" {
		base.DelegationMode = override.DelegationMode
	}
	if override.LocalChatDefault != "" {
		base.LocalChatDefault = override.LocalChatDefault
	}
	if override.ReviewRouting != "" {
		base.ReviewRouting = override.ReviewRouting
	}
	if override.SynthesisRouting != "" {
		base.SynthesisRouting = override.SynthesisRouting
	}
	if override.AllowsPeerMessaging != nil {
		base.AllowsPeerMessaging = override.AllowsPeerMessaging
	}
	if override.AllowsBroadcasts != nil {
		base.AllowsBroadcasts = override.AllowsBroadcasts
	}
	if override.WorkspaceModeDefault != "" {
		base.WorkspaceModeDefault = override.WorkspaceModeDefault
	}
	if override.LoopStrategy != "" {
		base.LoopStrategy = override.LoopStrategy
	}
	if override.ConcurrencyBudget != nil {
		base.ConcurrencyBudget = override.ConcurrencyBudget
	}
	if len(override.RequiredGates) > 0 {
		base.RequiredGates = override.RequiredGates
	}
	return base
}

func normalizeTeamTemplate(name string, tmpl TeamTemplate) TeamTemplate {
	if tmpl.Name == "" {
		tmpl.Name = name
	}
	if tmpl.Category == "" {
		tmpl.Category = "engineering"
	}
	if tmpl.Orientation == "" {
		tmpl.Orientation = "software delivery"
	}
	if tmpl.LeadershipMode == "" {
		tmpl.LeadershipMode = "leader-led"
	}
	if tmpl.SpawnTeammates == nil {
		tmpl.SpawnTeammates = boolPtr(true)
	}
	if tmpl.Policies.CommitMessageFormat == "" {
		tmpl.Policies.CommitMessageFormat = "type: subject"
	}
	if tmpl.Policies.MaxWIP == nil {
		tmpl.Policies.MaxWIP = intPtr(3)
	}
	if len(tmpl.Policies.HandoffRequires) == 0 {
		tmpl.Policies.HandoffRequires = []string{"summary", "artifacts"}
	}
	if tmpl.Policies.ReviewRequired == nil {
		tmpl.Policies.ReviewRequired = boolPtr(true)
	}
	if tmpl.Policies.AllowsSubagents == nil {
		tmpl.Policies.AllowsSubagents = boolPtr(true)
	}
	if tmpl.Policies.DelegationMode == "" {
		tmpl.Policies.DelegationMode = "leader-controlled"
	}
	if tmpl.Policies.LocalChatDefault == "" {
		tmpl.Policies.LocalChatDefault = "direct"
	}
	if tmpl.Policies.ReviewRouting == "" {
		tmpl.Policies.ReviewRouting = "lead"
	}
	if tmpl.Policies.SynthesisRouting == "" {
		tmpl.Policies.SynthesisRouting = "lead"
	}
	if tmpl.Policies.AllowsPeerMessaging == nil {
		tmpl.Policies.AllowsPeerMessaging = boolPtr(true)
	}
	if tmpl.Policies.AllowsBroadcasts == nil {
		tmpl.Policies.AllowsBroadcasts = boolPtr(true)
	}
	if tmpl.Policies.WorkspaceModeDefault == "" {
		tmpl.Policies.WorkspaceModeDefault = "shared"
	}
	if tmpl.Policies.LoopStrategy == "" {
		tmpl.Policies.LoopStrategy = "ralph"
	}
	if tmpl.Policies.ConcurrencyBudget == nil {
		tmpl.Policies.ConcurrencyBudget = intPtr(2)
	}
	if len(tmpl.Policies.RequiredGates) == 0 {
		tmpl.Policies.RequiredGates = []string{"all_items_passed"}
	}
	for i := range tmpl.Roles {
		if tmpl.Roles[i].Profile == "" {
			tmpl.Roles[i].Profile = "coder"
		}
		if tmpl.Roles[i].CanSpawnSubagents == nil {
			tmpl.Roles[i].CanSpawnSubagents = boolPtr(true)
		}
	}
	return tmpl
}

func boolPtr(value bool) *bool {
	return &value
}

func intPtr(value int) *int {
	return &value
}

func cloneTemplates(source map[string]TeamTemplate) map[string]TeamTemplate {
	cloned := make(map[string]TeamTemplate, len(source))
	for name, tmpl := range source {
		cloned[name] = tmpl
	}
	return cloned
}
