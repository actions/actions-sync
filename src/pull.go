package src

import (
	"context"
	"errors"
	"fmt"
	"os"
	"path"
	"strings"

	"github.com/go-git/go-git/v5"
	"github.com/go-git/go-git/v5/config"
	"github.com/go-git/go-git/v5/plumbing/transport/http"
	"github.com/spf13/cobra"
)

var ErrNoAuth = fmt.Errorf("no authentication configured")
var ErrPartialAuth = fmt.Errorf("both Username and Token must be set")

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
	if !f.hasUserAuth() && !f.hasNoUserAuth() {
		validations = append(validations, "to authenticate, both --source-token and --source-user must be set")
	}
	return validations
}

func (f *PullOnlyFlags) hasUserAuth() bool {
	return f.Username != "" && f.Token != ""
}

func (f *PullOnlyFlags) hasNoUserAuth() bool {
	return f.Username == "" && f.Token == ""
}

func (f *PullOnlyFlags) UserAuth() (http.AuthMethod, error) {
	if f.hasUserAuth() {
		return &http.BasicAuth{
			Username: f.Username,
			Password: f.Token,
		}, nil
	}

	if f.hasNoUserAuth() {
		return nil, ErrNoAuth
	}

	return nil, ErrPartialAuth
}

func Pull(ctx context.Context, flags *PullFlags) error {
	repoNames, err := getRepoNamesFromRepoFlags(&flags.CommonFlags)
	if err != nil {
		return err
	}

	userAuth, err := flags.UserAuth()
	if errors.Is(err, ErrPartialAuth) {
		return err
	}

	return PullManyWithGitImpl(ctx, flags.SourceURL, flags.CacheDir, userAuth, repoNames, gitImplementation{})
}

func PullManyWithGitImpl(ctx context.Context, sourceURL, cacheDir string, userAuth http.AuthMethod, repoNames []string, gitimpl GitImplementation) error {
	for _, repoName := range repoNames {
		if err := PullWithGitImpl(ctx, sourceURL, cacheDir, repoName, userAuth, gitimpl); err != nil {
			return err
		}
	}
	return nil
}

func PullWithGitImpl(ctx context.Context, sourceURL, cacheDir, repoName string, userAuth http.AuthMethod, gitimpl GitImplementation) error {
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
			Auth:         userAuth,
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
		Auth: userAuth,
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
