package cmd

import (
	"os"
	"path/filepath"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/user-cube/cluster-bootstrap/cli/internal/config"
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

func TestRenderDryRunOutput_Golden(t *testing.T) {
	envSecrets := &config.EnvironmentSecrets{
		Repo: config.RepoSecrets{
			URL:            "ssh://git@example.com/repo.git",
			TargetRevision: "main",
			SSHPrivateKey:  "test-key",
		},
	}

	output, err := renderDryRunOutput(envSecrets, "dev", "apps")
	require.NoError(t, err)

	goldenPath := filepath.Join("testdata", "dry-run.dev.golden.txt")
	golden, err := os.ReadFile(goldenPath)
	require.NoError(t, err)
	assert.Equal(t, string(golden), output)
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

	appPath = "apps"
	encryption = "sops"
	secretsFile = filepath.Join(tmpDir, "secrets.dev.enc.yaml")
	require.NoError(t, os.RemoveAll(filepath.Join(tmpDir, "apps")))
	require.NoError(t, os.MkdirAll(filepath.Join(tmpDir, "k8s", "apps", "templates"), 0755))
	require.NoError(t, os.WriteFile(filepath.Join(tmpDir, "k8s", "apps", "Chart.yaml"), []byte("apiVersion: v2\n"), 0644))
	require.NoError(t, os.WriteFile(filepath.Join(tmpDir, "k8s", "apps", "templates", "application.yaml"), []byte("kind: Application\n"), 0644))
	require.NoError(t, validateBootstrapInputs("dev"))
	assert.Equal(t, filepath.Join("k8s", "apps"), appPath)
}
