package config

import (
	"os"
	"path/filepath"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestSecretsFileName(t *testing.T) {
	tests := []struct {
		env      string
		expected string
	}{
		{"dev", "secrets.dev.enc.yaml"},
		{"prod", "secrets.prod.enc.yaml"},
		{"staging", "secrets.staging.enc.yaml"},
		{"us-east-1", "secrets.us-east-1.enc.yaml"},
	}

	for _, tt := range tests {
		t.Run(tt.env, func(t *testing.T) {
			assert.Equal(t, tt.expected, SecretsFileName(tt.env))
		})
	}
}

func TestSecretsFileNamePlain(t *testing.T) {
	tests := []struct {
		env      string
		expected string
	}{
		{"dev", "secrets.dev.yaml"},
		{"prod", "secrets.prod.yaml"},
		{"staging", "secrets.staging.yaml"},
	}

	for _, tt := range tests {
		t.Run(tt.env, func(t *testing.T) {
			assert.Equal(t, tt.expected, SecretsFileNamePlain(tt.env))
		})
	}
}

func TestLoadSecretsPlaintext_OK(t *testing.T) {
	dir := t.TempDir()
	content := `repo:
  url: git@github.com:user/repo.git
  targetRevision: main
  sshPrivateKey: "fake-ssh-key-for-testing"
`
	path := filepath.Join(dir, "secrets.dev.yaml")
	require.NoError(t, os.WriteFile(path, []byte(content), 0600))

	secrets, err := LoadSecretsPlaintext(path)
	require.NoError(t, err)
	assert.Equal(t, "git@github.com:user/repo.git", secrets.Repo.URL)
	assert.Equal(t, "main", secrets.Repo.TargetRevision)
	assert.Equal(t, "fake-ssh-key-for-testing", secrets.Repo.SSHPrivateKey)
}

func TestLoadSecretsPlaintext_GitCryptMagic(t *testing.T) {
	dir := t.TempDir()
	// Simulate a file still encrypted by git-crypt
	content := append([]byte("\x00GITCRYPT"), []byte("encrypted-garbage-data")...)
	path := filepath.Join(dir, "secrets.dev.yaml")
	require.NoError(t, os.WriteFile(path, content, 0600))

	_, err := LoadSecretsPlaintext(path)
	require.Error(t, err)
	assert.Contains(t, err.Error(), "git-crypt unlock")
}

func TestLoadSecretsPlaintext_FileNotFound(t *testing.T) {
	_, err := LoadSecretsPlaintext("/nonexistent/path/secrets.yaml")
	require.Error(t, err)
	assert.Contains(t, err.Error(), "failed to read secrets file")
}
