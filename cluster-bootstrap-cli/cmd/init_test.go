package cmd

import (
	"testing"

	"github.com/stretchr/testify/assert"
)

func TestValidateRepoURL(t *testing.T) {
	valid := []string{
		"git@github.com:user/repo.git",
		"ssh://git@github.com/user/repo.git",
		"https://github.com/user/repo.git",
	}
	for _, value := range valid {
		assert.NoError(t, validateRepoURL(value))
	}

	invalid := []string{
		"",
		"not-a-url",
		"git@github.com:user/repo with space.git",
		"https://",
	}
	for _, value := range invalid {
		assert.Error(t, validateRepoURL(value))
	}
}

func TestValidateTargetRevision(t *testing.T) {
	assert.NoError(t, validateTargetRevision("main"))
	assert.NoError(t, validateTargetRevision("release/v1.2.3"))
	assert.Error(t, validateTargetRevision(""))
	assert.Error(t, validateTargetRevision("bad rev"))
}
