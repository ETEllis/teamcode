package agency

import (
	"fmt"
	"os"
	"path/filepath"
	"strconv"
	"strings"

	projectconfig "github.com/ETEllis/teamcode/internal/config"
)

type Bootstrap struct {
	Config       Config
	Constitution AgencyConstitution
}

func LoadBootstrap(workingDir string, constitutionOverride string, runtimeMode RuntimeMode, actorBinary string) (Bootstrap, error) {
	if strings.TrimSpace(workingDir) == "" {
		cwd, err := os.Getwd()
		if err != nil {
			return Bootstrap{}, err
		}
		workingDir = cwd
	}

	cfg, err := projectconfig.Load(workingDir, false)
	if err != nil {
		return Bootstrap{}, err
	}

	constitution, err := ConstitutionFromConfig(cfg, constitutionOverride)
	if err != nil {
		return Bootstrap{}, err
	}

	baseDir := filepath.Join(cfg.Data.Directory, "agency")
	if strings.TrimSpace(cfg.Agency.Ledger.Path) != "" {
		baseDir = filepath.Dir(cfg.Agency.Ledger.Path)
	}
	baseDir = firstBootstrapValue(os.Getenv("AGENCY_BASE_DIR"), baseDir)

	sharedWorkplace := firstBootstrapValue(os.Getenv("AGENCY_SHARED_WORKPLACE"), cfg.Agency.Office.SharedWorkplace)
	redisAddr := firstBootstrapValue(os.Getenv("AGENCY_REDIS_ADDR"), cfg.Agency.Redis.Address)
	redisPassword := strings.TrimSpace(os.Getenv("AGENCY_REDIS_PASSWORD"))
	redisDB := cfg.Agency.Redis.DB
	if override := strings.TrimSpace(os.Getenv("AGENCY_REDIS_DB")); override != "" {
		if parsed, err := strconv.Atoi(override); err == nil {
			redisDB = parsed
		}
	}

	bootstrap := Bootstrap{
		Config: Config{
			BaseDir:         baseDir,
			WorkingDir:      workingDir,
			SharedWorkplace: sharedWorkplace,
			RuntimeMode:     runtimeMode,
			ActorBinaryPath: actorBinary,
			Redis: &RedisConfig{
				Addr:     redisAddr,
				Password: redisPassword,
				DB:       redisDB,
			},
		},
		Constitution: constitution,
	}
	return bootstrap, nil
}

func ConstitutionFromConfig(cfg *projectconfig.Config, preferred string) (AgencyConstitution, error) {
	if cfg == nil {
		return AgencyConstitution{}, fmt.Errorf("config not loaded")
	}

	selectedName := strings.TrimSpace(preferred)
	if selectedName == "" {
		selectedName = cfg.Agency.CurrentConstitution
	}
	if selectedName == "" {
		selectedName = cfg.Agency.SoloConstitution
	}
	selected, ok := cfg.Agency.Constitutions[selectedName]
	if !ok {
		return AgencyConstitution{}, fmt.Errorf("agency constitution %q not found", selectedName)
	}

	template := resolveTemplate(cfg, selected)
	roleSpecs := make(map[string]RoleSpec, len(template.Roles))
	for _, role := range template.Roles {
		roleSpecs[role.Name] = RoleSpec{
			Name:              role.Name,
			Mission:           firstBootstrapValue(role.Responsible, role.CurrentFocus, role.Profile, role.Name),
			Personality:       role.Profile,
			WorkingPosture:    firstBootstrapValue(role.CurrentFocus, template.Orientation, template.Category),
			SystemPrompt:      role.Prompt,
			AllowedActions:    defaultAllowedActions(role),
			ObservationScopes: []string{"default"},
			ToolBindings:      defaultToolBindings(role),
			PeerRouting:       defaultPeerRouting(role),
			CanSpawnAgents:    boolValue(role.CanSpawnSubagents),
		}
	}

	orgID := cfg.Team.ActiveTeam
	if strings.TrimSpace(orgID) == "" {
		orgID = firstBootstrapValue(selected.Blueprint, selected.TeamTemplate, selectedName)
	}

	return AgencyConstitution{
		ID:             "constitution-" + selectedName,
		Name:           selectedName,
		Description:    selected.Description,
		OrganizationID: orgID,
		GovernanceMode: mapGovernance(selected.Governance),
		Roles:          roleSpecs,
		Metadata: map[string]string{
			"blueprint":        selected.Blueprint,
			"teamTemplate":     selected.TeamTemplate,
			"runtimeMode":      selected.RuntimeMode,
			"entryMode":        selected.EntryMode,
			"defaultSchedule":  selected.DefaultSchedule,
			"workspaceMode":    template.Policies.WorkspaceModeDefault,
			"loopStrategy":     template.Policies.LoopStrategy,
			"consensusMode":    selected.Policies.ConsensusMode,
			"publicationMode":  selected.Policies.PublicationPolicy,
			"configSourcePath": cfg.Data.Directory,
		},
	}, nil
}

func resolveTemplate(cfg *projectconfig.Config, constitution projectconfig.AgencyConstitution) projectconfig.TeamTemplate {
	if constitution.Blueprint != "" {
		if template, ok := cfg.Team.Blueprints[constitution.Blueprint]; ok && template.Name != "" {
			return template
		}
	}
	if constitution.TeamTemplate != "" {
		if template, ok := cfg.Team.Templates[constitution.TeamTemplate]; ok && template.Name != "" {
			return template
		}
	}
	if template, ok := cfg.Team.Blueprints[cfg.Team.DefaultBlueprint]; ok && template.Name != "" {
		return template
	}
	if template, ok := cfg.Team.Templates[cfg.Team.DefaultTemplate]; ok && template.Name != "" {
		return template
	}
	return projectconfig.TeamTemplate{}
}

func mapGovernance(mode string) GovernanceMode {
	switch strings.TrimSpace(mode) {
	case "hierarchical":
		return GovernanceHierarchical
	case "peer":
		return GovernancePeer
	case "federated":
		return GovernanceFederated
	case "flat":
		return GovernanceFlat
	default:
		return GovernanceHybrid
	}
}

func defaultAllowedActions(role projectconfig.TeamRoleTemplate) []ActionType {
	actions := []ActionType{ActionUpdateTask, ActionBroadcast, ActionPingPeer, ActionHandoffShift}
	if role.Profile == "coder" {
		actions = append(actions, ActionWriteCode, ActionRunTest, ActionRequestReview)
	}
	if boolValue(role.CanSpawnSubagents) {
		actions = append(actions, ActionSpawnAgent)
	}
	return actions
}

func defaultToolBindings(role projectconfig.TeamRoleTemplate) []string {
	tools := []string{"ledger", "bus"}
	if role.Profile == "coder" {
		tools = append(tools, "shell", "files")
	}
	return tools
}

func defaultPeerRouting(role projectconfig.TeamRoleTemplate) map[string]string {
	if strings.TrimSpace(role.ReportsTo) == "" {
		return nil
	}
	return map[string]string{"reportsTo": role.ReportsTo}
}

func firstBootstrapValue(values ...string) string {
	for _, value := range values {
		if strings.TrimSpace(value) != "" {
			return strings.TrimSpace(value)
		}
	}
	return ""
}
