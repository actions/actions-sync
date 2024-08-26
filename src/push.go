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
	"github.com/google/go-github/v43/github"
	"github.com/pkg/errors"
	"github.com/spf13/cobra"
	"golang.org/x/oauth2"
)

const enterpriseAegisVersionHeaderValue = "GitHub AE"
const enterpriseAPIPath = "/api/v3"
const enterpriseVersionHeaderKey = "X-GitHub-Enterprise-Version"
const xOAuthScopesHeader = "X-OAuth-Scopes"

type PushOnlyFlags struct {
	BaseURL, Token, ActionsAdminUser string
	DisableGitAuth, KeepDescription  bool
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
	cmd.Flags().StringVar(&f.ActionsAdminUser, "actions-admin-user", "", "A user to impersonate for the push requests. To use the default name, pass 'actions-admin'. Note that the site_admin scope in the token is required for the impersonation to work.")
	cmd.Flags().StringVar(&f.Token, "destination-token", "", "Token to access API on GHES instance")
	cmd.Flags().BoolVar(&f.DisableGitAuth, "disable-push-git-auth", false, "Disables git authentication whilst pushing")
	cmd.Flags().BoolVar(&f.KeepDescription, "keep-description", false, "Wether to keep the description of the source destination repository or not")
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

func GetImpersonationToken(ctx context.Context, flags *PushFlags) (string, error) {
	fmt.Printf("getting an impersonation token for `%s` ...\n", flags.ActionsAdminUser)

	ts := oauth2.StaticTokenSource(&oauth2.Token{AccessToken: flags.Token})
	tc := oauth2.NewClient(ctx, ts)
	ghClient, err := github.NewEnterpriseClient(flags.BaseURL, flags.BaseURL, tc)
	if err != nil {
		return "", errors.Wrap(err, "error creating enterprise client")
	}

	rootRequest, err := ghClient.NewRequest("GET", enterpriseAPIPath, nil)
	if err != nil {
		return "", errors.Wrap(err, "error constructing request for GitHub Enterprise client.")
	}
	rootResponse, err := ghClient.Do(ctx, rootRequest, nil)
	if err != nil {
		return "", errors.Wrap(err, "error checking connectivity for GitHub Enterprise client.")
	}

	scopesHeader := rootResponse.Header.Get(xOAuthScopesHeader)
	fmt.Printf("these are the scopes we have for the current token `%s` ...\n", scopesHeader)

	if !strings.Contains(scopesHeader, "site_admin") {
		return "", errors.New("the current token doesn't have the `site_admin` scope, the impersonation function requires the `site_admin` permission to be able to impersonate")
	}

	isAE := rootResponse.Header.Get(enterpriseVersionHeaderKey) == enterpriseAegisVersionHeaderValue
	minimumRepositoryScope := "public_repo"
	if isAE {
		// the default repository scope for non-ae instances is 'public_repo'
		// while it is `repo` for ae.
		minimumRepositoryScope = "repo"
		fmt.Printf("running against GitHub AE, changing the repository scope to '%s' ...\n", minimumRepositoryScope)
	}

	impersonationToken, _, err := ghClient.Admin.CreateUserImpersonation(ctx, flags.ActionsAdminUser, &github.ImpersonateUserOptions{Scopes: []string{minimumRepositoryScope, "workflow"}})
	if err != nil {
		return "", errors.Wrap(err, "failed to impersonate Actions admin user.")
	}

	fmt.Printf("got the impersonation token for `%s` ...\n", flags.ActionsAdminUser)

	return impersonationToken.GetToken(), nil
}

func Push(ctx context.Context, flags *PushFlags) error {
	if flags.ActionsAdminUser != "" {
		var token, err = GetImpersonationToken(ctx, flags)
		if err != nil {
			return errors.Wrap(err, "error obtaining the impersonation token")
		}

		// Override the initial token with the one that we got in the exchange
		flags.Token = token
	} else {
		fmt.Print("not using impersonation for the requests \n")
	}

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

	tokenIdentifierSplit := strings.Split(flags.Token, "_")
	if len(tokenIdentifierSplit) != 2 {
		return errors.New("token is not in the expected format")
	}
	tokenIdentifier := tokenIdentifierSplit[0]

	return PushManyWithGitImpl(ctx, flags, tokenIdentifier, repoNames, ghClient, gitImplementation{})
}

func PushManyWithGitImpl(ctx context.Context, flags *PushFlags, tokenIdentifier string, repoNames []string, ghClient *github.Client, gitimpl GitImplementation) error {
	for _, repoName := range repoNames {
		if err := PushWithGitImpl(ctx, flags, tokenIdentifier, repoName, ghClient, gitimpl); err != nil {
			return err
		}
	}
	return nil
}

func PushWithGitImpl(ctx context.Context, flags *PushFlags, tokenIdentifier string, repoName string, ghClient *github.Client, gitimpl GitImplementation) error {
	orig, nwo, err := extractSourceDest(repoName)
	if err != nil {
		return err
	}

	origOwnerName, origRepoName, err := splitNwo(orig)
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
	ghRepo, err := getOrCreateGitHubRepo(ctx, ghClient, tokenIdentifier, bareRepoName, ownerName, origOwnerName, origRepoName, flags.KeepDescription)
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

func getOrCreateGitHubRepo(ctx context.Context, client *github.Client, tokenIdentifier string, repoName, ownerName string, origOwnerName string, origRepoName string, keepDescription bool) (*github.Repository, error) {
	var createRepoOrgName string
	// if the token is a Server-to-Server token (GitHub App), user API is not available
	if tokenIdentifier == "ghs" {
		createRepoOrgName = ownerName
	} else {
		// retrieve user associated to authentication credentials provided
		currentUser, _, err := client.Users.Get(ctx, "")
		if err != nil {
			return nil, errors.Wrap(err, "error retrieving authenticated user")
		}
		if currentUser == nil || currentUser.Login == nil {
			return nil, errors.New("error retrieving authenticated user's login name")
		}

		// check if the owner refers to the authenticated user or an organization.
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
	}

	// check if repository already exists
	ghRepo, resp, err := client.Repositories.Get(ctx, ownerName, repoName)
	if resp == nil || (err != nil && resp.StatusCode != 404) {
		// response is nil or repository neither exists nor not exists
		return nil, errors.Wrapf(err, "error checking repository %s/%s existence", ownerName, repoName)
	}

	if resp.StatusCode == 404 {
		// repo not existing yet - try to create
		visibility := github.String("public")
		if resp.Header.Get(enterpriseVersionHeaderKey) == enterpriseAegisVersionHeaderValue {
			visibility = github.String("internal")
		}

		// always fetch description on new repo creation, prevent SEGV on nil description in upstream repo
		ghRepoDescription := ""
		githubClient := github.NewClient(nil)
		origRepo, _, err := githubClient.Repositories.Get(ctx, origOwnerName, origRepoName)
		if err != nil {
			return nil, errors.Wrapf(err, "error retrieving repository %s/%s", origOwnerName, origRepoName)
		}
		ghRepoDescription = origRepo.GetDescription()

		repo := &github.Repository{
			Name:        github.String(repoName),
			Description: &ghRepoDescription,
			HasIssues:   github.Bool(false),
			HasWiki:     github.Bool(false),
			HasPages:    github.Bool(false),
			HasProjects: github.Bool(false),
			Visibility:  visibility,
		}

		ghRepo, _, err = client.Repositories.Create(ctx, createRepoOrgName, repo)
		if err == nil {
			fmt.Printf("Created repo `%s/%s`\n", ownerName, repoName)
		} else {
			return nil, errors.Wrapf(err, "error creating repository %s/%s", ownerName, repoName)
		}
	} else if resp.StatusCode == 200 && ghRepo != nil {
		// repo exists, update description if keepDescription flag is not set, prevent SEGV on nil description in upstream repo
		ghRepoDescription := ""
		if !keepDescription {
			githubClient := github.NewClient(nil)
			origRepo, _, err := githubClient.Repositories.Get(ctx, origOwnerName, origRepoName)
			if err != nil {
				return nil, errors.Wrapf(err, "error retrieving repository %s/%s", origOwnerName, origRepoName)
			}
			ghRepoDescription = origRepo.GetDescription()
		} else {
			ghRepoDescription = *ghRepo.Description
		}

		repo := &github.Repository{
			Description: &ghRepoDescription,
		}

		ghRepo, _, err = client.Repositories.Edit(ctx, ownerName, repoName, repo)
		if err == nil {
			fmt.Printf("Updated repo `%s/%s`\n", ownerName, repoName)
		} else {
			return nil, errors.Wrapf(err, "error updating repository %s/%s", ownerName, repoName)
		}
	} else {
		return nil, errors.Wrapf(err, "unexpected response status code %d", resp.StatusCode)
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
