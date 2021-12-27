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
	ghRepo, err := createOrEditGitHubRepo(ctx, ghClient, bareRepoName, ownerName)
	if err != nil {
		return errors.Wrapf(err, "error creating or editing github repository `%s`", nwo)
	}
	err = syncWithCachedRepository(ctx, flags, ghRepo, repoDirPath, gitimpl)
	if err != nil {
		return errors.Wrapf(err, "error syncing repository `%s`", nwo)
	}
	fmt.Printf("successfully synced `%s`\n", nwo)
	return nil
}

func createOrEditGitHubRepo(ctx context.Context, client *github.Client, repoName, ownerName string) (*github.Repository, error) {
	repo := &github.Repository{
		Name:        github.String(repoName),
		HasIssues:   github.Bool(false),
		HasWiki:     github.Bool(false),
		HasPages:    github.Bool(false),
		HasProjects: github.Bool(false),
	}

	currentUser, _, err := client.Users.Get(ctx, "")
	if err != nil {
		return nil, errors.Wrap(err, "error retrieving authenticated user")
	}
	if currentUser == nil || currentUser.Login == nil {
		return nil, errors.New("error retrieving authenticated user's login name")
	}

	// check if the owner refers to the authenticated user or an organization.
	var createRepoOrgName string
	if strings.EqualFold(*currentUser.Login, ownerName) {
		// we'll create the repo under the authenticated user's account.
		createRepoOrgName = ""
	} else {
		// ensure the org exists.
		createRepoOrgName = ownerName
		_, err := getOrCreateGitHubOrg(ctx, client, ownerName, *currentUser.Login)
		if err != nil {
			return nil, err
		}
	}

	ghRepo, resp, err := client.Repositories.Create(ctx, createRepoOrgName, repo)
	if err == nil {
		fmt.Printf("Created repo `%s/%s`\n", ownerName, repoName)
	} else if resp != nil && resp.StatusCode == 422 {
		ghRepo, _, err = client.Repositories.Edit(ctx, ownerName, repoName, repo)
	}
	if err != nil {
		return nil, errors.Wrapf(err, "error creating repository %s/%s", ownerName, repoName)
	}
	if ghRepo == nil {
		return nil, errors.New("error repository is nil")
	}
	return ghRepo, nil
}

func getOrCreateGitHubOrg(ctx context.Context, client *github.Client, orgName, admin string) (*github.Organization, error) {
	org := &github.Organization{Login: &orgName}

	var getErr error
	ghOrg, _, createErr := client.Admin.CreateOrg(ctx, org, admin)
	if createErr == nil {
		fmt.Printf("Created organization `%s` (admin: %s)\n", orgName, admin)
	} else {
		// Regardless of why create failed, see if we can retrieve the org
		ghOrg, _, getErr = client.Organizations.Get(ctx, orgName)
	}
	if createErr != nil && getErr != nil {
		return nil, errors.Wrapf(createErr, "error creating organization %s", orgName)
	}
	if ghOrg == nil {
		return nil, errors.New("error organization is nil")
	}

	return ghOrg, nil
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
