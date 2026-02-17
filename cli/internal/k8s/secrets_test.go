package k8s

import (
	"context"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	corev1 "k8s.io/api/core/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/client-go/kubernetes/fake"
)

// TestCreateRepoSSHSecret_Idempotent verifies that CreateRepoSSHSecret is idempotent:
// - Returns (secret, true, nil) when creating a new secret
// - Returns (secret, false, nil) when updating an existing secret
func TestCreateRepoSSHSecret_Idempotent(t *testing.T) {
	ctx := context.Background()
	repoURL := "ssh://git@github.com/org/repo.git"
	sshKey := "test-ssh-private-key"

	t.Run("creates new secret", func(t *testing.T) {
		//nolint:staticcheck // SA1019: fake.NewSimpleClientset is deprecated but alternative requires generated apply configs
		fakeClient := fake.NewSimpleClientset()
		client := &Client{Clientset: fakeClient}

		// Ensure namespace exists
		_, err := fakeClient.CoreV1().Namespaces().Create(ctx, &corev1.Namespace{
			ObjectMeta: metav1.ObjectMeta{Name: "argocd"},
		}, metav1.CreateOptions{})
		require.NoError(t, err)

		secret, created, err := client.CreateRepoSSHSecret(ctx, repoURL, sshKey, false)
		require.NoError(t, err)
		assert.True(t, created, "should indicate secret was created")
		assert.NotNil(t, secret)
		assert.Equal(t, "repo-ssh-key", secret.Name)
	})

	t.Run("updates existing secret", func(t *testing.T) {
		// Pre-create the secret
		existingSecret := &corev1.Secret{
			ObjectMeta: metav1.ObjectMeta{
				Name:      "repo-ssh-key",
				Namespace: "argocd",
			},
			StringData: map[string]string{
				"type":          "git",
				"url":           "old-url",
				"sshPrivateKey": "old-key",
			},
		}

		//nolint:staticcheck // SA1019: fake.NewSimpleClientset is deprecated but alternative requires generated apply configs
		fakeClient := fake.NewSimpleClientset(&corev1.Namespace{
			ObjectMeta: metav1.ObjectMeta{Name: "argocd"},
		}, existingSecret)
		client := &Client{Clientset: fakeClient}

		secret, created, err := client.CreateRepoSSHSecret(ctx, repoURL, sshKey, false)
		require.NoError(t, err)
		assert.False(t, created, "should indicate secret was updated, not created")
		assert.NotNil(t, secret)

		// Verify the secret was updated
		updated, err := fakeClient.CoreV1().Secrets("argocd").Get(ctx, "repo-ssh-key", metav1.GetOptions{})
		require.NoError(t, err)
		assert.Equal(t, repoURL, updated.StringData["url"])
	})
}

// TestCreateGitCryptKeySecret_Idempotent verifies idempotent behavior for git-crypt key
func TestCreateGitCryptKeySecret_Idempotent(t *testing.T) {
	ctx := context.Background()
	keyData := []byte("test-git-crypt-key-data")

	t.Run("creates new secret", func(t *testing.T) {
		//nolint:staticcheck // SA1019: fake.NewSimpleClientset is deprecated but alternative requires generated apply configs
		fakeClient := fake.NewSimpleClientset(&corev1.Namespace{
			ObjectMeta: metav1.ObjectMeta{Name: "argocd"},
		})
		client := &Client{Clientset: fakeClient}

		created, err := client.CreateGitCryptKeySecret(ctx, keyData)
		require.NoError(t, err)
		assert.True(t, created, "should indicate secret was created")

		// Verify secret exists
		secret, err := fakeClient.CoreV1().Secrets("argocd").Get(ctx, "git-crypt-key", metav1.GetOptions{})
		require.NoError(t, err)
		assert.Equal(t, keyData, secret.Data["git-crypt-key"])
	})

	t.Run("updates existing secret", func(t *testing.T) {
		existingSecret := &corev1.Secret{
			ObjectMeta: metav1.ObjectMeta{
				Name:      "git-crypt-key",
				Namespace: "argocd",
			},
			Data: map[string][]byte{
				"git-crypt-key": []byte("old-key-data"),
			},
		}

		//nolint:staticcheck // SA1019: fake.NewSimpleClientset is deprecated but alternative requires generated apply configs
		fakeClient := fake.NewSimpleClientset(&corev1.Namespace{
			ObjectMeta: metav1.ObjectMeta{Name: "argocd"},
		}, existingSecret)
		client := &Client{Clientset: fakeClient}

		created, err := client.CreateGitCryptKeySecret(ctx, keyData)
		require.NoError(t, err)
		assert.False(t, created, "should indicate secret was updated")

		// Verify secret was updated
		updated, err := fakeClient.CoreV1().Secrets("argocd").Get(ctx, "git-crypt-key", metav1.GetOptions{})
		require.NoError(t, err)
		assert.Equal(t, keyData, updated.Data["git-crypt-key"])
	})
}

// TestEnsureNamespace_Idempotent verifies namespace creation is idempotent
func TestEnsureNamespace_Idempotent(t *testing.T) {
	ctx := context.Background()

	t.Run("creates new namespace", func(t *testing.T) {
		//nolint:staticcheck // SA1019: fake.NewSimpleClientset is deprecated but alternative requires generated apply configs
		fakeClient := fake.NewSimpleClientset()
		client := &Client{Clientset: fakeClient}

		created, err := client.EnsureNamespace(ctx, "argocd")
		require.NoError(t, err)
		assert.True(t, created, "namespace should be created")

		ns, err := fakeClient.CoreV1().Namespaces().Get(ctx, "argocd", metav1.GetOptions{})
		require.NoError(t, err)
		assert.Equal(t, "argocd", ns.Name)
	})

	t.Run("does not fail if namespace exists", func(t *testing.T) {
		existingNS := &corev1.Namespace{
			ObjectMeta: metav1.ObjectMeta{Name: "argocd"},
		}
		//nolint:staticcheck // SA1019: fake.NewSimpleClientset is deprecated but alternative requires generated apply configs
		fakeClient := fake.NewSimpleClientset(existingNS)
		client := &Client{Clientset: fakeClient}

		created, err := client.EnsureNamespace(ctx, "argocd")
		require.NoError(t, err, "should not fail when namespace already exists")
		assert.False(t, created, "namespace should not be created when it already exists")
	})
}

// TestCreateRepoSSHSecret_DryRun verifies dry-run mode doesn't modify cluster state
func TestCreateRepoSSHSecret_DryRun(t *testing.T) {
	ctx := context.Background()
	//nolint:staticcheck // SA1019: fake.NewSimpleClientset is deprecated but alternative requires generated apply configs
	fakeClient := fake.NewSimpleClientset(&corev1.Namespace{
		ObjectMeta: metav1.ObjectMeta{Name: "argocd"},
	})
	client := &Client{Clientset: fakeClient}

	secret, created, err := client.CreateRepoSSHSecret(ctx, "ssh://example.com", "key", true)
	require.NoError(t, err)
	assert.True(t, created, "dry-run always returns true for created")
	assert.NotNil(t, secret)

	// Verify no secret was actually created
	secrets, err := fakeClient.CoreV1().Secrets("argocd").List(ctx, metav1.ListOptions{})
	require.NoError(t, err)
	assert.Empty(t, secrets.Items, "dry-run should not create any secrets")
}

// TestBootstrapIdempotence_Integration simulates running bootstrap multiple times
func TestBootstrapIdempotence_Integration(t *testing.T) {
	ctx := context.Background()
	repoURL := "ssh://git@github.com/org/repo.git"
	sshKey := "test-key"
	gitCryptKey := []byte("git-crypt-symmetric-key")

	//nolint:staticcheck // SA1019: fake.NewSimpleClientset is deprecated but alternative requires generated apply configs
	fakeClient := fake.NewSimpleClientset()
	client := &Client{Clientset: fakeClient}

	// First bootstrap run
	t.Log("First bootstrap run")
	created, err := client.EnsureNamespace(ctx, "argocd")
	require.NoError(t, err)
	assert.True(t, created, "first run should create namespace")

	_, created1, err := client.CreateRepoSSHSecret(ctx, repoURL, sshKey, false)
	require.NoError(t, err)
	assert.True(t, created1, "first run should create secret")

	created2, err := client.CreateGitCryptKeySecret(ctx, gitCryptKey)
	require.NoError(t, err)
	assert.True(t, created2, "first run should create git-crypt secret")

	// Second bootstrap run (idempotent)
	t.Log("Second bootstrap run (idempotent)")
	created, err = client.EnsureNamespace(ctx, "argocd")
	require.NoError(t, err, "namespace creation is idempotent")
	assert.False(t, created, "second run should not create namespace")

	_, created3, err := client.CreateRepoSSHSecret(ctx, repoURL, sshKey, false)
	require.NoError(t, err)
	assert.False(t, created3, "second run should update existing secret")

	created4, err := client.CreateGitCryptKeySecret(ctx, gitCryptKey)
	require.NoError(t, err)
	assert.False(t, created4, "second run should update existing git-crypt secret")

	// Third bootstrap run with updated values
	t.Log("Third bootstrap run with updated repo URL")
	newRepoURL := "ssh://git@github.com/org/new-repo.git"

	_, created5, err := client.CreateRepoSSHSecret(ctx, newRepoURL, sshKey, false)
	require.NoError(t, err)
	assert.False(t, created5, "third run should update with new URL")

	// Verify final state
	secret, err := fakeClient.CoreV1().Secrets("argocd").Get(ctx, "repo-ssh-key", metav1.GetOptions{})
	require.NoError(t, err)
	assert.Equal(t, newRepoURL, string(secret.Data["url"]), "secret should have latest URL")
}

// TestApplyAppOfApps_MultipleRuns ensures Apply is idempotent
func TestApplyAppOfApps_MultipleRuns(t *testing.T) {
	// This test would require setting up a fake dynamic client
	// which is more complex. The behavior is verified in integration tests.
	t.Skip("Requires dynamic client setup - covered by integration tests")
}
