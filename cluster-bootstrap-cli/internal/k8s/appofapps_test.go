package k8s

import (
	"context"
	"os"
	"path/filepath"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"k8s.io/apimachinery/pkg/apis/meta/v1/unstructured"
	"k8s.io/apimachinery/pkg/runtime"
	"k8s.io/apimachinery/pkg/runtime/schema"
	dynamicfake "k8s.io/client-go/dynamic/fake"
)

func newFakeAppClient(objects ...runtime.Object) *Client {
	scheme := runtime.NewScheme()
	listKinds := map[schema.GroupVersionResource]string{
		applicationGVR: "ApplicationList",
	}
	return &Client{
		DynamicClient: dynamicfake.NewSimpleDynamicClientWithCustomListKinds(scheme, listKinds, objects...),
	}
}

func TestGetAppOfApps_AbsentReportsNil(t *testing.T) {
	client := newFakeAppClient()

	app, err := client.GetAppOfApps(context.Background())
	require.NoError(t, err)
	assert.Nil(t, app, "a cluster without an App of Apps must not report one")
}

func TestGetAppOfApps_ReturnsExistingApplication(t *testing.T) {
	existing := buildAppOfApps("ssh://git@example.com/repo.git", "main", "dev", "apps", false)
	client := newFakeAppClient(existing)

	app, err := client.GetAppOfApps(context.Background())
	require.NoError(t, err)
	require.NotNil(t, app)
	assert.Equal(t, AppOfAppsName, app.GetName())
	assert.Equal(t, argoCDNamespace, app.GetNamespace())

	repoURL, found, err := unstructured.NestedString(app.Object, "spec", "source", "repoURL")
	require.NoError(t, err)
	require.True(t, found)
	assert.Equal(t, "ssh://git@example.com/repo.git", repoURL)
}

func TestGetAppOfApps_IgnoresOtherApplications(t *testing.T) {
	other := &unstructured.Unstructured{
		Object: map[string]interface{}{
			"apiVersion": "argoproj.io/v1alpha1",
			"kind":       "Application",
			"metadata": map[string]interface{}{
				"name":      "vault",
				"namespace": argoCDNamespace,
			},
		},
	}
	client := newFakeAppClient(other)

	app, err := client.GetAppOfApps(context.Background())
	require.NoError(t, err)
	assert.Nil(t, app)
}

func TestResolveContext(t *testing.T) {
	dir := t.TempDir()
	kubeconfigPath := filepath.Join(dir, "config")
	kubeconfig := `apiVersion: v1
kind: Config
current-context: kind-dev
clusters:
- name: dev
  cluster:
    server: https://dev.example.com
- name: prod
  cluster:
    server: https://prod.example.com
contexts:
- name: kind-dev
  context:
    cluster: dev
    user: dev
- name: kind-prod
  context:
    cluster: prod
    user: prod
users:
- name: dev
  user: {}
- name: prod
  user: {}
`
	require.NoError(t, os.WriteFile(kubeconfigPath, []byte(kubeconfig), 0o600))

	t.Run("falls back to the kubeconfig current context", func(t *testing.T) {
		resolved, err := ResolveContext(kubeconfigPath, "")
		require.NoError(t, err)
		assert.Equal(t, "kind-dev", resolved)
	})

	t.Run("prefers the explicit override", func(t *testing.T) {
		resolved, err := ResolveContext(kubeconfigPath, "kind-prod")
		require.NoError(t, err)
		assert.Equal(t, "kind-prod", resolved)
	})

	t.Run("errors when no current context is set", func(t *testing.T) {
		emptyPath := filepath.Join(dir, "empty")
		require.NoError(t, os.WriteFile(emptyPath, []byte("apiVersion: v1\nkind: Config\n"), 0o600))

		_, err := ResolveContext(emptyPath, "")
		require.Error(t, err)
		assert.Contains(t, err.Error(), "no current context set")
	})
}
