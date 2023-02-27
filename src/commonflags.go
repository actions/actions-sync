package src

import (
	"github.com/spf13/cobra"
)

// flags common to pull, push and sync operations
type CommonFlags struct {
    CacheDir, RepoName, RepoNameList, RepoNameListFile, GHPatToken string
	PackageSync bool
}

func (f *CommonFlags) Init(cmd *cobra.Command) {
	cmd.Flags().StringVar(&f.CacheDir, "cache-dir", "", "Directory containing the repositories cache created by the `pull` command")
	_ = cmd.MarkFlagRequired("cache-dir")

    cmd.Flags().StringVar(&f.RepoName, "repo-name", "", "Single repository name to pull")
    cmd.Flags().StringVar(&f.RepoNameList, "repo-name-list", "", "Comma delimited list of repository names to pull")
    cmd.Flags().StringVar(&f.RepoNameListFile, "repo-name-list-file", "", "Path to file containing a list of repository names to pull")
    cmd.Flags().BoolVar(&f.PackageSync, "enable-packages-sync", false, "Flag to enable syncing packages for the repository") //This flag is by default false for now as this feature would go GA in GHES 3.10
    cmd.Flags().StringVar(&f.GHPatToken, "gh-pat-token","","GitHub.com PAT token for packages pull")
	
}

func (f *CommonFlags) Validate(reposRequired bool) Validations {
	var validations Validations
	if reposRequired && !f.HasAtLeastOneRepoFlag() {
		validations = append(validations, "one of --repo-name, --repo-name-list, --repo-name-list-file must be set")
	}
	if f.PackageSync == true && f.GHPatToken == "" {
		validations = append(validations, "gh-pat-token must be set if packages is set to true")
	}
	return validations
}

func (f *CommonFlags) HasAtLeastOneRepoFlag() bool {
	return f.RepoName != "" || f.RepoNameList != "" || f.RepoNameListFile != ""
}
