package main

import (
	"encoding/json"
	"fmt"
	"os"

	"github.com/ETEllis/teamcode/internal/config"
	"github.com/ETEllis/teamcode/internal/llm/models"
)

// JSONSchemaType represents a JSON Schema type
type JSONSchemaType struct {
	Type                 string           `json:"type,omitempty"`
	Description          string           `json:"description,omitempty"`
	Properties           map[string]any   `json:"properties,omitempty"`
	Required             []string         `json:"required,omitempty"`
	AdditionalProperties any              `json:"additionalProperties,omitempty"`
	Enum                 []any            `json:"enum,omitempty"`
	Items                map[string]any   `json:"items,omitempty"`
	OneOf                []map[string]any `json:"oneOf,omitempty"`
	AnyOf                []map[string]any `json:"anyOf,omitempty"`
	Default              any              `json:"default,omitempty"`
}

func main() {
	schema := generateSchema()

	// Pretty print the schema
	encoder := json.NewEncoder(os.Stdout)
	encoder.SetIndent("", "  ")
	if err := encoder.Encode(schema); err != nil {
		fmt.Fprintf(os.Stderr, "Error encoding schema: %v\n", err)
		os.Exit(1)
	}
}

func generateSchema() map[string]any {
	schema := map[string]any{
		"$schema":     "http://json-schema.org/draft-07/schema#",
		"title":       "TeamCode Configuration",
		"description": "Configuration schema for the TeamCode application",
		"type":        "object",
		"properties":  map[string]any{},
	}

	// Add Data configuration
	schema["properties"].(map[string]any)["data"] = map[string]any{
		"type":        "object",
		"description": "Storage configuration",
		"properties": map[string]any{
			"directory": map[string]any{
				"type":        "string",
				"description": "Directory where application data is stored",
				"default":     ".teamcode",
			},
		},
		"required": []string{"directory"},
	}

	// Add working directory
	schema["properties"].(map[string]any)["wd"] = map[string]any{
		"type":        "string",
		"description": "Working directory for the application",
	}

	// Add debug flags
	schema["properties"].(map[string]any)["debug"] = map[string]any{
		"type":        "boolean",
		"description": "Enable debug mode",
		"default":     false,
	}

	schema["properties"].(map[string]any)["debugLSP"] = map[string]any{
		"type":        "boolean",
		"description": "Enable LSP debug mode",
		"default":     false,
	}

	schema["properties"].(map[string]any)["contextPaths"] = map[string]any{
		"type":        "array",
		"description": "Context paths for the application",
		"items": map[string]any{
			"type": "string",
		},
		"default": []string{
			".github/copilot-instructions.md",
			".cursorrules",
			".cursor/rules/",
			"CLAUDE.md",
			"CLAUDE.local.md",
			"teamcode.md",
			"teamcode.local.md",
			"TeamCode.md",
			"TeamCode.local.md",
			"opencode.md",
			"opencode.local.md",
			"OpenCode.md",
			"OpenCode.local.md",
			"OPENCODE.md",
			"OPENCODE.local.md",
		},
	}

	schema["properties"].(map[string]any)["tui"] = map[string]any{
		"type":        "object",
		"description": "Terminal User Interface configuration",
		"properties": map[string]any{
			"theme": map[string]any{
				"type":        "string",
				"description": "TUI theme name",
				"default":     "teamcode",
				"enum": []string{
					"teamcode",
					"opencode",
					"catppuccin",
					"dracula",
					"flexoki",
					"gruvbox",
					"monokai",
					"onedark",
					"tokyonight",
					"tron",
				},
			},
		},
	}

	schema["properties"].(map[string]any)["team"] = map[string]any{
		"type":        "object",
		"description": "Collaboration templates and team orchestration defaults",
		"properties": map[string]any{
			"activeTeam": map[string]any{
				"type":        "string",
				"description": "Preferred active team name for the UI and built-in team commands",
			},
			"defaultTemplate": map[string]any{
				"type":        "string",
				"description": "Default team template used by bootstrap flows",
				"default":     "leader-led",
			},
			"defaultBlueprint": map[string]any{
				"type":        "string",
				"description": "Default Agency blueprint used when loading the coding-office constitution",
				"default":     "software-team",
			},
			"collaborationHud": map[string]any{
				"type":        "boolean",
				"description": "Whether to show the collaboration HUD in the sidebar",
				"default":     true,
			},
			"templates": map[string]any{
				"type":        "object",
				"description": "User-defined or overridden collaboration templates",
				"additionalProperties": map[string]any{
					"type": "object",
					"properties": map[string]any{
						"name": map[string]any{
							"type":        "string",
							"description": "Template display name",
						},
						"description": map[string]any{
							"type":        "string",
							"description": "Short summary of the workflow",
						},
						"leadershipMode": map[string]any{
							"type":        "string",
							"description": "Leadership style, for example solo, leader-led, peer, or custom",
						},
						"spawnTeammates": map[string]any{
							"type":        "boolean",
							"description": "Whether bootstrap should spawn teammate sessions immediately",
						},
						"roles": map[string]any{
							"type":        "array",
							"description": "Ordered role definitions for the template",
							"items": map[string]any{
								"type": "object",
								"properties": map[string]any{
									"name": map[string]any{
										"type": "string",
									},
									"responsible": map[string]any{
										"type": "string",
									},
									"currentFocus": map[string]any{
										"type": "string",
									},
									"profile": map[string]any{
										"type": "string",
										"enum": []string{"coder", "task"},
									},
									"prompt": map[string]any{
										"type": "string",
									},
									"reportsTo": map[string]any{
										"type": "string",
									},
									"canSpawnSubagents": map[string]any{
										"type": "boolean",
									},
								},
								"required": []string{"name"},
							},
						},
						"policies": map[string]any{
							"type":        "object",
							"description": "Working agreement and routing defaults",
							"properties": map[string]any{
								"commitMessageFormat": map[string]any{
									"type": "string",
								},
								"maxWip": map[string]any{
									"type":    "integer",
									"minimum": 1,
								},
								"handoffRequires": map[string]any{
									"type": "array",
									"items": map[string]any{
										"type": "string",
									},
								},
								"reviewRequired": map[string]any{
									"type": "boolean",
								},
								"allowsSubagents": map[string]any{
									"type": "boolean",
								},
								"delegationMode": map[string]any{
									"type": "string",
								},
								"localChatDefault": map[string]any{
									"type": "string",
								},
								"reviewRouting": map[string]any{
									"type": "string",
								},
								"synthesisRouting": map[string]any{
									"type": "string",
								},
								"allowsPeerMessaging": map[string]any{
									"type": "boolean",
								},
								"allowsBroadcasts": map[string]any{
									"type": "boolean",
								},
								"workspaceModeDefault": map[string]any{
									"type": "string",
								},
								"loopStrategy": map[string]any{
									"type": "string",
								},
								"concurrencyBudget": map[string]any{
									"type":    "integer",
									"minimum": 1,
								},
								"requiredGates": map[string]any{
									"type": "array",
									"items": map[string]any{
										"type": "string",
									},
								},
							},
						},
					},
				},
			},
			"blueprints": map[string]any{
				"type":        "object",
				"description": "Agency blueprint definitions layered on top of the team template system",
				"additionalProperties": map[string]any{
					"type": "object",
				},
			},
		},
	}

	schema["properties"].(map[string]any)["agency"] = map[string]any{
		"type":        "object",
		"description": "The Agency runtime configuration surface",
		"properties": map[string]any{
			"enabled": map[string]any{
				"type":        "boolean",
				"description": "Whether The Agency runtime should be considered active for product surfaces",
				"default":     false,
			},
			"productName": map[string]any{
				"type":        "string",
				"description": "Public-facing product identity for the runtime",
				"default":     "The Agency",
			},
			"currentConstitution": map[string]any{
				"type":        "string",
				"description": "Currently selected constitution for Agency product paths",
				"default":     "coding-office",
			},
			"soloConstitution": map[string]any{
				"type":        "string",
				"description": "Constitution that preserves the solo TeamCode/OpenCode-derived behavior",
				"default":     "solo",
			},
			"office": map[string]any{
				"type":        "object",
				"description": "Shared office runtime settings",
				"properties": map[string]any{
					"enabled": map[string]any{"type": "boolean", "default": false},
					"mode": map[string]any{
						"type":        "string",
						"description": "Office runtime mode",
						"default":     "distributed-office",
					},
					"autoBoot": map[string]any{"type": "boolean", "default": false},
					"sharedWorkplace": map[string]any{
						"type":        "string",
						"description": "Shared office filesystem root for The Agency runtime",
					},
					"stateFile": map[string]any{
						"type":        "string",
						"description": "Persistent file storing office runtime status across CLI invocations",
					},
					"defaultWorkspaceMode": map[string]any{
						"type":        "string",
						"description": "Default workspace mode for agents in the office",
						"default":     "shared",
					},
					"allowSoloFallback": map[string]any{"type": "boolean", "default": true},
				},
			},
			"docker": map[string]any{
				"type":        "object",
				"description": "Docker topology settings for the shared office runtime",
				"properties": map[string]any{
					"enabled":        map[string]any{"type": "boolean", "default": true},
					"composeProject": map[string]any{"type": "string"},
					"composeFile":    map[string]any{"type": "string"},
					"image":          map[string]any{"type": "string"},
					"sharedVolume":   map[string]any{"type": "string"},
					"network":        map[string]any{"type": "string"},
				},
			},
			"redis": map[string]any{
				"type":        "object",
				"description": "Redis event bus settings for the Agency office",
				"properties": map[string]any{
					"enabled":       map[string]any{"type": "boolean", "default": true},
					"address":       map[string]any{"type": "string", "default": "127.0.0.1:6379"},
					"db":            map[string]any{"type": "integer", "minimum": 0, "default": 7},
					"channelPrefix": map[string]any{"type": "string", "default": "agency"},
				},
			},
			"ledger": map[string]any{
				"type":        "object",
				"description": "Append-only ledger and context projection settings",
				"properties": map[string]any{
					"backend":        map[string]any{"type": "string", "default": "append-only-log"},
					"path":           map[string]any{"type": "string"},
					"snapshotPath":   map[string]any{"type": "string"},
					"consensusMode":  map[string]any{"type": "string", "default": "distributed-consensus"},
					"defaultQuorum":  map[string]any{"type": "integer", "minimum": 1, "default": 2},
					"projectionFile": map[string]any{"type": "string"},
				},
			},
			"schedules": map[string]any{
				"type":        "object",
				"description": "Office-hour and shift defaults for persistent organizations",
				"properties": map[string]any{
					"timezone":            map[string]any{"type": "string", "default": "local"},
					"defaultCadence":      map[string]any{"type": "string", "default": "weekday-office-hours"},
					"wakeOnOfficeOpen":    map[string]any{"type": "boolean", "default": true},
					"requireShiftHandoff": map[string]any{"type": "boolean", "default": true},
					"windows": map[string]any{
						"type": "array",
						"items": map[string]any{
							"type": "object",
							"properties": map[string]any{
								"name":  map[string]any{"type": "string"},
								"days":  map[string]any{"type": "array", "items": map[string]any{"type": "string"}},
								"start": map[string]any{"type": "string"},
								"end":   map[string]any{"type": "string"},
							},
						},
					},
				},
			},
			"genesis": map[string]any{
				"type":        "object",
				"description": "Genesis defaults for intent-driven organization creation",
				"properties": map[string]any{
					"conversationDriven": map[string]any{"type": "boolean", "default": true},
					"autoResearch":       map[string]any{"type": "boolean", "default": true},
					"autoSkills":         map[string]any{"type": "boolean", "default": true},
					"autoToolBinding":    map[string]any{"type": "boolean", "default": true},
					"sequentialSpawn":    map[string]any{"type": "boolean", "default": true},
					"defaultTopology":    map[string]any{"type": "string", "default": "hierarchical"},
				},
			},
			"constitutions": map[string]any{
				"type":        "object",
				"description": "Named constitutions that unify solo TeamCode behavior and Agency office behavior",
				"additionalProperties": map[string]any{
					"type": "object",
					"properties": map[string]any{
						"name":            map[string]any{"type": "string"},
						"description":     map[string]any{"type": "string"},
						"blueprint":       map[string]any{"type": "string"},
						"teamTemplate":    map[string]any{"type": "string"},
						"governance":      map[string]any{"type": "string"},
						"runtimeMode":     map[string]any{"type": "string"},
						"entryMode":       map[string]any{"type": "string"},
						"defaultSchedule": map[string]any{"type": "string"},
						"policies": map[string]any{
							"type": "object",
							"properties": map[string]any{
								"wakeMode":          map[string]any{"type": "string"},
								"consensusMode":     map[string]any{"type": "string"},
								"publicationPolicy": map[string]any{"type": "string"},
								"spawnMode":         map[string]any{"type": "string"},
								"defaultQuorum":     map[string]any{"type": "integer", "minimum": 1},
							},
						},
					},
				},
			},
		},
	}

	// Add MCP servers
	schema["properties"].(map[string]any)["mcpServers"] = map[string]any{
		"type":        "object",
		"description": "Model Control Protocol server configurations",
		"additionalProperties": map[string]any{
			"type":        "object",
			"description": "MCP server configuration",
			"properties": map[string]any{
				"command": map[string]any{
					"type":        "string",
					"description": "Command to execute for the MCP server",
				},
				"env": map[string]any{
					"type":        "array",
					"description": "Environment variables for the MCP server",
					"items": map[string]any{
						"type": "string",
					},
				},
				"args": map[string]any{
					"type":        "array",
					"description": "Command arguments for the MCP server",
					"items": map[string]any{
						"type": "string",
					},
				},
				"type": map[string]any{
					"type":        "string",
					"description": "Type of MCP server",
					"enum":        []string{"stdio", "sse"},
					"default":     "stdio",
				},
				"url": map[string]any{
					"type":        "string",
					"description": "URL for SSE type MCP servers",
				},
				"headers": map[string]any{
					"type":        "object",
					"description": "HTTP headers for SSE type MCP servers",
					"additionalProperties": map[string]any{
						"type": "string",
					},
				},
			},
			"required": []string{"command"},
		},
	}

	// Add providers
	providerSchema := map[string]any{
		"type":        "object",
		"description": "LLM provider configurations",
		"additionalProperties": map[string]any{
			"type":        "object",
			"description": "Provider configuration",
			"properties": map[string]any{
				"apiKey": map[string]any{
					"type":        "string",
					"description": "API key for the provider",
				},
				"disabled": map[string]any{
					"type":        "boolean",
					"description": "Whether the provider is disabled",
					"default":     false,
				},
			},
		},
	}

	// Add known providers
	knownProviders := []string{
		string(models.ProviderAnthropic),
		string(models.ProviderOpenAI),
		string(models.ProviderGemini),
		string(models.ProviderGROQ),
		string(models.ProviderOpenRouter),
		string(models.ProviderBedrock),
		string(models.ProviderAzure),
		string(models.ProviderVertexAI),
	}

	providerSchema["additionalProperties"].(map[string]any)["properties"].(map[string]any)["provider"] = map[string]any{
		"type":        "string",
		"description": "Provider type",
		"enum":        knownProviders,
	}

	schema["properties"].(map[string]any)["providers"] = providerSchema

	// Add agents
	agentSchema := map[string]any{
		"type":        "object",
		"description": "Agent configurations",
		"additionalProperties": map[string]any{
			"type":        "object",
			"description": "Agent configuration",
			"properties": map[string]any{
				"model": map[string]any{
					"type":        "string",
					"description": "Model ID for the agent",
				},
				"maxTokens": map[string]any{
					"type":        "integer",
					"description": "Maximum tokens for the agent",
					"minimum":     1,
				},
				"reasoningEffort": map[string]any{
					"type":        "string",
					"description": "Reasoning effort for models that support it (OpenAI, Anthropic)",
					"enum":        []string{"low", "medium", "high"},
				},
			},
			"required": []string{"model"},
		},
	}

	// Add model enum
	modelEnum := []string{}
	for modelID := range models.SupportedModels {
		modelEnum = append(modelEnum, string(modelID))
	}
	agentSchema["additionalProperties"].(map[string]any)["properties"].(map[string]any)["model"].(map[string]any)["enum"] = modelEnum

	// Add specific agent properties
	agentProperties := map[string]any{}
	knownAgents := []string{
		string(config.AgentCoder),
		string(config.AgentTask),
		string(config.AgentTitle),
	}

	for _, agentName := range knownAgents {
		agentProperties[agentName] = map[string]any{
			"$ref": "#/definitions/agent",
		}
	}

	// Create a combined schema that allows both specific agents and additional ones
	combinedAgentSchema := map[string]any{
		"type":                 "object",
		"description":          "Agent configurations",
		"properties":           agentProperties,
		"additionalProperties": agentSchema["additionalProperties"],
	}

	schema["properties"].(map[string]any)["agents"] = combinedAgentSchema
	schema["definitions"] = map[string]any{
		"agent": agentSchema["additionalProperties"],
	}

	// Add LSP configuration
	schema["properties"].(map[string]any)["lsp"] = map[string]any{
		"type":        "object",
		"description": "Language Server Protocol configurations",
		"additionalProperties": map[string]any{
			"type":        "object",
			"description": "LSP configuration for a language",
			"properties": map[string]any{
				"disabled": map[string]any{
					"type":        "boolean",
					"description": "Whether the LSP is disabled",
					"default":     false,
				},
				"command": map[string]any{
					"type":        "string",
					"description": "Command to execute for the LSP server",
				},
				"args": map[string]any{
					"type":        "array",
					"description": "Command arguments for the LSP server",
					"items": map[string]any{
						"type": "string",
					},
				},
				"options": map[string]any{
					"type":        "object",
					"description": "Additional options for the LSP server",
				},
			},
			"required": []string{"command"},
		},
	}

	return schema
}
