package src

import (
	"context"
	"testing"

	"github.com/go-git/go-git/v5"
	"github.com/go-git/go-git/v5/config"
	"github.com/go-git/go-git/v5/plumbing/storer"
	"github.com/stretchr/testify/assert"
)

// Tests for GitRepository interface and implementations

func TestGitRepositoryInterface(t *testing.T) {
	// This test verifies that our mock implements the GitRepository interface
	var _ GitRepository = &mockGitRepository{}
}

func TestGitRemoteInterface(t *testing.T) {
	// This test verifies that our mock implements the GitRemote interface
	var _ GitRemote = &mockGitRemote{}
}

// Ensure the mockGitRepository implements all methods of GitRepository
func TestMockGitRepository_DeleteRemote(t *testing.T) {
	repo := &mockGitRepository{}
	err := repo.DeleteRemote("origin")
	assert.NoError(t, err)
}

func TestMockGitRepository_CreateRemote(t *testing.T) {
	repo := &mockGitRepository{}
	remote, err := repo.CreateRemote(&config.RemoteConfig{Name: "test"})
	assert.NoError(t, err)
	assert.Nil(t, remote)
}

func TestMockGitRepository_FetchContext(t *testing.T) {
	repo := &mockGitRepository{}
	err := repo.FetchContext(context.Background(), &git.FetchOptions{})
	assert.NoError(t, err)
}

func TestMockGitRepository_References(t *testing.T) {
	repo := &mockGitRepository{}
	refs, err := repo.References()
	assert.NoError(t, err)
	assert.NotNil(t, refs)

	// Verify it returns a valid iterator
	_, ok := refs.(storer.ReferenceIter)
	assert.True(t, ok)
}

// Ensure the mockGitRemote implements all methods of GitRemote
func TestMockGitRemote_PushContext(t *testing.T) {
	remote := &mockGitRemote{}
	err := remote.PushContext(context.Background(), &git.PushOptions{})
	assert.NoError(t, err)
}

func TestMockGitRemote_Config(t *testing.T) {
	remote := &mockGitRemote{}
	cfg := remote.Config()
	assert.NotNil(t, cfg)
	assert.Equal(t, "test-remote", cfg.Name)

	// Test with custom config
	customRemote := &mockGitRemote{
		remoteConfig: &config.RemoteConfig{Name: "custom-remote"},
	}
	cfg = customRemote.Config()
	assert.Equal(t, "custom-remote", cfg.Name)
}
