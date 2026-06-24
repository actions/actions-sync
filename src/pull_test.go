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
	cacheDir := t.TempDir()
	repo := &fakePullRepo{}
	impl := &fakePullGitImpl{repo: repo}

	err := PullWithGitImpl(context.Background(), "https://github.com", nil, cacheDir, true, "actions/setup-node", impl)
	require.NoError(t, err)

	assert.True(t, impl.cloneSingleBranch, "clone should be limited to the default branch")
	assert.Equal(t, plumbing.HEAD, impl.cloneRefName, "clone should reference HEAD to pick the default branch")
	require.Len(t, repo.fetchRefSpecs, 1)
	assert.Equal(t, config.RefSpec("+refs/tags/*:refs/tags/*"), repo.fetchRefSpecs[0], "fetch should only bring tags when limited to the default branch")
}

func TestPullManyWithGitImpl_ThreadsDefaultBranchOnlyToEachRepo(t *testing.T) {
	cacheDir := t.TempDir()
	repo := &fakePullRepo{}
	impl := &fakePullGitImpl{repo: repo}

	err := PullManyWithGitImpl(context.Background(), "https://github.com", nil, cacheDir, true, []string{"actions/a", "actions/b"}, impl)
	require.NoError(t, err)

	assert.True(t, impl.cloneSingleBranch, "clone should be limited to the default branch for each repo")
	require.Len(t, repo.fetchRefSpecs, 1)
	assert.Equal(t, config.RefSpec("+refs/tags/*:refs/tags/*"), repo.fetchRefSpecs[0])
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
