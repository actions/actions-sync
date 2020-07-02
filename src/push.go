package src

import (
	"context"
	"fmt"
	"io/ioutil"
	"path"

	"github.com/go-git/go-git/v5"
	"github.com/go-git/go-git/v5/config"
	"github.com/go-git/go-git/v5/plumbing/transport"
	"github.com/go-git/go-git/v5/plumbing/transport/http"
	"github.com/google/go-github/v31/github"
	"github.com/pkg/errors"
	"github.com/spf13/cobra"
	"golang.org/x/oauth2"
)

type PushFlags struct {
	BaseURL, Token string
	DisableGitAuth bool
}

func (f *PushFlags) Init(cmd *cobra.Command) {
	cmd.Flags().StringVar(&f.BaseURL, "destination-url", "", "URL of GHES instance")
	cmd.Flags().StringVar(&f.Token, "destination-token", "", "Token to access API on GHES instance")
	cmd.Flags().BoolVar(&f.DisableGitAuth, "disable-push-git-auth", false, "Disables git authentication whilst pushing")
}

func (f *PushFlags) Validate() Validations {
	var validations Validations
	if f.BaseURL == "" {
		validations = append(validations, "-baseURL must be set")
	}
	if f.Token == "" {
		validations = append(validations, "-token must be set")
	}
	return validations
}

func Push(ctx context.Context, cacheDir string, flags *PushFlags) error {
	return PushWithGitImpl(ctx, cacheDir, flags, gitImplementation{})
}

func PushWithGitImpl(ctx context.Context, cacheDir string, flags *PushFlags, gitimpl GitImplementation) error {
	ts := oauth2.StaticTokenSource(&oauth2.Token{AccessToken: flags.Token})
	tc := oauth2.NewClient(ctx, ts)
	ghClient, err := github.NewEnterpriseClient(flags.BaseURL, flags.BaseURL, tc)
	if err != nil {
		return errors.Wrap(err, "error creating enterprise client")
	}

	orgDirs, err := ioutil.ReadDir(cacheDir)
	if err != nil {
		return errors.Wrapf(err, "error opening cache directory `%s`", cacheDir)
	}
	for _, orgDir := range orgDirs {
		orgDirPath := path.Join(cacheDir, orgDir.Name())
		if !orgDir.IsDir() {
			return errors.Errorf("unexpected file in root of cache directory `%s`", orgDirPath)
		}
		repoDirs, err := ioutil.ReadDir(orgDirPath)
		if err != nil {
			return errors.Wrapf(err, "error opening repository cache directory `%s`", orgDirPath)
		}
		for _, repoDir := range repoDirs {
			repoDirPath := path.Join(orgDirPath, repoDir.Name())
			nwo := fmt.Sprintf("%s/%s", orgDir.Name(), repoDir.Name())
			if !orgDir.IsDir() {
				return errors.Errorf("unexpected file in cache directory `%s`", nwo)
			}
			fmt.Printf("syncing `%s`\n", nwo)
			ghRepo, err := getOrCreateGitHubRepo(ctx, ghClient, repoDir.Name(), orgDir.Name())
			if err != nil {
				return errors.Wrapf(err, "error creating github repository `%s`", nwo)
			}
			err = syncWithCachedRepository(ctx, cacheDir, flags, ghRepo, repoDirPath, gitimpl)
			if err != nil {
				return errors.Wrapf(err, "error syncing repository `%s`", nwo)
			}
			fmt.Printf("successfully synced `%s`\n", nwo)
		}
	}
	return nil
}

func getOrCreateGitHubRepo(ctx context.Context, client *github.Client, repoName, orgName string) (*github.Repository, error) {
	repo := &github.Repository{
		Name:        github.String(repoName),
		HasIssues:   github.Bool(false),
		HasWiki:     github.Bool(false),
		HasPages:    github.Bool(false),
		HasProjects: github.Bool(false),
	}
	ghRepo, resp, err := client.Repositories.Create(ctx, orgName, repo)
	if resp.StatusCode == 422 {
		ghRepo, _, err = client.Repositories.Get(ctx, orgName, repoName)
	}
	if err != nil {
		return nil, errors.Wrap(err, "error creating repository")
	}
	if ghRepo == nil {
		return nil, errors.New("error repository is nil")
	}
	return ghRepo, nil
}

func syncWithCachedRepository(ctx context.Context, cacheDir string, flags *PushFlags, ghRepo *github.Repository, repoDir string, gitimpl GitImplementation) error {
	gitRepo, err := gitimpl.NewGitRepository(repoDir)
	if err != nil {
		return errors.Wrapf(err, "error opening git repository %s", cacheDir)
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
