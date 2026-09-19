package cli

import (
	"bufio"
	"context"
	"crypto/sha256"
	"dossier/internal/config"
	"dossier/internal/core"
	"dossier/internal/harness"
	"dossier/internal/mcp"
	"dossier/internal/search"
	"dossier/internal/store"
	"dossier/internal/sync"
	"dossier/internal/tokenizer"
	"dossier/internal/tui"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"net/http"
	"os"
	"path/filepath"
	"runtime"
	"strings"
	"time"

	"github.com/spf13/cobra"
)

// Version is the binary's version string, set by main from a -ldflags
// "-X main.version=..." value at release build time. It is "dev" for
// local/unstamped builds.
var Version = "dev"

var (
	dossierHomeFlag     string
	yesFlag             bool
	statusFlag          string
	queryFlag           string
	mineFlag            bool
	jsonFlag            bool
	dossierSearchFlag   string
	distilledFlag       string
	distilledFileFlag   string
	descriptionFlag     string
	promotePriorityFlag string
	fromFileFlag        string
	forceFlag           bool
	sessionFlag         string
	leadFlag            string
	interfacesFlag      []string
)

// NewRootCmd constructs the root cobra command hierarchy.
func NewRootCmd() *cobra.Command {
	rootCmd := &cobra.Command{
		Use:     "dossier",
		Short:   "Dossier: durable memory layer for agent-driven work",
		Version: Version,
		RunE: func(cmd *cobra.Command, args []string) error {
			homeDir := resolveHomeDir()
			svc, cfg, err := wireWithConfig(homeDir)
			if err != nil {
				return err
			}
			return tui.Run(context.Background(), svc, cfg.OpenWith)
		},
	}

	rootCmd.PersistentFlags().StringVar(&dossierHomeFlag, "home", "", "Override default Dossier home directory")

	initCmd := &cobra.Command{
		Use:   "init",
		Short: "Initialize the Dossier workspace and config",
		Run: func(cmd *cobra.Command, args []string) {
			execPath, err := os.Executable()
			if err == nil && isVolatilePath(execPath) {
				shouldInstall := false
				if yesFlag {
					shouldInstall = true
				} else {
					fmt.Printf("Dossier is running from a volatile/temporary path: %s\n", execPath)
					fmt.Printf("Would you like to self-install to a stable location (~/.local/bin) first? [y/N]: ")
					var resp string
					_, _ = fmt.Scanln(&resp)
					resp = strings.ToLower(strings.TrimSpace(resp))
					if resp == "y" || resp == "yes" {
						shouldInstall = true
					}
				}

				if shouldInstall {
					if err := runInstall("~/.local/bin", yesFlag); err != nil {
						fmt.Printf("Self-install failed: %v\n", err)
						os.Exit(1)
					}
				}
			}

			homeDir := resolveHomeDir()
			cfgPath := filepath.Join(homeDir, "config.yaml")
			if _, statErr := os.Stat(cfgPath); os.IsNotExist(statErr) {
				cfg := config.Default()
				cfg.DossierHome = homeDir
				if err := cfg.SaveDefault(cfgPath); err != nil {
					fmt.Printf("Error writing config: %v\n", err)
					os.Exit(1)
				}
			}
			svc, err := wire(homeDir)
			if err != nil {
				fmt.Printf("Error wiring service: %v\n", err)
				os.Exit(1)
			}

			res, err := svc.Init(context.Background(), core.InitReq{
				YesToAll:         yesFlag,
				StableBinaryPath: getStableBinaryPath(),
			})
			if err != nil {
				fmt.Printf("Initialization failed: %v\n", err)
				os.Exit(1)
			}

			// Preconfigure the sync exclusion set at init rather than at first
			// sync, so a store that later joins a team already excludes the
			// machine-local set and the raw session stash before its first commit.
			// Git history is append-only across every clone, so an exclusion added
			// after the first push never retracts what was already published.
			if giErr := sync.EnsureGitignore(homeDir); giErr != nil {
				fmt.Printf("Warning: could not write store .gitignore: %v\n", giErr)
			}

			fmt.Printf("Dossier initialized at %s\n\n", homeDir)

			if dataMap, ok := res.Data.(map[string]any); ok {
				reports, _ := dataMap["harnesses"].([]core.HarnessReport)
				printHarnessReports(reports)
				if detected, _ := dataMap["harness_detected"].(bool); !detected {
					fmt.Println("No harness detected — run from within Claude Code or Pi for full integration.")
					fmt.Println()
				}
			}

			for _, warning := range res.Warnings {
				fmt.Printf("Warning: %s\n", warning)
			}
		},
	}
	initCmd.Flags().BoolVarP(&yesFlag, "yes", "y", false, "Skip confirmation prompts")

	var installDirFlag string
	installCmd := &cobra.Command{
		Use:   "install",
		Short: "Install the Dossier binary to a stable PATH location",
		Run: func(cmd *cobra.Command, args []string) {
			if err := runInstall(installDirFlag, yesFlag); err != nil {
				fmt.Printf("Installation failed: %v\n", err)
				os.Exit(1)
			}
		},
	}
	installCmd.Flags().StringVar(&installDirFlag, "dir", "~/.local/bin", "Directory to install the binary to")
	installCmd.Flags().BoolVarP(&yesFlag, "yes", "y", false, "Skip confirmation prompts")

	harnessCmd := &cobra.Command{
		Use:   "harness",
		Short: "Inspect and install client harness integrations",
	}

	harnessListCmd := &cobra.Command{
		Use:   "list",
		Short: "Show which harnesses are detected and what they give Dossier",
		Run: func(cmd *cobra.Command, args []string) {
			svc, err := wire(resolveHomeDir())
			if err != nil {
				fmt.Printf("Error: %v\n", err)
				os.Exit(1)
			}

			res, err := svc.HarnessStatus(context.Background())
			if err != nil {
				fmt.Printf("Harness detection failed: %v\n", err)
				os.Exit(1)
			}

			if jsonFlag {
				printJSON(res.Data)
				return
			}

			reports, _ := res.Data.([]core.HarnessReport)
			printHarnessReports(reports)
			for _, warning := range res.Warnings {
				fmt.Printf("Warning: %s\n", warning)
			}
		},
	}
	harnessListCmd.Flags().BoolVar(&jsonFlag, "json", false, "Output as JSON")

	harnessInstallCmd := &cobra.Command{
		Use:   "install <claude-code|pi>",
		Short: "Install the Dossier integration for a harness added after init",
		Args:  cobra.ExactArgs(1),
		Run: func(cmd *cobra.Command, args []string) {
			svc, err := wire(resolveHomeDir())
			if err != nil {
				fmt.Printf("Error: %v\n", err)
				os.Exit(1)
			}

			res, err := svc.InstallHarness(context.Background(), core.InstallHarnessReq{
				Name:             args[0],
				YesToAll:         yesFlag,
				StableBinaryPath: getStableBinaryPath(),
			})
			if err != nil {
				fmt.Printf("Harness install failed: %v\n", err)
				os.Exit(1)
			}

			if jsonFlag {
				printJSON(res.Data)
			} else if report, ok := res.Data.(core.HarnessReport); ok {
				printHarnessReports([]core.HarnessReport{report})
			}
			for _, warning := range res.Warnings {
				fmt.Printf("Warning: %s\n", warning)
			}
			// A skipped install wrote nothing. Exiting 0 would let a pipeline read
			// "no terminal to confirm on" as a successful install.
			if !res.OK {
				os.Exit(1)
			}
		},
	}
	harnessInstallCmd.Flags().BoolVarP(&yesFlag, "yes", "y", false, "Skip confirmation prompts")
	harnessInstallCmd.Flags().BoolVar(&jsonFlag, "json", false, "Output as JSON")

	harnessCmd.AddCommand(harnessListCmd)
	harnessCmd.AddCommand(harnessInstallCmd)

	doctorCmd := &cobra.Command{
		Use:   "doctor",
		Short: "Verify system health and configuration integrity",
		Run: func(cmd *cobra.Command, args []string) {
			homeDir := resolveHomeDir()
			svc, err := wire(homeDir)
			if err != nil {
				fmt.Printf("Error: %v\n", err)
				os.Exit(1)
			}

			res, err := svc.Doctor(context.Background())
			if err != nil {
				fmt.Printf("Doctor check failed: %v\n", err)
				os.Exit(1)
			}

			if res.OK {
				fmt.Println("Dossier workspace is healthy!")
			} else {
				fmt.Println("Dossier workspace checks failed.")
			}

			if report, ok := res.Data.(core.DoctorReport); ok {
				fmt.Printf("Health: %s\n", core.HealthSummaryFromDoctor(report).Line(time.Now()))
				fmt.Printf("\nChecked: %d dossiers, %d artifacts, %d audit logs\n", report.DossiersChecked, report.ArtifactsChecked, report.AuditLogsChecked)
				if report.SyncConfigured {
					fmt.Println("\nTeam Sync Status:")
					if report.SyncStatus != nil {
						fmt.Printf("  Last attempt: %s\n", formatTime(report.SyncStatus.LastAttempt))
						fmt.Printf("  Last pull: %s\n", formatTime(report.SyncStatus.LastSuccessPull))
						fmt.Printf("  Last push: %s\n", formatTime(report.SyncStatus.LastSuccessPush))
						if report.SyncStatus.LastError != "" {
							fmt.Printf("  Last error: %s\n", report.SyncStatus.LastError)
						}
						fmt.Printf("  Auth state: %s\n", report.SyncStatus.AuthState)
						fmt.Printf("  Ahead: %d, Behind: %d\n", report.SyncStatus.Ahead, report.SyncStatus.Behind)
						fmt.Printf("  Unresolved conflicts: %d\n", report.SyncStatus.ConflictsFound)
					} else {
						fmt.Println("  Configured but status unavailable")
					}
				}
				fmt.Println()
			}

			for _, warning := range res.Warnings {
				fmt.Printf("- Warning: %s\n", warning)
			}

			if !res.OK {
				os.Exit(1)
			}
		},
	}

	lsCmd := &cobra.Command{
		Use:   "ls",
		Short: "List open dossiers sorted by priority",
		Run: func(cmd *cobra.Command, args []string) {
			homeDir := resolveHomeDir()
			svc, err := wire(homeDir)
			if err != nil {
				fmt.Printf("Error: %v\n", err)
				os.Exit(1)
			}

			leadFilter := ""
			if mineFlag {
				leadFilter = "me"
			}
			res, err := svc.List(context.Background(), core.ListReq{Status: statusFlag, Lead: leadFilter, Interfaces: interfacesFlag, Query: queryFlag})
			if err != nil {
				fmt.Printf("List failed: %v\n", err)
				os.Exit(1)
			}

			if jsonFlag {
				printJSON(res.Data)
				return
			}

			items, ok := res.Data.([]core.ListItem)
			if !ok {
				fmt.Printf("Unexpected data type returned: %T\n", res.Data)
				os.Exit(1)
			}

			if len(items) == 0 {
				fmt.Println("No dossiers found.")
				return
			}

			fmt.Printf("%-30s %-15s %-11s %-8s %-5s %s\n", "NAME/SLUG", "LEAD", "STATUS", "PRIORITY", "DUE", "NEXT ACTION")
			fmt.Println(strings.Repeat("-", 96))
			for _, item := range items {
				nameOrSlug := item.Name
				if nameOrSlug == "" {
					nameOrSlug = item.Slug
				}
				if len(nameOrSlug) > 28 {
					nameOrSlug = nameOrSlug[:25] + "..."
				}

				lead := item.Lead
				if lead == "" {
					lead = "Unassigned"
				} else if len(lead) > 13 {
					lead = lead[:10] + "..."
				}

				nextAction := item.NextAction
				if len(nextAction) > 28 {
					nextAction = nextAction[:25] + "..."
				}

				fmt.Printf("%-30s %-15s %-11s %-8s %-5s %s\n", nameOrSlug, lead, item.Status, item.Priority, item.DueDate, nextAction)
			}
		},
	}
	lsCmd.Flags().StringVar(&statusFlag, "status", "", "Filter by status (spark|define|execute|review|blocked|done|all)")
	lsCmd.Flags().StringSliceVar(&interfacesFlag, "interface", nil, "Filter by interface (repeat or comma-separate)")
	lsCmd.Flags().StringVarP(&queryFlag, "query", "q", "", "Filter by name, description, lead, interface, or slug")
	lsCmd.Flags().BoolVar(&mineFlag, "mine", false, "Show dossiers assigned to the current user")
	lsCmd.Flags().BoolVar(&jsonFlag, "json", false, "Output results in JSON format")

	showCmd := &cobra.Command{
		Use:   "show <slug-or-id>",
		Short: "Show a dossier's details and distilled state",
		Args:  cobra.ExactArgs(1),
		Run: func(cmd *cobra.Command, args []string) {
			homeDir := resolveHomeDir()
			svc, err := wire(homeDir)
			if err != nil {
				fmt.Printf("Error: %v\n", err)
				os.Exit(1)
			}

			res, err := svc.Recall(context.Background(), core.RecallReq{ID: args[0]})
			if err != nil {
				fmt.Printf("Error showing dossier: %v\n", err)
				os.Exit(1)
			}

			if jsonFlag {
				printJSON(res.Data)
				return
			}

			recall, ok := res.Data.(core.RecallResult)
			if !ok {
				fmt.Printf("Unexpected data type returned: %T\n", res.Data)
				os.Exit(1)
			}

			fmt.Printf("Name:           %s\n", recall.Frontmatter.Name)
			if recall.Frontmatter.Description != "" {
				fmt.Printf("Description:    %s\n", recall.Frontmatter.Description)
			}
			fmt.Printf("ID:             %s\n", recall.Frontmatter.ID)
			fmt.Printf("Slug:           %s\n", recall.Frontmatter.Slug)
			lead := recall.Frontmatter.Lead
			if recall.LeadFormer {
				lead += " (former)"
			}
			fmt.Printf("Lead:           %s\n", lead)
			fmt.Printf("Interfaces:      %s\n", strings.Join(recall.Frontmatter.Interfaces, ", "))
			fmt.Printf("Status:         %s\n", recall.Frontmatter.Status)
			fmt.Printf("Priority:       %s\n", recall.Frontmatter.Priority)
			if recall.Frontmatter.DueDate != "" {
				fmt.Printf("Due Date:       %s\n", recall.Frontmatter.DueDate)
			}
			fmt.Printf("Token Estimate: %d\n", recall.TokenEstimate)
			fmt.Printf("Next Action:    %s\n", recall.Frontmatter.NextAction)
			fmt.Println(strings.Repeat("-", 80))
			fmt.Println(recall.DistilledState)
		},
	}
	showCmd.Flags().BoolVar(&jsonFlag, "json", false, "Output results in JSON format")

	pathCmd := &cobra.Command{
		Use:   "path [<slug-or-id>]",
		Short: "Get the directory path of a dossier or the workspace root",
		Run: func(cmd *cobra.Command, args []string) {
			homeDir := resolveHomeDir()
			if len(args) == 0 {
				if jsonFlag {
					printJSON(map[string]string{"path": homeDir})
				} else {
					fmt.Println(homeDir)
				}
				return
			}

			svc, err := wire(homeDir)
			if err != nil {
				fmt.Printf("Error: %v\n", err)
				os.Exit(1)
			}

			res, err := svc.Path(context.Background(), core.PathReq{ID: args[0]})
			if err != nil {
				fmt.Printf("Error: %v\n", err)
				os.Exit(1)
			}

			if jsonFlag {
				printJSON(map[string]string{"path": res.Data.(string)})
			} else {
				fmt.Println(res.Data)
			}
		},
	}
	pathCmd.Flags().BoolVar(&jsonFlag, "json", false, "Output results in JSON format")

	archiveCmd := &cobra.Command{
		Use:   "archive <slug-or-id>",
		Short: "Archive a dossier (marks status as done, keeping files)",
		Args:  cobra.ExactArgs(1),
		Run: func(cmd *cobra.Command, args []string) {
			homeDir := resolveHomeDir()
			svc, err := wire(homeDir)
			if err != nil {
				fmt.Printf("Error: %v\n", err)
				os.Exit(1)
			}

			res, err := svc.Archive(context.Background(), core.ArchiveReq{ID: args[0]})
			if err != nil {
				fmt.Printf("Archive failed: %v\n", err)
				os.Exit(1)
			}

			if jsonFlag {
				printJSON(res)
				return
			}

			fmt.Printf("Dossier archived successfully. New revision: %s\n", res.Data.(core.Revision))
		},
	}
	archiveCmd.Flags().BoolVar(&jsonFlag, "json", false, "Output results in JSON format")

	doneCmd := &cobra.Command{
		Use:   "done <slug-or-id>",
		Short: "Mark a dossier as done (terminal status, keeping files)",
		Args:  cobra.ExactArgs(1),
		Run: func(cmd *cobra.Command, args []string) {
			homeDir := resolveHomeDir()
			svc, err := wire(homeDir)
			if err != nil {
				fmt.Printf("Error: %v\n", err)
				os.Exit(1)
			}

			res, err := svc.Archive(context.Background(), core.ArchiveReq{ID: args[0]})
			if err != nil {
				fmt.Printf("Done failed: %v\n", err)
				os.Exit(1)
			}

			if jsonFlag {
				printJSON(res)
				return
			}

			fmt.Printf("Dossier marked done successfully. New revision: %s\n", res.Data.(core.Revision))
		},
	}
	doneCmd.Flags().BoolVar(&jsonFlag, "json", false, "Output results in JSON format")

	searchCmd := &cobra.Command{
		Use:   "search <query>",
		Short: "Search distilled state and artifacts across dossiers",
		Args:  cobra.ExactArgs(1),
		Run: func(cmd *cobra.Command, args []string) {
			homeDir := resolveHomeDir()
			svc, err := wire(homeDir)
			if err != nil {
				fmt.Printf("Error: %v\n", err)
				os.Exit(1)
			}

			req := core.SearchReq{
				Query: args[0],
				Scope: core.SearchScope{
					DossierID: dossierSearchFlag,
				},
			}

			res, err := svc.Search(context.Background(), req)
			if err != nil {
				fmt.Printf("Search failed: %v\n", err)
				os.Exit(1)
			}

			if jsonFlag {
				printJSON(res.Data)
				return
			}

			hits, ok := res.Data.([]core.Hit)
			if !ok {
				fmt.Printf("Unexpected data type returned: %T\n", res.Data)
				os.Exit(1)
			}

			if len(hits) == 0 {
				fmt.Println("No matches found.")
				return
			}

			for i, hit := range hits {
				fmt.Printf("Dossier:  %s (%s)\n", hit.DossierName, hit.DossierID)
				if hit.ArtifactID != "" {
					fmt.Printf("Artifact: %s (%s)\n", hit.Title, hit.ArtifactID)
				}
				fmt.Printf("File:     %s\n", hit.Path)
				if hit.LineNumber > 0 {
					fmt.Printf("Line %d:  %s\n", hit.LineNumber, hit.Snippet)
				} else {
					fmt.Printf("Match:    %s\n", hit.Snippet)
				}
				if i < len(hits)-1 {
					fmt.Println(strings.Repeat("-", 80))
				}
			}
		},
	}
	searchCmd.Flags().StringVarP(&dossierSearchFlag, "dossier", "d", "", "Scope search to a specific dossier (slug or ID)")
	searchCmd.Flags().BoolVar(&jsonFlag, "json", false, "Output results in JSON format")

	var artifactLinesFlag string
	artifactCmd := &cobra.Command{
		Use:   "artifact <slug-or-id> [<artifact-id>]",
		Short: "Show a dossier's evidence index, or fetch one artifact's content",
		Long: "With one argument, list every archived artifact and whether the distilled state cites it.\n" +
			"With two, print the artifact line-numbered, so a [src:art_x#L10-L20] citation can be followed to its source.",
		Args: cobra.RangeArgs(1, 2),
		Run: func(cmd *cobra.Command, args []string) {
			homeDir := resolveHomeDir()
			svc, err := wire(homeDir)
			if err != nil {
				fmt.Printf("Error: %v\n", err)
				os.Exit(1)
			}

			if len(args) == 1 {
				res, err := svc.ListArtifacts(context.Background(), core.ListArtifactsReq{DossierID: args[0]})
				if err != nil {
					fmt.Printf("Error: %v\n", err)
					os.Exit(1)
				}
				if jsonFlag {
					printJSON(res.Data)
					return
				}
				index, ok := res.Data.([]core.ArtifactSummary)
				if !ok {
					fmt.Printf("Unexpected data type returned: %T\n", res.Data)
					os.Exit(1)
				}
				if len(index) == 0 {
					fmt.Println("No artifacts archived for this dossier.")
					return
				}
				for _, a := range index {
					cited := "uncited"
					if a.Cited {
						cited = "cited"
					}
					fmt.Printf("%-28s %-18s %6d lines  %-8s %s\n", a.ID, a.Type, a.Lines, cited, a.Title)
				}
				for _, w := range res.Warnings {
					fmt.Printf("\nWarning: %s\n", w)
				}
				return
			}

			req := core.ReadArtifactReq{DossierID: args[0], ArtifactID: args[1]}
			if artifactLinesFlag != "" {
				req.Fragment = normalizeLineFlag(artifactLinesFlag)
			}

			res, err := svc.ReadArtifact(context.Background(), req)
			if err != nil {
				fmt.Printf("Error: %v\n", err)
				os.Exit(1)
			}
			if jsonFlag {
				printJSON(res.Data)
				return
			}
			content, ok := res.Data.(core.ArtifactContent)
			if !ok {
				fmt.Printf("Unexpected data type returned: %T\n", res.Data)
				os.Exit(1)
			}
			fmt.Printf("Artifact: %s (%s)\n", content.Title, content.ID)
			fmt.Printf("Type:     %s\n", content.Type)
			fmt.Printf("Lines:    %d-%d of %d\n\n", content.StartLine, content.EndLine, content.Lines)
			fmt.Print(content.Content)
			for _, w := range res.Warnings {
				fmt.Printf("\nWarning: %s\n", w)
			}
		},
	}
	artifactCmd.Flags().StringVarP(&artifactLinesFlag, "lines", "L", "", "Line range to fetch, e.g. 10-20 or L10-L20")
	artifactCmd.Flags().BoolVar(&jsonFlag, "json", false, "Output results in JSON format")

	contextCmd := &cobra.Command{
		Use:   "context",
		Short: "Manage the generated open-work context",
	}

	contextRefreshCmd := &cobra.Command{
		Use:   "refresh",
		Short: "Regenerate the context library",
		Run: func(cmd *cobra.Command, args []string) {
			homeDir := resolveHomeDir()
			svc, err := wire(homeDir)
			if err != nil {
				fmt.Printf("Error: %v\n", err)
				os.Exit(1)
			}

			res, err := svc.ContextRefresh(context.Background())
			if err != nil {
				fmt.Printf("Context refresh failed: %v\n", err)
				os.Exit(1)
			}

			if !res.OK {
				fmt.Println("Context refresh failed.")
				os.Exit(1)
			}

			fmt.Println("Context library regenerated successfully.")
		},
	}
	contextCmd.AddCommand(contextRefreshCmd)

	mcpCmd := &cobra.Command{
		Use:   "mcp",
		Short: "Manage the MCP server interface",
	}

	mcpServeCmd := &cobra.Command{
		Use:   "serve",
		Short: "Run the MCP server over stdio",
		Run: func(cmd *cobra.Command, args []string) {
			homeDir := resolveHomeDir()
			svc, err := wire(homeDir)
			if err != nil {
				fmt.Printf("Error: %v\n", err)
				os.Exit(1)
			}

			server := mcp.NewServer(svc, os.Stdin, os.Stdout)
			if err := server.Run(context.Background()); err != nil {
				fmt.Printf("MCP server exited with error: %v\n", err)
				os.Exit(1)
			}
		},
	}
	mcpCmd.AddCommand(mcpServeCmd)

	promoteCmd := &cobra.Command{
		Use:   "promote <name>",
		Short: "Promote a new dossier from session content or file",
		Args:  cobra.ExactArgs(1),
		Run: func(cmd *cobra.Command, args []string) {
			homeDir := resolveHomeDir()
			svc, err := wire(homeDir)
			if err != nil {
				fmt.Printf("Error: %v\n", err)
				os.Exit(1)
			}

			var content string
			distilled := distilledFlag
			if fromFileFlag != "" {
				data, err := os.ReadFile(fromFileFlag)
				if err != nil {
					fmt.Printf("Error reading file: %v\n", err)
					os.Exit(1)
				}
				content = string(data)
			}
			if distilledFileFlag != "" {
				if distilled != "" {
					fmt.Println("Error: --distilled and --distilled-file cannot be used together")
					os.Exit(1)
				}
				data, err := os.ReadFile(distilledFileFlag)
				if err != nil {
					fmt.Printf("Error reading distilled state file: %v\n", err)
					os.Exit(1)
				}
				distilled = string(data)
			}

			req := core.PromoteReq{
				Name:                   args[0],
				Description:            descriptionFlag,
				Priority:               core.Priority(promotePriorityFlag),
				DistilledStateMarkdown: distilled,
				Content:                content,
				Lead:                   leadFlag,
				Interfaces:             interfacesFlag,
				Force:                  forceFlag || yesFlag,
			}

			res, err := svc.Promote(context.Background(), req)
			if err != nil {
				if dErr, ok := err.(*core.DomainError); ok && dErr.Code == core.ErrAmbiguousTarget {
					if jsonFlag {
						printJSON(res)
						return
					}
					fmt.Println("Error: Multiple likely dossiers match this name. Disambiguation required:")
					suggestions := res.Data.([]core.Suggestion)
					for _, sug := range suggestions {
						fmt.Printf("- %s (ID: %s, Confidence: %s) - Reason: %s\n", sug.Name, sug.ID, sug.Confidence, sug.Reason)
					}
					fmt.Println("\nTo create anyway, re-run with --force or -y.")
					os.Exit(1)
				}

				fmt.Printf("Promote failed: %v\n", err)
				os.Exit(1)
			}

			if jsonFlag {
				printJSON(res)
				return
			}

			fmt.Printf("Dossier promoted successfully. ID: %s\n", res.Data.(string))
		},
	}
	promoteCmd.Flags().StringVar(&distilledFlag, "distilled", "", "Distilled state markdown body")
	promoteCmd.Flags().StringVar(&distilledFileFlag, "distilled-file", "", "Path to a file containing the distilled state markdown body")
	promoteCmd.Flags().StringVar(&descriptionFlag, "description", "", "Optional progressive-disclosure summary")
	promoteCmd.Flags().StringVar(&promotePriorityFlag, "priority", "", "Priority: low|medium|high|max")
	promoteCmd.Flags().StringVar(&fromFileFlag, "from-file", "", "Path to session content file")
	promoteCmd.Flags().StringVar(&leadFlag, "lead", "", "Lead assignee for the dossier")
	promoteCmd.Flags().StringSliceVar(&interfacesFlag, "interface", nil, "Discussion interface(s) for the dossier")
	promoteCmd.Flags().BoolVar(&forceFlag, "force", false, "Force create dossier even if matches exist")
	promoteCmd.Flags().BoolVar(&jsonFlag, "json", false, "Output results in JSON format")

	linkCmd := &cobra.Command{
		Use:   "link [<slug-or-id>]",
		Short: "Link session content or file to a dossier",
		Run: func(cmd *cobra.Command, args []string) {
			homeDir := resolveHomeDir()
			svc, err := wire(homeDir)
			if err != nil {
				fmt.Printf("Error: %v\n", err)
				os.Exit(1)
			}

			var content string
			var title string
			if fromFileFlag != "" {
				data, err := os.ReadFile(fromFileFlag)
				if err != nil {
					fmt.Printf("Error reading file: %v\n", err)
					os.Exit(1)
				}
				content = string(data)
				title = filepath.Base(fromFileFlag)
			}

			var targetID string
			if len(args) > 0 {
				targetID = args[0]
			}

			req := core.LinkReq{
				ID:      targetID,
				Content: content,
				Title:   title,
			}

			res, err := svc.Link(context.Background(), req)
			if err != nil {
				if dErr, ok := err.(*core.DomainError); ok && dErr.Code == core.ErrAmbiguousTarget {
					if jsonFlag {
						printJSON(res.Data)
						return
					}
					fmt.Println("Ambiguity detected. Top matching dossiers for this content:")
					suggestions := res.Data.([]core.Suggestion)
					for _, sug := range suggestions {
						fmt.Printf("- %s (ID: %s, Confidence: %s) - Reason: %s\n", sug.Name, sug.ID, sug.Confidence, sug.Reason)
					}
					fmt.Println("\nTo link, run again with: dossier link <id> --from-file <path>")
					os.Exit(1)
				}

				fmt.Printf("Link failed: %v\n", err)
				os.Exit(1)
			}

			if jsonFlag {
				printJSON(res)
				return
			}

			fmt.Printf("Dossier linked successfully. New revision: %s\n", res.Data.(core.Revision))
		},
	}
	linkCmd.Flags().StringVar(&fromFileFlag, "from-file", "", "Path to session content file")
	linkCmd.Flags().BoolVar(&jsonFlag, "json", false, "Output results in JSON format")

	activeCmd := &cobra.Command{
		Use:   "active",
		Short: "Show the active dossier bound to the current session",
		Run: func(cmd *cobra.Command, args []string) {
			homeDir := resolveHomeDir()
			svc, err := wire(homeDir)
			if err != nil {
				fmt.Printf("Error: %v\n", err)
				os.Exit(1)
			}

			sessID, _ := resolveSessionID()
			res, err := svc.Active(context.Background(), core.ActiveReq{SessionID: sessID})
			if err != nil {
				if jsonFlag {
					printJSON(map[string]any{"ok": false, "error": err.Error()})
					os.Exit(1)
				}
				fmt.Printf("No active dossier bound to this session: %v\n", err)
				os.Exit(1)
			}

			if jsonFlag {
				printJSON(res.Data)
				return
			}

			binding := res.Data.(*core.SessionBinding)
			fmt.Printf("Active Dossier ID:  %s\n", binding.DossierID)
			fmt.Printf("Bound At:           %s\n", binding.BoundAt.Format(time.RFC3339))
			fmt.Printf("Last Seen Revision: %s\n", binding.LastSeenRevision)
		},
	}
	activeCmd.Flags().StringVar(&sessionFlag, "session", "", "Session ID to check")
	activeCmd.Flags().BoolVar(&jsonFlag, "json", false, "Output results in JSON format")

	switchCmd := &cobra.Command{
		Use:   "switch <slug-or-id>",
		Short: "Switch the active dossier binding for the session",
		Args:  cobra.ExactArgs(1),
		Run: func(cmd *cobra.Command, args []string) {
			homeDir := resolveHomeDir()
			svc, err := wire(homeDir)
			if err != nil {
				fmt.Printf("Error: %v\n", err)
				os.Exit(1)
			}

			sessID, _ := resolveSessionID()
			res, err := svc.Switch(context.Background(), core.SwitchReq{
				ID:          args[0],
				SessionID:   sessID,
				HarnessName: resolveSessionHarness(),
			})
			if err != nil {
				fmt.Printf("Switch failed: %v\n", err)
				os.Exit(1)
			}

			if jsonFlag {
				printJSON(res.Data)
				return
			}

			recall := res.Data.(core.RecallResult)
			fmt.Printf("Switched active dossier to: %s (%s)\n", recall.Frontmatter.Name, recall.Frontmatter.ID)
			fmt.Printf("Revision: %s\n", recall.Revision)
		},
	}
	switchCmd.Flags().StringVar(&sessionFlag, "session", "", "Session ID to bind")
	switchCmd.Flags().BoolVar(&jsonFlag, "json", false, "Output results in JSON format")

	mergeCmd := &cobra.Command{
		Use:   "merge <source> <target>",
		Short: "Merge a source dossier into a surviving target dossier",
		Args:  cobra.ExactArgs(2),
		Run: func(cmd *cobra.Command, args []string) {
			homeDir := resolveHomeDir()
			svc, err := wire(homeDir)
			if err != nil {
				fmt.Printf("Error: %v\n", err)
				os.Exit(1)
			}

			res, err := svc.Merge(context.Background(), core.MergeReq{
				SourceID: args[0],
				TargetID: args[1],
			})
			if err != nil {
				if dErr, ok := err.(*core.DomainError); ok && dErr.Code == core.ErrConflictDetected {
					if jsonFlag {
						printJSON(map[string]any{"ok": false, "error": err.Error(), "conflict": res.Data})
						os.Exit(1)
					}
					fmt.Printf("Merge conflict detected: %v\n", err)
					conflict := res.Data.(*core.Conflict)
					fmt.Printf("Conflict ID: %s\n", conflict.ID)
					fmt.Println("\nTo resolve this conflict, please edit the Distilled State manually or run again specifying the resolved conflict.")
					os.Exit(1)
				}
				fmt.Printf("Merge failed: %v\n", err)
				os.Exit(1)
			}

			if jsonFlag {
				printJSON(res)
				return
			}

			fmt.Printf("Dossier merged successfully. Surviving target ID: %s. New revision: %s\n", args[1], res.Data.(core.Revision))
		},
	}
	mergeCmd.Flags().BoolVar(&jsonFlag, "json", false, "Output results in JSON format")

	var conflictsJSON bool
	conflictsCmd := &cobra.Command{
		Use:          "conflicts [conflict-id]",
		Short:        "List unresolved conflicts or show one comparison",
		SilenceUsage: true,
		Args:         cobra.MaximumNArgs(1),
		RunE: func(cmd *cobra.Command, args []string) error {
			svc, err := wire(resolveHomeDir())
			if err != nil {
				return err
			}
			if len(args) == 1 {
				detail, err := svc.ConflictDetail(context.Background(), args[0])
				if err != nil {
					return err
				}
				if conflictsJSON {
					printJSON(detail)
				} else {
					printConflictDetail(detail)
				}
				return nil
			}
			conflicts, err := svc.ListConflicts(context.Background())
			if err != nil {
				return err
			}
			if conflictsJSON {
				printJSON(conflicts)
				return nil
			}
			if len(conflicts) == 0 {
				fmt.Println("No unresolved conflicts")
				return nil
			}
			for _, conflict := range conflicts {
				fmt.Printf("%s\t%s\t%s\t%s\n", conflict.ID, conflict.DossierID, conflict.Kind, conflict.TS.Format(time.RFC3339))
			}
			return nil
		},
	}
	conflictsCmd.Flags().BoolVar(&conflictsJSON, "json", false, "Output results in JSON format")

	var keepShared, restoreMine, keepBoth, resolveJSON bool
	resolveCmd := &cobra.Command{
		Use:          "resolve <conflict-id>",
		Short:        "Resolve an unresolved conflict",
		SilenceUsage: true,
		Args:         cobra.ExactArgs(1),
		RunE: func(cmd *cobra.Command, args []string) error {
			choices := 0
			choice := ""
			if keepShared {
				choices++
				choice = core.ConflictChoiceKeepShared
			}
			if restoreMine {
				choices++
				choice = core.ConflictChoiceRestoreMine
			}
			if keepBoth {
				choices++
				choice = core.ConflictChoiceKeepBoth
			}
			if choices != 1 {
				return fmt.Errorf("exactly one of --keep-shared, --restore-mine, or --keep-both is required")
			}
			svc, err := wire(resolveHomeDir())
			if err != nil {
				return err
			}
			res, err := svc.ResolveConflict(context.Background(), core.ResolveConflictReq{
				ConflictID: args[0],
				Choice:     choice,
			})
			if err != nil {
				return err
			}
			if resolveJSON {
				printJSON(res)
			} else {
				fmt.Printf("Resolved conflict %s with %s\n", args[0], choice)
			}
			return nil
		},
	}
	resolveCmd.Flags().BoolVar(&keepShared, "keep-shared", false, "Keep the current shared state")
	resolveCmd.Flags().BoolVar(&restoreMine, "restore-mine", false, "Restore the rejected local state")
	resolveCmd.Flags().BoolVar(&keepBoth, "keep-both", false, "Keep both states with an unresolved disagreement section")
	resolveCmd.Flags().BoolVar(&resolveJSON, "json", false, "Output results in JSON format")

	var renameBaseRevision string
	var renameTitle string
	var renameSlug string
	renameCmd := &cobra.Command{
		Use:   "rename <slug-or-id> [<new-value>]",
		Short: "Rename a dossier title or slug while preserving its ID",
		Args:  cobra.RangeArgs(1, 2),
		RunE: func(cmd *cobra.Command, args []string) error {
			if len(args) == 2 && (renameTitle != "" || renameSlug != "") {
				return fmt.Errorf("provide the new value either as an argument or with --title/--slug")
			}
			if renameTitle != "" && renameSlug != "" {
				return fmt.Errorf("--title and --slug cannot be used together")
			}
			value := ""
			renameTitleMode := renameTitle != ""
			if len(args) == 2 {
				value = args[1]
			} else if renameTitleMode {
				value = renameTitle
			} else if renameSlug != "" {
				value = renameSlug
			} else {
				return fmt.Errorf("provide a new value or use --title/--slug")
			}

			svc, err := wire(resolveHomeDir())
			if err != nil {
				return err
			}
			base := core.Revision(renameBaseRevision)
			if base == "" {
				recalled, err := svc.Recall(context.Background(), core.RecallReq{ID: args[0]})
				if err != nil {
					return fmt.Errorf("rename failed: %w", err)
				}
				base = recalled.Data.(core.RecallResult).Revision
			}
			req := core.RenameReq{ID: args[0], BaseRevision: base}
			if renameTitleMode {
				req.NewName = value
			} else {
				req.NewSlug = value
			}
			res, err := svc.Rename(context.Background(), req)
			if err != nil {
				return fmt.Errorf("rename failed: %w", err)
			}
			if jsonFlag {
				printJSON(res)
				return nil
			}
			renamed := res.Data.(core.RenameResult)
			if renameTitleMode {
				fmt.Printf("Dossier title renamed: %s → %s (ID: %s, revision: %s)\n", renamed.OldName, renamed.Name, renamed.ID, renamed.Revision)
			} else {
				fmt.Printf("Dossier slug renamed: %s → %s (ID: %s, revision: %s)\n", renamed.OldSlug, renamed.Slug, renamed.ID, renamed.Revision)
				fmt.Printf("Use the new slug for future references. New path: %s\n", renamed.Path)
			}
			return nil
		},
	}
	renameCmd.Flags().StringVar(&renameBaseRevision, "base-revision", "", "Expected current revision (defaults to an immediate recall)")
	renameCmd.Flags().StringVar(&renameTitle, "title", "", "Rename the display title to this value")
	renameCmd.Flags().StringVar(&renameSlug, "slug", "", "Rename the canonical slug to this value (the default)")
	renameCmd.Flags().BoolVar(&jsonFlag, "json", false, "Output results in JSON format")

	statusCmd := &cobra.Command{
		Use:   "status <slug-or-id> <spark|define|execute|review|blocked|done>",
		Short: "Update status of a dossier",
		Args:  cobra.ExactArgs(2),
		Run: func(cmd *cobra.Command, args []string) {
			homeDir := resolveHomeDir()
			svc, err := wire(homeDir)
			if err != nil {
				fmt.Printf("Error: %v\n", err)
				os.Exit(1)
			}
			res, err := svc.Save(context.Background(), core.SaveReq{
				ID:                 args[0],
				FrontmatterUpdates: map[string]any{"status": args[1]},
			})
			if err != nil {
				fmt.Printf("Status update failed: %v\n", err)
				os.Exit(1)
			}
			fmt.Printf("Status updated successfully. New revision: %s\n", res.Data.(core.Revision))
		},
	}

	leadCmd := &cobra.Command{
		Use:   "lead <slug-or-id> <lead-name>",
		Short: "Update a dossier lead (available values come from config.yaml)",
		Args:  cobra.ExactArgs(2),
		Run: func(cmd *cobra.Command, args []string) {
			homeDir := resolveHomeDir()
			svc, err := wire(homeDir)
			if err != nil {
				fmt.Printf("Error: %v\n", err)
				os.Exit(1)
			}
			res, err := svc.Save(context.Background(), core.SaveReq{
				ID:                 args[0],
				FrontmatterUpdates: map[string]any{"lead": args[1]},
			})
			if err != nil {
				fmt.Printf("Lead update failed: %v\n", err)
				os.Exit(1)
			}
			fmt.Printf("Lead updated successfully. New revision: %s\n", res.Data.(core.Revision))
		},
	}

	descriptionCmd := &cobra.Command{
		Use:   "description <slug-or-id> <description>",
		Short: "Update the progressive-disclosure description of a dossier",
		Args:  cobra.ExactArgs(2),
		Run: func(cmd *cobra.Command, args []string) {
			svc, err := wire(resolveHomeDir())
			if err != nil {
				fmt.Printf("Error: %v\n", err)
				os.Exit(1)
			}
			res, err := svc.Save(context.Background(), core.SaveReq{
				ID:                 args[0],
				FrontmatterUpdates: map[string]any{"description": args[1]},
			})
			if err != nil {
				fmt.Printf("Description update failed: %v\n", err)
				os.Exit(1)
			}
			fmt.Printf("Description updated successfully. New revision: %s\n", res.Data.(core.Revision))
		},
	}

	interfaceCmd := &cobra.Command{
		Use:   "interface <slug-or-id> [interface]...",
		Short: "Set configured discussion interfaces, or omit them to clear",
		Args:  cobra.MinimumNArgs(1),
		Run: func(cmd *cobra.Command, args []string) {
			svc, err := wire(resolveHomeDir())
			if err != nil {
				fmt.Printf("Error: %v\n", err)
				os.Exit(1)
			}
			res, err := svc.Save(context.Background(), core.SaveReq{
				ID:                 args[0],
				FrontmatterUpdates: map[string]any{"interfaces": args[1:]},
			})
			if err != nil {
				fmt.Printf("Interface update failed: %v\n", err)
				os.Exit(1)
			}
			fmt.Printf("Interfaces updated successfully. New revision: %s\n", res.Data.(core.Revision))
		},
	}

	nextCmd := &cobra.Command{
		Use:   "next <slug-or-id> <next-action>",
		Short: "Update next action of a dossier",
		Args:  cobra.ExactArgs(2),
		Run: func(cmd *cobra.Command, args []string) {
			homeDir := resolveHomeDir()
			svc, err := wire(homeDir)
			if err != nil {
				fmt.Printf("Error: %v\n", err)
				os.Exit(1)
			}
			res, err := svc.Save(context.Background(), core.SaveReq{
				ID:                 args[0],
				FrontmatterUpdates: map[string]any{"next_action": args[1]},
			})
			if err != nil {
				fmt.Printf("Next action update failed: %v\n", err)
				os.Exit(1)
			}
			fmt.Printf("Next action updated successfully. New revision: %s\n", res.Data.(core.Revision))
		},
	}

	var priorityFlag string
	var dueFlag string
	priorityCmd := &cobra.Command{
		Use:   "priority <slug-or-id>",
		Short: "Update priority and due date of a dossier",
		Args:  cobra.ExactArgs(1),
		Run: func(cmd *cobra.Command, args []string) {
			homeDir := resolveHomeDir()
			svc, err := wire(homeDir)
			if err != nil {
				fmt.Printf("Error: %v\n", err)
				os.Exit(1)
			}

			recallRes, err := svc.Recall(context.Background(), core.RecallReq{ID: args[0]})
			if err != nil {
				fmt.Printf("Failed to read dossier: %v\n", err)
				os.Exit(1)
			}
			recall := recallRes.Data.(core.RecallResult)

			updates := make(map[string]any)
			if priorityFlag != "" {
				priority := core.Priority(strings.ToLower(priorityFlag))
				if !priority.IsValid() {
					fmt.Printf("Invalid priority %q. Choose low, medium, high, or max.\n", priorityFlag)
					return
				}
				updates["priority"] = string(priority)
			}
			if dueFlag != "" {
				updates["due_date"] = dueFlag
			}

			res, err := svc.Save(context.Background(), core.SaveReq{
				ID:                 args[0],
				BaseRevision:       recall.Revision,
				FrontmatterUpdates: updates,
			})
			if err != nil {
				fmt.Printf("Priority update failed: %v\n", err)
				os.Exit(1)
			}
			fmt.Printf("Priority updated successfully. New revision: %s\n", res.Data.(core.Revision))
		},
	}
	priorityCmd.Flags().StringVar(&priorityFlag, "priority", "", "Priority: low|medium|high|max")
	priorityCmd.Flags().StringVar(&dueFlag, "due", "", "Due date (YYYY-MM-DD or relative)")

	updateCmd := &cobra.Command{
		Use:   "update",
		Short: "Update the Dossier binary to the latest release",
		Args:  cobra.NoArgs,
		Run: func(cmd *cobra.Command, args []string) {
			targetPath := os.Getenv("DOSSIER_UPDATE_TARGET")
			if targetPath == "" {
				var err error
				targetPath, err = os.Executable()
				if err != nil {
					fmt.Printf("Failed to get current executable path: %v\n", err)
					os.Exit(1)
				}

				if isVolatilePath(targetPath) {
					targetPath = getStableBinaryPath()
					if isVolatilePath(targetPath) {
						home, err := os.UserHomeDir()
						if err == nil {
							targetPath = filepath.Join(home, ".local", "bin", stableBinaryName())
						} else {
							fmt.Println("Error: could not determine stable installation path. Run 'dossier install' first.")
							os.Exit(1)
						}
					}
				}
			}

			updateURL := os.Getenv("DOSSIER_UPDATE_URL")
			if updateURL == "" {
				updateURL = fmt.Sprintf("https://github.com/execsumo/dossier/releases/latest/download/dossier-%s-%s", runtime.GOOS, runtime.GOARCH)
			}

			fmt.Printf("Downloading latest release from %s...\n", updateURL)

			ctx, cancel := context.WithTimeout(context.Background(), 2*time.Minute)
			defer cancel()

			req, err := http.NewRequestWithContext(ctx, "GET", updateURL, nil)
			if err != nil {
				fmt.Printf("Failed to create request: %v\n", err)
				os.Exit(1)
			}

			req.Header.Set("User-Agent", "dossier-updater")

			resp, err := http.DefaultClient.Do(req)
			if err != nil {
				fmt.Printf("Failed to download release: %v\n", err)
				os.Exit(1)
			}
			defer resp.Body.Close()

			if resp.StatusCode != http.StatusOK {
				fmt.Printf("Failed to download release: HTTP %s\n", resp.Status)
				os.Exit(1)
			}

			destDir := filepath.Dir(targetPath)
			if err := os.MkdirAll(destDir, 0755); err != nil {
				fmt.Printf("Failed to create directory %s: %v\n", destDir, err)
				os.Exit(1)
			}

			tmpFile, err := os.CreateTemp(destDir, "dossier-update-*")
			if err != nil {
				fmt.Printf("Failed to create temporary file: %v\n", err)
				os.Exit(1)
			}
			tmpName := tmpFile.Name()
			defer func() {
				if tmpFile != nil {
					tmpFile.Close()
					os.Remove(tmpName)
				}
			}()

			if _, err := io.Copy(tmpFile, resp.Body); err != nil {
				fmt.Printf("Failed to write download content: %v\n", err)
				os.Exit(1)
			}

			if err := tmpFile.Sync(); err != nil {
				fmt.Printf("Failed to sync file: %v\n", err)
				os.Exit(1)
			}

			if err := tmpFile.Close(); err != nil {
				fmt.Printf("Failed to close temporary file: %v\n", err)
				os.Exit(1)
			}
			tmpFile = nil

			if err := os.Chmod(tmpName, 0755); err != nil {
				fmt.Printf("Failed to make updated binary executable: %v\n", err)
				os.Exit(1)
			}

			if runtime.GOOS == "windows" {
				// Windows will not replace a read-only installed executable.
				_ = os.Chmod(targetPath, 0644)
			}
			if err := os.Rename(tmpName, targetPath); err != nil {
				fmt.Printf("Failed to install updated binary over %s: %v\n", targetPath, err)
				os.Exit(1)
			}

			fmt.Printf("Dossier successfully updated to the latest release at %s\n", targetPath)
		},
	}

	hookCmd := &cobra.Command{
		Use:   "hook <session-start|session-end|pre-compaction>",
		Short: "Run lifecycle integration hooks",
		Args:  cobra.ExactArgs(1),
		Run: func(cmd *cobra.Command, args []string) {
			homeDir := resolveHomeDir()
			svc, err := wire(homeDir)
			if err != nil {
				fmt.Printf("Error: %v\n", err)
				os.Exit(1)
			}

			var payload struct {
				SessionID      string `json:"session_id"`
				HookEventName  string `json:"hook_event_name"`
				TranscriptPath string `json:"transcript_path"`
				DistilledState string `json:"distilled_state"`
			}

			stat, _ := os.Stdin.Stat()
			if (stat.Mode() & os.ModeCharDevice) == 0 {
				dec := json.NewDecoder(os.Stdin)
				_ = dec.Decode(&payload)
			}

			sessID := payload.SessionID
			if sessID == "" {
				sessID, _ = resolveSessionID()
			}
			if payload.TranscriptPath == "" {
				payload.TranscriptPath = piTranscriptPath()
			}

			transcript := harness.ResolveTranscript(sessID, payload.TranscriptPath)

			switch args[0] {
			case "session-start":
				resText, err := svc.SessionStart(context.Background(), sessID)
				if err != nil {
					fmt.Printf("Session start hook failed: %v\n", err)
					os.Exit(1)
				}
				fmt.Print(resText)

			case "session-end", "pre-compaction":
				warnings, err := svc.SessionEnd(context.Background(), sessID, payload.DistilledState, transcript)
				if err != nil {
					fmt.Printf("Session end hook failed: %v\n", err)
					os.Exit(1)
				}
				for _, w := range warnings {
					fmt.Printf("Warning: %s\n", w)
				}
				fmt.Println("Session hook completed successfully.")

			default:
				fmt.Printf("Unknown hook event: %s\n", args[0])
				os.Exit(1)
			}
		},
	}

	tuiCmd := &cobra.Command{
		Use:   "tui",
		Short: "Launch the interactive text user interface (TUI)",
		RunE: func(cmd *cobra.Command, args []string) error {
			homeDir := resolveHomeDir()
			svc, cfg, err := wireWithConfig(homeDir)
			if err != nil {
				return err
			}
			return tui.Run(context.Background(), svc, cfg.OpenWith)
		},
	}

	openCmd := &cobra.Command{
		Use:   "open <slug-or-id>",
		Short: "Open a dossier in the configured agent",
		Long: "Mint a new Claude Code session id, bind the dossier to it, and launch\n" +
			"the configured agent in the dossier's directory with a fresh binding.\n" +
			"This is the same handoff the TUI's 'c' key performs.",
		Args: cobra.ExactArgs(1),
		// Nothing this command can fail on is a usage error — a missing binary or
		// a non-zero exit from claude should not bury the message under a help
		// dump.
		SilenceUsage: true,
		RunE: func(cmd *cobra.Command, args []string) error {
			homeDir := resolveHomeDir()
			svc, cfg, err := wireWithConfig(homeDir)
			if err != nil {
				return err
			}

			openWith, err := harness.NormalizeOpenWith(cfg.OpenWith)
			if err != nil {
				return err
			}

			ctx := context.Background()
			if cfg.Team.Remote != "" {
				syncCtx, cancel := context.WithTimeout(ctx, 5*time.Second)
				_, _ = svc.Sync(syncCtx)
				cancel()

				if summary, herr := svc.LocalHealthSummary(ctx); herr == nil {
					fmt.Fprintf(cmd.ErrOrStderr(), "Health: %s\n", summary.Line(time.Now()))
				}
			}

			res, err := svc.Path(ctx, core.PathReq{ID: args[0]})
			if err != nil {
				return err
			}
			dir := res.Data.(string)
			recallRes, err := svc.Recall(ctx, core.RecallReq{ID: args[0]})
			if err != nil {
				return err
			}
			recall := recallRes.Data.(core.RecallResult)

			sessionID, err := harness.NewSessionID()
			if err != nil {
				return err
			}

			plan, err := harness.PlanOpenWith(openWith, harness.LaunchRequest{
				SessionID:  sessionID,
				DossierDir: dir,
				Name:       recall.Frontmatter.Name,
				Slug:       recall.Frontmatter.Slug,
			})
			if err != nil {
				return err
			}

			if openWith != "pi" {
				_, err = svc.Switch(ctx, core.SwitchReq{
					ID:          args[0],
					SessionID:   sessionID,
					HarnessName: openWith,
				})
				if err != nil {
					return err
				}
			} else {
				// Pi mints its own session id and ignores an inherited one, so there is
				// nothing to pre-bind to. Name the CLI first: Pi ships no MCP client, so
				// `dossier_session` only exists if the user runs an MCP adapter extension.
				fmt.Fprintln(cmd.ErrOrStderr(), "Notice: Pi mints its own session id, so this dossier was not pre-bound. The agent binds it on its first `dossier switch` call (or `dossier_session`, if an MCP adapter extension is installed).")
			}

			agent := plan.Command()
			agent.Stdin = os.Stdin
			agent.Stdout = cmd.OutOrStdout()
			agent.Stderr = cmd.ErrOrStderr()
			return agent.Run()
		},
	}

	versionCmd := &cobra.Command{
		Use:   "version",
		Short: "Print the Dossier version",
		Args:  cobra.NoArgs,
		Run: func(cmd *cobra.Command, args []string) {
			fmt.Fprintf(cmd.OutOrStdout(), "dossier %s\n", Version)
		},
	}

	rootCmd.AddCommand(versionCmd)
	rootCmd.AddCommand(initCmd)
	rootCmd.AddCommand(installCmd)
	rootCmd.AddCommand(harnessCmd)
	rootCmd.AddCommand(doctorCmd)
	rootCmd.AddCommand(lsCmd)
	rootCmd.AddCommand(showCmd)
	rootCmd.AddCommand(pathCmd)
	rootCmd.AddCommand(archiveCmd)
	rootCmd.AddCommand(doneCmd)
	rootCmd.AddCommand(searchCmd)
	rootCmd.AddCommand(artifactCmd)
	rootCmd.AddCommand(contextCmd)
	rootCmd.AddCommand(mcpCmd)
	rootCmd.AddCommand(promoteCmd)
	rootCmd.AddCommand(linkCmd)
	rootCmd.AddCommand(activeCmd)
	rootCmd.AddCommand(switchCmd)
	rootCmd.AddCommand(mergeCmd)
	rootCmd.AddCommand(conflictsCmd)
	rootCmd.AddCommand(resolveCmd)
	rootCmd.AddCommand(renameCmd)
	rootCmd.AddCommand(statusCmd)
	rootCmd.AddCommand(leadCmd)
	rootCmd.AddCommand(descriptionCmd)
	rootCmd.AddCommand(interfaceCmd)
	rootCmd.AddCommand(nextCmd)
	rootCmd.AddCommand(priorityCmd)
	rootCmd.AddCommand(updateCmd)
	rootCmd.AddCommand(hookCmd)

	rootCmd.AddCommand(tuiCmd)
	rootCmd.AddCommand(openCmd)

	// Match `dossier version` output for the built-in `--version` flag.
	rootCmd.SetVersionTemplate("dossier {{.Version}}\n")

	var signinJSON bool
	signinCmd := &cobra.Command{
		Use:   "signin",
		Short: "Sign in to GitHub for team sync",
		Args:  cobra.NoArgs,
		Run: func(cmd *cobra.Command, args []string) {
			homeDir := resolveHomeDir()
			cfg, err := config.Load(filepath.Join(homeDir, "config.yaml"))
			if err != nil {
				fmt.Printf("Error loading config: %v\n", err)
				os.Exit(1)
			}
			remote := cfg.Team.Remote
			if remote == "" {
				remote = "https://github.com/"
			}
			if err := ensureRemoteCredentials(cmd, remote); err != nil {
				fmt.Printf("Sign-in failed: %v\n", err)
				os.Exit(1)
			}
			_, method, err := sync.GetAuth("", remote)
			if err != nil {
				fmt.Printf("Sign-in failed: %v\n", err)
				os.Exit(1)
			}
			if signinJSON {
				printJSON(map[string]string{"method": method})
				return
			}
			switch method {
			case "gh":
				fmt.Println("GitHub sign-in active via gh.")
			case "file":
				fmt.Println("GitHub sign-in active via ~/.dossier/credentials.")
			default:
				fmt.Printf("GitHub sign-in method: %s\n", method)
			}
		},
	}
	signinCmd.Flags().BoolVar(&signinJSON, "json", false, "Output results in JSON format")
	rootCmd.AddCommand(signinCmd)

	var syncStatusFlag bool
	syncCmd := &cobra.Command{
		Use:   "sync",
		Short: "Synchronize the dossier store with the team remote",
		Run: func(cmd *cobra.Command, args []string) {
			homeDir := resolveHomeDir()
			svc, err := wire(homeDir)
			if err != nil {
				fmt.Printf("Error: %v\n", err)
				os.Exit(1)
			}

			if syncStatusFlag {
				res, err := svc.Health(context.Background())
				if err != nil {
					if jsonFlag {
						printJSON(map[string]any{"ok": false, "error": err.Error()})
						os.Exit(1)
					}
					fmt.Printf("Status failed: %v\n", err)
					os.Exit(1)
				}
				health := res.Data.(core.HealthReport)
				if jsonFlag {
					printJSON(health)
					return
				}
				fmt.Printf("Health: %s\n", health.Summary.Line(time.Now()))
				if st := health.Doctor.SyncStatus; st != nil {
					fmt.Printf("Last attempt:    %s\n", formatTime(st.LastAttempt))
					fmt.Printf("Last pull:       %s\n", formatTime(st.LastSuccessPull))
					fmt.Printf("Last push:       %s\n", formatTime(st.LastSuccessPush))
					if st.LastError != "" {
						fmt.Printf("Last error:      %s\n", st.LastError)
					}
					fmt.Printf("Auth state:      %s\n", st.AuthState)
					fmt.Printf("Ahead:           %d\n", st.Ahead)
					fmt.Printf("Behind:          %d\n", st.Behind)
					fmt.Printf("Dirty:           %d\n", st.Dirty)
					fmt.Printf("Conflicts:       %d\n", st.ConflictsFound)
				}
				return
			}

			res, err := svc.Sync(context.Background())
			if err != nil {
				if jsonFlag {
					printJSON(map[string]any{"ok": false, "error": err.Error()})
					os.Exit(1)
				}
				// Conflicts written in the same run must still be announced.
				for _, warning := range res.Warnings {
					fmt.Printf("Warning: %s\n", warning)
				}
				fmt.Printf("Sync failed: %v\n", err)
				os.Exit(1)
			}

			if jsonFlag {
				printJSON(res)
				if !res.OK {
					os.Exit(1)
				}
				return
			}

			for _, warning := range res.Warnings {
				fmt.Printf("Warning: %s\n", warning)
			}

			report := res.Data.(core.SyncReport)
			if !res.OK || report.Error != "" {
				errMsg := report.Error
				if errMsg == "" {
					errMsg = "unknown error"
				}
				fmt.Printf("Sync failed: %s. Your changes are committed locally and will be sent on the next successful sync.\n", errMsg)
				os.Exit(1)
			}

			fmt.Println("Sync successful")
			if report.Pulled {
				fmt.Println("- Pulled remote changes")
			}
			if report.Pushed {
				fmt.Println("- Pushed local changes")
			}
			if !report.Pulled && !report.Pushed {
				fmt.Println("- Already up to date")
			}
		},
	}
	syncCmd.Flags().BoolVar(&syncStatusFlag, "status", false, "Show sync status without syncing")
	syncCmd.Flags().BoolVar(&jsonFlag, "json", false, "Output results in JSON format")
	rootCmd.AddCommand(syncCmd)

	teamCmd := &cobra.Command{
		Use:   "team",
		Short: "Manage team sync setup",
	}

	var teamCreateYes, teamCreateJSON bool
	var teamCreateName string
	teamCreateCmd := &cobra.Command{
		Use:   "create <url>",
		Short: "Turn the current store into a team's shared store",
		Args:  cobra.ExactArgs(1),
		Run: func(cmd *cobra.Command, args []string) {
			if err := checkRemoteBeforeStore(cmd, args[0]); err != nil {
				fmt.Printf("Team create failed: %v\n", err)
				os.Exit(1)
			}
			homeDir := resolveHomeDir()
			cfgPath := filepath.Join(homeDir, "config.yaml")
			cfg, err := config.Load(cfgPath)
			if err != nil {
				fmt.Printf("Error loading config: %v\n", err)
				os.Exit(1)
			}
			cfg.Team.Remote = args[0]
			cfg.Team.Branch = "main"
			svc, _, err := wireWithLoadedConfig(homeDir, cfg, cfgPath)
			if err != nil {
				fmt.Printf("Error: %v\n", err)
				os.Exit(1)
			}

			preview, err := svc.TeamCreate(context.Background(), core.TeamCreateReq{RemoteURL: args[0], Branch: "main"})
			if err != nil {
				errStr := err.Error()
				if dErr, ok := err.(*core.DomainError); ok {
					errStr = dErr.Error()
				}
				fmt.Printf("Team create failed: %v\n", errStr)
				os.Exit(1)
			}
			if !teamCreateYes {
				for _, warning := range preview.Warnings {
					fmt.Printf("Warning: %s\n", warning)
				}
				fmt.Println("Dossiers to publish:")
				for _, dossier := range preview.Data.([]core.ListedFrontmatter) {
					fmt.Printf("- %s (%s) [%s]\n", dossier.Name, dossier.Slug, dossier.Status)
				}
				fmt.Fprint(cmd.OutOrStdout(), "Publish these Dossiers? [y/N]: ")
				answer, readErr := bufio.NewReader(cmd.InOrStdin()).ReadString('\n')
				answer = strings.ToLower(strings.TrimSpace(answer))
				if readErr != nil && answer == "" {
					fmt.Println("Team create refused: confirmation required; use --yes in non-interactive mode")
					os.Exit(1)
				}
				if answer != "y" && answer != "yes" {
					fmt.Println("Team create refused.")
					os.Exit(1)
				}
			}
			managerName := strings.TrimSpace(teamCreateName)
			if managerName == "" && strings.TrimSpace(cfg.DisplayName) == "" && !teamCreateYes {
				username := core.NormalizeUsername(cfg.Author)
				fmt.Fprintf(cmd.OutOrStdout(), "Your name as teammates will see it [%s]: ", username)
				answer, readErr := bufio.NewReader(cmd.InOrStdin()).ReadString('\n')
				managerName = strings.TrimSpace(answer)
				if readErr != nil && managerName == "" {
					fmt.Println("Team create refused: manager display name is required; use --name or --yes in non-interactive mode")
					os.Exit(1)
				}
			}
			res, err := svc.TeamCreate(context.Background(), core.TeamCreateReq{RemoteURL: args[0], Branch: "main", Confirmed: true, ManagerDisplayName: managerName})
			if err != nil {
				fmt.Printf("Team create failed: %v\n", err)
				os.Exit(1)
			}
			if err := cfg.Save(cfgPath); err != nil {
				fmt.Printf("Team create succeeded but saving config failed: %v\n", err)
				os.Exit(1)
			}
			if teamCreateJSON {
				printJSON(res)
				return
			}
			fmt.Println("Team store created successfully.")
		},
	}
	teamCreateCmd.Flags().BoolVarP(&teamCreateYes, "yes", "y", false, "Skip confirmation prompt")
	teamCreateCmd.Flags().StringVar(&teamCreateName, "name", "", "Manager display name for teammates")
	teamCreateCmd.Flags().BoolVar(&teamCreateJSON, "json", false, "Output results in JSON format")

	var teamJoinJSON bool
	teamJoinCmd := &cobra.Command{
		Use:   "join <url>",
		Short: "Join an existing team store",
		Args:  cobra.ExactArgs(1),
		Run: func(cmd *cobra.Command, args []string) {
			if err := checkRemoteBeforeStore(cmd, args[0]); err != nil {
				fmt.Printf("Team join failed: %v\n", err)
				os.Exit(1)
			}
			homeDir := resolveHomeDir()
			cfgPath := filepath.Join(homeDir, "config.yaml")
			cfg, err := config.Load(cfgPath)
			if err != nil {
				fmt.Printf("Error loading config: %v\n", err)
				os.Exit(1)
			}
			cfg.Team.Remote = args[0]
			cfg.Team.Branch = "main"
			svc, _, err := wireWithLoadedConfig(homeDir, cfg, cfgPath)
			if err != nil {
				fmt.Printf("Error: %v\n", err)
				os.Exit(1)
			}
			res, err := svc.TeamJoin(context.Background(), core.TeamJoinReq{RemoteURL: args[0], Branch: "main"})
			if err != nil {
				errStr := err.Error()
				if dErr, ok := err.(*core.DomainError); ok {
					errStr = dErr.Error()
				}
				fmt.Printf("Team join failed: %v\n", errStr)
				os.Exit(1)
			}
			if err := cfg.Save(cfgPath); err != nil {
				fmt.Printf("Team join succeeded but saving config failed: %v\n", err)
				os.Exit(1)
			}
			if teamJoinJSON {
				printJSON(res)
				return
			}
			fmt.Println("Successfully joined team store.")
			for _, warning := range res.Warnings {
				fmt.Printf("Warning: %s\n", warning)
			}
			printJoinRosterMessage(svc)
		},
	}
	teamJoinCmd.Flags().BoolVar(&teamJoinJSON, "json", false, "Output results in JSON format")

	var teamAddJSON bool
	teamAddCmd := &cobra.Command{
		Use:   "add <username> <display-name>",
		Short: "Add a member to the team roster",
		Args:  cobra.ExactArgs(2),
		Run: func(cmd *cobra.Command, args []string) {
			svc, err := wire(resolveHomeDir())
			if err != nil {
				fmt.Printf("Error: %v\n", err)
				os.Exit(1)
			}
			res, err := svc.TeamAdd(context.Background(), args[0], args[1])
			if err != nil {
				fmt.Printf("Team add failed: %v\n", err)
				os.Exit(1)
			}
			if teamAddJSON {
				printJSON(res.Data)
				return
			}
			fmt.Printf("Added %s to the team roster.\n", args[1])
			for _, warning := range res.Warnings {
				fmt.Printf("Warning: %s\n", warning)
			}
		},
	}
	teamAddCmd.Flags().BoolVar(&teamAddJSON, "json", false, "Output results in JSON format")

	var teamRemoveJSON bool
	teamRemoveCmd := &cobra.Command{
		Use:   "remove <username>",
		Short: "Move a member to the former team roster",
		Args:  cobra.ExactArgs(1),
		Run: func(cmd *cobra.Command, args []string) {
			svc, err := wire(resolveHomeDir())
			if err != nil {
				fmt.Printf("Error: %v\n", err)
				os.Exit(1)
			}
			res, err := svc.TeamRemove(context.Background(), args[0])
			if err != nil {
				fmt.Printf("Team remove failed: %v\n", err)
				os.Exit(1)
			}
			if teamRemoveJSON {
				printJSON(res.Data)
				return
			}
			fmt.Printf("Moved %s to former team members.\n", core.NormalizeUsername(args[0]))
			for _, warning := range res.Warnings {
				fmt.Printf("Warning: %s\n", warning)
			}
		},
	}
	teamRemoveCmd.Flags().BoolVar(&teamRemoveJSON, "json", false, "Output results in JSON format")

	var teamMembersJSON bool
	teamMembersCmd := &cobra.Command{
		Use:   "members",
		Short: "List the team roster",
		Args:  cobra.NoArgs,
		Run: func(cmd *cobra.Command, args []string) {
			svc, err := wire(resolveHomeDir())
			if err != nil {
				fmt.Printf("Error: %v\n", err)
				os.Exit(1)
			}
			roster, err := svc.Members(context.Background())
			if err != nil {
				fmt.Printf("Team members failed: %v\n", err)
				os.Exit(1)
			}
			view := roster.View()
			if teamMembersJSON {
				printJSON(view)
				return
			}
			fmt.Printf("Manager: %s (%s)\n", roster.Manager, roster.DisplayName(roster.Manager))
			fmt.Println("Members:")
			for _, member := range view.Members {
				fmt.Printf("- %s (%s)\n", member.DisplayName, member.Username)
			}
			if len(view.Former) > 0 {
				fmt.Println("Former members:")
				for _, member := range view.Former {
					fmt.Printf("- %s (%s)\n", member.DisplayName, member.Username)
				}
			}
		},
	}
	teamMembersCmd.Flags().BoolVar(&teamMembersJSON, "json", false, "Output results in JSON format")

	teamCmd.AddCommand(teamCreateCmd)
	teamCmd.AddCommand(teamJoinCmd)
	teamCmd.AddCommand(teamAddCmd)
	teamCmd.AddCommand(teamRemoveCmd)
	teamCmd.AddCommand(teamMembersCmd)
	rootCmd.AddCommand(teamCmd)

	return rootCmd

}

// Execute runs the cobra command parser.
func Execute() {
	if err := NewRootCmd().Execute(); err != nil {
		os.Exit(1)
	}
}

func resolveHomeDir() string {
	if dossierHomeFlag != "" {
		return dossierHomeFlag
	}
	return config.Default().DossierHome
}

// resolveSessionID determines the session ID and whether it is a "real" session (i.e.
// resolved from a real harness/explicit source rather than the sess_default fallback).
//
// NOTE (ADR 0003 / Divergence): The CLI and TUI deliberately fall back to the DefaultSessionID
// ("sess_default") when no explicit session/harness session is resolved (allowDefault=true).
// This differs from the MCP adapter, which uses allowDefault=false and errors (ErrNoSessionID)
// if no real session resolves, preventing silent cross-contamination of concurrent agent sessions.
// The interactive local TUI is allowed to fall back to a local default bucket for convenience.
func resolveSessionID() (string, bool) {
	// Attempt to resolve a session ID without allowing the default fallback.
	sid, _, err := harness.ResolveSession(sessionFlag, false)
	if err == nil {
		return sid, true
	}
	// Fall back to the default fallback bucket.
	defaultSid, _ := harness.ResolveSessionID(sessionFlag, true)
	return defaultSid, false
}

// resolveSessionHarness names the harness the session id came from, so a binding
// records the harness the session actually ran under. Empty when the id was
// supplied explicitly or fell back to the shared bucket.
func resolveSessionHarness() string {
	_, harnessName, err := harness.ResolveSession(sessionFlag, false)
	if err != nil {
		return ""
	}
	return harnessName
}

// piTranscriptPath resolves Pi's session JSONL for a hook invocation: the
// bash-tool environment first, then the pointer the Dossier Pi extension
// publishes (the only source when Pi did not spawn this process via bash).
func piTranscriptPath() string {
	if path := os.Getenv("PI_SESSION_FILE"); path != "" {
		return path
	}
	if pointer, ok := harness.LookupPiSessionPointer(); ok {
		return pointer.SessionFile
	}
	return ""
}

// harnessCapabilityOrder fixes the reporting order, so the same integration
// reads the same way on every run.
var harnessCapabilityOrder = []struct{ key, label string }{
	{"SessionIdentity", "Session identity"},
	{"MCP", "MCP"},
	{"SessionStartHook", "Session-start hook"},
	{"SessionEndHook", "Session-end hook"},
	{"PreCompactionHook", "Pre-compaction hook"},
	{"TranscriptCapture", "Transcript capture"},
}

// printHarnessReports renders per-harness detection for `init` and
// `harness list`. Capabilities a harness does not provide are printed, not
// omitted: a missing integration has to be visible to be fixable. What is
// printed is the capability's *state* rather than a bare boolean, because
// "unavailable" and "not applicable by design" are different messages to a
// user — and a complete integration says so outright, so a line the user
// cannot act on is never mistaken for a broken install.
func printHarnessReports(reports []core.HarnessReport) {
	for _, r := range reports {
		fmt.Printf("%s integration:\n", r.DisplayName)
		if !r.Detected {
			fmt.Println("- not detected on this device")
			fmt.Println()
			continue
		}
		fmt.Println("- detected")
		for _, c := range harnessCapabilityOrder {
			status, ok := r.CapabilityStatuses[c.key]
			if !ok {
				// A report from an older payload carries booleans only.
				status = core.CapabilityStatus{State: core.CapabilityUnavailable}
				if r.Capabilities[c.key] {
					status.State = core.CapabilityAvailable
				}
			}
			line := fmt.Sprintf("- %s: %s", c.label, status.State)
			if status.Note != "" {
				line += fmt.Sprintf(" (%s)", status.Note)
			}
			fmt.Println(line)
		}
		if r.IntegrationComplete {
			fmt.Printf("- Dossier is fully functional in %s. Nothing further to install.\n", r.DisplayName)
		}
		for _, note := range r.Notes {
			fmt.Printf("- %s\n", note)
		}
		fmt.Println()
	}
}

func formatTime(t time.Time) string {
	if t.IsZero() {
		return "never"
	}
	return t.Format(time.RFC3339)
}

func printConflictDetail(detail core.ConflictDetail) {
	fmt.Printf("Dossier: %s (%s)\nConflict: %s\nKind: %s\nWhen: %s\n\n", detail.DossierName, detail.DossierSlug, detail.Conflict.ID, detail.Conflict.Kind, detail.Conflict.TS.Format(time.RFC3339))
	fmt.Println("Shared (current)")
	fmt.Println(detail.Shared)
	fmt.Println("Yours (preserved)")
	fmt.Println(detail.Mine)
	fmt.Println("Diff")
	fmt.Println(detail.Diff)
}

func printJSON(data any) {
	jsonBytes, err := json.MarshalIndent(data, "", "  ")
	if err != nil {
		fmt.Printf("Error formatting JSON: %v\n", err)
		os.Exit(1)
	}
	fmt.Println(string(jsonBytes))
}

func isHTTPRemote(remote string) bool {
	return strings.HasPrefix(strings.ToLower(strings.TrimSpace(remote)), "http://") || strings.HasPrefix(strings.ToLower(strings.TrimSpace(remote)), "https://")
}

func isInteractiveReader(reader io.Reader) bool {
	file, ok := reader.(*os.File)
	if !ok {
		return false
	}
	info, err := file.Stat()
	return err == nil && info.Mode()&os.ModeCharDevice != 0
}

func printGitHubFallback(w io.Writer, nonInteractive bool) {
	if nonInteractive {
		fmt.Fprintln(w, "Non-interactive input: browser sign-in was not started.")
	}
	fmt.Fprintln(w, "Install GitHub CLI with `brew install gh` (macOS), `winget install --id GitHub.cli` (Windows), or https://cli.github.com, then run the same command again.")
	fmt.Fprintln(w, "Alternatively, write a fine-grained token with Contents read/write to ~/.dossier/credentials (chmod 600).")
}

func ensureRemoteCredentials(cmd *cobra.Command, remote string) error {
	if !isHTTPRemote(remote) {
		return nil
	}
	if _, _, err := sync.GetAuth("", remote); err == nil {
		return nil
	} else if !errors.Is(err, sync.ErrNoCredentials) {
		return err
	}

	installed, loggedIn := sync.GitHubAuthStatus()
	if !installed {
		printGitHubFallback(cmd.ErrOrStderr(), false)
		return errors.New("GitHub credentials are required")
	}
	if loggedIn {
		printGitHubFallback(cmd.ErrOrStderr(), false)
		return errors.New("gh is installed but did not return a token")
	}
	if !isInteractiveReader(cmd.InOrStdin()) {
		printGitHubFallback(cmd.ErrOrStderr(), true)
		return errors.New("GitHub sign-in requires an interactive terminal")
	}

	fmt.Fprint(cmd.OutOrStdout(), "Sign in to GitHub now? Your browser will open. [Y/n] ")
	answer, readErr := bufio.NewReader(cmd.InOrStdin()).ReadString('\n')
	answer = strings.ToLower(strings.TrimSpace(answer))
	if readErr != nil && answer == "" || (answer != "" && answer != "y" && answer != "yes") {
		fmt.Fprintln(cmd.OutOrStdout(), "Sign-in declined.")
		printGitHubFallback(cmd.ErrOrStderr(), false)
		return errors.New("GitHub sign-in was declined")
	}
	if err := sync.GitHubLogin(cmd.InOrStdin(), cmd.OutOrStdout(), cmd.ErrOrStderr()); err != nil {
		fmt.Fprintf(cmd.ErrOrStderr(), "GitHub sign-in failed: %v\n", err)
		printGitHubFallback(cmd.ErrOrStderr(), false)
		return err
	}
	if _, _, err := sync.GetAuth("", remote); err != nil {
		fmt.Fprintln(cmd.ErrOrStderr(), "GitHub sign-in completed, but no token was returned.")
		printGitHubFallback(cmd.ErrOrStderr(), false)
		return err
	}
	return nil
}

func checkRemoteBeforeStore(cmd *cobra.Command, remote string) error {
	if !isHTTPRemote(remote) {
		return nil
	}
	if err := ensureRemoteCredentials(cmd, remote); err != nil {
		return err
	}
	auth, _, err := sync.GetAuth("", remote)
	if err != nil {
		return err
	}
	if err := sync.CheckRemoteAccess(context.Background(), remote, auth); err != nil {
		return errors.New(core.RemoteAccessMessage(remote, err))
	}
	return nil
}

func printJoinRosterMessage(svc *core.Service) {
	roster, err := svc.Members(context.Background())
	if err != nil || (len(roster.Members) == 0 && len(roster.Former) == 0) {
		return
	}
	username, displayName := svc.CurrentUser()
	if roster.Has(username) {
		if memberName := roster.DisplayName(username); strings.TrimSpace(memberName) != "" {
			displayName = memberName
		}
		fmt.Printf("You'll appear to teammates as %s (%s).\n", displayName, username)
		return
	}
	managerName := roster.DisplayName(roster.Manager)
	if managerName == "" {
		managerName = roster.Manager
	}
	fmt.Printf("You're not in the team roster yet. Ask %s to run: dossier team add %s \"Your Name\"\n", managerName, username)
}

type realClock struct{}

func (r *realClock) Now() time.Time {
	return time.Now()
}

func wire(dossierHome string) (*core.Service, error) {
	svc, _, err := wireWithConfig(dossierHome)
	return svc, err
}

func wireWithConfig(dossierHome string) (*core.Service, *config.Config, error) {
	cfgPath := filepath.Join(dossierHome, "config.yaml")
	cfg, err := config.Load(cfgPath)
	if err != nil {
		return nil, nil, err
	}
	return wireWithLoadedConfig(dossierHome, cfg, cfgPath)
}

func wireWithLoadedConfig(dossierHome string, cfg *config.Config, cfgPath string) (*core.Service, *config.Config, error) {
	if canonical, err := harness.NormalizeOpenWith(cfg.OpenWith); err != nil {
		return nil, nil, err
	} else {
		cfg.OpenWith = canonical
	}

	// Write default config to disk if not exists. Team onboarding passes a
	// loaded config with team.remote only in memory and saves it after success.
	if _, err := os.Stat(cfgPath); os.IsNotExist(err) {
		if cfg.Team.Remote == "" {
			if err := cfg.SaveDefault(cfgPath); err != nil {
				return nil, nil, fmt.Errorf("failed to save default config: %w", err)
			}
		}
	}

	storeAdapter := store.NewFSStore(dossierHome)

	// Refresh the on-disk context assets against the ones compiled into this
	// binary. Every command wires through here, so an upgraded binary can never
	// go on reading the previous release's Guide — the failure `dossier init`
	// used to be the only cure for. Two byte comparisons when nothing changed;
	// a store that isn't initialised yet simply has nothing to refresh.
	if _, err := storeAdapter.EnsureContextAssets(); err != nil {
		fmt.Fprintf(os.Stderr, "Warning: failed to refresh context assets: %v\n", err)
	}

	var searchAdapter core.Searcher
	if search.IsRipgrepAvailable() {
		searchAdapter = search.NewRipgrepSearcher(dossierHome)
	} else {
		searchAdapter = search.NewNativeSearcher(dossierHome)
	}

	tokAdapter, err := tokenizer.NewBPETokenizer()
	if err != nil {
		return nil, nil, fmt.Errorf("failed to initialize BPE tokenizer: %w", err)
	}

	hregAdapter := harness.NewRegistry(dossierHome)
	clockAdapter := &realClock{}

	var syncerAdapter core.Syncer
	if cfg.Team.Remote != "" {
		auth, authState, err := sync.GetAuth("", cfg.Team.Remote)
		if errors.Is(err, sync.ErrNoCredentials) {
			fmt.Fprintf(os.Stderr, "Warning: no credentials found for %s\n", cfg.Team.Remote)
		} else if err != nil {
			fmt.Fprintf(os.Stderr, "Warning: failed to load credentials: %v\n", err)
		}
		gs := sync.New(sync.Config{
			AuthorName: cfg.Author,
			RemoteURL:  cfg.Team.Remote,
			StoreDir:   dossierHome,
			Branch:     cfg.Team.Branch,
			Auth:       auth,
			AuthState:  authState,
		})
		syncerAdapter = sync.NewAdapter(gs)
	}

	svc := core.NewService(storeAdapter, searchAdapter, tokAdapter, hregAdapter, clockAdapter, cfg.ToCoreConfig(), syncerAdapter)

	return svc, cfg, nil
}

func expandTilde(path string) string {
	if strings.HasPrefix(path, "~/") {
		home, err := os.UserHomeDir()
		if err == nil {
			return filepath.Join(home, path[2:])
		}
	}
	return path
}

func isDirOnPath(dir string) bool {
	dir = filepath.Clean(expandTilde(dir))
	pathEnv := os.Getenv("PATH")
	for _, p := range filepath.SplitList(pathEnv) {
		if filepath.Clean(expandTilde(p)) == dir {
			return true
		}
	}
	return false
}

func fileSHA256(path string) (string, error) {
	f, err := os.Open(path)
	if err != nil {
		return "", err
	}
	defer f.Close()
	h := sha256.New()
	if _, err := io.Copy(h, f); err != nil {
		return "", err
	}
	return fmt.Sprintf("%x", h.Sum(nil)), nil
}

func isSameFile(src, dest string) bool {
	sInfo, err := os.Stat(src)
	if err != nil {
		return false
	}
	dInfo, err := os.Stat(dest)
	if err != nil {
		return false
	}
	if sInfo.Size() != dInfo.Size() {
		return false
	}
	sHash, err := fileSHA256(src)
	if err != nil {
		return false
	}
	dHash, err := fileSHA256(dest)
	if err != nil {
		return false
	}
	return sHash == dHash
}

func copyFile(src, dest string) error {
	srcFile, err := os.Open(src)
	if err != nil {
		return err
	}
	defer srcFile.Close()

	dir := filepath.Dir(dest)
	if err := os.MkdirAll(dir, 0755); err != nil {
		return err
	}

	tmpFile, err := os.CreateTemp(dir, "dossier-install-*")
	if err != nil {
		return err
	}
	tmpName := tmpFile.Name()
	defer func() {
		if tmpFile != nil {
			tmpFile.Close()
			os.Remove(tmpName)
		}
	}()

	if _, err := io.Copy(tmpFile, srcFile); err != nil {
		return err
	}
	if err := tmpFile.Sync(); err != nil {
		return err
	}
	if err := tmpFile.Close(); err != nil {
		return err
	}
	tmpFile = nil

	if err := os.Chmod(tmpName, 0755); err != nil {
		return err
	}

	if runtime.GOOS == "windows" {
		// Reinstalling over a read-only executable otherwise fails on Windows.
		_ = os.Chmod(dest, 0644)
	}
	if err := os.Rename(tmpName, dest); err != nil {
		return err
	}

	return nil
}

func isVolatilePath(path string) bool {
	path = strings.ToLower(filepath.ToSlash(path))
	if strings.Contains(path, "/tmp/") ||
		strings.Contains(path, "/temp/") ||
		strings.Contains(path, "go-build") ||
		strings.Contains(path, "/var/folders/") {
		return true
	}
	wd, err := os.Getwd()
	if err == nil {
		if strings.HasPrefix(path, strings.ToLower(filepath.ToSlash(wd))) {
			return true
		}
	}
	return false
}

func runInstall(destDir string, yesToAll bool) error {
	execPath, err := os.Executable()
	if err != nil {
		return fmt.Errorf("failed to get current executable path: %w", err)
	}

	destDir = expandTilde(destDir)
	destPath := filepath.Join(destDir, stableBinaryName())

	if !isDirOnPath(destDir) {
		fmt.Printf("Warning: Target directory %s is not in your PATH.\n", destDir)
		if !yesToAll {
			fmt.Printf("Would you like to install to /usr/local/bin instead? [y/N]: ")
			var resp string
			_, _ = fmt.Scanln(&resp)
			resp = strings.ToLower(strings.TrimSpace(resp))
			if resp == "y" || resp == "yes" {
				destDir = filepath.Join(string(os.PathSeparator), "usr", "local", "bin")
				destPath = filepath.Join(destDir, stableBinaryName())
			}
		}
	}

	if isSameFile(execPath, destPath) {
		fmt.Printf("Dossier is already installed and up to date at %s\n", destPath)
		return nil
	}

	if err := os.MkdirAll(destDir, 0755); err != nil {
		return fmt.Errorf("failed to create directory %s: %w", destDir, err)
	}

	err = copyFile(execPath, destPath)
	if err != nil {
		return fmt.Errorf("failed to copy binary to %s: %w", destPath, err)
	}

	fmt.Printf("Dossier successfully installed to %s\n", destPath)
	return nil
}

func stableBinaryName() string {
	if runtime.GOOS == "windows" {
		return "dossier.exe"
	}
	return "dossier"
}

func getStableBinaryPath() string {
	home, err := os.UserHomeDir()
	if err == nil {
		p := filepath.Join(home, ".local", "bin", stableBinaryName())
		if info, err := os.Stat(p); err == nil && !info.IsDir() {
			return p
		}
	}
	if runtime.GOOS != "windows" {
		p2 := filepath.Join(string(os.PathSeparator), "usr", "local", "bin", stableBinaryName())
		if info, err := os.Stat(p2); err == nil && !info.IsDir() {
			return p2
		}
	}
	exec, err := os.Executable()
	if err == nil {
		return exec
	}
	return "dossier"
}

// normalizeLineFlag accepts the shapes a user is likely to paste for a line
// range -- "10-20", "L10-L20", "#L10-L20" -- and returns the citation fragment
// form the service parses.
func normalizeLineFlag(v string) string {
	v = strings.TrimSpace(v)
	v = strings.TrimPrefix(v, "#")
	if v == "" {
		return ""
	}
	parts := strings.SplitN(v, "-", 2)
	for i, p := range parts {
		p = strings.TrimSpace(p)
		if !strings.HasPrefix(p, "L") {
			p = "L" + p
		}
		parts[i] = p
	}
	return strings.Join(parts, "-")
}
