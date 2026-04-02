package cmd

import (
	"context"
	"database/sql"
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
	"sync"
	"time"

	agencyrt "github.com/ETEllis/teamcode/internal/agency"
	"github.com/ETEllis/teamcode/internal/app"
	"github.com/ETEllis/teamcode/internal/config"
	"github.com/ETEllis/teamcode/internal/db"
	"github.com/ETEllis/teamcode/internal/logging"
	"github.com/spf13/cobra"
)

type commandRuntime struct {
	ctx       context.Context
	cancel    context.CancelFunc
	conn      *sql.DB
	app       *app.App
	closeOnce sync.Once
}

type agencyCommandRuntime struct {
	cfg    *config.Config
	agency *app.AgencyService
	voice  *agencyrt.VoiceGateway
}

func resolveRuntimeFlags(cmd *cobra.Command) (bool, string, error) {
	debug, _ := cmd.Flags().GetBool("debug")
	cwd, _ := cmd.Flags().GetString("cwd")

	if cwd != "" {
		if err := os.Chdir(cwd); err != nil {
			return false, "", fmt.Errorf("failed to change directory: %w", err)
		}
	}
	if cwd == "" {
		currentDir, err := os.Getwd()
		if err != nil {
			return false, "", fmt.Errorf("failed to get current working directory: %w", err)
		}
		cwd = currentDir
	}
	return debug, cwd, nil
}

func bootstrapRuntime(cmd *cobra.Command) (*commandRuntime, error) {
	debug, cwd, err := resolveRuntimeFlags(cmd)
	if err != nil {
		return nil, err
	}

	if _, err := config.Load(cwd, debug); err != nil {
		return nil, err
	}

	conn, err := db.Connect()
	if err != nil {
		return nil, err
	}

	ctx, cancel := context.WithCancel(context.Background())
	application, err := app.New(ctx, conn)
	if err != nil {
		cancel()
		_ = conn.Close()
		logging.Error("Failed to create app: %v", err)
		return nil, err
	}

	initMCPTools(ctx, application)

	return &commandRuntime{
		ctx:    ctx,
		cancel: cancel,
		conn:   conn,
		app:    application,
	}, nil
}

func bootstrapAgencyRuntime(cmd *cobra.Command) (*agencyCommandRuntime, error) {
	debug, cwd, err := resolveRuntimeFlags(cmd)
	if err != nil {
		return nil, err
	}
	cfg, err := config.Load(cwd, debug)
	if err != nil {
		return nil, err
	}
	voice, err := bootstrapVoiceGateway(cfg)
	if err != nil {
		return nil, err
	}
	return &agencyCommandRuntime{
		cfg:    cfg,
		agency: app.NewAgencyService(cfg),
		voice:  voice,
	}, nil
}

func bootstrapVoiceGateway(cfg *config.Config) (*agencyrt.VoiceGateway, error) {
	if cfg == nil {
		return nil, fmt.Errorf("config not loaded")
	}

	baseDir := filepath.Dir(cfg.Agency.Office.StateFile)
	if cfg.Agency.Ledger.Path != "" {
		baseDir = filepath.Dir(cfg.Agency.Ledger.Path)
	}
	ledgerDir := filepath.Join(baseDir, "ledger")
	ledger, err := agencyrt.NewLedgerService(ledgerDir)
	if err != nil {
		return nil, err
	}

	var bus agencyrt.EventBus = agencyrt.NewMemoryEventBus()
	if cfg.Agency.Redis.Address != "" {
		bus = agencyrt.NewRedisEventBus(agencyrt.RedisConfig{
			Addr: cfg.Agency.Redis.Address,
			DB:   cfg.Agency.Redis.DB,
		})
	}

	return agencyrt.NewVoiceGateway(agencyrt.VoiceGatewayConfig{
		Enabled:              boolValue(cfg.Agency.Voice.Enabled),
		Provider:             cfg.Agency.Voice.Provider,
		StatePath:            cfg.Agency.Voice.GatewayState,
		AssetDir:             cfg.Agency.Voice.AssetDir,
		ControlChannel:       cfg.Agency.Voice.ControlChannel,
		SynthesisChannel:     cfg.Agency.Voice.SynthesisChannel,
		MeetingTranscriptDir: cfg.Agency.Voice.MeetingTranscriptDir,
		DefaultRoom:          cfg.Agency.Voice.Projection.DefaultRoom,
		Projection: agencyrt.VoiceProjectionDefaults{
			TranscriptProjection:  boolValue(cfg.Agency.Voice.Projection.TranscriptProjection),
			AudioProjection:       boolValue(cfg.Agency.Voice.Projection.AudioProjection),
			AutoProjectTranscript: boolValue(cfg.Agency.Voice.Projection.AutoProjectTranscript),
			AutoProjectSynthesis:  boolValue(cfg.Agency.Voice.Projection.AutoProjectSynthesis),
		},
		STT: agencyrt.SpeechRuntimeConfig{
			Enabled:     boolValue(cfg.Agency.Voice.STT.Enabled),
			Command:     cfg.Agency.Voice.STT.Command,
			Args:        append([]string(nil), cfg.Agency.Voice.STT.Args...),
			InputMode:   cfg.Agency.Voice.STT.InputMode,
			OutputMode:  cfg.Agency.Voice.STT.OutputMode,
			Language:    cfg.Agency.Voice.STT.Language,
			Voice:       cfg.Agency.Voice.STT.Voice,
			AudioFormat: cfg.Agency.Voice.STT.AudioFormat,
			Timeout:     parseAgencyDuration(cfg.Agency.Voice.STT.Timeout),
		},
		TTS: agencyrt.SpeechRuntimeConfig{
			Enabled:     boolValue(cfg.Agency.Voice.TTS.Enabled),
			Command:     cfg.Agency.Voice.TTS.Command,
			Args:        append([]string(nil), cfg.Agency.Voice.TTS.Args...),
			InputMode:   cfg.Agency.Voice.TTS.InputMode,
			OutputMode:  cfg.Agency.Voice.TTS.OutputMode,
			Language:    cfg.Agency.Voice.TTS.Language,
			Voice:       cfg.Agency.Voice.TTS.Voice,
			AudioFormat: cfg.Agency.Voice.TTS.AudioFormat,
			Timeout:     parseAgencyDuration(cfg.Agency.Voice.TTS.Timeout),
		},
	}, ledger, bus), nil
}

func parseAgencyDuration(value string) time.Duration {
	if value == "" {
		return 0
	}
	d, err := time.ParseDuration(value)
	if err != nil {
		return 0
	}
	return d
}

func boolValue(value *bool) bool {
	return value != nil && *value
}

func (r *commandRuntime) Close() {
	if r == nil {
		return
	}

	r.closeOnce.Do(func() {
		if r.app != nil {
			r.app.Shutdown()
		}
		if r.cancel != nil {
			r.cancel()
		}
		if r.conn != nil {
			_ = r.conn.Close()
		}
	})
}

func outputJSON(cmd *cobra.Command, value any) (bool, error) {
	asJSON, _ := cmd.Flags().GetBool("json")
	if !asJSON {
		return false, nil
	}

	data, err := json.MarshalIndent(value, "", "  ")
	if err != nil {
		return false, fmt.Errorf("marshal json output: %w", err)
	}
	fmt.Println(string(data))
	return true, nil
}

func addJSONFlag(cmd *cobra.Command) {
	cmd.Flags().Bool("json", false, "Render command output as JSON")
}
