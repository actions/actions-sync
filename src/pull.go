package src

import (
	"context"
	"fmt"
	"os"
	"path"
	"strings"

	"github.com/go-git/go-git/v5"
	"github.com/go-git/go-git/v5/config"
	"github.com/go-git/go-git/v5/plumbing"
	"github.com/spf13/cobra"
)

type PullOnlyFlags struct {
	DefaultBranchOnly bool
	SourceURL         string
}

type PullFlags struct {
	CommonFlags
	PullOnlyFlags
}

func (f *PullFlags) Init(cmd *cobra.Command) {
	f.CommonFlags.Init(cmd)
	f.PullOnlyFlags.Init(cmd)
}

func (f *PullOnlyFlags) Init(cmd *cobra.Command) {
	cmd.Flags().BoolVar(&f.DefaultBranchOnly, "default-branch-only", false, "Only pull the default branch")
	cmd.Flags().StringVar(&f.SourceURL, "source-url", "https://github.com", "The domain to pull from")
}

func (f *PullFlags) Validate() Validations {
	return f.CommonFlags.Validate(true).Join(f.PullOnlyFlags.Validate())
}

func (f *PullOnlyFlags) Validate() Validations {
	var validations Validations
	return validations
}

func Pull(ctx context.Context, flags *PullFlags) error {
	repoNames, err := getRepoNamesFromRepoFlags(&flags.CommonFlags)
	if err != nil {
		return err
	}

	return PullManyWithGitImpl(ctx, flags.SourceURL, flags.CacheDir, flags.DefaultBranchOnly, repoNames, gitImplementation{})
}

func PullManyWithGitImpl(ctx context.Context, sourceURL, cacheDir string, defaultBranchOnly bool, repoNames []string, gitimpl GitImplementation) error {
	for _, repoName := range repoNames {
		if err := PullWithGitImpl(ctx, sourceURL, cacheDir, defaultBranchOnly, repoName, gitimpl); err != nil {
			return err
		}
	}
	return nil
}

func PullWithGitImpl(ctx context.Context, sourceURL, cacheDir string, defaultBranchOnly bool, repoName string, gitimpl GitImplementation) error {
	originRepoName, destRepoName, err := extractSourceDest(repoName)
	if err != nil {
		return err
	}

	_ = os.MkdirAll(cacheDir, 0755)
	_, err = os.Stat(cacheDir)
	if err != nil {
		return err
	}

	dst := path.Join(cacheDir, destRepoName)

	if !gitimpl.RepositoryExists(dst) {
		fmt.Fprintf(os.Stdout, "pulling %s to %s ...\n", originRepoName, dst)
		_, err := gitimpl.CloneRepository(dst, &git.CloneOptions{
			ReferenceName: plumbing.HEAD,
			SingleBranch:  defaultBranchOnly,
			URL:           fmt.Sprintf("%s/%s", sourceURL, originRepoName),
		})
		if err != nil {
			if strings.Contains(err.Error(), "authentication required") {
				return fmt.Errorf("could not pull %s, the repository may require authentication or does not exist", originRepoName)
			}
			return err
		}
	}

	repo, err := gitimpl.NewGitRepository(dst)
	if err != nil {
		return err
	}

	refspec := config.RefSpec("+refs/heads/*:refs/heads/*")
	if defaultBranchOnly {
		refspec = "+refs/tags/*:refs/tags/*"
	}

	fmt.Fprintf(os.Stdout, "fetching * refs for %s ...\n", originRepoName)
	err = repo.FetchContext(ctx, &git.FetchOptions{
		RefSpecs: []config.RefSpec{
			refspec,
		},
		Tags: git.AllTags,
	})
	if err != nil && err != git.NoErrAlreadyUpToDate {
		return err
	}

	return nil
}
