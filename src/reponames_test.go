package src

import (
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func Test_extractSourceDest(t *testing.T) {
	src, dst, err := extractSourceDest("owner/repo")
	require.NoError(t, err)
	assert.Equal(t, "owner/repo", src)
	assert.Equal(t, "owner/repo", dst)

	src, dst, err = extractSourceDest("src_owner/src_repo:dst_owner/dst_repo")
	require.NoError(t, err)
	assert.Equal(t, "src_owner/src_repo", src)
	assert.Equal(t, "dst_owner/dst_repo", dst)

	_, _, err = extractSourceDest("src_owner/src_repo:dst_owner/dst_repo:bogus/bogus")
	require.Error(t, err)
}

func Test_validateNwo(t *testing.T) {
	nwo, err := validateNwo("owner/repo")
	require.NoError(t, err)
	assert.Equal(t, "owner/repo", nwo)

	nwo, err = validateNwo(" owner/repo ")
	require.NoError(t, err)
	assert.Equal(t, "owner/repo", nwo)

	// test shortest valid name
	nwo, err = validateNwo("a/b")
	require.NoError(t, err)
	assert.Equal(t, "a/b", nwo)

	// no slash separator
	_, err = validateNwo("bogus")
	require.Error(t, err)

	// no owner
	_, err = validateNwo("/bogus")
	require.Error(t, err)

	// no repo name
	_, err = validateNwo("bogus/")
	require.Error(t, err)

	_, err = validateNwo("bogus whitespace/bogus")
	require.Error(t, err)

	_, err = validateNwo("bogus/bogus/bogus")
	require.Error(t, err)

	// A separate destination is only permitted for "repo names", not NWOs.
	_, err = validateNwo("owner/repo:bogus/bogus")
	require.Error(t, err)
}
