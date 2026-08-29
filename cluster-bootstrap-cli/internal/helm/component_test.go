package helm

import (
	"errors"
	"os"
	"path/filepath"
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"helm.sh/helm/v3/pkg/chart/loader"
	"helm.sh/helm/v3/pkg/chartutil"
	"helm.sh/helm/v3/pkg/engine"
	"helm.sh/helm/v3/pkg/storage/driver"
)

func TestLoadChartConfigForComponent(t *testing.T) {
	baseDir := t.TempDir()
	componentDir := filepath.Join(baseDir, "components", "cilium")
	require.NoError(t, os.MkdirAll(componentDir, 0755))
	require.NoError(t, os.WriteFile(filepath.Join(componentDir, "Chart.yaml"), []byte(`apiVersion: v2
name: cilium
version: 1.0.0
dependencies:
  - name: cilium
    version: 1.20.1
    repository: https://helm.cilium.io/
`), 0600))

	name, version, repoURL, err := loadChartConfig(baseDir, "cilium", "cilium")
	require.NoError(t, err)
	assert.Equal(t, "cilium", name)
	assert.Equal(t, "1.20.1", version)
	assert.Equal(t, "https://helm.cilium.io/", repoURL)
}

func TestLoadComponentValues(t *testing.T) {
	baseDir := t.TempDir()
	valuesDir := filepath.Join(baseDir, "components", "cilium", "values")
	require.NoError(t, os.MkdirAll(valuesDir, 0755))
	require.NoError(t, os.WriteFile(filepath.Join(valuesDir, "base.yaml"), []byte(`cilium:
  operator:
    replicas: 1
  hubble:
    enabled: false
`), 0600))
	require.NoError(t, os.WriteFile(filepath.Join(valuesDir, "dev.yaml"), []byte(`cilium:
  operator:
    replicas: 2
`), 0600))

	values, err := loadComponentValues(baseDir, "cilium", "dev")
	require.NoError(t, err)
	dependency := values["cilium"].(map[string]interface{})

	operator := dependency["operator"].(map[string]interface{})
	assert.Equal(t, float64(2), operator["replicas"])
	hubble := dependency["hubble"].(map[string]interface{})
	assert.Equal(t, false, hubble["enabled"])
}

func TestLoadValuesPreservesArgoCDBootstrapPrecedence(t *testing.T) {
	baseDir := t.TempDir()
	valuesDir := filepath.Join(baseDir, "components", "argocd", "values")
	require.NoError(t, os.MkdirAll(valuesDir, 0755))
	require.NoError(t, os.WriteFile(filepath.Join(valuesDir, "base.yaml"), []byte("replicas: 1\n"), 0600))
	require.NoError(t, os.WriteFile(filepath.Join(valuesDir, "dev.yaml"), []byte("replicas: 2\n"), 0600))

	values, err := loadValues(baseDir, "dev")
	require.NoError(t, err)
	assert.Equal(t, float64(1), values["replicas"])
}

func TestCiliumComponentUsesWaitedIdempotentRelease(t *testing.T) {
	assert.Equal(t, "cilium", ciliumComponent.name)
	assert.Equal(t, "cilium", ciliumComponent.releaseName)
	assert.Equal(t, "kube-system", ciliumComponent.namespace)
	assert.Equal(t, "cilium", ciliumComponent.dependencyName)
	assert.True(t, ciliumComponent.installWrapper)
	assert.True(t, ciliumComponent.envOverridesBase)
	assert.True(t, ciliumComponent.wait)
	assert.Equal(t, 5*time.Minute, ciliumComponent.timeout)
}

func TestHelmReleaseExists(t *testing.T) {
	exists, err := helmReleaseExists(nil)
	require.NoError(t, err)
	assert.True(t, exists)

	exists, err = helmReleaseExists(driver.ErrReleaseNotFound)
	require.NoError(t, err)
	assert.False(t, exists)

	historyErr := errors.New("permission denied")
	exists, err = helmReleaseExists(historyErr)
	assert.ErrorIs(t, err, historyErr)
	assert.False(t, exists)
}

func TestLoadInstallChartAttachesDependencyToCiliumWrapper(t *testing.T) {
	baseDir := t.TempDir()
	wrapperDir := filepath.Join(baseDir, "components", "cilium")
	dependencyDir := filepath.Join(baseDir, "downloaded-cilium")
	require.NoError(t, os.MkdirAll(wrapperDir, 0755))
	require.NoError(t, os.MkdirAll(dependencyDir, 0755))
	require.NoError(t, os.WriteFile(filepath.Join(wrapperDir, "Chart.yaml"), []byte(`apiVersion: v2
name: cilium
version: 1.0.0
dependencies:
  - name: cilium
    version: 1.20.1
    repository: https://helm.cilium.io/
`), 0600))
	require.NoError(t, os.WriteFile(filepath.Join(dependencyDir, "Chart.yaml"), []byte(`apiVersion: v2
name: cilium
version: 1.20.1
`), 0600))

	installChart, err := loadInstallChart(baseDir, dependencyDir, ciliumComponent)
	require.NoError(t, err)
	assert.Equal(t, "1.0.0", installChart.Metadata.Version)
	require.Len(t, installChart.Dependencies(), 1)
	assert.Equal(t, "cilium", installChart.Dependencies()[0].Name())
	assert.Equal(t, "1.20.1", installChart.Dependencies()[0].Metadata.Version)
}

func TestLoadInstallChartReplacesCachedDependency(t *testing.T) {
	baseDir := t.TempDir()
	wrapperDir := filepath.Join(baseDir, "components", "cilium")
	cachedDependencyDir := filepath.Join(wrapperDir, "charts", "cilium")
	downloadedDependencyDir := filepath.Join(baseDir, "downloaded-cilium")
	require.NoError(t, os.MkdirAll(cachedDependencyDir, 0755))
	require.NoError(t, os.MkdirAll(downloadedDependencyDir, 0755))
	require.NoError(t, os.WriteFile(filepath.Join(wrapperDir, "Chart.yaml"), []byte(`apiVersion: v2
name: cilium
version: 1.0.0
dependencies:
  - name: cilium
    version: 1.20.1
    repository: https://helm.cilium.io/
`), 0600))
	require.NoError(t, os.WriteFile(filepath.Join(cachedDependencyDir, "Chart.yaml"), []byte(`apiVersion: v2
name: cilium
version: 0.0.1
`), 0600))
	require.NoError(t, os.WriteFile(filepath.Join(downloadedDependencyDir, "Chart.yaml"), []byte(`apiVersion: v2
name: cilium
version: 1.20.1
`), 0600))

	installChart, err := loadInstallChart(baseDir, downloadedDependencyDir, ciliumComponent)
	require.NoError(t, err)
	require.Len(t, installChart.Dependencies(), 1)
	assert.Equal(t, "1.20.1", installChart.Dependencies()[0].Metadata.Version)
}

func TestBootstrapWrapperRendersLikeRepositoryWrapper(t *testing.T) {
	baseDir := t.TempDir()
	wrapperDir := filepath.Join(baseDir, "components", "cilium")
	cachedDependencyDir := filepath.Join(wrapperDir, "charts", "cilium")
	downloadedDependencyDir := filepath.Join(baseDir, "downloaded-cilium")
	for _, directory := range []string{
		filepath.Join(cachedDependencyDir, "templates"),
		filepath.Join(downloadedDependencyDir, "templates"),
	} {
		require.NoError(t, os.MkdirAll(directory, 0755))
	}
	require.NoError(t, os.WriteFile(filepath.Join(wrapperDir, "Chart.yaml"), []byte(`apiVersion: v2
name: cilium
version: 1.0.0
dependencies:
  - name: cilium
    version: 1.20.1
    repository: https://helm.cilium.io/
`), 0600))
	dependencyChart := []byte("apiVersion: v2\nname: cilium\nversion: 1.20.1\n")
	dependencyTemplate := []byte("apiVersion: v1\nkind: ConfigMap\nmetadata:\n  name: cilium\ndata:\n  mode: {{ .Values.mode | quote }}\n")
	for _, directory := range []string{cachedDependencyDir, downloadedDependencyDir} {
		require.NoError(t, os.WriteFile(filepath.Join(directory, "Chart.yaml"), dependencyChart, 0600))
		require.NoError(t, os.WriteFile(filepath.Join(directory, "templates", "configmap.yaml"), dependencyTemplate, 0600))
	}

	bootstrapChart, err := loadInstallChart(baseDir, downloadedDependencyDir, ciliumComponent)
	require.NoError(t, err)
	repositoryChart, err := loader.Load(wrapperDir)
	require.NoError(t, err)
	values := map[string]interface{}{
		"cilium": map[string]interface{}{"mode": "gitops"},
	}
	release := chartutil.ReleaseOptions{Name: "cilium", Namespace: "kube-system", IsInstall: true}

	bootstrapValues, err := chartutil.ToRenderValues(bootstrapChart, values, release, chartutil.DefaultCapabilities)
	require.NoError(t, err)
	bootstrapRendered, err := engine.Render(bootstrapChart, bootstrapValues)
	require.NoError(t, err)
	repositoryValues, err := chartutil.ToRenderValues(repositoryChart, values, release, chartutil.DefaultCapabilities)
	require.NoError(t, err)
	repositoryRendered, err := engine.Render(repositoryChart, repositoryValues)
	require.NoError(t, err)

	assert.Equal(t, repositoryRendered, bootstrapRendered)
}
