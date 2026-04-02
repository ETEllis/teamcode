package agent

import (
	"context"

	"github.com/ETEllis/teamcode/internal/history"
	"github.com/ETEllis/teamcode/internal/llm/tools"
	"github.com/ETEllis/teamcode/internal/lsp"
	"github.com/ETEllis/teamcode/internal/message"
	"github.com/ETEllis/teamcode/internal/orchestration"
	"github.com/ETEllis/teamcode/internal/permission"
	"github.com/ETEllis/teamcode/internal/session"
	"github.com/ETEllis/teamcode/internal/team"
)

func CoderAgentTools(
	permissions permission.Service,
	sessions session.Service,
	messages message.Service,
	history history.Service,
	lspClients map[string]*lsp.Client,
	teamService *team.Service,
	manager *orchestration.Manager,
) []tools.BaseTool {
	ctx := context.Background()
	otherTools := GetMcpTools(ctx, permissions)
	if len(lspClients) > 0 {
		otherTools = append(otherTools, tools.NewDiagnosticsTool(lspClients))
	}
	return append(
		[]tools.BaseTool{
			tools.NewBashTool(permissions),
			tools.NewEditTool(lspClients, permissions, history),
			tools.NewFetchTool(permissions),
			tools.NewGlobTool(),
			tools.NewGrepTool(),
			tools.NewLsTool(),
			tools.NewSourcegraphTool(),
			tools.NewViewTool(lspClients),
			tools.NewPatchTool(lspClients, permissions, history),
			tools.NewWriteTool(lspClients, permissions, history),
			tools.NewAgencyGenesisTool(teamService, manager),
			tools.NewTeamBootstrapTool(teamService, manager),
			tools.NewTeamTool(teamService),
			tools.NewTeamAddRoleTool(teamService),
			tools.NewTeamAssignRoleTool(teamService),
			tools.NewTaskBoardTool(teamService),
			tools.NewTaskMoveTool(teamService),
			tools.NewHandoffCreateTool(teamService),
			tools.NewHandoffAcceptTool(teamService),
			tools.NewInboxReadTool(teamService),
			tools.NewTeamMessageSendTool(teamService),
			tools.NewTeamBroadcastTool(teamService),
			tools.NewOfficeStatusTool(teamService, manager),
			tools.NewTeamStatusTool(teamService, manager),
			tools.NewTeammateSpawnTool(teamService, manager),
			tools.NewTeammateWaitTool(teamService, manager),
			tools.NewSubagentSpawnTool(teamService, manager),
			tools.NewSubagentWaitTool(manager),
			NewAgentTool(sessions, messages, lspClients),
		}, otherTools...,
	)
}

func TaskAgentTools(lspClients map[string]*lsp.Client) []tools.BaseTool {
	return []tools.BaseTool{
		tools.NewGlobTool(),
		tools.NewGrepTool(),
		tools.NewLsTool(),
		tools.NewSourcegraphTool(),
		tools.NewViewTool(lspClients),
	}
}
