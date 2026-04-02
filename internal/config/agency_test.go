package config

import "testing"

func TestNormalizeAgencyConfigAppliesDefaultsAndBlueprintConstitutions(t *testing.T) {
	teamCfg := normalizeTeamConfig(TeamConfig{})
	cfg := normalizeAgencyConfig(AgencyConfig{}, teamCfg, ".teamcode")

	if cfg.ProductName != "The Agency" {
		t.Fatalf("expected Agency product name default, got %q", cfg.ProductName)
	}
	if cfg.CurrentConstitution != "coding-office" {
		t.Fatalf("expected coding-office default constitution, got %q", cfg.CurrentConstitution)
	}
	if cfg.SoloConstitution != "solo" {
		t.Fatalf("expected solo constitution default, got %q", cfg.SoloConstitution)
	}
	if cfg.Office.SharedWorkplace == "" || cfg.Office.StateFile == "" {
		t.Fatalf("expected office paths to be populated")
	}
	if _, ok := cfg.Constitutions["software-team"]; !ok {
		t.Fatalf("expected software-team constitution to be derived from blueprints")
	}
	if _, ok := cfg.Constitutions["solo"]; !ok {
		t.Fatalf("expected solo constitution to be present")
	}
}

func TestNormalizeAgencyConfigPreservesOverrides(t *testing.T) {
	teamCfg := normalizeTeamConfig(TeamConfig{})
	cfg := normalizeAgencyConfig(AgencyConfig{
		ProductName:         "Custom Agency",
		CurrentConstitution: "research-lab",
		SoloConstitution:    "solo",
		Office: OfficeRuntimeConfig{
			SharedWorkplace: "/tmp/agency/shared",
			StateFile:       "/tmp/agency/state.json",
		},
		Redis: RedisRuntimeConfig{
			Address:       "redis.internal:6379",
			ChannelPrefix: "custom-agency",
		},
		Ledger: LedgerRuntimeConfig{
			Path:           "/tmp/agency/ledger.log",
			SnapshotPath:   "/tmp/agency/snaps",
			ProjectionFile: "/tmp/agency/context.json",
		},
	}, teamCfg, ".teamcode")

	if cfg.ProductName != "Custom Agency" {
		t.Fatalf("expected product name override, got %q", cfg.ProductName)
	}
	if cfg.CurrentConstitution != "research-lab" {
		t.Fatalf("expected constitution override, got %q", cfg.CurrentConstitution)
	}
	if cfg.Office.SharedWorkplace != "/tmp/agency/shared" {
		t.Fatalf("expected shared workplace override, got %q", cfg.Office.SharedWorkplace)
	}
	if cfg.Redis.Address != "redis.internal:6379" {
		t.Fatalf("expected redis address override, got %q", cfg.Redis.Address)
	}
	if cfg.Ledger.Path != "/tmp/agency/ledger.log" {
		t.Fatalf("expected ledger path override, got %q", cfg.Ledger.Path)
	}
}

func TestValidateAgencyConfigRejectsUnknownCurrentConstitution(t *testing.T) {
	err := ValidateAgencyConfig(AgencyConfig{
		CurrentConstitution: "missing",
		SoloConstitution:    "solo",
		Office: OfficeRuntimeConfig{
			SharedWorkplace: ".teamcode/agency/shared_workplace",
			StateFile:       ".teamcode/agency/office-state.json",
		},
		Redis: RedisRuntimeConfig{
			DB: 0,
		},
		Ledger: LedgerRuntimeConfig{
			DefaultQuorum: 1,
		},
		Constitutions: map[string]AgencyConstitution{
			"solo": {Name: "solo"},
		},
	})
	if err == nil {
		t.Fatalf("expected unknown constitution validation failure")
	}
}
