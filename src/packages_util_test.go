package src

import (
	"testing"
	"github.com/stretchr/testify/assert"
)

func Test_GetPackageTagsListFromGHCR(t *testing.T){
	expected := "Z2hfMTIzNDU2"

    data:= Base64Encode("gh_123456")
	assert.Equal(t, expected, data)
}
