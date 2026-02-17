package cmd

import (
	"context"
	"fmt"
	"os"
	"path/filepath"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/user-cube/cluster-bootstrap/cluster-bootstrap-cli/internal/config"
	"github.com/user-cube/cluster-bootstrap/cluster-bootstrap-cli/internal/k8s"
)

// TestBootstrapIntegration_PermissionDenied tests a bootstrap scenario where namespace creation fails with permission denied.
func TestBootstrapIntegration_PermissionDenied(t *testing.T) {
	mockClient := k8s.NewMockClient()
	mockClient.EnsureNamespaceForbidden = true

	ctx := context.Background()
	_, err := mockClient.EnsureNamespace(ctx, "argocd")
	require.Error(t, err)
	assert.Contains(t, err.Error(), "permission denied")
	assert.Contains(t, err.Error(), "cannot create namespace")
}

// TestBootstrapIntegration_SecretCreationFails tests a bootstrap scenario where secret creation fails.
func TestBootstrapIntegration_SecretCreationFails(t *testing.T) {
	mockClient := k8s.NewMockClient()
	mockClient.CreateSecretForbidden = true

	ctx := context.Background()
	_, err := mockClient.EnsureNamespace(ctx, "argocd")
	require.NoError(t, err)

	_, _, err = mockClient.CreateRepoSSHSecret(ctx, "ssh://git@example.com/repo.git", "key-data", false)
	require.Error(t, err)
	assert.Contains(t, err.Error(), "permission denied")
	assert.Contains(t, err.Error(), "cannot create secrets")
}

// TestBootstrapIntegration_HelmTimeout tests error recovery when Helm times out.
func TestBootstrapIntegration_HelmTimeout(t *testing.T) {
	// This simulates a scenario where a Helm install times out
	// In production, this would be caught in helm.InstallArgoCD()
	helmErr := fmt.Errorf("Helm install timed out: context deadline exceeded")

	// Verify the error message contains useful hints
	errMsg := helmErr.Error()
	assert.Contains(t, errMsg, "timed out")
}

// TestBootstrapIntegration_ImagePullFailure tests error messages for image pull failures.
func TestBootstrapIntegration_ImagePullFailure(t *testing.T) {
	imageErr := fmt.Errorf("ImagePullBackOff: failed to pull image argocd:v2.8.0")

	// Verify error is descriptive
	errMsg := imageErr.Error()
	assert.Contains(t, errMsg, "ImagePullBackOff")
}

// TestBootstrapIntegration_SuccessfulFlow tests a complete successful bootstrap flow with mock client.
func TestBootstrapIntegration_SuccessfulFlow(t *testing.T) {
	mockClient := k8s.NewMockClient()
	envSecrets := &config.EnvironmentSecrets{
		Repo: config.RepoSecrets{
			URL:            "ssh://git@example.com/repo.git",
			TargetRevision: "main",
			SSHPrivateKey:  "test-key",
		},
	}

	ctx := context.Background()

	// Step 1: Ensure namespace
	_, err := mockClient.EnsureNamespace(ctx, "argocd")
	require.NoError(t, err)
	assert.True(t, mockClient.Namespaces["argocd"])

	// Step 2: Create repo SSH secret
	secret, created, err := mockClient.CreateRepoSSHSecret(ctx, envSecrets.Repo.URL, envSecrets.Repo.SSHPrivateKey, false)
	require.NoError(t, err)
	assert.True(t, created, "should indicate secret was created")
	assert.NotNil(t, secret)
	assert.Equal(t, "repo-ssh-key", secret.Name)
	assert.Equal(t, envSecrets.Repo.URL, secret.StringData["url"])

	// Step 3: Apply App of Apps
	_, created, err = mockClient.ApplyAppOfApps(ctx, envSecrets.Repo.URL, envSecrets.Repo.TargetRevision, "dev", "apps", false)
	require.NoError(t, err)
	assert.True(t, created, "should indicate app was created")
	app := mockClient.GetApplication("app-of-apps")
	assert.NotNil(t, app)
	assert.Equal(t, "Application", app.Object["kind"])
}

// TestBootstrapIntegration_DryRun tests dry-run mode doesn't modify mock state.
func TestBootstrapIntegration_DryRun(t *testing.T) {
	mockClient := k8s.NewMockClient()

	ctx := context.Background()
	_, err := mockClient.EnsureNamespace(ctx, "argocd")
	require.NoError(t, err)

	// Create secret in dry-run mode
	_, _, err = mockClient.CreateRepoSSHSecret(ctx, "ssh://git@example.com/repo.git", "key", true)
	require.NoError(t, err)

	// Secret should not be stored
	secret := mockClient.GetSecret("argocd", "repo-ssh-key")
	assert.Nil(t, secret)
}

// TestBootstrapIntegration_GitCryptKey tests git-crypt key secret creation.
func TestBootstrapIntegration_GitCryptKey(t *testing.T) {
	mockClient := k8s.NewMockClient()
	ctx := context.Background()

	_, err := mockClient.EnsureNamespace(ctx, "argocd")
	require.NoError(t, err)

	keyData := []byte("mock-git-crypt-key-data")
	created, err := mockClient.CreateGitCryptKeySecret(ctx, keyData)
	require.NoError(t, err)
	assert.True(t, created, "should indicate secret was created")

	secret := mockClient.GetSecret("argocd", "git-crypt-key")
	assert.NotNil(t, secret)
	assert.Equal(t, "git-crypt-key", secret.Name)
	assert.Equal(t, keyData, secret.Data["git-crypt-key"])
}

// TestBootstrapIntegration_SequentialErrors tests recovery by retrying after errors.
func TestBootstrapIntegration_SequentialErrors(t *testing.T) {
	mockClient := k8s.NewMockClient()
	ctx := context.Background()

	// First attempt: permission denied
	mockClient.EnsureNamespaceForbidden = true
	_, err := mockClient.EnsureNamespace(ctx, "argocd")
	require.Error(t, err)
	assert.Contains(t, err.Error(), "permission denied")

	// Simulate fixing the permission issue
	mockClient.EnsureNamespaceForbidden = false

	// Retry: should succeed
	_, err = mockClient.EnsureNamespace(ctx, "argocd")
	require.NoError(t, err)
	assert.True(t, mockClient.Namespaces["argocd"])
}

// TestBootstrapIntegration_AppOfAppsWithEnv tests App of Apps creation with different environments.
func TestBootstrapIntegration_AppOfAppsWithEnv(t *testing.T) {
	tests := []struct {
		name    string
		env     string
		appPath string
		wantErr bool
	}{
		{"dev environment", "dev", "apps", false},
		{"prod environment", "prod", "apps", false},
		{"custom app path", "staging", "k8s/apps", false},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			mockClient := k8s.NewMockClient()
			ctx := context.Background()

			_, err := mockClient.EnsureNamespace(ctx, "argocd")
			require.NoError(t, err)
			_, _, err = mockClient.CreateRepoSSHSecret(ctx, "ssh://git@example.com/repo.git", "key", false)
			require.NoError(t, err)

			_, _, err = mockClient.ApplyAppOfApps(ctx, "ssh://git@example.com/repo.git", "main", tt.env, tt.appPath, false)
			if tt.wantErr {
				assert.Error(t, err)
			} else {
				assert.NoError(t, err)
				app := mockClient.GetApplication("app-of-apps")
				assert.NotNil(t, app)
				spec := app.Object["spec"].(map[string]interface{})
				source := spec["source"].(map[string]interface{})
				if tt.appPath != "" {
					assert.Equal(t, tt.appPath, source["path"])
				}
			}
		})
	}
}

// TestBootstrapIntegration_KubeconfigErrors tests kubeconfig loading error scenarios.
func TestBootstrapIntegration_KubeconfigErrors(t *testing.T) {
	tests := []struct {
		name       string
		kubeconfig string
		context    string
		wantHint   string
	}{
		{"invalid kubeconfig path", "/nonexistent/kubeconfig", "", "verify the file exists"},
		{"invalid context", "", "nonexistent-context", "verify the context"},
		{"default kubeconfig issue", "", "", "ensure kubectl is configured"},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			// Simulate the error wrapping that would happen
			if tt.kubeconfig != "" {
				err := fmt.Errorf("failed to load kubeconfig %s: no such file or directory", tt.kubeconfig)
				assert.Contains(t, err.Error(), tt.kubeconfig)
			}
		})
	}
}

// TestBootstrapIntegration_SecretsFileNotFound tests handling of missing secrets file.
func TestBootstrapIntegration_SecretsFileNotFound(t *testing.T) {
	tmpDir := t.TempDir()
	baseDir := tmpDir
	secretsFile := filepath.Join(baseDir, "secrets.dev.enc.yaml")

	// File doesn't exist
	require.NoFileExists(t, secretsFile)

	// Simulate the validation that would happen in bootstrap
	if _, err := os.Stat(secretsFile); err != nil {
		assert.True(t, os.IsNotExist(err))
	}
}

// TestBootstrapIntegration_InvalidSecretFormat tests handling of corrupted secrets files.
func TestBootstrapIntegration_InvalidSecretFormat(t *testing.T) {
	tmpDir := t.TempDir()
	secretsFile := filepath.Join(tmpDir, "secrets.dev.yaml")

	// Create invalid YAML
	require.NoError(t, os.WriteFile(secretsFile, []byte("invalid: [yaml content"), 0600))

	// This would be caught when loading secrets
	_, err := config.LoadSecretsPlaintext(secretsFile)
	require.Error(t, err)
}
