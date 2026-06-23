package src

import (
	"context"
	"encoding/json"
	"fmt"
	"net/http"
	"net/http/httptest"
	"testing"

	"github.com/go-git/go-git/v5/config"
	"github.com/go-git/go-git/v5/plumbing"
	"github.com/google/go-github/v43/github"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

// Mocks, fakes and helpers shared across the package's tests live in
// testutils_test.go.

// Tests for PushOnlyFlags.Validate batch size validation

func TestPushOnlyFlags_Validate_BatchSize(t *testing.T) {
	tests := []struct {
		name       string
		batchSize  int
		expectErr  bool
		errMessage string
	}{
		{
			name:      "batch size 0 (no batching) is valid",
			batchSize: 0,
			expectErr: false,
		},
		{
			name:      "batch size at minimum (10) is valid",
			batchSize: MinBatchSize,
			expectErr: false,
		},
		{
			name:      "batch size above minimum is valid",
			batchSize: 100,
			expectErr: false,
		},
		{
			name:       "batch size below minimum is invalid",
			batchSize:  5,
			expectErr:  true,
			errMessage: fmt.Sprintf("--batch-size must be 0 (no batching) or at least %d", MinBatchSize),
		},
		{
			name:       "batch size of 1 is invalid",
			batchSize:  1,
			expectErr:  true,
			errMessage: fmt.Sprintf("--batch-size must be 0 (no batching) or at least %d", MinBatchSize),
		},
		{
			name:       "batch size of 9 is invalid",
			batchSize:  9,
			expectErr:  true,
			errMessage: fmt.Sprintf("--batch-size must be 0 (no batching) or at least %d", MinBatchSize),
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			flags := PushOnlyFlags{
				BaseURL:   "https://example.com",
				Token:     "test-token",
				BatchSize: tt.batchSize,
			}

			validations := flags.Validate()

			if tt.expectErr {
				require.NotEmpty(t, validations, "expected validation error")
				found := false
				for _, v := range validations {
					if v == tt.errMessage {
						found = true
						break
					}
				}
				assert.True(t, found, "expected error message not found: %s", tt.errMessage)
			} else {
				// Check that batch size validation didn't add an error
				for _, v := range validations {
					assert.NotContains(t, v, "batch-size", "unexpected batch-size validation error")
				}
			}
		})
	}
}

// Tests for collectRefs function

func TestCollectRefs(t *testing.T) {
	tests := []struct {
		name         string
		refs         []*plumbing.Reference
		expectedLen  int
		expectedRefs []plumbing.ReferenceName
		expectErr    bool
	}{
		{
			name:        "empty repository",
			refs:        []*plumbing.Reference{},
			expectedLen: 0,
		},
		{
			name: "branches only",
			refs: []*plumbing.Reference{
				plumbing.NewHashReference(plumbing.NewBranchReferenceName("main"), plumbing.NewHash("abc123")),
				plumbing.NewHashReference(plumbing.NewBranchReferenceName("feature"), plumbing.NewHash("def456")),
			},
			expectedLen: 2,
			expectedRefs: []plumbing.ReferenceName{
				plumbing.NewBranchReferenceName("main"),
				plumbing.NewBranchReferenceName("feature"),
			},
		},
		{
			name: "tags only",
			refs: []*plumbing.Reference{
				plumbing.NewHashReference(plumbing.NewTagReferenceName("v1.0.0"), plumbing.NewHash("abc123")),
				plumbing.NewHashReference(plumbing.NewTagReferenceName("v2.0.0"), plumbing.NewHash("def456")),
			},
			expectedLen: 2,
			expectedRefs: []plumbing.ReferenceName{
				plumbing.NewTagReferenceName("v1.0.0"),
				plumbing.NewTagReferenceName("v2.0.0"),
			},
		},
		{
			name: "mixed branches and tags",
			refs: []*plumbing.Reference{
				plumbing.NewHashReference(plumbing.NewBranchReferenceName("main"), plumbing.NewHash("abc123")),
				plumbing.NewHashReference(plumbing.NewTagReferenceName("v1.0.0"), plumbing.NewHash("def456")),
				plumbing.NewHashReference(plumbing.NewBranchReferenceName("develop"), plumbing.NewHash("ghi789")),
			},
			expectedLen: 3,
		},
		{
			name: "filters out HEAD and other refs",
			refs: []*plumbing.Reference{
				plumbing.NewHashReference(plumbing.HEAD, plumbing.NewHash("abc123")),
				plumbing.NewHashReference(plumbing.NewBranchReferenceName("main"), plumbing.NewHash("def456")),
				plumbing.NewHashReference(plumbing.NewRemoteReferenceName("origin", "main"), plumbing.NewHash("ghi789")),
				plumbing.NewHashReference(plumbing.NewTagReferenceName("v1.0.0"), plumbing.NewHash("jkl012")),
			},
			expectedLen: 2, // Only main branch and v1.0.0 tag
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			repo := &mockGitRepository{refs: tt.refs}

			refs, err := collectRefs(repo)

			if tt.expectErr {
				require.Error(t, err)
				return
			}

			require.NoError(t, err)
			assert.Len(t, refs, tt.expectedLen)

			if tt.expectedRefs != nil {
				for i, expected := range tt.expectedRefs {
					assert.Equal(t, expected, refs[i])
				}
			}
		})
	}
}

func TestCollectRefs_Error(t *testing.T) {
	repo := &mockGitRepository{err: fmt.Errorf("failed to get references")}

	refs, err := collectRefs(repo)

	require.Error(t, err)
	assert.Nil(t, refs)
	assert.Contains(t, err.Error(), "failed to get references")
}

// Tests for pushRefsInBatches function

func TestPushRefsInBatches(t *testing.T) {
	tests := []struct {
		name              string
		refs              []plumbing.ReferenceName
		batchSize         int
		expectedBatches   int
		alreadyUpToDate   bool
		pushError         error
		expectErr         bool
		expectedErrSubstr string
	}{
		{
			name: "single batch - fewer refs than batch size",
			refs: []plumbing.ReferenceName{
				plumbing.NewBranchReferenceName("main"),
				plumbing.NewBranchReferenceName("feature"),
			},
			batchSize:       10,
			expectedBatches: 1,
		},
		{
			name:            "single batch - exact batch size",
			refs:            createNRefs(10),
			batchSize:       10,
			expectedBatches: 1,
		},
		{
			name:            "multiple batches - exactly divisible",
			refs:            createNRefs(30),
			batchSize:       10,
			expectedBatches: 3,
		},
		{
			name:            "multiple batches - not exactly divisible",
			refs:            createNRefs(25),
			batchSize:       10,
			expectedBatches: 3, // 10 + 10 + 5
		},
		{
			name:            "empty refs",
			refs:            []plumbing.ReferenceName{},
			batchSize:       10,
			expectedBatches: 0,
		},
		{
			name: "all batches already up to date",
			refs: []plumbing.ReferenceName{
				plumbing.NewBranchReferenceName("main"),
			},
			batchSize:       10,
			expectedBatches: 1,
			alreadyUpToDate: true,
		},
		{
			name: "push error",
			refs: []plumbing.ReferenceName{
				plumbing.NewBranchReferenceName("main"),
			},
			batchSize:         10,
			pushError:         fmt.Errorf("network error"),
			expectErr:         true,
			expectedErrSubstr: "failed to push batch",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			remote := &mockGitRemote{
				alreadyUpToDate: tt.alreadyUpToDate,
				pushError:       tt.pushError,
			}

			err := pushRefsInBatches(context.Background(), remote, tt.refs, tt.batchSize, nil, "https://example.com/repo.git")

			if tt.expectErr {
				require.Error(t, err)
				assert.Contains(t, err.Error(), tt.expectedErrSubstr)
				return
			}

			require.NoError(t, err)
			assert.Len(t, remote.pushCalls, tt.expectedBatches)
		})
	}
}

func TestPushRefsInBatches_RefSpecFormat(t *testing.T) {
	refs := []plumbing.ReferenceName{
		plumbing.NewBranchReferenceName("main"),
		plumbing.NewTagReferenceName("v1.0.0"),
	}

	remote := &mockGitRemote{}

	err := pushRefsInBatches(context.Background(), remote, refs, 10, nil, "https://example.com/repo.git")

	require.NoError(t, err)
	require.Len(t, remote.pushCalls, 1)
	require.Len(t, remote.pushCalls[0], 2)

	// Check refspec format: should be "+refs/heads/main:refs/heads/main"
	assert.Equal(t, config.RefSpec("+refs/heads/main:refs/heads/main"), remote.pushCalls[0][0])
	assert.Equal(t, config.RefSpec("+refs/tags/v1.0.0:refs/tags/v1.0.0"), remote.pushCalls[0][1])
}

func TestPushRefsInBatches_BatchSizes(t *testing.T) {
	// Create 25 refs
	refs := createNRefs(25)
	batchSize := 10

	remote := &mockGitRemote{}

	err := pushRefsInBatches(context.Background(), remote, refs, batchSize, nil, "https://example.com/repo.git")

	require.NoError(t, err)
	require.Len(t, remote.pushCalls, 3)

	// First batch should have 10 refs
	assert.Len(t, remote.pushCalls[0], 10)
	// Second batch should have 10 refs
	assert.Len(t, remote.pushCalls[1], 10)
	// Third batch should have 5 refs (remainder)
	assert.Len(t, remote.pushCalls[2], 5)
}

// Tests for constants

func TestConstants(t *testing.T) {
	assert.Equal(t, 0, DefaultBatchSize, "DefaultBatchSize should be 0 for backward compatibility")
	assert.Equal(t, 10, MinBatchSize, "MinBatchSize should be 10")
}

// Tests for the --github-app-auth flag

func TestPushOnlyFlags_GitHubAppAuth_Valid(t *testing.T) {
	flags := PushOnlyFlags{
		BaseURL:   "https://example.com",
		Token:     "ghs_token",
		GitHubApp: true,
	}

	validations := flags.Validate()

	for _, v := range validations {
		assert.NotContains(t, v, "github-app-auth", "unexpected github-app-auth validation error")
	}
}

func TestPushOnlyFlags_GitHubAppAuth_RejectsImpersonation(t *testing.T) {
	flags := PushOnlyFlags{
		BaseURL:          "https://example.com",
		Token:            "ghs_token",
		GitHubApp:        true,
		ActionsAdminUser: "actions-admin",
	}

	validations := flags.Validate()

	require.NotEmpty(t, validations)
	require.Contains(t, validations.Error().Error(), "--github-app-auth cannot be used with --actions-admin-user")
}

// Tests for resolveCreateOrgName

func TestResolveCreateOrgName_GitHubApp(t *testing.T) {
	// With App auth the user API must never be called, so a nil client is safe.
	orgName, isAE, aeDetermined, err := resolveCreateOrgName(context.Background(), nil, "my-org", true)

	require.NoError(t, err)
	assert.Equal(t, "my-org", orgName)
	assert.False(t, isAE)
	assert.False(t, aeDetermined, "AE determination is deferred to the caller for App auth")
}

func TestResolveCreateOrgName_PAT_OwnerIsAuthenticatedUser(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		switch r.URL.Path {
		case "/api/v3/user":
			user := github.User{Login: github.String("monalisa")}
			b, _ := json.Marshal(user)
			_, _ = w.Write(b)
		default:
			t.Errorf("unexpected request to %s", r.URL.Path)
			w.WriteHeader(http.StatusInternalServerError)
		}
	}))
	defer server.Close()

	client := newTestGitHubClient(t, server.URL)

	orgName, _, aeDetermined, err := resolveCreateOrgName(context.Background(), client, "monalisa", false)

	require.NoError(t, err)
	assert.Equal(t, "", orgName, "repo should be created under the authenticated user's account")
	assert.True(t, aeDetermined)
}

func TestResolveCreateOrgName_PAT_OwnerIsOrg(t *testing.T) {
	createOrgCalled := false
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		switch {
		case r.URL.Path == "/api/v3/user":
			user := github.User{Login: github.String("monalisa")}
			b, _ := json.Marshal(user)
			_, _ = w.Write(b)
		case r.URL.Path == "/api/v3/admin/organizations" && r.Method == http.MethodPost:
			createOrgCalled = true
			org := github.Organization{Login: github.String("my-org")}
			b, _ := json.Marshal(org)
			_, _ = w.Write(b)
		default:
			t.Errorf("unexpected request to %s %s", r.Method, r.URL.Path)
			w.WriteHeader(http.StatusInternalServerError)
		}
	}))
	defer server.Close()

	client := newTestGitHubClient(t, server.URL)

	orgName, _, aeDetermined, err := resolveCreateOrgName(context.Background(), client, "my-org", false)

	require.NoError(t, err)
	assert.Equal(t, "my-org", orgName)
	assert.True(t, aeDetermined)
	assert.True(t, createOrgCalled, "expected org creation to be attempted")
}

func TestResolveCreateOrgName_PAT_GitHubAE(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path == "/api/v3/user" {
			w.Header().Set(enterpriseVersionHeaderKey, enterpriseAegisVersionHeaderValue)
			user := github.User{Login: github.String("monalisa")}
			b, _ := json.Marshal(user)
			_, _ = w.Write(b)
			return
		}
		t.Errorf("unexpected request to %s", r.URL.Path)
		w.WriteHeader(http.StatusInternalServerError)
	}))
	defer server.Close()

	client := newTestGitHubClient(t, server.URL)

	orgName, isAE, aeDetermined, err := resolveCreateOrgName(context.Background(), client, "monalisa", false)

	require.NoError(t, err)
	assert.Equal(t, "", orgName)
	assert.True(t, isAE, "AE should be detected from the user response header")
	assert.True(t, aeDetermined)
}

func TestResolveCreateOrgName_PAT_NilLogin(t *testing.T) {
	// Regression: a user response with no login must produce a clear error.
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path == "/api/v3/user" {
			_, _ = w.Write([]byte(`{}`))
			return
		}
		t.Errorf("unexpected request to %s", r.URL.Path)
		w.WriteHeader(http.StatusInternalServerError)
	}))
	defer server.Close()

	client := newTestGitHubClient(t, server.URL)

	orgName, _, _, err := resolveCreateOrgName(context.Background(), client, "my-org", false)

	require.Error(t, err)
	assert.Equal(t, "", orgName)
	assert.Contains(t, err.Error(), "login name")
}

func TestResolveCreateOrgName_PAT_UserError(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		// Simulate a ghs_* token hitting the user API: no user context available.
		w.WriteHeader(http.StatusForbidden)
		_, _ = w.Write([]byte(`{"message":"Resource not accessible by integration"}`))
	}))
	defer server.Close()

	client := newTestGitHubClient(t, server.URL)

	orgName, _, _, err := resolveCreateOrgName(context.Background(), client, "my-org", false)

	require.Error(t, err)
	assert.Equal(t, "", orgName)
	assert.Contains(t, err.Error(), "--github-app-auth", "error should hint at GitHub App auth flag")
}

// Tests for getOrCreateGitHubRepo with GitHub App auth (new behaviour)

func TestGetOrCreateGitHubRepo_GitHubApp_CreatesUnderOrg(t *testing.T) {
	f := &fakeGitHub{repoExists: false}
	client := f.start(t)

	repo, err := getOrCreateGitHubRepo(context.Background(), client, "my-repo", "my-org", true)

	require.NoError(t, err)
	require.NotNil(t, repo)
	assert.Equal(t, "my-repo", repo.GetName())
	assert.False(t, f.userCalled, "App auth must not call the user API")
	assert.True(t, f.created, "missing repo should be created")
	assert.False(t, f.createdUnderUser, "App auth must create under the org, not a user account")
	assert.Equal(t, "my-org", f.createdOrg, "repo should be created under the owner from the repo name")
	assert.Equal(t, "public", f.createdVis)
}

func TestGetOrCreateGitHubRepo_GitHubApp_ExistingRepo(t *testing.T) {
	f := &fakeGitHub{repoExists: true}
	client := f.start(t)

	repo, err := getOrCreateGitHubRepo(context.Background(), client, "existing-repo", "my-org", true)

	require.NoError(t, err)
	require.NotNil(t, repo)
	assert.False(t, f.userCalled, "App auth must not call the user API")
	assert.False(t, f.created, "existing repo must not be recreated")
}

func TestGetOrCreateGitHubRepo_GitHubApp_GHAE_InternalVisibility(t *testing.T) {
	// With App auth the AE version is detected from the repo response header,
	// not the user response, so internal visibility must still be selected.
	f := &fakeGitHub{repoExists: false, repoGetAE: true}
	client := f.start(t)

	_, err := getOrCreateGitHubRepo(context.Background(), client, "ghae-repo", "my-org", true)

	require.NoError(t, err)
	assert.False(t, f.userCalled, "App auth must not call the user API")
	assert.True(t, f.created)
	assert.Equal(t, "internal", f.createdVis, "AE detected from the repo response should yield internal visibility")
}

// Regression tests for getOrCreateGitHubRepo with PAT auth (pre-existing
// behaviour). These had no unit coverage before; they guard against regressions
// from the App-auth refactor.

func TestGetOrCreateGitHubRepo_PAT_ExistingRepo(t *testing.T) {
	f := &fakeGitHub{repoExists: true, userLogin: "monalisa"}
	client := f.start(t)

	repo, err := getOrCreateGitHubRepo(context.Background(), client, "existing-repo", "monalisa", false)

	require.NoError(t, err)
	require.NotNil(t, repo)
	assert.True(t, f.userCalled, "PAT auth resolves the authenticated user")
	assert.False(t, f.created, "existing repo must not be recreated")
}

func TestGetOrCreateGitHubRepo_PAT_CreatesUnderUser(t *testing.T) {
	f := &fakeGitHub{repoExists: false, userLogin: "monalisa"}
	client := f.start(t)

	_, err := getOrCreateGitHubRepo(context.Background(), client, "new-repo", "monalisa", false)

	require.NoError(t, err)
	assert.True(t, f.created)
	assert.True(t, f.createdUnderUser, "owner matching the authenticated user must create under the user account")
	assert.False(t, f.createOrgCalled, "no org should be created when owner is the authenticated user")
	assert.Equal(t, "public", f.createdVis)
}

func TestGetOrCreateGitHubRepo_PAT_CreatesUnderOrg(t *testing.T) {
	f := &fakeGitHub{repoExists: false, userLogin: "monalisa"}
	client := f.start(t)

	_, err := getOrCreateGitHubRepo(context.Background(), client, "new-repo", "my-org", false)

	require.NoError(t, err)
	assert.True(t, f.createOrgCalled, "owner differing from the authenticated user must ensure the org exists")
	assert.True(t, f.created)
	assert.False(t, f.createdUnderUser)
	assert.Equal(t, "my-org", f.createdOrg)
}

func TestGetOrCreateGitHubRepo_PAT_GHAE_InternalVisibility(t *testing.T) {
	// AE detected from the user response (pre-existing behaviour) must still
	// select internal visibility.
	f := &fakeGitHub{repoExists: false, userLogin: "monalisa", userAE: true}
	client := f.start(t)

	_, err := getOrCreateGitHubRepo(context.Background(), client, "ghae-repo", "monalisa", false)

	require.NoError(t, err)
	assert.True(t, f.created)
	assert.Equal(t, "internal", f.createdVis, "AE detected from the user response should yield internal visibility")
}

func TestGetOrCreateGitHubRepo_PAT_RepoGetError(t *testing.T) {
	// A non-404 error from the existence check must be surfaced, not swallowed.
	f := &fakeGitHub{repoGetStatus: http.StatusInternalServerError, userLogin: "monalisa"}
	client := f.start(t)

	repo, err := getOrCreateGitHubRepo(context.Background(), client, "some-repo", "monalisa", false)

	require.Error(t, err)
	assert.Nil(t, repo)
	assert.False(t, f.created, "no repo should be created when the existence check errors")
}

func TestGetOrCreateGitHubRepo_GitHubApp_UserNeverCalledOnError(t *testing.T) {
	// Even when repo creation cannot proceed, App auth must never hit /user.
	f := &fakeGitHub{repoGetStatus: http.StatusInternalServerError}
	client := f.start(t)

	_, err := getOrCreateGitHubRepo(context.Background(), client, "some-repo", "my-org", true)

	require.Error(t, err)
	assert.False(t, f.userCalled, "App auth must not call the user API even on error paths")
}

func TestGetOrCreateGitHubRepo_CreateFailureIsWrapped(t *testing.T) {
	// A failed repo creation must surface a wrapped error, not a nil repo.
	f := &fakeGitHub{repoExists: false, createRepoStatus: http.StatusUnprocessableEntity}
	client := f.start(t)

	repo, err := getOrCreateGitHubRepo(context.Background(), client, "bad-repo", "my-org", true)

	require.Error(t, err)
	assert.Nil(t, repo)
	assert.Contains(t, err.Error(), "error creating repository my-org/bad-repo")
}

func TestGetOrCreateGitHubRepo_PAT_OrgAlreadyExistsFallback(t *testing.T) {
	// Regression: when org creation fails because the org already exists, the
	// code falls back to fetching the org and continues.
	f := &fakeGitHub{
		repoExists:        false,
		userLogin:         "monalisa",
		orgCreateConflict: true,
		orgGetExists:      true,
	}
	client := f.start(t)

	_, err := getOrCreateGitHubRepo(context.Background(), client, "new-repo", "org-already-exists", false)

	require.NoError(t, err)
	assert.True(t, f.createOrgCalled, "org creation should be attempted")
	assert.True(t, f.orgGetCalled, "org should be fetched as a fallback when creation conflicts")
	assert.True(t, f.created)
	assert.Equal(t, "org-already-exists", f.createdOrg)
}
