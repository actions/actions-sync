package src

import (
	"context"
	"fmt"
	"os"
	"path"
	"strings"

	"github.com/go-git/go-git/v5"
	"github.com/go-git/go-git/v5/config"
	"github.com/go-git/go-git/v5/plumbing/transport"
	"github.com/go-git/go-git/v5/plumbing/transport/http"
	"github.com/google/go-github/v31/github"
	"github.com/pkg/errors"
	"github.com/spf13/cobra"
	"golang.org/x/oauth2"
)

type PushOnlyFlags struct {
	BaseURL, Token string
	DisableGitAuth bool
}

type PushFlags struct {
	CommonFlags
	PushOnlyFlags
}

func (f *PushFlags) Init(cmd *cobra.Command) {
	f.CommonFlags.Init(cmd)
	f.PushOnlyFlags.Init(cmd)
}

func (f *PushOnlyFlags) Init(cmd *cobra.Command) {
	cmd.Flags().StringVar(&f.BaseURL, "destination-url", "", "URL of GHES instance")
	cmd.Flags().StringVar(&f.Token, "destination-token", "", "Token to access API on GHES instance")
	cmd.Flags().BoolVar(&f.DisableGitAuth, "disable-push-git-auth", false, "Disables git authentication whilst pushing")
}

func (f *PushFlags) Validate() Validations {
	return f.CommonFlags.Validate(false).Join(f.PushOnlyFlags.Validate())
}

func (f *PushOnlyFlags) Validate() Validations {
	var validations Validations
	if f.BaseURL == "" {
		validations = append(validations, "--destination-url must be set")
	}
	if f.Token == "" {
		validations = append(validations, "--destination-token must be set")
	}
	return validations
}

func Push(ctx context.Context, flags *PushFlags) error {
	ts := oauth2.StaticTokenSource(&oauth2.Token{AccessToken: flags.Token})
	tc := oauth2.NewClient(ctx, ts)
	ghClient, err := github.NewEnterpriseClient(flags.BaseURL, flags.BaseURL, tc)
	if err != nil {
		return errors.Wrap(err, "error creating enterprise client")
	}

	repoNames, err := getRepoNamesFromRepoFlags(&flags.CommonFlags)
	if err != nil {
		return err
	}

	if repoNames == nil {
		repoNames, err = getRepoNamesFromCacheDir(&flags.CommonFlags)
		if err != nil {
			return err
		}
	}

	return PushManyWithGitImpl(ctx, flags, repoNames, ghClient, gitImplementation{})
}

func PushManyWithGitImpl(ctx context.Context, flags *PushFlags, repoNames []string, ghClient *github.Client, gitimpl GitImplementation) error {
	for _, repoName := range repoNames {
		if err := PushWithGitImpl(ctx, flags, repoName, ghClient, gitimpl); err != nil {
			return err
		}
	}
	return nil
}

func PushWithGitImpl(ctx context.Context, flags *PushFlags, repoName string, ghClient *github.Client, gitimpl GitImplementation) error {
	_, nwo, err := extractSourceDest(repoName)
	if err != nil {
		return err
	}

	ownerName, bareRepoName, err := splitNwo(nwo)
	if err != nil {
		return err
	}

	repoDirPath := path.Join(flags.CacheDir, nwo)
	_, err = os.Stat(repoDirPath)
	if err != nil {
		return err
	}

	fmt.Printf("syncing `%s`\n", nwo)
	ghRepo, err := getOrCreateGitHubRepo(ctx, flags, ghClient, bareRepoName, ownerName)
	if err != nil {
		return errors.Wrapf(err, "error creating github repository `%s`", nwo)
	}
	err = syncWithCachedRepository(ctx, flags, ghRepo, repoDirPath, gitimpl)
	if err != nil {
		return errors.Wrapf(err, "error syncing repository `%s`", nwo)
	}
	fmt.Printf("successfully synced `%s`\n", nwo)
	return nil
}

func getOrCreateGitHubRepo(ctx context.Context, flags *PushFlags, client *github.Client, repoName, ownerName string) (*github.Repository, error) {
	repo := &github.Repository{
		Name:        github.String(repoName),
		HasIssues:   github.Bool(false),
		HasWiki:     github.Bool(false),
		HasPages:    github.Bool(false),
		HasProjects: github.Bool(false),
	}

	orgName := ownerName

	// Confirm the org exists
	_, resp, err := client.Organizations.Get(ctx, orgName)
	if resp != nil && resp.StatusCode == 404 {
		// Check if the destination owner matches the authenticated user. (best effort)
		currentUser, _, _ := client.Users.Get(ctx, "")
		if currentUser != nil && strings.EqualFold(*currentUser.Login, ownerName) {
			// create the new repo under the authenticated user's account.
			orgName = ""
			err = nil
		} else {
			return nil, errors.Errorf("Organization `%s` doesn't exist at %s. You must create it first.", ownerName, flags.BaseURL)
		}
	}
	if err != nil {
		return nil, errors.Wrapf(err, "error retrieving organization %s", ownerName)
	}

	// Create the repo if necessary
	ghRepo, resp, err := client.Repositories.Create(ctx, orgName, repo)
	if resp != nil && resp.StatusCode == 422 {
		ghRepo, _, err = client.Repositories.Get(ctx, ownerName, repoName)
	}
	if err != nil {
		return nil, errors.Wrap(err, "error creating repository")
	}
	if ghRepo == nil {
		return nil, errors.New("error repository is nil")
	}
	return ghRepo, nil
}

func syncWithCachedRepository(ctx context.Context, flags *PushFlags, ghRepo *github.Repository, repoDir string, gitimpl GitImplementation) error {
	gitRepo, err := gitimpl.NewGitRepository(repoDir)
	if err != nil {
		return errors.Wrapf(err, "error opening git repository %s", repoDir)
	}
	_ = gitRepo.DeleteRemote("ghes")
	remote, err := gitRepo.CreateRemote(&config.RemoteConfig{
		Name: "ghes",
		URLs: []string{ghRepo.GetCloneURL()},
	})
	if err != nil {
		return errors.Wrap(err, "error creating remote")
	}

	var auth transport.AuthMethod
	if !flags.DisableGitAuth {
		auth = &http.BasicAuth{
			Username: "username",
			Password: flags.Token,
		}
	}
	err = remote.PushContext(ctx, &git.PushOptions{
		RemoteName: remote.Config().Name,
		RefSpecs: []config.RefSpec{
			"+refs/heads/*:refs/heads/*",
			"+refs/tags/*:refs/tags/*",
		},
		Auth: auth,
	})
	if errors.Cause(err) == git.NoErrAlreadyUpToDate {
		return nil
	}
	return errors.Wrapf(err, "failed to push to repo: %s", ghRepo.GetCloneURL())
}
