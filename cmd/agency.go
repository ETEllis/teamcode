package cmd

import (
	"fmt"
	"strings"

	agencyrt "github.com/ETEllis/teamcode/internal/agency"
	"github.com/ETEllis/teamcode/internal/app"
	"github.com/ETEllis/teamcode/internal/config"
	"github.com/spf13/cobra"
)

func newAgencyCmd() *cobra.Command {
	cmd := &cobra.Command{
		Use:   "agency",
		Short: "Inspect and manage The Agency organizational runtime",
		Long:  "The Agency commands expose constitutions, organizational state, and office lifecycle without replacing the preserved solo coding flow.",
	}

	cmd.AddCommand(
		newAgencyStatusCmd(),
		newAgencyConstitutionsCmd(),
		newAgencyConstitutionCmd(),
		newAgencySwitchCmd(),
		newAgencyVoiceCmd(),
	)

	return cmd
}

func newOfficeCmd() *cobra.Command {
	cmd := &cobra.Command{
		Use:   "office",
		Short: "Boot, inspect, and stop the shared Agency office",
		Long:  "Office commands are the runtime-facing product surface for the shared Agency environment.",
	}

	cmd.AddCommand(
		newOfficeBootCmd(),
		newOfficeStatusCmd(),
		newOfficeServicesCmd(),
		newOfficeStopCmd(),
		newOfficeGenesisCmd(),
		newOfficeConstitutionsCmd(),
		newOfficeConstitutionCmd(),
		newOfficeSchedulesCmd(),
		newOfficeVoiceCmd(),
	)

	return cmd
}

func newAgencyStatusCmd() *cobra.Command {
	cmd := &cobra.Command{
		Use:   "status",
		Short: "Inspect the active Agency organization",
		RunE: func(cmd *cobra.Command, args []string) error {
			runtime, err := bootstrapAgencyRuntime(cmd)
			if err != nil {
				return err
			}
			view, err := runtime.agency.InspectOrganization()
			if err != nil {
				return err
			}
			if rendered, err := outputJSON(cmd, view); err != nil {
				return err
			} else if rendered {
				return nil
			}

			roleNames := roleNames(view.Roles)
			fmt.Printf("Product: %s\n", view.ProductName)
			fmt.Printf("Current constitution: %s\n", view.CurrentConstitution)
			fmt.Printf("Solo constitution: %s\n", view.SoloConstitution)
			fmt.Printf("Blueprint: %s\n", view.Blueprint)
			fmt.Printf("Governance: %s\n", view.Governance)
			fmt.Printf("Workspace mode: %s\n", view.WorkspaceMode)
			fmt.Printf("Loop strategy: %s\n", view.LoopStrategy)
			fmt.Printf("Roles: %s\n", strings.Join(roleNames, ", "))
			if len(view.RequiredGates) > 0 {
				fmt.Printf("Required gates: %s\n", strings.Join(view.RequiredGates, ", "))
			}
			return nil
		},
	}
	addJSONFlag(cmd)
	return cmd
}

func newAgencyConstitutionsCmd() *cobra.Command {
	cmd := &cobra.Command{
		Use:   "constitutions",
		Short: "List available Agency constitutions",
		RunE: func(cmd *cobra.Command, args []string) error {
			runtime, err := bootstrapAgencyRuntime(cmd)
			if err != nil {
				return err
			}
			raw := runtime.agency.ListConstitutions()
			views := make([]app.AgencyConstitutionView, 0, len(raw))
			for _, constitution := range raw {
				view, err := runtime.agency.InspectConstitution(constitution.Name)
				if err != nil {
					return err
				}
				views = append(views, view)
			}
			if rendered, err := outputJSON(cmd, views); err != nil {
				return err
			} else if rendered {
				return nil
			}

			for _, view := range views {
				markers := constitutionMarkers(view)
				suffix := ""
				if len(markers) > 0 {
					suffix = " [" + strings.Join(markers, ", ") + "]"
				}
				fmt.Printf("- %s%s\n", view.Name, suffix)
				fmt.Printf("  %s\n", view.Description)
				fmt.Printf("  blueprint=%s governance=%s workspace=%s loop=%s\n",
					view.Blueprint, view.Constitution.Governance, view.WorkspaceMode, view.LoopStrategy)
			}
			return nil
		},
	}
	addJSONFlag(cmd)
	return cmd
}

func newAgencyConstitutionCmd() *cobra.Command {
	cmd := &cobra.Command{
		Use:   "constitution [name]",
		Short: "Inspect one Agency constitution in detail",
		Args:  cobra.MaximumNArgs(1),
		RunE: func(cmd *cobra.Command, args []string) error {
			runtime, err := bootstrapAgencyRuntime(cmd)
			if err != nil {
				return err
			}
			name := ""
			if len(args) > 0 {
				name = args[0]
			} else {
				org, err := runtime.agency.InspectOrganization()
				if err != nil {
					return err
				}
				name = org.CurrentConstitution
			}

			view, err := runtime.agency.InspectConstitution(name)
			if err != nil {
				return err
			}
			if rendered, err := outputJSON(cmd, view); err != nil {
				return err
			} else if rendered {
				return nil
			}

			fmt.Printf("Constitution: %s\n", view.Name)
			fmt.Printf("Description: %s\n", view.Description)
			fmt.Printf("Blueprint: %s\n", view.Blueprint)
			fmt.Printf("Governance: %s\n", view.Constitution.Governance)
			fmt.Printf("Runtime mode: %s\n", view.Constitution.RuntimeMode)
			fmt.Printf("Workspace mode: %s\n", view.WorkspaceMode)
			fmt.Printf("Loop strategy: %s\n", view.LoopStrategy)
			fmt.Printf("Roles: %s\n", strings.Join(roleNames(view.Roles), ", "))
			if len(view.RequiredGates) > 0 {
				fmt.Printf("Required gates: %s\n", strings.Join(view.RequiredGates, ", "))
			}
			return nil
		},
	}
	addJSONFlag(cmd)
	return cmd
}

func newAgencySwitchCmd() *cobra.Command {
	cmd := &cobra.Command{
		Use:   "switch <constitution>",
		Short: "Switch the active Agency constitution",
		Args:  cobra.ExactArgs(1),
		RunE: func(cmd *cobra.Command, args []string) error {
			runtime, err := bootstrapAgencyRuntime(cmd)
			if err != nil {
				return err
			}
			constitution, err := runtime.agency.SwitchConstitution(args[0])
			if err != nil {
				return err
			}
			if rendered, err := outputJSON(cmd, constitution); err != nil {
				return err
			} else if rendered {
				return nil
			}

			fmt.Printf("Active constitution switched to %s\n", constitution.Name)
			fmt.Printf("Blueprint: %s\n", constitution.Blueprint)
			fmt.Printf("Governance: %s\n", constitution.Governance)
			return nil
		},
	}
	addJSONFlag(cmd)
	return cmd
}

func newOfficeBootCmd() *cobra.Command {
	var constitution string

	cmd := &cobra.Command{
		Use:   "boot",
		Short: "Boot the Agency office runtime",
		RunE: func(cmd *cobra.Command, args []string) error {
			runtime, err := bootstrapAgencyRuntime(cmd)
			if err != nil {
				return err
			}
			office, err := runtime.agency.BootOffice(cmd.Context(), constitution)
			if err != nil {
				return err
			}
			if rendered, err := outputJSON(cmd, office); err != nil {
				return err
			} else if rendered {
				return nil
			}

			fmt.Printf("Office booted: %s\n", office.ProductName)
			fmt.Printf("Constitution: %s\n", office.Constitution)
			fmt.Printf("Mode: %s\n", office.Mode)
			fmt.Printf("Shared workplace: %s\n", office.SharedWorkplace)
			fmt.Printf("Ledger path: %s\n", office.LedgerPath)
			fmt.Printf("Redis address: %s\n", office.RedisAddress)
			return nil
		},
	}
	cmd.Flags().StringVar(&constitution, "constitution", "", "Boot the office with a specific constitution")
	addJSONFlag(cmd)
	return cmd
}

func newOfficeStatusCmd() *cobra.Command {
	cmd := &cobra.Command{
		Use:   "status",
		Short: "Inspect office, organization, and schedule state together",
		RunE: func(cmd *cobra.Command, args []string) error {
			runtime, err := bootstrapAgencyRuntime(cmd)
			if err != nil {
				return err
			}
			office, err := runtime.agency.InspectOffice()
			if err != nil {
				return err
			}
			org, err := runtime.agency.InspectOrganization()
			if err != nil {
				return err
			}
			schedules := runtime.agency.InspectSchedules()
			services := runtime.agency.InspectRuntimeServices()

			payload := map[string]any{
				"office":       office,
				"organization": org,
				"schedules":    schedules,
				"services":     services,
			}
			if rendered, err := outputJSON(cmd, payload); err != nil {
				return err
			} else if rendered {
				return nil
			}

			fmt.Printf("Office running: %t\n", office.Running)
			fmt.Printf("Constitution: %s\n", org.CurrentConstitution)
			fmt.Printf("Blueprint: %s\n", org.Blueprint)
			fmt.Printf("Governance: %s\n", org.Governance)
			fmt.Printf("Shared workplace: %s\n", office.SharedWorkplace)
			fmt.Printf("Ledger path: %s\n", office.LedgerPath)
			fmt.Printf("Redis address: %s\n", office.RedisAddress)
			fmt.Printf("Default cadence: %s\n", schedules.DefaultCadence)
			fmt.Printf("Docker: enabled=%t compose=%s image=%s\n", services.Docker.Enabled, services.Docker.ComposeFile, services.Docker.Image)
			fmt.Printf("Redis: enabled=%t addr=%s db=%d\n", services.Redis.Enabled, services.Redis.Address, services.Redis.DB)
			fmt.Printf("Ledger: backend=%s quorum=%d projection=%s\n", services.Ledger.Backend, services.Ledger.DefaultQuorum, services.Ledger.ProjectionFile)
			return nil
		},
	}
	addJSONFlag(cmd)
	return cmd
}

func newOfficeServicesCmd() *cobra.Command {
	cmd := &cobra.Command{
		Use:   "services",
		Short: "Inspect configured Docker, Redis, and ledger services for the office",
		RunE: func(cmd *cobra.Command, args []string) error {
			runtime, err := bootstrapAgencyRuntime(cmd)
			if err != nil {
				return err
			}
			services := runtime.agency.InspectRuntimeServices()
			if rendered, err := outputJSON(cmd, services); err != nil {
				return err
			} else if rendered {
				return nil
			}

			fmt.Printf("Docker: enabled=%t composeProject=%s composeFile=%s image=%s volume=%s network=%s\n",
				services.Docker.Enabled, services.Docker.ComposeProject, services.Docker.ComposeFile, services.Docker.Image, services.Docker.SharedVolume, services.Docker.Network)
			fmt.Printf("Redis: enabled=%t address=%s db=%d prefix=%s\n",
				services.Redis.Enabled, services.Redis.Address, services.Redis.DB, services.Redis.ChannelPrefix)
			fmt.Printf("Ledger: backend=%s path=%s snapshots=%s consensus=%s quorum=%d projection=%s\n",
				services.Ledger.Backend, services.Ledger.Path, services.Ledger.SnapshotPath, services.Ledger.ConsensusMode, services.Ledger.DefaultQuorum, services.Ledger.ProjectionFile)
			return nil
		},
	}
	addJSONFlag(cmd)
	return cmd
}

func newOfficeStopCmd() *cobra.Command {
	cmd := &cobra.Command{
		Use:   "stop",
		Short: "Stop the Agency office runtime",
		RunE: func(cmd *cobra.Command, args []string) error {
			runtime, err := bootstrapAgencyRuntime(cmd)
			if err != nil {
				return err
			}
			office, err := runtime.agency.StopOffice()
			if err != nil {
				return err
			}
			if rendered, err := outputJSON(cmd, office); err != nil {
				return err
			} else if rendered {
				return nil
			}

			fmt.Printf("Office stopped: %s\n", office.ProductName)
			fmt.Printf("Last event: %s\n", office.LastEvent)
			return nil
		},
	}
	addJSONFlag(cmd)
	return cmd
}

func newOfficeGenesisCmd() *cobra.Command {
	var req app.AgencyGenesisRequest

	cmd := &cobra.Command{
		Use:   "genesis [intent]",
		Short: "Start Agency genesis from a natural-language mission",
		RunE: func(cmd *cobra.Command, args []string) error {
			runtime, err := bootstrapAgencyRuntime(cmd)
			if err != nil {
				return err
			}
			if strings.TrimSpace(req.Intent) == "" && len(args) > 0 {
				req.Intent = strings.Join(args, " ")
			}
			if strings.TrimSpace(req.Intent) == "" {
				return fmt.Errorf("genesis intent is required")
			}

			if req.Constitution != "" {
				if _, err := runtime.agency.SwitchConstitution(req.Constitution); err != nil {
					return err
				}
			}
			if _, err := runtime.agency.BootOffice(cmd.Context(), req.Constitution); err != nil {
				return err
			}

			result, err := runtime.agency.StartGenesis(req)
			if err != nil {
				return err
			}
			if rendered, err := outputJSON(cmd, result); err != nil {
				return err
			} else if rendered {
				return nil
			}

			fmt.Printf("Genesis started for constitution %s\n", result.ConstitutionName)
			fmt.Println(result.Summary)
			if len(result.Roles) > 0 {
				fmt.Printf("Roles: %s\n", strings.Join(roleNames(result.Roles), ", "))
			}
			return nil
		},
	}

	cmd.Flags().StringVar(&req.Intent, "intent", "", "Office mission or objective")
	cmd.Flags().StringVar(&req.Domain, "domain", "", "Primary work domain")
	cmd.Flags().StringVar(&req.TimeHorizon, "time-horizon", "", "Time horizon for the office")
	cmd.Flags().StringVar(&req.WorkingStyle, "working-style", "", "Preferred working style")
	cmd.Flags().StringVar(&req.Governance, "governance", "", "Governance preference")
	cmd.Flags().StringVar(&req.GoalShape, "goal-shape", "", "Goal shape or work pattern")
	cmd.Flags().StringVar(&req.Constitution, "constitution", "", "Agency constitution to use")
	cmd.Flags().StringSliceVar(&req.Roles, "role", nil, "Explicit role name to include (repeatable)")
	addJSONFlag(cmd)
	return cmd
}

func newOfficeConstitutionsCmd() *cobra.Command {
	cmd := newAgencyConstitutionsCmd()
	cmd.Use = "constitutions"
	cmd.Short = "List constitutions available to the office runtime"
	return cmd
}

func newOfficeConstitutionCmd() *cobra.Command {
	cmd := newAgencyConstitutionCmd()
	cmd.Use = "constitution [name]"
	cmd.Short = "Inspect a constitution from the office runtime surface"
	return cmd
}

func newOfficeSchedulesCmd() *cobra.Command {
	cmd := &cobra.Command{
		Use:   "schedules",
		Short: "Inspect default office schedules and shift-handoff policy",
		RunE: func(cmd *cobra.Command, args []string) error {
			runtime, err := bootstrapAgencyRuntime(cmd)
			if err != nil {
				return err
			}
			schedules := runtime.agency.InspectSchedules()
			if rendered, err := outputJSON(cmd, schedules); err != nil {
				return err
			} else if rendered {
				return nil
			}

			fmt.Printf("Timezone: %s\n", schedules.Timezone)
			fmt.Printf("Default cadence: %s\n", schedules.DefaultCadence)
			fmt.Printf("Wake on office open: %t\n", schedules.WakeOnOfficeOpen)
			fmt.Printf("Require shift handoff: %t\n", schedules.RequireShiftHandoff)
			for _, window := range schedules.Windows {
				fmt.Printf("- %s: %s %s-%s\n", window.Name, strings.Join(window.Days, ","), window.Start, window.End)
			}
			return nil
		},
	}
	addJSONFlag(cmd)
	return cmd
}

func newAgencyVoiceCmd() *cobra.Command {
	cmd := &cobra.Command{
		Use:   "voice",
		Short: "Inspect and control the Agency voice gateway",
		Long:  "Voice commands manage ledger-backed transcript truth, synthesis, and room projection state for the Agency runtime.",
	}

	cmd.AddCommand(
		newAgencyVoiceStatusCmd(),
		newAgencyVoiceProjectCmd(),
		newAgencyVoiceTranscribeCmd(),
		newAgencyVoiceSpeakCmd(),
		newAgencyVoiceServeCmd(),
	)

	return cmd
}

func newOfficeVoiceCmd() *cobra.Command {
	cmd := newAgencyVoiceCmd()
	cmd.Use = "voice"
	cmd.Short = "Operate the office voice gateway"
	return cmd
}

func newAgencyVoiceStatusCmd() *cobra.Command {
	var organizationID string

	cmd := &cobra.Command{
		Use:   "status",
		Short: "Inspect voice gateway configuration and projected room state",
		RunE: func(cmd *cobra.Command, args []string) error {
			runtime, err := bootstrapAgencyRuntime(cmd)
			if err != nil {
				return err
			}
			status, err := runtime.voice.Status(cmd.Context(), resolveVoiceOrganizationID(runtime, organizationID))
			if err != nil {
				return err
			}
			if rendered, err := outputJSON(cmd, status); err != nil {
				return err
			} else if rendered {
				return nil
			}

			fmt.Printf("Voice enabled: %t\n", status.Enabled)
			fmt.Printf("Provider: %s\n", status.Provider)
			fmt.Printf("Organization: %s\n", status.OrganizationID)
			fmt.Printf("State path: %s\n", status.StatePath)
			fmt.Printf("Assets: %s\n", status.AssetDir)
			fmt.Printf("Control channel: %s\n", status.ControlChannel)
			fmt.Printf("Synthesis channel: %s\n", status.SynthesisChannel)
			fmt.Printf("Transcript dir: %s\n", status.MeetingTranscriptDir)
			fmt.Printf("Default room: %s\n", status.DefaultRoom)
			fmt.Printf("STT configured: %t\n", status.STTConfigured)
			fmt.Printf("TTS configured: %t\n", status.TTSConfigured)
			for _, room := range status.Rooms {
				fmt.Printf("- %s projection=%t transcript=%t audio=%t\n", room.RoomID, room.ProjectionEnabled, room.TranscriptProjection, room.AudioProjection)
			}
			return nil
		},
	}
	cmd.Flags().StringVar(&organizationID, "organization-id", "", "Organization identifier for voice state")
	addJSONFlag(cmd)
	return cmd
}

func newAgencyVoiceProjectCmd() *cobra.Command {
	var organizationID string
	var room string
	var enabled bool
	var transcript bool
	var audio bool

	cmd := &cobra.Command{
		Use:   "project",
		Short: "Set room projection controls for transcript and synthesized audio",
		RunE: func(cmd *cobra.Command, args []string) error {
			runtime, err := bootstrapAgencyRuntime(cmd)
			if err != nil {
				return err
			}
			req := agencyrt.VoiceProjectionRequest{
				OrganizationID:    resolveVoiceOrganizationID(runtime, organizationID),
				RoomID:            room,
				ProjectionEnabled: enabled,
			}
			if cmd.Flags().Changed("transcript") {
				req.TranscriptProjection = &transcript
			}
			if cmd.Flags().Changed("audio") {
				req.AudioProjection = &audio
			}
			roomState, err := runtime.voice.SetProjection(cmd.Context(), req)
			if err != nil {
				return err
			}
			if rendered, err := outputJSON(cmd, roomState); err != nil {
				return err
			} else if rendered {
				return nil
			}

			fmt.Printf("Room: %s\n", roomState.RoomID)
			fmt.Printf("Projection enabled: %t\n", roomState.ProjectionEnabled)
			fmt.Printf("Transcript projection: %t\n", roomState.TranscriptProjection)
			fmt.Printf("Audio projection: %t\n", roomState.AudioProjection)
			return nil
		},
	}
	cmd.Flags().StringVar(&organizationID, "organization-id", "", "Organization identifier for voice state")
	cmd.Flags().StringVar(&room, "room", "", "Room to project into")
	cmd.Flags().BoolVar(&enabled, "enabled", true, "Whether projection is enabled for the room")
	cmd.Flags().BoolVar(&transcript, "transcript", false, "Project canonical transcript text into the room")
	cmd.Flags().BoolVar(&audio, "audio", false, "Project synthesized audio into the room")
	addJSONFlag(cmd)
	return cmd
}

func newAgencyVoiceTranscribeCmd() *cobra.Command {
	var organizationID string
	var actorID string
	var room string
	var audioPath string
	var text string

	cmd := &cobra.Command{
		Use:   "transcribe",
		Short: "Commit a canonical voice transcript event to the Agency ledger",
		RunE: func(cmd *cobra.Command, args []string) error {
			runtime, err := bootstrapAgencyRuntime(cmd)
			if err != nil {
				return err
			}
			result, err := runtime.voice.IngestTranscript(cmd.Context(), agencyrt.VoiceTranscriptRequest{
				OrganizationID: resolveVoiceOrganizationID(runtime, organizationID),
				ActorID:        actorID,
				RoomID:         room,
				Text:           text,
				AudioPath:      audioPath,
			})
			if err != nil {
				return err
			}
			if rendered, err := outputJSON(cmd, result); err != nil {
				return err
			} else if rendered {
				return nil
			}

			fmt.Printf("Transcript committed: %s\n", result.Event.ID)
			fmt.Printf("Room: %s\n", result.Room.RoomID)
			fmt.Printf("Canonical text: %s\n", result.Event.CanonicalText)
			return nil
		},
	}
	cmd.Flags().StringVar(&organizationID, "organization-id", "", "Organization identifier for voice state")
	cmd.Flags().StringVar(&actorID, "actor-id", "", "Actor responsible for the transcript event")
	cmd.Flags().StringVar(&room, "room", "", "Room to attach the transcript event to")
	cmd.Flags().StringVar(&audioPath, "audio", "", "Audio file to transcribe with the configured local STT command")
	cmd.Flags().StringVar(&text, "text", "", "Canonical transcript text to commit directly")
	addJSONFlag(cmd)
	return cmd
}

func newAgencyVoiceSpeakCmd() *cobra.Command {
	var organizationID string
	var actorID string
	var room string
	var text string
	var outputPath string

	cmd := &cobra.Command{
		Use:   "speak",
		Short: "Synthesize canonical text into audio through the configured local TTS command",
		RunE: func(cmd *cobra.Command, args []string) error {
			runtime, err := bootstrapAgencyRuntime(cmd)
			if err != nil {
				return err
			}
			result, err := runtime.voice.Synthesize(cmd.Context(), agencyrt.VoiceSynthesisRequest{
				OrganizationID: resolveVoiceOrganizationID(runtime, organizationID),
				ActorID:        actorID,
				RoomID:         room,
				Text:           text,
				OutputPath:     outputPath,
			})
			if err != nil {
				return err
			}
			if rendered, err := outputJSON(cmd, result); err != nil {
				return err
			} else if rendered {
				return nil
			}

			fmt.Printf("Synthesis committed: %s\n", result.Event.ID)
			fmt.Printf("Room: %s\n", result.Room.RoomID)
			fmt.Printf("Audio output: %s\n", result.Event.AudioOutputRef)
			return nil
		},
	}
	cmd.Flags().StringVar(&organizationID, "organization-id", "", "Organization identifier for voice state")
	cmd.Flags().StringVar(&actorID, "actor-id", "", "Actor responsible for the synthesis event")
	cmd.Flags().StringVar(&room, "room", "", "Room to attach the synthesis event to")
	cmd.Flags().StringVar(&text, "text", "", "Canonical text to synthesize")
	cmd.Flags().StringVar(&outputPath, "output", "", "Optional audio output path")
	addJSONFlag(cmd)
	return cmd
}

func newAgencyVoiceServeCmd() *cobra.Command {
	var organizationID string

	cmd := &cobra.Command{
		Use:   "serve",
		Short: "Run the voice gateway event loop and persist inspectable gateway state",
		RunE: func(cmd *cobra.Command, args []string) error {
			runtime, err := bootstrapAgencyRuntime(cmd)
			if err != nil {
				return err
			}
			return runtime.voice.Serve(cmd.Context(), resolveVoiceOrganizationID(runtime, organizationID))
		},
	}
	cmd.Flags().StringVar(&organizationID, "organization-id", "", "Organization identifier for voice state")
	return cmd
}

func constitutionMarkers(view app.AgencyConstitutionView) []string {
	markers := make([]string, 0, 2)
	if view.Current {
		markers = append(markers, "current")
	}
	if view.Solo {
		markers = append(markers, "solo")
	}
	return markers
}

func roleNames(roles []config.TeamRoleTemplate) []string {
	names := make([]string, 0, len(roles))
	for _, role := range roles {
		names = append(names, role.Name)
	}
	return names
}

func resolveVoiceOrganizationID(runtime *agencyCommandRuntime, override string) string {
	if strings.TrimSpace(override) != "" {
		return strings.TrimSpace(override)
	}
	if runtime != nil && runtime.cfg != nil {
		return sanitizeVoiceIdentifier(runtime.cfg.Agency.ProductName + "-" + runtime.cfg.Agency.CurrentConstitution)
	}
	return "the-agency"
}

func sanitizeVoiceIdentifier(value string) string {
	value = strings.TrimSpace(strings.ToLower(value))
	value = strings.ReplaceAll(value, " ", "-")
	return strings.ReplaceAll(value, "/", "-")
}
