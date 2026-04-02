package app

import (
	"context"
	"database/sql"
	"errors"
	"fmt"
	"maps"
	"sync"
	"time"

	"github.com/ETEllis/teamcode/internal/config"
	"github.com/ETEllis/teamcode/internal/db"
	"github.com/ETEllis/teamcode/internal/format"
	"github.com/ETEllis/teamcode/internal/history"
	"github.com/ETEllis/teamcode/internal/llm/agent"
	"github.com/ETEllis/teamcode/internal/logging"
	"github.com/ETEllis/teamcode/internal/lsp"
	"github.com/ETEllis/teamcode/internal/message"
	"github.com/ETEllis/teamcode/internal/orchestration"
	"github.com/ETEllis/teamcode/internal/permission"
	"github.com/ETEllis/teamcode/internal/session"
	"github.com/ETEllis/teamcode/internal/team"
	"github.com/ETEllis/teamcode/internal/tui/theme"
)

type App struct {
	Sessions    session.Service
	Messages    message.Service
	History     history.Service
	Permissions permission.Service
	Team        *team.Service
	Workers     *orchestration.Manager
	Agency      *AgencyService

	CoderAgent agent.Service

	LSPClients map[string]*lsp.Client

	clientsMutex sync.RWMutex

	watcherCancelFuncs []context.CancelFunc
	cancelFuncsMutex   sync.Mutex
	watcherWG          sync.WaitGroup
}

func New(ctx context.Context, conn *sql.DB) (*App, error) {
	q := db.New(conn)
	sessions := session.NewService(q)
	messages := message.NewService(q)
	files := history.NewService(q, conn)

	app := &App{
		Sessions:    sessions,
		Messages:    messages,
		History:     files,
		Permissions: permission.NewPermissionService(),
		Team:        team.NewService(),
		Workers:     orchestration.NewManager(sessions),
		LSPClients:  make(map[string]*lsp.Client),
	}

	if cfg := config.Get(); cfg != nil {
		app.Agency = NewAgencyService(cfg)
		if err := app.Agency.MaybeBootOnStartup(ctx); err != nil {
			logging.Warn("Failed to auto-boot Agency office", "error", err)
		}
	}

	// Initialize theme based on configuration
	app.initTheme()

	// Initialize LSP clients in the background
	go app.initLSPClients(ctx)

	// Try to create the coder agent, but don't fail if it can't be created
	// This allows the TUI to start even without API keys configured
	var err error
	app.CoderAgent, err = agent.NewAgent(
		config.AgentCoder,
		app.Sessions,
		app.Messages,
		agent.CoderAgentTools(
			app.Permissions,
			app.Sessions,
			app.Messages,
			app.History,
			app.LSPClients,
			app.Team,
			app.Workers,
		),
	)
	if err != nil {
		logging.Warn("Failed to create coder agent - API keys may not be configured", "error", err)
		// Don't return error - allow the app to start so TUI can show setup wizard
		// The coder agent will be nil, and the TUI should handle this case
	}

	app.Workers.SetRunner(func(ctx context.Context, sessionID string, request orchestration.RunRequest) (<-chan orchestration.RunResult, error) {
		agentName := config.AgentCoder
		toolset := agent.CoderAgentTools(
			app.Permissions,
			app.Sessions,
			app.Messages,
			app.History,
			app.LSPClients,
			app.Team,
			app.Workers,
		)
		if request.Profile == "task" {
			agentName = config.AgentTask
			toolset = agent.TaskAgentTools(app.LSPClients)
		}

		childAgent, err := agent.NewAgent(agentName, app.Sessions, app.Messages, toolset)
		if err != nil {
			return nil, err
		}
		done, err := childAgent.Run(ctx, sessionID, request.Prompt)
		if err != nil {
			return nil, err
		}

		out := make(chan orchestration.RunResult, 1)
		go func() {
			defer close(out)
			result := <-done
			runResult := orchestration.RunResult{}
			if result.Error != nil {
				runResult.Error = result.Error
			}
			runResult.Content = result.Message.Content().String()
			out <- runResult
		}()
		return out, nil
	})

	return app, nil
}

// initTheme sets the application theme based on the configuration
func (app *App) initTheme() {
	cfg := config.Get()
	if cfg == nil || cfg.TUI.Theme == "" {
		return // Use default theme
	}

	// Try to set the theme from config
	err := theme.SetTheme(cfg.TUI.Theme)
	if err != nil {
		logging.Warn("Failed to set theme from config, using default theme", "theme", cfg.TUI.Theme, "error", err)
	} else {
		logging.Debug("Set theme from config", "theme", cfg.TUI.Theme)
	}
}

// RunNonInteractive handles the execution flow when a prompt is provided via CLI flag.
func (a *App) RunNonInteractive(ctx context.Context, prompt string, outputFormat string, quiet bool) error {
	logging.Info("Running in non-interactive mode")

	// Start spinner if not in quiet mode
	var spinner *format.Spinner
	if !quiet {
		spinner = format.NewSpinner("Thinking...")
		spinner.Start()
		defer spinner.Stop()
	}

	const maxPromptLengthForTitle = 100
	titlePrefix := "Non-interactive: "
	var titleSuffix string

	if len(prompt) > maxPromptLengthForTitle {
		titleSuffix = prompt[:maxPromptLengthForTitle] + "..."
	} else {
		titleSuffix = prompt
	}
	title := titlePrefix + titleSuffix

	sess, err := a.Sessions.Create(ctx, title)
	if err != nil {
		return fmt.Errorf("failed to create session for non-interactive mode: %w", err)
	}
	logging.Info("Created session for non-interactive run", "session_id", sess.ID)

	// Automatically approve all permission requests for this non-interactive session
	a.Permissions.AutoApproveSession(sess.ID)

	done, err := a.CoderAgent.Run(ctx, sess.ID, prompt)
	if err != nil {
		return fmt.Errorf("failed to start agent processing stream: %w", err)
	}

	result := <-done
	if result.Error != nil {
		if errors.Is(result.Error, context.Canceled) || errors.Is(result.Error, agent.ErrRequestCancelled) {
			logging.Info("Agent processing cancelled", "session_id", sess.ID)
			return nil
		}
		return fmt.Errorf("agent processing failed: %w", result.Error)
	}

	// Stop spinner before printing output
	if !quiet && spinner != nil {
		spinner.Stop()
	}

	// Get the text content from the response
	content := "No content available"
	if result.Message.Content().String() != "" {
		content = result.Message.Content().String()
	}

	fmt.Println(format.FormatOutput(content, outputFormat))

	logging.Info("Non-interactive run completed", "session_id", sess.ID)

	return nil
}

// Shutdown performs a clean shutdown of the application
func (app *App) Shutdown() {
	// Cancel all watcher goroutines
	app.cancelFuncsMutex.Lock()
	for _, cancel := range app.watcherCancelFuncs {
		cancel()
	}
	app.cancelFuncsMutex.Unlock()
	app.watcherWG.Wait()

	// Perform additional cleanup for LSP clients
	app.clientsMutex.RLock()
	clients := make(map[string]*lsp.Client, len(app.LSPClients))
	maps.Copy(clients, app.LSPClients)
	app.clientsMutex.RUnlock()

	for name, client := range clients {
		shutdownCtx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
		if err := client.Shutdown(shutdownCtx); err != nil {
			logging.Error("Failed to shutdown LSP client", "name", name, "error", err)
		}
		cancel()
	}
}
