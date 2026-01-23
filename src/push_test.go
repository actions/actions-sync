package src

import (
	"context"
	"fmt"
	"testing"

	"github.com/go-git/go-git/v5"
	"github.com/go-git/go-git/v5/config"
	"github.com/go-git/go-git/v5/plumbing"
	"github.com/go-git/go-git/v5/plumbing/storer"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

// Mock implementations for testing

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

type mockGitRemote struct {
	pushCalls      [][]config.RefSpec
	pushError      error
	alreadyUpToDate bool
	remoteConfig   *config.RemoteConfig
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
			name: "single batch - exact batch size",
			refs: createNRefs(10),
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

// Helper function to create N test refs
func createNRefs(n int) []plumbing.ReferenceName {
	refs := make([]plumbing.ReferenceName, n)
	for i := 0; i < n; i++ {
		refs[i] = plumbing.NewBranchReferenceName(fmt.Sprintf("branch-%d", i))
	}
	return refs
}
