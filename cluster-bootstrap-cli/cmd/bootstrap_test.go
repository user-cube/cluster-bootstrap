package cmd

import (
	"os"
	"path/filepath"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/user-cube/cluster-bootstrap/cluster-bootstrap-cli/internal/config"
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
