package cmd

import (
	"context"
	"errors"
	"os"
	"path/filepath"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/user-cube/cluster-bootstrap/cluster-bootstrap-cli/internal/config"
)

func TestCiliumFlagIsOptIn(t *testing.T) {
	flag := bootstrapCmd.Flags().Lookup("enable-cilium")
	require.NotNil(t, flag)
	assert.Equal(t, "false", flag.DefValue)
}

func TestRunBootstrapComponents_OrderAndGating(t *testing.T) {
	tests := []struct {
		name              string
		enableCilium      bool
		skipArgoCDInstall bool
		wantCalls         []string
	}{
		{name: "default installs only ArgoCD", wantCalls: []string{"argocd"}},
		{name: "Cilium is installed before ArgoCD", enableCilium: true, wantCalls: []string{"cilium", "argocd"}},
		{name: "skipped ArgoCD still installs Cilium", enableCilium: true, skipArgoCDInstall: true, wantCalls: []string{"cilium"}},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			calls := []string{}
			err := runBootstrapComponents(
				context.Background(),
				tt.enableCilium,
				tt.skipArgoCDInstall,
				func(context.Context) error {
					calls = append(calls, "cilium")
					return nil
				},
				func(context.Context) error {
					calls = append(calls, "argocd")
					return nil
				},
			)

			require.NoError(t, err)
			assert.Equal(t, tt.wantCalls, calls)
		})
	}
}

func TestRunBootstrapComponents_StopsWhenCiliumIsUnhealthy(t *testing.T) {
	healthErr := errors.New("Cilium agents are not ready")
	argocdCalled := false

	err := runBootstrapComponents(
		context.Background(),
		true,
		false,
		func(context.Context) error { return healthErr },
		func(context.Context) error {
			argocdCalled = true
			return nil
		},
	)

	assert.ErrorIs(t, err, healthErr)
	assert.ErrorContains(t, err, "Cilium")
	assert.False(t, argocdCalled)
}

func TestEnsureArgoCDReadyForCilium(t *testing.T) {
	tests := []struct {
		name         string
		enableCilium bool
		skipArgoCD   bool
		wantWait     bool
	}{
		{name: "disabled", skipArgoCD: true},
		{name: "ArgoCD installed by bootstrap", enableCilium: true},
		{name: "pre-existing ArgoCD", enableCilium: true, skipArgoCD: true, wantWait: true},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			waitCalled := false
			err := ensureArgoCDReadyForCilium(context.Background(), tt.enableCilium, tt.skipArgoCD, func(context.Context) error {
				waitCalled = true
				return nil
			})
			require.NoError(t, err)
			assert.Equal(t, tt.wantWait, waitCalled)
		})
	}
}

func TestEnsureArgoCDReadyForCilium_FailsHandoffWhenUnhealthy(t *testing.T) {
	healthErr := errors.New("argocd-server is not ready")
	err := ensureArgoCDReadyForCilium(context.Background(), true, true, func(context.Context) error {
		return healthErr
	})

	assert.ErrorIs(t, err, healthErr)
	assert.ErrorContains(t, err, "not ready for Cilium ownership handoff")
}

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

func TestRenderDryRunOutput_WithCilium(t *testing.T) {
	envSecrets := &config.EnvironmentSecrets{
		Repo: config.RepoSecrets{
			URL:            "ssh://git@example.com/repo.git",
			TargetRevision: "main",
			SSHPrivateKey:  "test-key",
		},
	}

	output, err := renderDryRunOutputWithOptions(envSecrets, "dev", "k8s/apps", true)
	require.NoError(t, err)
	assert.NotContains(t, output, "DRY RUN: Cilium Application")
	assert.Contains(t, output, `"name": "components.cilium.enabled"`)
	assert.Contains(t, output, `"value": "true"`)
}

func TestValidateCiliumEnvironment(t *testing.T) {
	for _, env := range []string{"dev", "eu-west.dev", "staging_1"} {
		assert.NoError(t, validateCiliumEnvironment(env))
	}
	for _, env := range []string{"..", "../prod", `dev\\prod`} {
		assert.Error(t, validateCiliumEnvironment(env))
	}
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

	_, err := validateBootstrapInputs("dev", "apps")
	require.NoError(t, err)

	secretsFile = filepath.Join(tmpDir, "secrets.dev.yaml")
	_, err = validateBootstrapInputs("dev", "apps")
	assert.ErrorContains(t, err, "must end with .enc.yaml")

	encryption = "git-crypt"
	secretsFile = filepath.Join(tmpDir, "secrets.dev.enc.yaml")
	_, err = validateBootstrapInputs("dev", "apps")
	assert.ErrorContains(t, err, "not .enc.yaml")

	_, err = validateBootstrapInputs("dev", "/abs/path")
	assert.ErrorContains(t, err, "app-path must be relative")

	appPath = "apps"
	encryption = "sops"
	secretsFile = filepath.Join(tmpDir, "secrets.dev.enc.yaml")
	require.NoError(t, os.RemoveAll(filepath.Join(tmpDir, "apps")))
	require.NoError(t, os.MkdirAll(filepath.Join(tmpDir, "k8s", "apps", "templates"), 0755))
	require.NoError(t, os.WriteFile(filepath.Join(tmpDir, "k8s", "apps", "Chart.yaml"), []byte("apiVersion: v2\n"), 0644))
	require.NoError(t, os.WriteFile(filepath.Join(tmpDir, "k8s", "apps", "templates", "application.yaml"), []byte("kind: Application\n"), 0644))

	// Test with baseDir pointing to k8s subfolder (simulating --base-dir ./k8s)
	baseDir = filepath.Join(tmpDir, "k8s")
	localPath, err := validateBootstrapInputs("dev", "k8s/apps")
	require.NoError(t, err)
	assert.Equal(t, "apps", localPath)
}
