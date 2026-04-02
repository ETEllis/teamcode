package agent

import (
	"context"
	"encoding/json"
	"fmt"

	"github.com/ETEllis/teamcode/internal/config"
	"github.com/ETEllis/teamcode/internal/llm/tools"
	"github.com/ETEllis/teamcode/internal/lsp"
	"github.com/ETEllis/teamcode/internal/message"
	"github.com/ETEllis/teamcode/internal/session"
)

type agentTool struct {
	sessions   session.Service
	messages   message.Service
	lspClients map[string]*lsp.Client
}

const (
	AgentToolName = "agent"
)

type AgentParams struct {
	Prompt string `json:"prompt"`
}

func (b *agentTool) Info() tools.ToolInfo {
	return tools.ToolInfo{
		Name:        AgentToolName,
		Description: "Launch a new stateless helper agent for parallel search and analysis. This is useful when you want several independent agent processes working concurrently without turning them into persistent office agents. Under the solo constitution, these helper agents act as short-lived parallel scouts. Under office mode, they remain outside durable office continuity. The helper agent has access to GlobTool, GrepTool, LS, and View.\n\nExamples:\n- If you are searching for a keyword like \"config\" or \"logger\", or for questions like \"which file does X?\", the Agent tool is strongly recommended\n- If you want to read a specific file path, use the View or GlobTool tool instead of the Agent tool, to find the match more quickly\n- If you are searching for a specific class definition like \"class Foo\", use the GlobTool tool instead, to find the match more quickly\n\nUsage notes:\n1. Launch multiple agents concurrently whenever possible to maximize performance; do that with a single message containing multiple tool uses\n2. When the agent is done, it returns a single message back to you. The result is not shown to the user automatically, so summarize it yourself when needed\n3. Each agent invocation is stateless. It cannot receive follow-up messages and it cannot participate in office continuity, so your prompt should contain a highly detailed task description and say exactly what information to return\n4. The agent's outputs should generally be trusted\n5. IMPORTANT: The agent cannot use Bash, Replace, or Edit, so it cannot modify files. If you need mutation, use those tools directly instead of routing through the helper agent.",
		Parameters: map[string]any{
			"prompt": map[string]any{
				"type":        "string",
				"description": "The task for the agent to perform",
			},
		},
		Required: []string{"prompt"},
	}
}

func (b *agentTool) Run(ctx context.Context, call tools.ToolCall) (tools.ToolResponse, error) {
	var params AgentParams
	if err := json.Unmarshal([]byte(call.Input), &params); err != nil {
		return tools.NewTextErrorResponse(fmt.Sprintf("error parsing parameters: %s", err)), nil
	}
	if params.Prompt == "" {
		return tools.NewTextErrorResponse("prompt is required"), nil
	}

	sessionID, messageID := tools.GetContextValues(ctx)
	if sessionID == "" || messageID == "" {
		return tools.ToolResponse{}, fmt.Errorf("session_id and message_id are required")
	}

	agent, err := NewAgent(config.AgentTask, b.sessions, b.messages, TaskAgentTools(b.lspClients))
	if err != nil {
		return tools.ToolResponse{}, fmt.Errorf("error creating agent: %s", err)
	}

	session, err := b.sessions.CreateTaskSession(ctx, call.ID, sessionID, "New Agent Session")
	if err != nil {
		return tools.ToolResponse{}, fmt.Errorf("error creating session: %s", err)
	}

	done, err := agent.Run(ctx, session.ID, params.Prompt)
	if err != nil {
		return tools.ToolResponse{}, fmt.Errorf("error generating agent: %s", err)
	}
	result := <-done
	if result.Error != nil {
		return tools.ToolResponse{}, fmt.Errorf("error generating agent: %s", result.Error)
	}

	response := result.Message
	if response.Role != message.Assistant {
		return tools.NewTextErrorResponse("no response"), nil
	}

	updatedSession, err := b.sessions.Get(ctx, session.ID)
	if err != nil {
		return tools.ToolResponse{}, fmt.Errorf("error getting session: %s", err)
	}
	parentSession, err := b.sessions.Get(ctx, sessionID)
	if err != nil {
		return tools.ToolResponse{}, fmt.Errorf("error getting parent session: %s", err)
	}

	parentSession.Cost += updatedSession.Cost

	_, err = b.sessions.Save(ctx, parentSession)
	if err != nil {
		return tools.ToolResponse{}, fmt.Errorf("error saving parent session: %s", err)
	}
	return tools.NewTextResponse(response.Content().String()), nil
}

func NewAgentTool(
	Sessions session.Service,
	Messages message.Service,
	LspClients map[string]*lsp.Client,
) tools.BaseTool {
	return &agentTool{
		sessions:   Sessions,
		messages:   Messages,
		lspClients: LspClients,
	}
}
