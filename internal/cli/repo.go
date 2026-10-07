package cli

import (
	"context"
	"fmt"
	"io"

	"github.com/spf13/cobra"

	"dossier/internal/core"
)

// newRepoCmd builds `dossier repo`: the repos a Dossier's work lives in, named
// by remote identity and resolved to a checkout on each machine (ADR 0015).
func newRepoCmd() *cobra.Command {
	repoCmd := &cobra.Command{
		Use:   "repo",
		Short: "Manage the repos a dossier's work lives in (agents start in the primary one)",
	}

	addCmd := &cobra.Command{
		Use:   "add <slug-or-id> <remote|identity|path>",
		Short: "Add a repo by remote URL, host/owner/name, or a local checkout path",
		Long: "Add a repo to the dossier. The first repo is the primary one: agents launched from Dossier start in it.\n" +
			"A local checkout path (e.g. `.`) uses its origin remote and records where it lives on this machine.",
		Args:         cobra.ExactArgs(2),
		SilenceUsage: true,
		RunE: func(cmd *cobra.Command, args []string) error {
			svc, err := wire(resolveHomeDir())
			if err != nil {
				return err
			}
			res, err := svc.RepoAdd(context.Background(), core.RepoAddReq{Actor: actorForCLI(svc), ID: args[0], Ref: args[1]})
			if err != nil {
				return err
			}
			printWarnings(cmd.ErrOrStderr(), res.Warnings)
			return printRepoStatus(cmd, svc, args[0])
		},
	}

	removeCmd := &cobra.Command{
		Use:          "remove <slug-or-id> <remote|identity|path>",
		Short:        "Remove a repo from the dossier",
		Args:         cobra.ExactArgs(2),
		SilenceUsage: true,
		RunE: func(cmd *cobra.Command, args []string) error {
			svc, err := wire(resolveHomeDir())
			if err != nil {
				return err
			}
			res, err := svc.RepoRemove(context.Background(), core.RepoRemoveReq{Actor: actorForCLI(svc), ID: args[0], Ref: args[1]})
			if err != nil {
				return err
			}
			printWarnings(cmd.ErrOrStderr(), res.Warnings)
			return printRepoStatus(cmd, svc, args[0])
		},
	}

	locateCmd := &cobra.Command{
		Use:          "locate <slug-or-id> <path>",
		Short:        "Record where one of the dossier's repos is checked out on this machine",
		Args:         cobra.ExactArgs(2),
		SilenceUsage: true,
		RunE: func(cmd *cobra.Command, args []string) error {
			svc, err := wire(resolveHomeDir())
			if err != nil {
				return err
			}
			res, err := svc.RepoLocate(context.Background(), args[0], args[1])
			if err != nil {
				return err
			}
			r := res.Data.(core.ResolvedRepo)
			fmt.Fprintf(cmd.OutOrStdout(), "%s is at %s on this machine.\n", r.Identity, r.Path)
			return nil
		},
	}

	statusCmd := &cobra.Command{
		Use:          "status <slug-or-id>",
		Short:        "Show the dossier's repos and where each resolves on this machine",
		Args:         cobra.ExactArgs(1),
		SilenceUsage: true,
		RunE: func(cmd *cobra.Command, args []string) error {
			svc, err := wire(resolveHomeDir())
			if err != nil {
				return err
			}
			return printRepoStatus(cmd, svc, args[0])
		},
	}

	repoCmd.AddCommand(addCmd, removeCmd, locateCmd, statusCmd)
	return repoCmd
}

func printRepoStatus(cmd *cobra.Command, svc *core.Service, id string) error {
	res, err := svc.RepoStatus(context.Background(), id)
	if err != nil {
		return err
	}
	repos, _ := res.Data.([]core.ResolvedRepo)
	out := cmd.OutOrStdout()
	if len(repos) == 0 {
		fmt.Fprintln(out, "No repos. Agents start in the dossier folder.")
		return nil
	}
	for i, r := range repos {
		role := ""
		if i == 0 {
			role = " (primary)"
		}
		switch {
		case r.Path != "":
			fmt.Fprintf(out, "%s%s  %s  [%s]\n", r.Identity, role, r.Path, r.Via)
		default:
			fmt.Fprintf(out, "%s%s  not on this machine\n", r.Identity, role)
		}
	}
	printWarnings(cmd.ErrOrStderr(), res.Warnings)
	return nil
}

func printWarnings(w io.Writer, warnings []core.Warning) {
	for _, warning := range warnings {
		fmt.Fprintf(w, "Warning: %s\n", warning)
	}
}
