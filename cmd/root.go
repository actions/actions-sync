package cmd

import (
	"context"
	"fmt"
	"os"

	"github.com/actions/actions-sync/src"

	"github.com/spf13/cobra"
)

var (
	rootCmd = &cobra.Command{
		Use:   "actions-sync",
		Short: "GHES Actions Sync",
		Long:  "Sync Actions from github.com to a GHES instance.",
	}

	versionCmd = &cobra.Command{
		Use:   "version",
		Short: "The version of actions-sync in use.",
		Run: func(cmd *cobra.Command, args []string) {
			fmt.Fprintln(os.Stdout, "GHES Actions Sync v0.2")
		},
	}

	pushRepoFlags = &src.PushFlags{}
	pushRepoCmd   = &cobra.Command{
		Use:   "push",
		Short: "Push a repo from disk to GHES instance",
		Run: func(cmd *cobra.Command, args []string) {
			if err := pushRepoFlags.Validate().Error(); err != nil {
				fmt.Fprintln(os.Stderr, err.Error())
				_ = cmd.Usage()
				os.Exit(1)
				return
			}
			if err := src.Push(cmd.Context(), pushRepoFlags); err != nil {
				fmt.Fprintln(os.Stderr, err.Error())
				os.Exit(1)
				return
			}
		},
	}

	pullRepoFlags = &src.PullFlags{}
	pullRepoCmd   = &cobra.Command{
		Use:   "pull",
		Short: "Pull a repo from GitHub.com to disk",
		Run: func(cmd *cobra.Command, args []string) {
			if err := pullRepoFlags.Validate().Error(); err != nil {
				fmt.Fprintln(os.Stderr, err.Error())
				_ = cmd.Usage()
				os.Exit(1)
				return
			}
			if err := src.Pull(cmd.Context(), pullRepoFlags); err != nil {
				fmt.Fprintln(os.Stderr, err.Error())
				os.Exit(1)
				return
			}
		},
	}

	syncRepoFlags = &src.SyncFlags{}
	syncRepoCmd   = &cobra.Command{
		Use:   "sync",
		Short: "Sync a repo from GitHub.com to a GHES instance",
		Run: func(cmd *cobra.Command, args []string) {
			if err := syncRepoFlags.Validate().Error(); err != nil {
				fmt.Fprintln(os.Stderr, err.Error())
				_ = cmd.Usage()
				os.Exit(1)
				return
			}
			if err := src.Sync(cmd.Context(), syncRepoFlags); err != nil {
				fmt.Fprintln(os.Stderr, err.Error())
				os.Exit(1)
				return
			}
		},
	}
)

func Execute(ctx context.Context) error {
	rootCmd.AddCommand(versionCmd)

	rootCmd.AddCommand(pushRepoCmd)
	pushRepoFlags.Init(pushRepoCmd)

	rootCmd.AddCommand(pullRepoCmd)
	pullRepoFlags.Init(pullRepoCmd)

	rootCmd.AddCommand(syncRepoCmd)
	syncRepoFlags.Init(syncRepoCmd)

	return rootCmd.ExecuteContext(ctx)
}
