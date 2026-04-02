package config

import "testing"

func TestNormalizeTeamConfigPreservesDefaultsAndOverrides(t *testing.T) {
	cfg := normalizeTeamConfig(TeamConfig{
		ActiveTeam:      "alpha",
		DefaultTemplate: "research-heavy",
		Templates: map[string]TeamTemplate{
			"leader-led": {
				Description: "Customized default team",
				Policies: TeamPolicies{
					DelegationMode: "peer-reviewed",
				},
			},
			"ship-room": {
				LeadershipMode: "custom",
				Roles: []TeamRoleTemplate{
					{Name: "captain"},
				},
			},
		},
	})

	if cfg.ActiveTeam != "alpha" {
		t.Fatalf("expected active team override, got %q", cfg.ActiveTeam)
	}
	if cfg.DefaultTemplate != "research-heavy" {
		t.Fatalf("expected default template override, got %q", cfg.DefaultTemplate)
	}
	if cfg.Templates["leader-led"].Description != "Customized default team" {
		t.Fatalf("expected default template description override")
	}
	if cfg.Templates["leader-led"].Policies.DelegationMode != "peer-reviewed" {
		t.Fatalf("expected custom delegation mode override")
	}
	if cfg.Templates["leader-led"].Policies.MaxWIP == nil || *cfg.Templates["leader-led"].Policies.MaxWIP != 3 {
		t.Fatalf("expected leader-led defaults to remain intact")
	}
	if cfg.Templates["ship-room"].LeadershipMode != "custom" {
		t.Fatalf("expected custom template leadership mode")
	}
	if cfg.Templates["ship-room"].SpawnTeammates == nil || !*cfg.Templates["ship-room"].SpawnTeammates {
		t.Fatalf("expected custom template to receive normalized defaults")
	}
	if len(cfg.Templates["ship-room"].Roles) != 1 || cfg.Templates["ship-room"].Roles[0].Profile != "coder" {
		t.Fatalf("expected custom role defaults to be applied")
	}
}
