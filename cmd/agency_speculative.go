package cmd

import (
	"fmt"
	"strings"

	agencyrt "github.com/ETEllis/teamcode/internal/agency"
	"github.com/ETEllis/teamcode/internal/db"
	"github.com/spf13/cobra"
)

// newAgencyGISTSpeculativeCmd is the CLI counterpart to the Lattice
// Cathedral's /lattice/spec/<id> route. It reads the persisted
// SpeculativeBundle for a trace and prints the cohort's convergence
// status, reconciliation report, and dyad compression summary in a
// terminal-friendly layout.
//
// `--json` returns the raw envelope so it can be piped into jq without
// re-parsing the human format.
func newAgencyGISTSpeculativeCmd() *cobra.Command {
	cmd := &cobra.Command{
		Use:   "speculative <trace-id>",
		Short: "Pretty-print a trace's persisted SpeculativeBundle (cohort convergence + reconciliation + dyads)",
		Args:  cobra.ExactArgs(1),
		RunE: func(cmd *cobra.Command, args []string) error {
			id := strings.TrimSpace(args[0])
			conn, err := db.Connect()
			if err != nil {
				return err
			}
			defer conn.Close()

			row, err := db.New(conn).GetAgencyGistTrace(cmd.Context(), id)
			if err != nil {
				return err
			}

			bundle, parseErr := agencyrt.ParseSpeculativeBundle(row.SpeculativeJSON)
			source := "speculative_json"
			if parseErr != nil {
				source = "speculative_json:parse_error:" + parseErr.Error()
			}
			if bundle == nil && parseErr == nil {
				source = "none"
			}

			if rendered, err := outputJSON(cmd, map[string]any{
				"id":           row.ID,
				"officeId":     row.OfficeID,
				"agentId":      row.AgentID,
				"verdict":      row.Verdict,
				"riskLevel":    row.RiskLevel,
				"confidence":   row.Confidence,
				"createdAt":    row.CreatedAt,
				"bundle":       bundle,
				"bundleSource": source,
			}); err != nil {
				return err
			} else if rendered {
				return nil
			}

			fmt.Printf("Trace      %s\n", row.ID)
			fmt.Printf("Office     %s   Agent  %s\n", emptyDash(row.OfficeID), emptyDash(row.AgentID))
			fmt.Printf("Source     %s\n", source)
			if bundle == nil {
				fmt.Println()
				fmt.Println("No SpeculativeBundle persisted for this trace.")
				fmt.Println("Run `agency gist inspect` for the per-trace causal graph,")
				fmt.Println("or visit /lattice/spec/<id> to see the demo cohort cathedral.")
				return nil
			}

			fmt.Printf("Cohort     %s   Headline  %s\n", emptyDash(bundle.CohortID), bundle.HeadlineStatus())
			fmt.Println()

			if bundle.Convergence != nil {
				c := bundle.Convergence
				fmt.Println("Convergence:")
				fmt.Printf("  status      %s\n", c.Status)
				fmt.Printf("  quorum      %d / %d (threshold %.2f)\n",
					c.QuorumSize, c.TotalPeers, c.Threshold)
				if c.ConsensusRoot != "" {
					fmt.Printf("  root        %s\n", truncMid(c.ConsensusRoot, 24))
				}
				if len(c.DivergenceLoci) > 0 {
					fmt.Println("  divergence loci:")
					for _, l := range c.DivergenceLoci {
						sevTag := strings.ToUpper(string(l.Severity))
						fmt.Printf("    [%s] %s   missing-on=%v   extra-on=%v\n",
							sevTag, l.LeafHash[:min(16, len(l.LeafHash))]+"…",
							l.MissingOn, l.ExtraOn)
					}
				}
				fmt.Println()
			}

			if bundle.Reconciliation != nil {
				r := bundle.Reconciliation
				fmt.Println("Reconciliation:")
				fmt.Printf("  status      %s\n", r.Status)
				fmt.Printf("  coverage    %.2f\n", r.Coverage)
				if len(r.UnsupportedNodes) > 0 {
					fmt.Printf("  unsupported %v\n", r.UnsupportedNodes)
				}
				if len(r.Nodes) > 0 {
					fmt.Println("  nodes:")
					for _, n := range r.Nodes {
						fmt.Printf("    [%-11s] %-22s support=%.2f\n",
							n.Status, string(n.NodeID), n.Support)
					}
				}
				fmt.Println()
			}

			if bundle.Dyads != nil {
				d := bundle.Dyads
				saved := d.SlotsBefore - d.SlotsAfter
				fmt.Println("Dyad compression:")
				fmt.Printf("  slots       %d → %d  (saved %d)\n",
					d.SlotsBefore, d.SlotsAfter, saved)
				fmt.Printf("  pairs       %d\n", len(d.Deltas))
				for _, dl := range d.Deltas {
					fmt.Printf("    %s : base=%s  sib=%s  shape=%s\n",
						truncMid(dl.MerkleRootBase, 12), dl.BaseAgentID,
						dl.SiblingAgentID, dl.Shape)
				}
				fmt.Println()
			}

			if len(bundle.Peers) > 0 {
				fmt.Println("Peers:")
				for _, p := range bundle.Peers {
					mark := "·"
					if p.InCohort {
						mark = "✓"
					} else {
						mark = "✗"
					}
					fmt.Printf("  %s %-14s verdict=%-12s conf=%.2f  root=%s\n",
						mark, p.AgentID, emptyDash(p.Verdict),
						p.Confidence, truncMid(p.Attestation.Root, 16))
				}
				fmt.Println()
			}

			if bundle.Meta != nil {
				m := bundle.Meta
				fmt.Println("Meta:")
				fmt.Printf("  agent       %s   verdict=%s   conf=%.2f\n",
					emptyDash(m.AgentID), emptyDash(m.Verdict), m.Confidence)
				fmt.Printf("  root        %s\n", truncMid(m.Attestation.Root, 24))
			}

			return nil
		},
	}
	addJSONFlag(cmd)
	return cmd
}

func truncMid(s string, n int) string {
	if len(s) <= n {
		return s
	}
	if n < 8 {
		return s[:n]
	}
	half := (n - 1) / 2
	return s[:half] + "…" + s[len(s)-half:]
}

func min(a, b int) int {
	if a < b {
		return a
	}
	return b
}
