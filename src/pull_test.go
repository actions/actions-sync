package src

import (
	"context"
	"errors"
	"fmt"
	"testing"

	"github.com/go-git/go-git/v5"
	"github.com/go-git/go-git/v5/config"
	"github.com/go-git/go-git/v5/plumbing"
	"github.com/go-git/go-git/v5/plumbing/transport/http"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestGitAuthMethod_EmptyTokenIsAnonymous(t *testing.T) {
	assert.Nil(t, gitAuthMethod(""))
}

func TestGitAuthMethod_TokenBuildsBasicAuth(t *testing.T) {
	auth := gitAuthMethod("ghs_exampletoken")
	basic, ok := auth.(*http.BasicAuth)
	require.True(t, ok, "expected BasicAuth")
	assert.Equal(t, "x-access-token", basic.Username)
	assert.Equal(t, "ghs_exampletoken", basic.Password)
}

func TestPullOnlyFlags_Validate_TokenOverHTTPRejected(t *testing.T) {
	f := &PullOnlyFlags{SourceURL: "http://insecure.example.com", Token: "secret"}
	validations := f.Validate()
	require.Len(t, validations, 1)
	assert.Contains(t, validations[0], "--source-token")
}

func TestPullOnlyFlags_Validate_TokenOverHTTPSAllowed(t *testing.T) {
	f := &PullOnlyFlags{SourceURL: "https://github.com", Token: "secret"}
	assert.Empty(t, f.Validate())
}

func TestPullOnlyFlags_Validate_TokenOverHTTPCaseInsensitiveRejected(t *testing.T) {
	for _, sourceURL := range []string{"HTTP://insecure.example.com", "Http://insecure.example.com"} {
		f := &PullOnlyFlags{SourceURL: sourceURL, Token: "secret"}
		assert.Len(t, f.Validate(), 1, sourceURL)
	}
}

func TestPullOnlyFlags_Validate_TokenRequiresHTTPS(t *testing.T) {
	for _, sourceURL := range []string{"ssh://git@example.com", "git://example.com", "example.com"} {
		f := &PullOnlyFlags{SourceURL: sourceURL, Token: "secret"}
		assert.Len(t, f.Validate(), 1, sourceURL)
	}
}

func TestPullOnlyFlags_Validate_TokenOverHTTPSCaseInsensitiveAllowed(t *testing.T) {
	f := &PullOnlyFlags{SourceURL: "HTTPS://github.com", Token: "secret"}
	assert.Empty(t, f.Validate())
}

func TestPullOnlyFlags_Validate_NoTokenAllowsHTTP(t *testing.T) {
	f := &PullOnlyFlags{SourceURL: "http://insecure.example.com"}
	assert.Empty(t, f.Validate())
}

func TestPullWithGitImpl_ThreadsAuthToCloneAndFetch(t *testing.T) {
	cacheDir := t.TempDir()
	repo := &fakePullRepo{}
	impl := &fakePullGitImpl{repo: repo}
	auth := gitAuthMethod("secret-token")

	err := PullWithGitImpl(context.Background(), "https://github.com", auth, cacheDir, false, "actions/setup-node", impl)
	require.NoError(t, err)

	assert.Same(t, auth, impl.cloneAuth, "clone should use the provided auth")
	assert.True(t, repo.fetchCalled)
	assert.Same(t, auth, repo.fetchAuth, "fetch should use the provided auth")
}

func TestPullWithGitImpl_NilAuthWhenNoToken(t *testing.T) {
	cacheDir := t.TempDir()
	repo := &fakePullRepo{}
	impl := &fakePullGitImpl{repo: repo}

	err := PullWithGitImpl(context.Background(), "https://github.com", nil, cacheDir, false, "actions/setup-node", impl)
	require.NoError(t, err)

	assert.Nil(t, impl.cloneAuth)
	assert.Nil(t, repo.fetchAuth)
}

func TestPullWithGitImpl_SkipsCloneWhenRepositoryExists(t *testing.T) {
	cacheDir := t.TempDir()
	repo := &fakePullRepo{}
	impl := &fakePullGitImpl{repo: repo, exists: true}
	auth := gitAuthMethod("secret-token")

	err := PullWithGitImpl(context.Background(), "https://github.com", auth, cacheDir, false, "actions/setup-node", impl)
	require.NoError(t, err)

	assert.Nil(t, impl.cloneAuth, "clone should be skipped when the repo already exists")
	assert.Same(t, auth, repo.fetchAuth, "fetch should still authenticate")
}

func TestPullWithGitImpl_AuthRequiredReturnsFriendlyError(t *testing.T) {
	cacheDir := t.TempDir()
	impl := &fakePullGitImpl{repo: &fakePullRepo{}, cloneErr: errors.New("authentication required")}

	err := PullWithGitImpl(context.Background(), "https://github.com", nil, cacheDir, false, "actions/private", impl)
	require.Error(t, err)
	assert.Contains(t, err.Error(), "may require authentication or does not exist")
}

func TestPullWithGitImpl_FetchAuthRequiredReturnsFriendlyError(t *testing.T) {
	cacheDir := t.TempDir()
	repo := &fakePullRepo{fetchErr: errors.New("authentication required")}
	impl := &fakePullGitImpl{repo: repo, exists: true}

	err := PullWithGitImpl(context.Background(), "https://github.com", nil, cacheDir, false, "actions/private", impl)
	require.Error(t, err)
	assert.Contains(t, err.Error(), "may require authentication or does not exist")
}

func TestPullManyWithGitImpl_ThreadsAuthToEachRepo(t *testing.T) {
	cacheDir := t.TempDir()
	impl := &fakePullGitImpl{repo: &fakePullRepo{}}
	auth := gitAuthMethod("secret-token")

	err := PullManyWithGitImpl(context.Background(), "https://github.com", auth, cacheDir, false, []string{"actions/a", "actions/b"}, impl)
	require.NoError(t, err)

	assert.Equal(t, 2, impl.cloneCount, "each repo should be cloned")
	assert.Same(t, auth, impl.cloneAuth, "clone should use the provided auth")
	assert.Same(t, auth, impl.repo.fetchAuth, "fetch should use the provided auth")
}

func TestPullManyWithGitImpl_StopsOnFirstError(t *testing.T) {
	cacheDir := t.TempDir()
	impl := &fakePullGitImpl{repo: &fakePullRepo{}, cloneErr: errors.New("boom")}

	err := PullManyWithGitImpl(context.Background(), "https://github.com", nil, cacheDir, false, []string{"actions/a", "actions/b"}, impl)
	require.Error(t, err)
	assert.Equal(t, 1, impl.cloneCount, "iteration should stop after the first failing repo")
}

func TestPullWithGitImpl_AllBranchesByDefault(t *testing.T) {
	cacheDir := t.TempDir()
	repo := &fakePullRepo{}
	impl := &fakePullGitImpl{repo: repo}

	err := PullWithGitImpl(context.Background(), "https://github.com", nil, cacheDir, false, "actions/setup-node", impl)
	require.NoError(t, err)

	assert.False(t, impl.cloneSingleBranch, "clone should not be limited to a single branch")
	assert.Equal(t, plumbing.HEAD, impl.cloneRefName, "clone should reference HEAD")
	require.Len(t, repo.fetchRefSpecs, 1)
	assert.Equal(t, config.RefSpec("+refs/heads/*:refs/heads/*"), repo.fetchRefSpecs[0], "fetch should mirror all branches")
}

func TestPullWithGitImpl_DefaultBranchOnly(t *testing.T) {
	// The cache has several branches but HEAD points at "trunk"; only that
	// branch should be fetched, proving the selection is driven by the default
	// branch rather than just happening to be the sole branch present.
	cacheDir := t.TempDir()
	repo := &fakePullRepo{
		headBranch: "trunk",
		branches:   []string{"trunk", "feature-a", "feature-b"},
	}
	impl := &fakePullGitImpl{repo: repo}

	err := PullWithGitImpl(context.Background(), "https://github.com", nil, cacheDir, true, "actions/setup-node", impl)
	require.NoError(t, err)

	assert.True(t, impl.cloneSingleBranch, "clone should be limited to the default branch")
	assert.Equal(t, plumbing.HEAD, impl.cloneRefName, "clone should reference HEAD to pick the default branch")
	require.Len(t, repo.fetchRefSpecs, 1, "only the default branch should be fetched")
	assert.Equal(t, config.RefSpec("+refs/heads/trunk:refs/heads/trunk"), repo.fetchRefSpecs[0], "fetch should refresh only the default branch, not every cached branch")
}

func TestPullWithGitImpl_DefaultBranchOnlyRefreshesCachedBranchOnReSync(t *testing.T) {
	// Repository already exists in the cache with multiple branches, so the
	// clone is skipped. The fetch must update the cached default branch
	// (regression: it previously only fetched tags, leaving the branch stale)
	// while leaving the other cached branches untouched.
	cacheDir := t.TempDir()
	repo := &fakePullRepo{
		headBranch: "main",
		branches:   []string{"main", "feature-a", "feature-b"},
	}
	impl := &fakePullGitImpl{repo: repo, exists: true}

	err := PullWithGitImpl(context.Background(), "https://github.com", nil, cacheDir, true, "actions/setup-node", impl)
	require.NoError(t, err)

	assert.Equal(t, 0, impl.cloneCount, "clone should be skipped when the repo already exists")
	require.Len(t, repo.fetchRefSpecs, 1, "only the default branch should be refreshed, not every cached branch")
	assert.Equal(t, config.RefSpec("+refs/heads/main:refs/heads/main"), repo.fetchRefSpecs[0], "the cached default branch must be updated on re-sync")
}

func TestPullWithGitImpl_DefaultBranchOnlyHeadError(t *testing.T) {
	cacheDir := t.TempDir()
	repo := &fakePullRepo{headErr: errors.New("reference not found")}
	impl := &fakePullGitImpl{repo: repo, exists: true}

	err := PullWithGitImpl(context.Background(), "https://github.com", nil, cacheDir, true, "actions/setup-node", impl)
	require.Error(t, err)
	assert.Contains(t, err.Error(), "default branch")
	assert.False(t, repo.fetchCalled, "fetch should not run when the default branch cannot be resolved")
}

func TestPullManyWithGitImpl_ThreadsDefaultBranchOnlyToEachRepo(t *testing.T) {
	cacheDir := t.TempDir()
	repo := &fakePullRepo{
		headBranch: "main",
		branches:   []string{"main", "feature-a", "feature-b"},
	}
	impl := &fakePullGitImpl{repo: repo}

	err := PullManyWithGitImpl(context.Background(), "https://github.com", nil, cacheDir, true, []string{"actions/a", "actions/b"}, impl)
	require.NoError(t, err)

	assert.True(t, impl.cloneSingleBranch, "clone should be limited to the default branch for each repo")
	require.Len(t, repo.fetchRefSpecs, 1, "only the default branch should be fetched")
	assert.Equal(t, config.RefSpec("+refs/heads/main:refs/heads/main"), repo.fetchRefSpecs[0])
}

func TestPullWithGitImpl_TagsAlwaysSynced(t *testing.T) {
	for _, defaultBranchOnly := range []bool{false, true} {
		t.Run(fmt.Sprintf("defaultBranchOnly=%t", defaultBranchOnly), func(t *testing.T) {
			cacheDir := t.TempDir()
			repo := &fakePullRepo{}
			impl := &fakePullGitImpl{repo: repo}

			err := PullWithGitImpl(context.Background(), "https://github.com", nil, cacheDir, defaultBranchOnly, "actions/setup-node", impl)
			require.NoError(t, err)

			assert.Equal(t, git.AllTags, repo.fetchTags, "all tags should be fetched regardless of default-branch-only")
		})
	}
}
