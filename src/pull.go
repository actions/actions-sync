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
	"github.com/go-git/go-git/v5/plumbing/transport"
	"github.com/go-git/go-git/v5/plumbing/transport/http"
	"github.com/spf13/cobra"
)

type PullOnlyFlags struct {
	SourceURL, Token  string
	DefaultBranchOnly bool
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
	cmd.Flags().StringVar(&f.Token, "source-token", "", "Token used to authenticate against the source when pulling private repositories. Works with a personal access token or a GitHub App installation token (ghs_*).")
	cmd.Flags().BoolVar(&f.DefaultBranchOnly, "default-branch-only", false, "Only synchronize the default branch rather than all branches")
}

func (f *PullFlags) Validate() Validations {
	return f.CommonFlags.Validate(true).Join(f.PullOnlyFlags.Validate())
}

func (f *PullOnlyFlags) Validate() Validations {
	var validations Validations
	if f.Token != "" && !strings.HasPrefix(strings.ToLower(f.SourceURL), "https://") {
		validations = append(validations, "--source-token requires an https:// --source-url so the token is sent over a secure transport")
	}
	return validations
}

// gitAuthMethod returns a BasicAuth transport for the given token, or nil when
// no token is set (anonymous access).
func gitAuthMethod(token string) transport.AuthMethod {
	if token == "" {
		return nil
	}
	return &http.BasicAuth{
		Username: "x-access-token",
		Password: token,
	}
}

func Pull(ctx context.Context, flags *PullFlags) error {
	repoNames, err := getRepoNamesFromRepoFlags(&flags.CommonFlags)
	if err != nil {
		return err
	}

	return PullManyWithGitImpl(ctx, flags.SourceURL, gitAuthMethod(flags.Token), flags.CacheDir, flags.DefaultBranchOnly, repoNames, gitImplementation{})
}

func PullManyWithGitImpl(ctx context.Context, sourceURL string, auth transport.AuthMethod, cacheDir string, defaultBranchOnly bool, repoNames []string, gitimpl GitImplementation) error {
	for _, repoName := range repoNames {
		if err := PullWithGitImpl(ctx, sourceURL, auth, cacheDir, defaultBranchOnly, repoName, gitimpl); err != nil {
			return err
		}
	}
	return nil
}

func PullWithGitImpl(ctx context.Context, sourceURL string, auth transport.AuthMethod, cacheDir string, defaultBranchOnly bool, repoName string, gitimpl GitImplementation) error {
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
			ReferenceName: plumbing.HEAD,
			SingleBranch:  defaultBranchOnly,
			URL:           fmt.Sprintf("%s/%s", sourceURL, originRepoName),
			Auth:          auth,
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

	// By default we mirror every remote head. When limiting to the default
	// branch we resolve HEAD (the branch the clone checked out) and refresh
	// only that branch, so re-syncs keep the default branch up to date without
	// pulling down or updating any other branches. Tags are always synced via
	// Tags: git.AllTags below.
	refSpecs := []config.RefSpec{config.RefSpec("+refs/heads/*:refs/heads/*")}
	fetchDesc := "all branches and tags"
	if defaultBranchOnly {
		refSpec, err := defaultBranchRefSpec(repo)
		if err != nil {
			return err
		}
		refSpecs = []config.RefSpec{refSpec}
		fetchDesc = "the default branch and tags"
	}

	fmt.Fprintf(os.Stdout, "fetching %s for %s ...\n", fetchDesc, originRepoName)
	err = repo.FetchContext(ctx, &git.FetchOptions{
		RefSpecs: refSpecs,
		Auth:     auth,
		Tags:     git.AllTags,
	})
	if err != nil && err != git.NoErrAlreadyUpToDate {
		if strings.Contains(err.Error(), "authentication required") {
			return fmt.Errorf("could not fetch %s, the repository may require authentication or does not exist", originRepoName)
		}
		return err
	}

	return nil
}

// defaultBranchRefSpec resolves the repository's HEAD to the default branch and
// returns a refspec that updates only that branch. This keeps the cached
// default branch current on re-syncs while leaving any other branches that may
// already be in the cache untouched.
func defaultBranchRefSpec(repo GitRepository) (config.RefSpec, error) {
	head, err := repo.Head()
	if err != nil {
		return "", fmt.Errorf("could not resolve the default branch: %w", err)
	}

	name := head.Name()
	if !name.IsBranch() {
		return "", fmt.Errorf("HEAD does not point at a branch (%s)", name)
	}

	return config.RefSpec(fmt.Sprintf("+%s:%s", name, name)), nil
}
