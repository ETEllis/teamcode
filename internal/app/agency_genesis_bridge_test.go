package app

import (
	"context"
	"encoding/json"
	"os"
	"path/filepath"
	"testing"

	agencypkg "github.com/ETEllis/teamcode/internal/agency"
	"github.com/ETEllis/teamcode/internal/config"
	"github.com/stretchr/testify/require"
)

// TestManifestActorsBridgesGenesisToActorSpecs guards the genesis→actor-spawn
// bridge (HANDOFF 2026-04-02). It verifies that ManifestActors converts every
// GenesisRoleBundle persisted in office-state.json into an ActorRuntimeSpec
// JSON file under {baseDir}/runtime/actors/{id}.json, preserving the bundle's
// role, organization, and runtime mode.
//
// Regression class: prior to the fix, role bundles were saved to
// office-state.json but never converted to actor specs, so the runtime daemon
// found nothing to spawn — the canonical V1 silent-failure mode.
func TestManifestActorsBridgesGenesisToActorSpecs(t *testing.T) {
	t.Parallel()

	// Hermetic env: AGENCY_BASE_DIR could otherwise redirect the spec writer.
	t.Setenv("AGENCY_BASE_DIR", "")

	tmp := t.TempDir()

	cfg := &config.Config{
		Data: config.Data{Directory: tmp},
	}
	cfg.Agency.Office.StateFile = filepath.Join(tmp, "agency", "office-state.json")
	cfg.Team.ActiveTeam = "test-org"

	svc := NewAgencyService(cfg)

	// Persist a genesis result with two role bundles, mirroring what
	// StartGenesis would have written before the bridge ran.
	state := agencyState{
		LastGenesis: &AgencyGenesisResult{
			ConstitutionName: "test-constitution",
			RoleBundles: []agencypkg.GenesisRoleBundle{
				{
					RoleName:    "lead",
					DisplayName: "Lead",
					Profile:     "coder",
					Mission:     "coordinate the office",
					SpawnOrder:  0,
				},
				{
					RoleName:    "implementer",
					DisplayName: "Implementer",
					Profile:     "coder",
					ReportsTo:   "lead",
					Mission:     "build the changes",
					SpawnOrder:  1,
				},
			},
		},
	}

	require.NoError(t, os.MkdirAll(filepath.Dir(cfg.Agency.Office.StateFile), 0o755))
	raw, err := json.MarshalIndent(state, "", "  ")
	require.NoError(t, err)
	require.NoError(t, os.WriteFile(cfg.Agency.Office.StateFile, raw, 0o644))

	count, err := svc.ManifestActors(context.Background())
	require.NoError(t, err)
	require.Equal(t, 2, count, "expected one actor spec per role bundle")

	specsDir := filepath.Join(tmp, "agency", "runtime", "actors")
	cases := []struct {
		id   string
		role string
	}{
		{"lead", "lead"},
		{"implementer", "implementer"},
	}
	for _, c := range cases {
		specPath := filepath.Join(specsDir, c.id+".json")
		body, err := os.ReadFile(specPath)
		require.NoErrorf(t, err, "expected actor spec at %s", specPath)

		var spec agencypkg.ActorRuntimeSpec
		require.NoError(t, json.Unmarshal(body, &spec))
		require.Equal(t, c.id, spec.Identity.ID)
		require.Equal(t, c.role, spec.Identity.Role)
		require.Equal(t, "test-org", spec.OrganizationID)
		require.Equal(t, agencypkg.RuntimeModeDaemonized, spec.RuntimeMode)
	}
}

// TestManifestActorsNoOpsWhenStateMissing covers the empty-state path: if
// office-state.json has not been written yet, ManifestActors must report
// zero actors and not error. Without this guard, a fresh checkout would
// surface a confusing error on the first `/agency bootstrap` call.
func TestManifestActorsNoOpsWhenStateMissing(t *testing.T) {
	t.Parallel()

	t.Setenv("AGENCY_BASE_DIR", "")

	tmp := t.TempDir()
	cfg := &config.Config{
		Data: config.Data{Directory: tmp},
	}
	cfg.Agency.Office.StateFile = filepath.Join(tmp, "agency", "office-state.json")
	cfg.Team.ActiveTeam = "test-org"

	svc := NewAgencyService(cfg)

	count, err := svc.ManifestActors(context.Background())
	require.NoError(t, err)
	require.Equal(t, 0, count)
}
