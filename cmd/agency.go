package cmd

import (
	"context"
	"fmt"
	"os"
	"strings"

	agencyrt "github.com/ETEllis/teamcode/internal/agency"
	"github.com/ETEllis/teamcode/internal/app"
	"github.com/ETEllis/teamcode/internal/config"
	"github.com/ETEllis/teamcode/internal/db"
	"github.com/spf13/cobra"
)

func newAgencyCmd() *cobra.Command {
	cmd := &cobra.Command{
		Use:   "agency",
		Short: "Run and inspect the Agency office experience",
		Long:  "Agency is the canonical product surface for office controls, setup, runtime status, constitutions, and voice.",
	}

	cmd.AddCommand(
		newAgencyStatusCmd(),
		newAgencyGenesisCmd(),
		newAgencyBootstrapCmd(),
		newAgencyStopCmd(),
		newAgencyBootCmd(),
		newAgencyServicesCmd(),
		newAgencySchedulesCmd(),
		newAgencyOrganizationCmd(),
		newAgencyConstitutionsCmd(),
		newAgencyConstitutionCmd(),
		newAgencySwitchCmd(),
		newAgencyVoiceCmd(),
		newAgencyGISTCmd(),
		newAgencyDirectorCmd(),
	)

	return cmd
}

func newAgencyGISTCmd() *cobra.Command {
	cmd := &cobra.Command{
		Use:   "gist",
		Short: "Inspect Agency GIST lattice traces and proof packets",
	}
	cmd.AddCommand(newAgencyGISTTracesCmd())
	return cmd
}

func newAgencyGISTTracesCmd() *cobra.Command {
	var limit int
	cmd := &cobra.Command{
		Use:   "traces",
		Short: "List recent durable GIST trace/proof packets for the current office",
		RunE: func(cmd *cobra.Command, args []string) error {
			cwd, _ := os.Getwd()
			bootstrap, err := agencyrt.LoadBootstrap(cwd, os.Getenv("AGENCY_CONSTITUTION_NAME"), agencyrt.RuntimeModeEmbedded, "")
			if err != nil {
				return err
			}
			conn, err := db.Connect()
			if err != nil {
				return err
			}
			defer conn.Close()
			traces, err := db.New(conn).ListAgencyGistTracesByOffice(cmd.Context(), bootstrap.Constitution.OrganizationID, limit)
			if err != nil {
				return err
			}
			if rendered, err := outputJSON(cmd, traces); err != nil {
				return err
			} else if rendered {
				return nil
			}
			if len(traces) == 0 {
				fmt.Println("No GIST traces recorded yet.")
				return nil
			}
			for _, trace := range traces {
				fmt.Printf("%s agent=%s verdict=%s risk=%s confidence=%.2f input=%s lattice=%s\n",
					trace.ID,
					trace.AgentID,
					trace.Verdict,
					emptyDash(trace.RiskLevel),
					trace.Confidence,
					emptyDash(trace.InputHash),
					emptyDash(trace.NextLatticeHash),
				)
			}
			return nil
		},
	}
	cmd.Flags().IntVar(&limit, "limit", 10, "Maximum number of traces to list")
	addJSONFlag(cmd)
	return cmd
}

func emptyDash(value string) string {
	if strings.TrimSpace(value) == "" {
		return "-"
	}
	return value
}

func newOfficeCmd() *cobra.Command {
	cmd := &cobra.Command{
		Use:    "office",
		Short:  "Legacy alias for Agency office commands",
		Long:   "Legacy alias for the Agency office controls. Prefer `agency ...` for user-facing guidance.",
		Hidden: true,
	}

	cmd.AddCommand(
		newOfficeBootCmd(),
		newOfficeStatusCmd(),
		newOfficeServicesCmd(),
		newOfficeStopCmd(),
		newOfficeGenesisCmd(),
		newOfficeBootstrapCmd(),
		newOfficeConstitutionsCmd(),
		newOfficeConstitutionCmd(),
		newOfficeSchedulesCmd(),
		newOfficeVoiceCmd(),
	)

	return cmd
}

func newAgencyStatusCmd() *cobra.Command {
	cmd := newOfficeStatusCmd()
	cmd.Use = "status"
	cmd.Short = "Inspect the Agency office, organization, and schedule state"
	return cmd
}

func newAgencyOrganizationCmd() *cobra.Command {
	cmd := &cobra.Command{
		Use:   "organization",
		Short: "Inspect the active Agency organization profile",
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

func newAgencyBootCmd() *cobra.Command {
	cmd := newOfficeBootCmd()
	cmd.Use = "boot"
	cmd.Short = "Boot the Agency office runtime"
	return cmd
}

func newAgencyBootstrapCmd() *cobra.Command {
	cmd := newOfficeBootstrapCmd()
	cmd.Use = "bootstrap"
	cmd.Short = "Write actor specs from the latest Agency genesis result"
	return cmd
}

func newAgencyGenesisCmd() *cobra.Command {
	cmd := newOfficeGenesisCmd()
	cmd.Use = "genesis [intent]"
	cmd.Short = "Start Agency genesis from a natural-language mission"
	return cmd
}

func newAgencyStopCmd() *cobra.Command {
	cmd := newOfficeStopCmd()
	cmd.Use = "stop"
	cmd.Short = "Stop the Agency office runtime"
	return cmd
}

func newAgencyServicesCmd() *cobra.Command {
	cmd := newOfficeServicesCmd()
	cmd.Use = "services"
	cmd.Short = "Inspect Agency runtime services"
	return cmd
}

func newAgencySchedulesCmd() *cobra.Command {
	cmd := newOfficeSchedulesCmd()
	cmd.Use = "schedules"
	cmd.Short = "Inspect Agency office schedules and shift-handoff policy"
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
			fmt.Printf("Docker: enabled=%t compose=%s image=%s (optional packaging path)\n", services.Docker.Enabled, services.Docker.ComposeFile, services.Docker.Image)
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
		Short: "Inspect configured local runtime, optional Docker, Redis, and ledger services for the office",
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

func newOfficeBootstrapCmd() *cobra.Command {
	cmd := &cobra.Command{
		Use:   "bootstrap",
		Short: "Write actor specs from the last genesis result so the runtime daemon can spawn them",
		RunE: func(cmd *cobra.Command, args []string) error {
			runtime, err := bootstrapAgencyRuntime(cmd)
			if err != nil {
				return err
			}
			count, err := runtime.agency.ManifestActors(cmd.Context())
			if err != nil {
				return err
			}
			if count == 0 {
				fmt.Println("No genesis result found. Run: /agency genesis \"<your intent>\" first.")
				return nil
			}
			fmt.Printf("Manifested %d actors.\n", count)
			fmt.Println("Run 'scripts/build-daemons && overmind start' to start the runtime.")
			return nil
		},
	}
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

			fmt.Printf("Agency genesis complete\n")
			fmt.Printf("  Constitution : %s\n", result.ConstitutionName)
			if result.Topology != "" {
				fmt.Printf("  Topology     : %s\n", result.Topology)
			}
			if len(result.Roles) > 0 {
				names := roleNames(result.Roles)
				fmt.Printf("  Actors       : %s  (%d manifested)\n", strings.Join(names, ", "), result.ManifestCount)
			}
			fmt.Println()
			if result.ManifestCount > 0 {
				fmt.Println("  Actor specs written. Agents will wake on schedule.")
				fmt.Println("  Start the runtime:  scripts/build-daemons && overmind start")
			} else {
				fmt.Println("  Run '/agency bootstrap' to write actor specs, then: overmind start")
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

func newAgencyDirectorCmd() *cobra.Command {
	cmd := &cobra.Command{
		Use:   "director",
		Short: "Talk to and inspect the minimal personal Director agent",
		Long:  "Director is the Pi-like personal agent that watches Agency, opens tickets, dispatches work, and exposes the local web portal.",
	}
	cmd.AddCommand(
		newAgencyDirectorStatusCmd(),
		newAgencyDirectorPolicyCmd(),
		newAgencyDirectorMonitorCmd(),
		newAgencyDirectorSubmitCmd(),
		newAgencyDirectorDispatchCmd(),
		newAgencyDirectorServeCmd(),
	)
	return cmd
}

func newAgencyDirectorStatusCmd() *cobra.Command {
	cmd := &cobra.Command{
		Use:   "status",
		Short: "Inspect Director agent status",
		RunE: func(cmd *cobra.Command, args []string) error {
			director, cleanup, err := bootstrapDirectorService(cmd)
			if err != nil {
				return err
			}
			defer cleanup()
			status, err := director.Status(cmd.Context())
			if err != nil {
				return err
			}
			if rendered, err := outputJSON(cmd, status); err != nil {
				return err
			} else if rendered {
				return nil
			}
			fmt.Printf("Director: %s (%s)\n", status.Agent.Name, status.Agent.ID)
			fmt.Printf("Organization: %s\n", status.OrganizationID)
			fmt.Printf("Open tickets: %d\n", status.OpenTickets)
			fmt.Printf("Dispatched: %d\n", status.Dispatched)
			fmt.Printf("Pending approvals: %d\n", status.PendingApprovals)
			fmt.Printf("Ledger sequence: %d\n", status.LedgerSequence)
			if status.LastEvent != nil {
				fmt.Printf("Last event: %s\n", status.LastEvent.Message)
			}
			return nil
		},
	}
	addJSONFlag(cmd)
	return cmd
}

func newAgencyDirectorMonitorCmd() *cobra.Command {
	cmd := &cobra.Command{
		Use:   "monitor",
		Short: "Run one Director monitoring pass",
		RunE: func(cmd *cobra.Command, args []string) error {
			director, cleanup, err := bootstrapDirectorService(cmd)
			if err != nil {
				return err
			}
			defer cleanup()
			status, err := director.Monitor(cmd.Context())
			if err != nil {
				return err
			}
			if rendered, err := outputJSON(cmd, status); err != nil {
				return err
			} else if rendered {
				return nil
			}
			if status.LastEvent != nil {
				fmt.Println(status.LastEvent.Message)
			} else {
				fmt.Println("Director monitor check complete.")
			}
			return nil
		},
	}
	addJSONFlag(cmd)
	return cmd
}

func newAgencyDirectorPolicyCmd() *cobra.Command {
	cmd := &cobra.Command{
		Use:   "policy",
		Short: "Inspect Director auto-dispatch policy",
		RunE: func(cmd *cobra.Command, args []string) error {
			director, cleanup, err := bootstrapDirectorService(cmd)
			if err != nil {
				return err
			}
			defer cleanup()
			policy := director.Policy()
			if rendered, err := outputJSON(cmd, policy); err != nil {
				return err
			} else if rendered {
				return nil
			}
			fmt.Printf("Auto-dispatch risks: %s\n", joinDirectorRisks(policy.AutoDispatchRisks))
			fmt.Printf("Auto-dispatch priorities: %s\n", joinDirectorPriorities(policy.AutoDispatchPriorities))
			fmt.Printf("Review-required risks: %s\n", joinDirectorRisks(policy.RequireApprovalRisks))
			fmt.Printf("Review-required priorities: %s\n", joinDirectorPriorities(policy.RequireApprovalPriorities))
			fmt.Printf("Pause when approvals pending: %t\n", policy.PauseWhenApprovalsPending)
			return nil
		},
	}
	addJSONFlag(cmd)
	return cmd
}

func newAgencyDirectorSubmitCmd() *cobra.Command {
	var dispatch bool
	var title string
	var priority string
	var risk string

	cmd := &cobra.Command{
		Use:   "submit [request]",
		Short: "Open a Director ticket from the terminal",
		Args:  cobra.ExactArgs(1),
		RunE: func(cmd *cobra.Command, args []string) error {
			director, cleanup, err := bootstrapDirectorService(cmd)
			if err != nil {
				return err
			}
			defer cleanup()
			ticket, err := director.SubmitTicket(cmd.Context(), agencyrt.DirectorTicketRequest{
				Title:        title,
				Body:         args[0],
				Source:       "director.cli",
				Priority:     priority,
				Risk:         risk,
				AutoDispatch: dispatch,
			})
			if err != nil {
				return err
			}
			if rendered, err := outputJSON(cmd, ticket); err != nil {
				return err
			} else if rendered {
				return nil
			}
			fmt.Printf("Director ticket opened: %s\n", ticket.ID)
			fmt.Printf("Title: %s\n", ticket.Title)
			fmt.Printf("Status: %s\n", ticket.Status)
			return nil
		},
	}
	cmd.Flags().BoolVar(&dispatch, "dispatch", false, "Dispatch the ticket into Agency immediately")
	cmd.Flags().StringVar(&title, "title", "", "Ticket title")
	cmd.Flags().StringVar(&priority, "priority", "normal", "Ticket priority")
	cmd.Flags().StringVar(&risk, "risk", "unknown", "Ticket risk level")
	addJSONFlag(cmd)
	return cmd
}

func newAgencyDirectorDispatchCmd() *cobra.Command {
	cmd := &cobra.Command{
		Use:   "dispatch [ticket-id]",
		Short: "Manually dispatch an open Director ticket into Agency",
		Args:  cobra.ExactArgs(1),
		RunE: func(cmd *cobra.Command, args []string) error {
			director, cleanup, err := bootstrapDirectorService(cmd)
			if err != nil {
				return err
			}
			defer cleanup()
			ticket, err := director.DispatchTicket(cmd.Context(), args[0])
			if err != nil {
				return err
			}
			if rendered, err := outputJSON(cmd, ticket); err != nil {
				return err
			} else if rendered {
				return nil
			}
			fmt.Printf("Director ticket dispatched: %s\n", ticket.ID)
			fmt.Printf("Title: %s\n", ticket.Title)
			fmt.Printf("Status: %s\n", ticket.Status)
			return nil
		},
	}
	addJSONFlag(cmd)
	return cmd
}

func newAgencyDirectorServeCmd() *cobra.Command {
	var addr string
	cmd := &cobra.Command{
		Use:   "serve",
		Short: "Serve the local Director web portal",
		RunE: func(cmd *cobra.Command, args []string) error {
			director, cleanup, err := bootstrapDirectorService(cmd)
			if err != nil {
				return err
			}
			defer cleanup()
			if addr == "" {
				addr = getenvForCmd("AGENCY_DIRECTOR_ADDR", "127.0.0.1:8765")
			}
			server := agencyrt.NewDirectorHTTPServer(agencyrt.DirectorHTTPConfig{
				Addr:  addr,
				Token: os.Getenv("AGENCY_DIRECTOR_TOKEN"),
			}, director)
			fmt.Printf("Agency Director portal: %s\n", directorPortalURL(server.URL(), os.Getenv("AGENCY_DIRECTOR_TOKEN")))
			return server.Serve(cmd.Context())
		},
	}
	cmd.Flags().StringVar(&addr, "addr", "", "HTTP listen address")
	return cmd
}

func bootstrapDirectorService(cmd *cobra.Command) (*agencyrt.DirectorService, func(), error) {
	cwd, _ := os.Getwd()
	bootstrap, err := agencyrt.LoadBootstrap(cwd, os.Getenv("AGENCY_CONSTITUTION_NAME"), agencyrt.RuntimeModeEmbedded, "")
	if err != nil {
		return nil, nil, err
	}
	svc, err := agencyrt.NewService(cmd.Context(), bootstrap.Config)
	if err != nil {
		return nil, nil, err
	}
	director, err := agencyrt.NewDirectorService(agencyrt.DirectorConfig{
		BaseDir:         bootstrap.Config.BaseDir,
		OrganizationID:  bootstrap.Constitution.OrganizationID,
		SharedWorkplace: bootstrap.Config.SharedWorkplace,
		Ledger:          svc.Ledger,
		Bus:             svc.Bus,
		Router: agencyrt.NewModelRouter(
			agencyrt.BuiltinProviderAdaptersForDirector(),
			agencyrt.NewCredentialBroker(),
			agencyrt.ExecutionPolicy{PrivacyLevel: "any", PreferLocal: true},
		),
	})
	if err != nil {
		_ = svc.Shutdown(context.Background())
		return nil, nil, err
	}
	return director, func() { _ = svc.Shutdown(context.Background()) }, nil
}

func getenvForCmd(key string, fallback string) string {
	if value := os.Getenv(key); value != "" {
		return value
	}
	return fallback
}

func directorPortalURL(baseURL string, token string) string {
	if token == "" {
		return baseURL
	}
	return baseURL + "?token=" + token
}

func joinDirectorRisks(values []agencyrt.DirectorRisk) string {
	parts := make([]string, 0, len(values))
	for _, value := range values {
		parts = append(parts, string(value))
	}
	return strings.Join(parts, ", ")
}

func joinDirectorPriorities(values []agencyrt.DirectorPriority) string {
	parts := make([]string, 0, len(values))
	for _, value := range values {
		parts = append(parts, string(value))
	}
	return strings.Join(parts, ", ")
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
