package dialog

import (
	"fmt"
	"os"
	"path/filepath"
	"sort"
	"strings"

	"github.com/ETEllis/teamcode/internal/config"
)

func ActiveTeamName() string {
	cfg := config.Get()
	if cfg == nil {
		return "teamcode"
	}
	if strings.TrimSpace(cfg.Team.ActiveTeam) != "" {
		return strings.TrimSpace(cfg.Team.ActiveTeam)
	}
	return "teamcode"
}

func DefaultTemplateName() string {
	cfg := config.Get()
	if cfg == nil || strings.TrimSpace(cfg.Team.DefaultTemplate) == "" {
		return "leader-led"
	}
	return strings.TrimSpace(cfg.Team.DefaultTemplate)
}

func ConfiguredTemplateNames() []string {
	cfg := config.Get()
	if cfg == nil {
		return nil
	}
	names := make([]string, 0, len(cfg.Team.Templates))
	for name := range cfg.Team.Templates {
		names = append(names, name)
	}
	sort.Strings(names)
	return names
}

func TeamStatusPrompt() string {
	return fmt.Sprintf("Inspect the current collaboration state for team %q using the team_status tool. Summarize the leader, template, role assignments, worker tree, inbox and broadcast load, handoffs, and task board state.", ActiveTeamName())
}

func AgencyStatusPrompt() string {
	return fmt.Sprintf("Inspect the current Agency office state for team %q using the team_status tool. Summarize the current constitution/template, leader, role assignments, active workers, message load, pending handoffs, board state, and the most important next signals in a concise office-status format.", ActiveTeamName())
}

func TeamBootstrapPrompt(templateName string) string {
	if strings.TrimSpace(templateName) == "" {
		templateName = DefaultTemplateName()
	}
	return fmt.Sprintf("Bootstrap or reconcile the active team %q using the configured %q team template. Prefer team_bootstrap, respect the template's roles and policies, and summarize the resulting topology and delegation rules.", ActiveTeamName(), templateName)
}

func AgencyBootstrapPrompt(templateName string) string {
	if strings.TrimSpace(templateName) == "" {
		templateName = DefaultTemplateName()
	}
	return fmt.Sprintf("Stand up or reconcile the active Agency office %q using the configured %q constitution/template. Prefer team_bootstrap, preserve existing shared state, respect the configured roles and policies, and summarize the resulting org topology, routing, and next actions.", ActiveTeamName(), templateName)
}

func AgencyGenesisPrompt() string {
	return fmt.Sprintf("Treat this project as entering The Agency. Infer the likely office shape for team %q from the repo and current context, then use the available team tools to bootstrap or reconcile the right working structure. Summarize the chosen constitution, roles, delegation posture, and what the user can do next from solo mode or Agency mode.", ActiveTeamName())
}

func TeamTemplatesPrompt() string {
	names := ConfiguredTemplateNames()
	if len(names) == 0 {
		return "Summarize the available TeamCode team templates and explain how to customize them in .teamcode.json."
	}
	return fmt.Sprintf("Summarize the available TeamCode team templates for this project (%s) and explain how to customize them in .teamcode.json.", strings.Join(names, ", "))
}

func AgencyTemplatesPrompt() string {
	names := ConfiguredTemplateNames()
	if len(names) == 0 {
		return "Summarize the available Agency constitutions/templates and explain how to customize them in .teamcode.json."
	}
	return fmt.Sprintf("Summarize the available Agency constitutions/templates for this project (%s) and explain how to customize them in .teamcode.json.", strings.Join(names, ", "))
}

func ListSkillsPrompt() string {
	skills := InstalledSkillNames()
	if len(skills) == 0 {
		return "Check whether any local Codex or agent skills are installed and explain how to use them from The Agency custom commands."
	}
	return fmt.Sprintf("Summarize the installed local skills available to The Agency runtime: %s. Explain how to invoke them with slash commands or the command dialog.", strings.Join(skills, ", "))
}

func SkillUsePrompt(name, task string) string {
	path := InstalledSkillPath(name)
	if path == "" {
		if task == "" {
			return fmt.Sprintf("Look for an installed skill named %q and explain whether it exists.", name)
		}
		return fmt.Sprintf("Look for an installed skill named %q. If found, read it first and then help with: %s", name, task)
	}
	if task == "" {
		task = "the current user request"
	}
	return fmt.Sprintf("Use the skill defined at %s. Read it first, then help with: %s", path, task)
}

func InstalledSkillNames() []string {
	paths := installedSkillIndex()
	names := make([]string, 0, len(paths))
	for name := range paths {
		names = append(names, name)
	}
	sort.Strings(names)
	return names
}

func InstalledSkillPath(name string) string {
	return installedSkillIndex()[name]
}

func installedSkillIndex() map[string]string {
	index := map[string]string{}
	for _, root := range installedSkillRoots() {
		entries, err := os.ReadDir(root)
		if err != nil {
			continue
		}
		for _, entry := range entries {
			if !entry.IsDir() {
				continue
			}
			skillPath := filepath.Join(root, entry.Name(), "SKILL.md")
			if _, err := os.Stat(skillPath); err == nil {
				if _, exists := index[entry.Name()]; !exists {
					index[entry.Name()] = skillPath
				}
			}
		}
	}
	return index
}

func installedSkillRoots() []string {
	var roots []string
	if codexHome := strings.TrimSpace(os.Getenv("CODEX_HOME")); codexHome != "" {
		roots = append(roots, filepath.Join(codexHome, "skills"))
	}
	if home, err := os.UserHomeDir(); err == nil {
		roots = append(roots,
			filepath.Join(home, ".codex", "skills"),
			filepath.Join(home, ".agents", "skills"),
		)
	}
	return roots
}
