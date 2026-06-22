package src

import (
	"context"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	"github.com/go-git/go-git/v5"
	"github.com/go-git/go-git/v5/config"
	"github.com/go-git/go-git/v5/plumbing"
	"github.com/go-git/go-git/v5/plumbing/storer"
	"github.com/google/go-github/v43/github"
	"github.com/stretchr/testify/require"
)

// This file holds test utilities (mocks, fakes and helpers) shared across the
// package's tests. Keeping them in one place avoids duplication between
// push_test.go, git_test.go and any future test files.

// mockReferenceIter is an in-memory storer.ReferenceIter over a fixed slice of
// references.
type mockReferenceIter struct {
	refs  []*plumbing.Reference
	index int
}

func (m *mockReferenceIter) Next() (*plumbing.Reference, error) {
	if m.index >= len(m.refs) {
		return nil, storer.ErrStop
	}
	ref := m.refs[m.index]
	m.index++
	return ref, nil
}

func (m *mockReferenceIter) ForEach(fn func(*plumbing.Reference) error) error {
	for _, ref := range m.refs {
		if err := fn(ref); err != nil {
			if err == storer.ErrStop {
				return nil
			}
			return err
		}
	}
	return nil
}

func (m *mockReferenceIter) Close() {}

// mockGitRepository is a GitRepository test double backed by a fixed set of refs.
type mockGitRepository struct {
	refs []*plumbing.Reference
	err  error
}

func (m *mockGitRepository) DeleteRemote(name string) error {
	return nil
}

func (m *mockGitRepository) CreateRemote(c *config.RemoteConfig) (GitRemote, error) {
	return nil, nil
}

func (m *mockGitRepository) FetchContext(ctx context.Context, o *git.FetchOptions) error {
	return nil
}

func (m *mockGitRepository) References() (storer.ReferenceIter, error) {
	if m.err != nil {
		return nil, m.err
	}
	return &mockReferenceIter{refs: m.refs, index: 0}, nil
}

// mockGitRemote is a GitRemote test double that records the refspecs it was
// asked to push.
type mockGitRemote struct {
	pushCalls       [][]config.RefSpec
	pushError       error
	alreadyUpToDate bool
	remoteConfig    *config.RemoteConfig
}

func (m *mockGitRemote) PushContext(ctx context.Context, o *git.PushOptions) error {
	m.pushCalls = append(m.pushCalls, o.RefSpecs)
	if m.alreadyUpToDate {
		return git.NoErrAlreadyUpToDate
	}
	return m.pushError
}

func (m *mockGitRemote) Config() *config.RemoteConfig {
	if m.remoteConfig != nil {
		return m.remoteConfig
	}
	return &config.RemoteConfig{Name: "test-remote"}
}

// createNRefs returns n distinct branch reference names, useful for batching tests.
func createNRefs(n int) []plumbing.ReferenceName {
	refs := make([]plumbing.ReferenceName, n)
	for i := 0; i < n; i++ {
		refs[i] = plumbing.NewBranchReferenceName(fmt.Sprintf("branch-%d", i))
	}
	return refs
}

// newTestGitHubClient returns a github.Client whose API requests are routed to
// the provided test server.
func newTestGitHubClient(t *testing.T, serverURL string) *github.Client {
	t.Helper()
	client, err := github.NewEnterpriseClient(serverURL, serverURL, nil)
	require.NoError(t, err)
	return client
}

// fakeGitHub is a configurable mock of the GitHub REST API exercised by
// getOrCreateGitHubRepo. It records which endpoints were hit so tests can assert
// on the resulting behaviour (e.g. that App auth never calls /user and that
// repositories are created under the expected owner with the expected visibility).
type fakeGitHub struct {
	// config
	userLogin         string // login returned by GET /user
	userAE            bool   // set the AE version header on the GET /user response
	userStatus        int    // override GET /user status (0 => 200)
	repoExists        bool   // GET /repos/{owner}/{repo} returns 200 vs 404
	repoGetAE         bool   // set the AE version header on the GET /repos response
	repoGetStatus     int    // override GET /repos status (0 => derived from repoExists)
	createRepoStatus  int    // override POST repos status (0 => 201 Created)
	orgCreateConflict bool   // POST /admin/organizations returns 422 (already exists)
	orgGetExists      bool   // GET /orgs/{org} returns 200 (used as create fallback)

	// recorded
	userCalled       bool
	created          bool
	createdUnderUser bool
	createdOrg       string
	createdName      string
	createdVis       string
	createOrgCalled  bool
	orgGetCalled     bool
}

func (f *fakeGitHub) handler(t *testing.T) http.HandlerFunc {
	t.Helper()
	return func(w http.ResponseWriter, r *http.Request) {
		switch {
		case r.URL.Path == "/api/v3/user" && r.Method == http.MethodGet:
			f.userCalled = true
			if f.userStatus != 0 {
				w.WriteHeader(f.userStatus)
				_, _ = w.Write([]byte(`{"message":"no user context"}`))
				return
			}
			if f.userAE {
				w.Header().Set(enterpriseVersionHeaderKey, enterpriseAegisVersionHeaderValue)
			}
			login := f.userLogin
			b, _ := json.Marshal(github.User{Login: &login})
			_, _ = w.Write(b)

		case strings.HasPrefix(r.URL.Path, "/api/v3/repos/") && r.Method == http.MethodGet:
			parts := strings.Split(strings.TrimPrefix(r.URL.Path, "/api/v3/repos/"), "/")
			owner, repo := parts[0], parts[1]
			status := f.repoGetStatus
			if status == 0 {
				if f.repoExists {
					status = http.StatusOK
				} else {
					status = http.StatusNotFound
				}
			}
			if f.repoGetAE {
				w.Header().Set(enterpriseVersionHeaderKey, enterpriseAegisVersionHeaderValue)
			}
			if status == http.StatusOK {
				cloneURL := "https://example.com/" + owner + "/" + repo + ".git"
				b, _ := json.Marshal(github.Repository{Name: github.String(repo), CloneURL: &cloneURL})
				_, _ = w.Write(b)
				return
			}
			w.WriteHeader(status)
			_, _ = w.Write([]byte(`{"message":"not found"}`))

		case strings.HasPrefix(r.URL.Path, "/api/v3/orgs/") && strings.HasSuffix(r.URL.Path, "/repos") && r.Method == http.MethodPost:
			org := strings.TrimSuffix(strings.TrimPrefix(r.URL.Path, "/api/v3/orgs/"), "/repos")
			f.createdOrg = org
			f.recordCreate(r, w, org)

		case r.URL.Path == "/api/v3/user/repos" && r.Method == http.MethodPost:
			f.createdUnderUser = true
			f.recordCreate(r, w, "")

		case r.URL.Path == "/api/v3/admin/organizations" && r.Method == http.MethodPost:
			f.createOrgCalled = true
			if f.orgCreateConflict {
				w.WriteHeader(http.StatusUnprocessableEntity)
				_, _ = w.Write([]byte(`{"message":"Organization already exists"}`))
				return
			}
			var body struct {
				Login string `json:"login"`
			}
			data, _ := io.ReadAll(r.Body)
			_ = json.Unmarshal(data, &body)
			b, _ := json.Marshal(github.Organization{Login: github.String(body.Login)})
			_, _ = w.Write(b)

		case strings.HasPrefix(r.URL.Path, "/api/v3/orgs/") && r.Method == http.MethodGet:
			f.orgGetCalled = true
			org := strings.TrimPrefix(r.URL.Path, "/api/v3/orgs/")
			if !f.orgGetExists {
				w.WriteHeader(http.StatusNotFound)
				_, _ = w.Write([]byte(`{"message":"not found"}`))
				return
			}
			b, _ := json.Marshal(github.Organization{Login: github.String(org)})
			_, _ = w.Write(b)

		default:
			t.Errorf("unexpected request %s %s", r.Method, r.URL.Path)
			w.WriteHeader(http.StatusInternalServerError)
		}
	}
}

func (f *fakeGitHub) recordCreate(r *http.Request, w http.ResponseWriter, org string) {
	f.created = true
	var body struct {
		Name       string `json:"name"`
		Visibility string `json:"visibility"`
	}
	data, _ := io.ReadAll(r.Body)
	_ = json.Unmarshal(data, &body)
	f.createdName = body.Name
	f.createdVis = body.Visibility
	if f.createRepoStatus != 0 {
		w.WriteHeader(f.createRepoStatus)
		_, _ = w.Write([]byte(`{"message":"validation failed"}`))
		return
	}
	cloneURL := "https://example.com/" + org + "/" + body.Name + ".git"
	w.WriteHeader(http.StatusCreated)
	b, _ := json.Marshal(github.Repository{Name: github.String(body.Name), CloneURL: &cloneURL})
	_, _ = w.Write(b)
}

// start launches an httptest server backed by the fake and returns a github
// client pointed at it. The server is closed automatically when the test ends.
func (f *fakeGitHub) start(t *testing.T) *github.Client {
	t.Helper()
	server := httptest.NewServer(f.handler(t))
	t.Cleanup(server.Close)
	return newTestGitHubClient(t, server.URL)
}
