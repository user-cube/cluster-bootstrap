package cmd

import (
	"os"
	"path/filepath"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/user-cube/cluster-bootstrap/cluster-bootstrap/internal/config"
)

func TestBuildDryRunObjects(t *testing.T) {
	envSecrets := &config.EnvironmentSecrets{
		Repo: config.RepoSecrets{
			URL:            "ssh://git@example.com/repo.git",
			TargetRevision: "main",
			SSHPrivateKey:  "test-key",
		},
	}

	repoSecret, appOfApps := buildDryRunObjects(envSecrets, "dev", "apps")

	metadata, ok := repoSecret["metadata"].(map[string]interface{})
	require.True(t, ok)
	assert.Equal(t, "repo-ssh-key", metadata["name"])

	appSpec, ok := appOfApps["spec"].(map[string]interface{})
	require.True(t, ok)
	source, ok := appSpec["source"].(map[string]interface{})
	require.True(t, ok)
	assert.Equal(t, "apps", source["path"])
	assert.Equal(t, "main", source["targetRevision"])
}

func TestValidateBootstrapInputs(t *testing.T) {
	prevBaseDir := baseDir
	prevAppPath := appPath
	prevEncryption := encryption
	prevSecretsFile := secretsFile

	t.Cleanup(func() {
		baseDir = prevBaseDir
		appPath = prevAppPath
		encryption = prevEncryption
		secretsFile = prevSecretsFile
	})

	tmpDir := t.TempDir()
	require.NoError(t, os.MkdirAll(filepath.Join(tmpDir, "apps"), 0755))

	baseDir = tmpDir
	appPath = "apps"
	encryption = "sops"
	secretsFile = filepath.Join(tmpDir, "secrets.dev.enc.yaml")

	require.NoError(t, validateBootstrapInputs("dev"))

	secretsFile = filepath.Join(tmpDir, "secrets.dev.yaml")
	assert.ErrorContains(t, validateBootstrapInputs("dev"), "must end with .enc.yaml")

	encryption = "git-crypt"
	secretsFile = filepath.Join(tmpDir, "secrets.dev.enc.yaml")
	assert.ErrorContains(t, validateBootstrapInputs("dev"), "not .enc.yaml")

	appPath = "/abs/path"
	assert.ErrorContains(t, validateBootstrapInputs("dev"), "app-path must be relative")
}
