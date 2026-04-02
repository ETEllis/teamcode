package agency

import (
	"fmt"
	"sort"
	"strings"

	"github.com/ETEllis/teamcode/internal/config"
)

type GenesisManufacturingInput struct {
	Intent           string
	Domain           string
	TimeHorizon      string
	WorkingStyle     string
	Governance       string
	GoalShape        string
	ConstitutionName string
	Constitution     config.AgencyConstitution
	Template         config.TeamTemplate
	RequestedRoles   []string
	DefaultCadence   string
	Timezone         string
	SharedWorkplace  string
	RedisAddress     string
	LedgerPath       string
}

type GenesisRoleBundle struct {
	RoleName            string            `json:"roleName"`
	DisplayName         string            `json:"displayName"`
	Profile             string            `json:"profile,omitempty"`
	ReportsTo           string            `json:"reportsTo,omitempty"`
	Archetype           string            `json:"archetype,omitempty"`
	Mission             string            `json:"mission"`
	WorkingPosture      string            `json:"workingPosture,omitempty"`
	Personality         string            `json:"personality,omitempty"`
	OfficePresence      string            `json:"officePresence,omitempty"`
	AvatarPrompt        string            `json:"avatarPrompt,omitempty"`
	SystemPrompt        string            `json:"systemPrompt,omitempty"`
	Skills              []string          `json:"skills,omitempty"`
	Tools               []string          `json:"tools,omitempty"`
	AllowedActions      []ActionType      `json:"allowedActions,omitempty"`
	ObservationScopes   []string          `json:"observationScopes,omitempty"`
	PeerRouting         map[string]string `json:"peerRouting,omitempty"`
	CapabilityPack      CapabilityPack    `json:"capabilityPack"`
	RecommendedSchedule AgentSchedule     `json:"recommendedSchedule"`
	SpawnOrder          int               `json:"spawnOrder"`
	CanSpawnAgents      bool              `json:"canSpawnAgents"`
}

type GenesisPlan struct {
	Intent               OrgIntent           `json:"intent"`
	Summary              string              `json:"summary"`
	ConstitutionName     string              `json:"constitutionName"`
	Blueprint            string              `json:"blueprint,omitempty"`
	TeamTemplate         string              `json:"teamTemplate,omitempty"`
	Topology             string              `json:"topology,omitempty"`
	RoleBundles          []GenesisRoleBundle `json:"roleBundles,omitempty"`
	SocialThread         []string            `json:"socialThread,omitempty"`
	ManufacturingSignals []string            `json:"manufacturingSignals,omitempty"`
}

func ManufactureGenesis(input GenesisManufacturingInput) GenesisPlan {
	constitutionName := strings.TrimSpace(input.ConstitutionName)
	if constitutionName == "" {
		constitutionName = genesisFirstNonEmpty(input.Constitution.Name, input.Template.Name, "agency")
	}

	topology := genesisFirstNonEmpty(
		input.Governance,
		input.Constitution.Governance,
		input.Template.LeadershipMode,
		"hybrid",
	)

	intent := OrgIntent{
		ID:           normalizeHandle(constitutionName + "-" + genesisFirstNonEmpty(input.Domain, "office")),
		Domain:       genesisFirstNonEmpty(input.Domain, inferDomain(input.Intent, input.Template)),
		TimeHorizon:  genesisFirstNonEmpty(input.TimeHorizon, input.Constitution.DefaultSchedule, "ongoing"),
		WorkingStyle: genesisFirstNonEmpty(input.WorkingStyle, input.Template.Orientation, "collaborative"),
		GoalShape:    genesisFirstNonEmpty(input.GoalShape, "deliver a working outcome"),
		Governance:   GovernanceMode(genesisFirstNonEmpty(topology, "hybrid")),
		Summary:      summarizeIntent(input),
		Requirements: manufacturingRequirements(input),
		Metadata: map[string]string{
			"constitution": constitutionName,
			"blueprint":    genesisFirstNonEmpty(input.Constitution.Blueprint, input.Template.Name),
			"template":     genesisFirstNonEmpty(input.Constitution.TeamTemplate, input.Template.Name),
		},
	}

	roles := materializeRoleTemplates(input)
	bundles := make([]GenesisRoleBundle, 0, len(roles))
	for idx, role := range roles {
		bundles = append(bundles, manufactureRoleBundle(intent, input, role, idx))
	}
	sort.SliceStable(bundles, func(i, j int) bool {
		if bundles[i].SpawnOrder != bundles[j].SpawnOrder {
			return bundles[i].SpawnOrder < bundles[j].SpawnOrder
		}
		return bundles[i].RoleName < bundles[j].RoleName
	})

	thread := make([]string, 0, len(bundles))
	for _, bundle := range bundles {
		thread = append(thread, bundle.OfficePresence)
		if len(thread) >= 5 {
			break
		}
	}

	return GenesisPlan{
		Intent:           intent,
		Summary:          buildManufacturedSummary(intent, input, len(bundles)),
		ConstitutionName: constitutionName,
		Blueprint:        genesisFirstNonEmpty(input.Constitution.Blueprint, input.Template.Name),
		TeamTemplate:     genesisFirstNonEmpty(input.Constitution.TeamTemplate, input.Template.Name),
		Topology:         topology,
		RoleBundles:      bundles,
		SocialThread:     thread,
		ManufacturingSignals: []string{
			fmt.Sprintf("wake policy: %s", genesisFirstNonEmpty(input.Constitution.Policies.WakeMode, "event-reactive")),
			fmt.Sprintf("consensus: %s", genesisFirstNonEmpty(input.Constitution.Policies.ConsensusMode, "distributed-consensus")),
			fmt.Sprintf("schedule: %s", genesisFirstNonEmpty(input.DefaultCadence, "office-hours")),
		},
	}
}

func materializeRoleTemplates(input GenesisManufacturingInput) []config.TeamRoleTemplate {
	templateRoles := append([]config.TeamRoleTemplate(nil), input.Template.Roles...)
	if len(input.RequestedRoles) == 0 {
		return templateRoles
	}

	templateByName := make(map[string]config.TeamRoleTemplate, len(templateRoles))
	for _, role := range templateRoles {
		templateByName[normalizeHandle(role.Name)] = role
	}

	roles := make([]config.TeamRoleTemplate, 0, len(input.RequestedRoles))
	for _, requested := range input.RequestedRoles {
		key := normalizeHandle(requested)
		if role, ok := templateByName[key]; ok {
			roles = append(roles, role)
			continue
		}
		roles = append(roles, config.TeamRoleTemplate{
			Name:              requested,
			Responsible:       fmt.Sprintf("Own the %s lane for %s.", requested, strings.TrimSpace(input.Intent)),
			CurrentFocus:      "genesis manufacturing",
			Profile:           "coder",
			CanSpawnSubagents: boolPointer(true),
		})
	}
	return roles
}

func manufactureRoleBundle(intent OrgIntent, input GenesisManufacturingInput, role config.TeamRoleTemplate, index int) GenesisRoleBundle {
	roleName := genesisFirstNonEmpty(role.Name, fmt.Sprintf("role-%d", index+1))
	archetype := roleArchetype(roleName)
	displayName := displayRoleName(roleName)
	reportsTo := strings.TrimSpace(role.ReportsTo)
	if reportsTo == "" && GovernanceMode(intent.Governance) != GovernanceFlat && GovernanceMode(intent.Governance) != GovernancePeer && archetype != "lead" {
		reportsTo = "lead"
	}

	allowedActions := actionSetForArchetype(archetype, boolValue(role.CanSpawnSubagents))
	skills := skillSetForArchetype(archetype, input)
	tools := toolSetForArchetype(archetype)
	scopes := observationScopesForArchetype(archetype)
	peerRouting := peerRoutingForRole(archetype, reportsTo)
	mission := genesisFirstNonEmpty(role.Responsible, fmt.Sprintf("Advance the %s function for %s.", displayName, intent.Summary))
	posture := workingPostureForArchetype(archetype, intent)
	personality := personalityForArchetype(archetype)
	avatarPrompt := avatarPromptForBundle(displayName, archetype)
	officePresence := fmt.Sprintf("%s checks in as %s, %s", displayName, roleLabel(roleName, role.Profile), compactMission(mission))
	systemPrompt := buildSystemPrompt(intent, input, role, archetype, mission, posture, reportsTo, allowedActions, scopes, peerRouting)

	return GenesisRoleBundle{
		RoleName:          roleName,
		DisplayName:       displayName,
		Profile:           genesisFirstNonEmpty(role.Profile, "coder"),
		ReportsTo:         reportsTo,
		Archetype:         archetype,
		Mission:           mission,
		WorkingPosture:    posture,
		Personality:       personality,
		OfficePresence:    officePresence,
		AvatarPrompt:      avatarPrompt,
		SystemPrompt:      systemPrompt,
		Skills:            skills,
		Tools:             tools,
		AllowedActions:    allowedActions,
		ObservationScopes: scopes,
		PeerRouting:       peerRouting,
		CapabilityPack: CapabilityPack{
			Skills:            append([]string(nil), skills...),
			Tools:             append([]string(nil), tools...),
			ActionConstraints: append([]ActionType(nil), allowedActions...),
			ContextScopes:     append([]string(nil), scopes...),
			Metadata: map[string]string{
				"profile":   genesisFirstNonEmpty(role.Profile, "coder"),
				"reportsTo": reportsTo,
				"archetype": archetype,
			},
		},
		RecommendedSchedule: AgentSchedule{
			ID:                normalizeHandle(roleName) + "-schedule",
			ActorID:           normalizeHandle(roleName),
			Expression:        genesisFirstNonEmpty(input.DefaultCadence, "office-hours"),
			Timezone:          genesisFirstNonEmpty(input.Timezone, "local"),
			Enabled:           true,
			DefaultSignalKind: SignalSchedule,
			Metadata: map[string]string{
				"role":      roleName,
				"archetype": archetype,
			},
		},
		SpawnOrder:     spawnOrderForArchetype(archetype, index),
		CanSpawnAgents: boolValue(role.CanSpawnSubagents),
	}
}

func actionSetForArchetype(archetype string, canSpawn bool) []ActionType {
	actions := []ActionType{ActionBroadcast, ActionUpdateTask}
	switch archetype {
	case "lead":
		actions = append(actions, ActionPingPeer, ActionRequestReview, ActionPublishArtifact, ActionHandoffShift)
	case "research":
		actions = append(actions, ActionPingPeer, ActionRequestReview)
	case "review":
		actions = append(actions, ActionRunTest, ActionRequestReview, ActionPublishArtifact)
	case "ops":
		actions = append(actions, ActionPingPeer, ActionHandoffShift, ActionPublishArtifact)
	default:
		actions = append(actions, ActionWriteCode, ActionRunTest, ActionPingPeer)
	}
	if canSpawn {
		actions = append(actions, ActionSpawnAgent)
	}
	return dedupeActions(actions)
}

func skillSetForArchetype(archetype string, input GenesisManufacturingInput) []string {
	base := []string{"office-rhythm", "ledger-discipline"}
	switch archetype {
	case "lead":
		base = append(base, "planning", "synthesis", "review-routing")
	case "research":
		base = append(base, "codebase-mapping", "evidence-gathering", "comparison")
	case "review":
		base = append(base, "verification", "regression-hunting", "release-gating")
	case "ops":
		base = append(base, "handoffs", "status-routing", "continuity")
	default:
		base = append(base, "implementation", "tool-use", "delivery")
	}
	if strings.TrimSpace(input.Domain) != "" {
		base = append(base, normalizeHandle(input.Domain)+"-domain")
	}
	return dedupeStrings(base)
}

func toolSetForArchetype(archetype string) []string {
	tools := []string{"ledger", "mailbox", "workspace"}
	switch archetype {
	case "lead":
		tools = append(tools, "task-board", "review-queue", "broadcast")
	case "research":
		tools = append(tools, "search", "diff", "notes")
	case "review":
		tools = append(tools, "tests", "diff", "artifacts")
	default:
		tools = append(tools, "shell", "files", "tests")
	}
	return dedupeStrings(tools)
}

func observationScopesForArchetype(archetype string) []string {
	scopes := []string{"office_snapshot", "ledger_head", "peer_signals"}
	switch archetype {
	case "lead":
		scopes = append(scopes, "task_board", "handoff_queue", "review_state")
	case "research":
		scopes = append(scopes, "codebase", "open_questions")
	case "review":
		scopes = append(scopes, "changes", "checks", "release_gates")
	default:
		scopes = append(scopes, "assigned_scope", "shared_workplace")
	}
	return dedupeStrings(scopes)
}

func peerRoutingForRole(archetype, reportsTo string) map[string]string {
	routing := map[string]string{}
	if reportsTo != "" {
		routing["escalate"] = reportsTo
	}
	switch archetype {
	case "lead":
		routing["review"] = "reviewer"
		routing["research"] = "researcher"
	case "review":
		routing["implementation"] = "implementer"
	}
	return routing
}

func workingPostureForArchetype(archetype string, intent OrgIntent) string {
	switch archetype {
	case "lead":
		return fmt.Sprintf("Continuously synthesize the office state, keep the %s mission coherent, and only fan work out when the next step is clear.", intent.Domain)
	case "research":
		return "Work evidence-first, reduce ambiguity before asking the office to commit, and surface crisp findings instead of vague possibilities."
	case "review":
		return "Stay skeptical, verify before approving, and convert fuzzy readiness into explicit gates."
	case "ops":
		return "Keep the office moving across time, make handoffs legible, and protect continuity."
	default:
		return "Prefer direct execution, keep artifacts tidy, and update the office the moment reality changes."
	}
}

func personalityForArchetype(archetype string) string {
	switch archetype {
	case "lead":
		return "calm, decisive, and coordinating"
	case "research":
		return "curious, methodical, and evidence-driven"
	case "review":
		return "skeptical, precise, and release-minded"
	case "ops":
		return "steady, logistical, and continuity-focused"
	default:
		return "practical, fast-moving, and delivery-oriented"
	}
}

func avatarPromptForBundle(displayName, archetype string) string {
	switch archetype {
	case "lead":
		return fmt.Sprintf("%s as a composed staff lead with a clipboard, midnight-blue palette, and confident posture", displayName)
	case "research":
		return fmt.Sprintf("%s as a focused analyst with notes, warm neutrals, and observant expression", displayName)
	case "review":
		return fmt.Sprintf("%s as a quality sentinel with sharp glasses, steel tones, and careful expression", displayName)
	case "ops":
		return fmt.Sprintf("%s as an organized coordinator with schedule cards, olive accents, and steady presence", displayName)
	default:
		return fmt.Sprintf("%s as a hands-on builder with tool roll, clean lines, and energetic expression", displayName)
	}
}

func buildSystemPrompt(intent OrgIntent, input GenesisManufacturingInput, role config.TeamRoleTemplate, archetype, mission, posture, reportsTo string, actions []ActionType, scopes []string, routing map[string]string) string {
	lines := []string{
		fmt.Sprintf("You are %s in The Agency.", displayRoleName(role.Name)),
		fmt.Sprintf("Mission: %s", mission),
		fmt.Sprintf("Working posture: %s", posture),
		fmt.Sprintf("Office objective: %s", strings.TrimSpace(input.Intent)),
		fmt.Sprintf("Governance: %s", genesisFirstNonEmpty(input.Governance, input.Constitution.Governance, input.Template.LeadershipMode, "hybrid")),
		fmt.Sprintf("Observation scopes: %s", strings.Join(scopes, ", ")),
		fmt.Sprintf("Allowed actions: %s", joinActions(actions)),
	}
	if reportsTo != "" {
		lines = append(lines, fmt.Sprintf("Escalate blockers and review-ready work to %s.", reportsTo))
	}
	if len(routing) > 0 {
		routes := make([]string, 0, len(routing))
		for key, value := range routing {
			routes = append(routes, fmt.Sprintf("%s->%s", key, value))
		}
		sort.Strings(routes)
		lines = append(lines, "Peer routing: "+strings.Join(routes, ", "))
	}
	lines = append(lines,
		fmt.Sprintf("Shared workplace: %s", genesisFirstNonEmpty(input.SharedWorkplace, "shared_workplace")),
		fmt.Sprintf("Ledger path: %s", genesisFirstNonEmpty(input.LedgerPath, "ledger")),
		fmt.Sprintf("Bus: %s", genesisFirstNonEmpty(input.RedisAddress, "local event bus")),
		"Do not claim work is real until it has been persisted, signaled, or published through the office runtime.",
	)
	return strings.Join(lines, " ")
}

func buildManufacturedSummary(intent OrgIntent, input GenesisManufacturingInput, roleCount int) string {
	parts := []string{
		fmt.Sprintf("Intent: %s.", strings.TrimSpace(input.Intent)),
		fmt.Sprintf("Constitution: %s.", genesisFirstNonEmpty(input.Constitution.Name, input.ConstitutionName)),
		fmt.Sprintf("Topology: %s.", genesisFirstNonEmpty(input.Governance, input.Constitution.Governance, input.Template.LeadershipMode, "hybrid")),
		fmt.Sprintf("Manufactured bundles: %d.", roleCount),
		fmt.Sprintf("Domain: %s.", intent.Domain),
		fmt.Sprintf("Working style: %s.", intent.WorkingStyle),
	}
	if intent.TimeHorizon != "" {
		parts = append(parts, fmt.Sprintf("Time horizon: %s.", intent.TimeHorizon))
	}
	if intent.GoalShape != "" {
		parts = append(parts, fmt.Sprintf("Goal shape: %s.", intent.GoalShape))
	}
	return strings.Join(parts, " ")
}

func manufacturingRequirements(input GenesisManufacturingInput) []string {
	reqs := []string{
		"manufacture role-specific prompts",
		"bind tools and allowed actions",
		"establish peer routing and reporting lines",
	}
	if strings.TrimSpace(input.WorkingStyle) != "" {
		reqs = append(reqs, "match the requested working style")
	}
	if strings.TrimSpace(input.GoalShape) != "" {
		reqs = append(reqs, "optimize for the requested goal shape")
	}
	return reqs
}

func summarizeIntent(input GenesisManufacturingInput) string {
	parts := []string{strings.TrimSpace(input.Intent)}
	if strings.TrimSpace(input.Domain) != "" {
		parts = append(parts, "domain="+strings.TrimSpace(input.Domain))
	}
	if strings.TrimSpace(input.WorkingStyle) != "" {
		parts = append(parts, "style="+strings.TrimSpace(input.WorkingStyle))
	}
	return strings.Join(parts, " | ")
}

func inferDomain(intent string, template config.TeamTemplate) string {
	intent = strings.ToLower(intent)
	switch {
	case strings.Contains(intent, "research"), strings.Contains(intent, "analysis"):
		return "research"
	case strings.Contains(intent, "design"), strings.Contains(intent, "brand"):
		return "creative"
	case strings.Contains(intent, "launch"), strings.Contains(intent, "market"):
		return "go-to-market"
	case strings.TrimSpace(template.Category) != "":
		return template.Category
	default:
		return "software delivery"
	}
}

func roleArchetype(name string) string {
	n := normalizeHandle(name)
	switch {
	case strings.Contains(n, "lead"), strings.Contains(n, "director"), strings.Contains(n, "chief"), strings.Contains(n, "manager"):
		return "lead"
	case strings.Contains(n, "research"), strings.Contains(n, "analyst"), strings.Contains(n, "strategist"):
		return "research"
	case strings.Contains(n, "review"), strings.Contains(n, "tester"), strings.Contains(n, "qa"), strings.Contains(n, "verifier"):
		return "review"
	case strings.Contains(n, "ops"), strings.Contains(n, "scheduler"), strings.Contains(n, "coordinator"):
		return "ops"
	default:
		return "builder"
	}
}

func spawnOrderForArchetype(archetype string, index int) int {
	switch archetype {
	case "lead":
		return 10 + index
	case "research":
		return 20 + index
	case "builder":
		return 30 + index
	case "review":
		return 40 + index
	case "ops":
		return 50 + index
	default:
		return 60 + index
	}
}

func roleLabel(roleName, profile string) string {
	if strings.TrimSpace(profile) != "" {
		return fmt.Sprintf("%s / %s", displayRoleName(roleName), profile)
	}
	return displayRoleName(roleName)
}

func compactMission(value string) string {
	value = strings.Join(strings.Fields(strings.TrimSpace(value)), " ")
	if len(value) <= 88 {
		return value
	}
	return value[:85] + "..."
}

func displayRoleName(name string) string {
	parts := strings.Fields(strings.NewReplacer("-", " ", "_", " ").Replace(strings.TrimSpace(name)))
	for i, part := range parts {
		if part == "" {
			continue
		}
		parts[i] = strings.ToUpper(part[:1]) + strings.ToLower(part[1:])
	}
	return strings.Join(parts, " ")
}

func normalizeHandle(value string) string {
	value = strings.ToLower(strings.TrimSpace(value))
	value = strings.NewReplacer(" ", "-", "_", "-", "/", "-").Replace(value)
	for strings.Contains(value, "--") {
		value = strings.ReplaceAll(value, "--", "-")
	}
	return strings.Trim(value, "-")
}

func joinActions(actions []ActionType) string {
	parts := make([]string, 0, len(actions))
	for _, action := range actions {
		parts = append(parts, string(action))
	}
	sort.Strings(parts)
	return strings.Join(parts, ", ")
}

func dedupeActions(actions []ActionType) []ActionType {
	seen := map[ActionType]struct{}{}
	out := make([]ActionType, 0, len(actions))
	for _, action := range actions {
		if _, ok := seen[action]; ok {
			continue
		}
		seen[action] = struct{}{}
		out = append(out, action)
	}
	return out
}

func dedupeStrings(values []string) []string {
	seen := map[string]struct{}{}
	out := make([]string, 0, len(values))
	for _, value := range values {
		if _, ok := seen[value]; ok {
			continue
		}
		seen[value] = struct{}{}
		out = append(out, value)
	}
	return out
}

func genesisFirstNonEmpty(values ...string) string {
	for _, value := range values {
		if strings.TrimSpace(value) != "" {
			return strings.TrimSpace(value)
		}
	}
	return ""
}

func boolValue(value *bool) bool {
	return value != nil && *value
}

func boolPointer(value bool) *bool {
	return &value
}
