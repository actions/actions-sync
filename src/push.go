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
	"github.com/google/go-github/v43/github"
	"github.com/pkg/errors"
	"github.com/spf13/cobra"
	"golang.org/x/oauth2"
)

const enterpriseAegisVersionHeaderValue = "GitHub AE"
const enterpriseAPIPath = "/api/v3"
const enterpriseVersionHeaderKey = "X-GitHub-Enterprise-Version"
const xOAuthScopesHeader = "X-OAuth-Scopes"

// DefaultBatchSize of 0 means no batching (push all refs at once, original behavior)
const DefaultBatchSize = 0

// MinBatchSize is the minimum allowed batch size when batching is enabled
const MinBatchSize = 10

type PushOnlyFlags struct {
	BaseURL, Token, ActionsAdminUser string
	DisableGitAuth                   bool
	BatchSize                        int
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
	cmd.Flags().IntVar(&f.BatchSize, "batch-size", DefaultBatchSize, "Number of refs to push in each batch (0 = no batching). Use a value like 100 if pushing fails for large repositories.")
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
	if f.BatchSize != 0 && f.BatchSize < MinBatchSize {
		validations = append(validations, fmt.Sprintf("--batch-size must be 0 (no batching) or at least %d", MinBatchSize))
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
	ghRepo, err := getOrCreateGitHubRepo(ctx, ghClient, bareRepoName, ownerName)
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

func getOrCreateGitHubRepo(ctx context.Context, client *github.Client, repoName, ownerName string) (*github.Repository, error) {
	// retrieve user associated to authentication credentials provided
	currentUser, userResponse, err := client.Users.Get(ctx, "")
	if err != nil {
		return nil, errors.Wrap(err, "error retrieving authenticated user")
	}
	if currentUser == nil || currentUser.Login == nil {
		return nil, errors.New("error retrieving authenticated user's login name")
	}
	// checking if we talk to GHAE
	isAE := userResponse.Header.Get(enterpriseVersionHeaderKey) == enterpriseAegisVersionHeaderValue

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

	// check if repository already exists
	ghRepo, resp, err := client.Repositories.Get(ctx, ownerName, repoName)

	if err == nil {
		fmt.Printf("Existing repo `%s/%s`\n", ownerName, repoName)
	} else if resp != nil && resp.StatusCode == 404 {
		// repo not existing yet - try to create
		visibility := github.String("public")
		if isAE {
			visibility = github.String("internal")
		}
		repo := &github.Repository{
			Name:        github.String(repoName),
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
	} else if err != nil {
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

	// If batch size is 0 or negative, use original wildcard approach (no batching)
	if flags.BatchSize <= 0 {
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

	// Batching requested - collect all refs and push in batches
	refs, err := collectRefs(gitRepo)
	if err != nil {
		return errors.Wrap(err, "error collecting refs")
	}

	return pushRefsInBatches(ctx, remote, refs, flags.BatchSize, auth, ghRepo.GetCloneURL())
}

// collectRefs gathers all branch and tag refs from the repository
func collectRefs(gitRepo GitRepository) ([]plumbing.ReferenceName, error) {
	refIter, err := gitRepo.References()
	if err != nil {
		return nil, err
	}

	var refs []plumbing.ReferenceName
	err = refIter.ForEach(func(ref *plumbing.Reference) error {
		name := ref.Name()
		// Only include branches and tags
		if name.IsBranch() || name.IsTag() {
			refs = append(refs, name)
		}
		return nil
	})
	if err != nil {
		return nil, err
	}

	return refs, nil
}

// pushRefsInBatches pushes refs in smaller batches to avoid server-side limits
func pushRefsInBatches(ctx context.Context, remote GitRemote, refs []plumbing.ReferenceName, batchSize int, auth transport.AuthMethod, cloneURL string) error {
	totalRefs := len(refs)

	for i := 0; i < totalRefs; i += batchSize {
		end := i + batchSize
		if end > totalRefs {
			end = totalRefs
		}

		batch := refs[i:end]
		refSpecs := make([]config.RefSpec, len(batch))
		for j, ref := range batch {
			// Create a refspec like "+refs/heads/main:refs/heads/main"
			refSpecs[j] = config.RefSpec("+" + ref.String() + ":" + ref.String())
		}

		err := remote.PushContext(ctx, &git.PushOptions{
			RemoteName: remote.Config().Name,
			RefSpecs:   refSpecs,
			Auth:       auth,
		})

		if err != nil {
			if errors.Cause(err) == git.NoErrAlreadyUpToDate {
				// This batch was already up to date, continue to next batch
				continue
			}
			return errors.Wrapf(err, "failed to push batch %d-%d of %d refs to repo: %s", i+1, end, totalRefs, cloneURL)
		}
	}

	return nil
}
