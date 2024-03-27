package src

import (
	"context"
	"fmt"
	"os"
	"path"
	"strings"

	"github.com/go-git/go-git/v5"
	"github.com/go-git/go-git/v5/config"
	"github.com/go-git/go-git/v5/plumbing/transport/http"
	"github.com/spf13/cobra"
)

type PullOnlyFlags struct {
	Username, Token, SourceURL string
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
	cmd.Flags().StringVar(&f.SourceURL, "source-url", "https://github.com", "The domain to pull from")
	cmd.Flags().StringVar(&f.Token, "source-token", "", "The optional token to use to pull the repository")
	cmd.Flags().StringVar(&f.Username, "source-user", "", "The optional username to use to pull the repository")
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

	return PullManyWithGitImpl(ctx, flags.SourceURL, flags.Username, flags.Token, flags.CacheDir, repoNames, gitImplementation{})
}

func PullManyWithGitImpl(ctx context.Context, sourceURL, sourceUsername, sourceToken, cacheDir string, repoNames []string, gitimpl GitImplementation) error {
	for _, repoName := range repoNames {
		if err := PullWithGitImpl(ctx, sourceURL, sourceUsername, sourceToken, cacheDir, repoName, gitimpl); err != nil {
			return err
		}
	}
	return nil
}

func PullWithGitImpl(ctx context.Context, sourceURL, sourceUsername, sourceToken, cacheDir string, repoName string, gitimpl GitImplementation) error {
	originRepoName, destRepoName, err := extractSourceDest(repoName)
	if err != nil {
		return err
	}

	_, err = os.Stat(cacheDir)
	if err != nil {
		return err
	}

	dst := path.Join(cacheDir, destRepoName)

	if !gitimpl.RepositoryExists(dst) {
		fmt.Fprintf(os.Stdout, "pulling %s to %s ...\n", originRepoName, dst)
		_, err := gitimpl.CloneRepository(dst, &git.CloneOptions{
			Auth: &http.BasicAuth{
				Username: sourceUsername,
				Password: sourceToken,
			},
			SingleBranch: false,
			URL:          fmt.Sprintf("%s/%s", sourceURL, originRepoName),
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

	fmt.Fprintf(os.Stdout, "fetching * refs for %s ...\n", originRepoName)
	err = repo.FetchContext(ctx, &git.FetchOptions{
		Auth: &http.BasicAuth{
			Username: sourceUsername,
			Password: sourceToken,
		},
		RefSpecs: []config.RefSpec{
			config.RefSpec("+refs/heads/*:refs/heads/*"),
		},
		Tags: git.AllTags,
	})
	if err != nil && err != git.NoErrAlreadyUpToDate {
		return err
	}

	return nil
}
