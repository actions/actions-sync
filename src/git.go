package src

import (
	"context"

	"github.com/go-git/go-git/v5"
	"github.com/go-git/go-git/v5/config"
)

// A really thin Git wrapper so we can stub it out in our tests

type GitImplementation interface {
	NewGitRepository(dir string) (GitRepository, error)
	CloneRepository(dir string, o *git.CloneOptions) (GitRepository, error)
	RepositoryExists(dir string) bool
}

type GitRepository interface {
	DeleteRemote(string) error
	CreateRemote(*config.RemoteConfig) (GitRemote, error)
	FetchContext(context.Context, *git.FetchOptions) error
}

type GitRemote interface {
	PushContext(context.Context, *git.PushOptions) error
	Config() *config.RemoteConfig
}

type gitImplementation struct {
}

func (i gitImplementation) NewGitRepository(dir string) (GitRepository, error) {
	gitRepo, err := git.PlainOpen(dir)
	if err != nil {
		return nil, err
	}
	return &gitRepository{gitRepo}, nil
}

func (i gitImplementation) CloneRepository(dir string, o *git.CloneOptions) (GitRepository, error) {
	gitRepo, err := git.PlainClone(dir, false, o)
	if err != nil {
		return nil, err
	}
	return &gitRepository{gitRepo}, nil
}

func (i gitImplementation) RepositoryExists(dir string) bool {
	_, err := git.PlainOpen(dir)
	return err == nil
}

type gitRepository struct {
	inner *git.Repository
}

func (r *gitRepository) DeleteRemote(remote string) error {
	return r.inner.DeleteRemote(remote)
}

func (r *gitRepository) CreateRemote(c *config.RemoteConfig) (GitRemote, error) {
	return r.inner.CreateRemote(c)
}

func (r *gitRepository) FetchContext(ctx context.Context, o *git.FetchOptions) error {
	return r.inner.FetchContext(ctx, o)
}
