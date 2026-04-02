package tools

import (
	"testing"

	"github.com/ETEllis/teamcode/internal/config"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestAgencyToolSurfacesExposeAgencyNames(t *testing.T) {
	assert.Equal(t, AgencyGenesisToolName, NewAgencyGenesisTool(nil, nil).Info().Name)
	assert.Equal(t, OfficeStatusToolName, NewOfficeStatusTool(nil, nil).Info().Name)
	assert.Contains(t, NewTeammateSpawnTool(nil, nil).Info().Description, "office agent")
	assert.Contains(t, NewSubagentSpawnTool(nil, nil).Info().Description, "bounded delegated agent")
}

func TestConstitutionTemplateNameUsesAgencyConfig(t *testing.T) {
	ensureToolsConfigLoaded(t)

	assert.Equal(t, "leader-led", constitutionTemplateName("coding-office"))
	assert.Equal(t, "solo", constitutionTemplateName("solo"))
}

func TestAgencyWorkingAgreementUsesConstitutionDefaults(t *testing.T) {
	ensureToolsConfigLoaded(t)

	agreement := agencyWorkingAgreement("coding-office", "shared", "weekday-office-hours", nil)
	require.NotNil(t, agreement)
	assert.Equal(t, "hierarchical", agreement.LeadershipMode)
	assert.Equal(t, "delegated", agreement.DelegationMode)
	assert.Equal(t, "review-gated", agreement.ReviewRouting)
	assert.Equal(t, "role-quorum", agreement.SynthesisRouting)
	assert.Equal(t, "shared", agreement.LocalChatDefault)
	assert.Contains(t, agreement.HandoffRequires, "shift-handoff")
}

func ensureToolsConfigLoaded(t *testing.T) {
	t.Helper()
	if config.Get() != nil {
		return
	}
	_, err := config.Load(t.TempDir(), false)
	require.NoError(t, err)
}
