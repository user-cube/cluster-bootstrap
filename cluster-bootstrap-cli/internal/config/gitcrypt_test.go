package config

import (
	"os"
	"path/filepath"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestEnsureGitCryptAttributes_CreatesFile(t *testing.T) {
	dir := t.TempDir()

	err := EnsureGitCryptAttributes(dir)
	require.NoError(t, err)

	data, err := os.ReadFile(filepath.Join(dir, ".gitattributes"))
	require.NoError(t, err)
	assert.Contains(t, string(data), GitCryptAttributesPattern)
}

func TestEnsureGitCryptAttributes_Idempotent(t *testing.T) {
	dir := t.TempDir()

	// Call twice
	require.NoError(t, EnsureGitCryptAttributes(dir))
	require.NoError(t, EnsureGitCryptAttributes(dir))

	data, err := os.ReadFile(filepath.Join(dir, ".gitattributes"))
	require.NoError(t, err)

	// Should appear exactly once
	content := string(data)
	first := indexOf(content, GitCryptAttributesPattern)
	assert.GreaterOrEqual(t, first, 0)
	second := indexOf(content[first+len(GitCryptAttributesPattern):], GitCryptAttributesPattern)
	assert.Equal(t, -1, second, "pattern should appear only once")
}

func TestEnsureGitCryptAttributes_AppendsToExisting(t *testing.T) {
	dir := t.TempDir()
	existing := "*.key filter=git-crypt diff=git-crypt\n"
	require.NoError(t, os.WriteFile(filepath.Join(dir, ".gitattributes"), []byte(existing), 0644))

	err := EnsureGitCryptAttributes(dir)
	require.NoError(t, err)

	data, err := os.ReadFile(filepath.Join(dir, ".gitattributes"))
	require.NoError(t, err)
	content := string(data)
	assert.Contains(t, content, "*.key filter=git-crypt diff=git-crypt")
	assert.Contains(t, content, GitCryptAttributesPattern)
}

func indexOf(s, substr string) int {
	for i := 0; i+len(substr) <= len(s); i++ {
		if s[i:i+len(substr)] == substr {
			return i
		}
	}
	return -1
}
