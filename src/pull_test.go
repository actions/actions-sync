package src

import (
	"testing"

	"github.com/go-git/go-git/v5/plumbing/transport/http"
	"github.com/stretchr/testify/require"
)

func Test_PullFlagsNoAuth(t *testing.T) {
	t.Parallel()
	flags := PullOnlyFlags{
		Username:  "",
		Token:     "",
		SourceURL: "github.com",
	}
	auth, err := flags.UserAuth()
	require.Nil(t, auth)
	require.EqualValues(t, ErrNoAuth, err)
	require.Nil(t, flags.Validate().Error())
}

func Test_PullFlagsAuth(t *testing.T) {
	t.Parallel()
	user := "SuperSecureUser"
	password := "password123"
	flags := PullOnlyFlags{
		Username:  user,
		Token:     password,
		SourceURL: "github.com",
	}
	expectedAuth := http.BasicAuth{
		Username: user,
		Password: password,
	}
	auth, err := flags.UserAuth()
	require.EqualValues(t, &expectedAuth, auth)
	require.Nil(t, err)
	require.Nil(t, flags.Validate().Error())
}

func Test_PullFlagsError(t *testing.T) {
	t.Parallel()
	flags := PullOnlyFlags{
		Username:  "SuperSecureUser",
		Token:     "",
		SourceURL: "github.com",
	}
	auth, err := flags.UserAuth()
	require.Nil(t, auth)
	require.EqualValues(t, ErrPartialAuth, err)
	require.Error(t, flags.Validate().Error())
}
