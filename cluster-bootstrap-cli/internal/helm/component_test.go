package helm

import (
	"context"
	"errors"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"helm.sh/helm/v3/pkg/chart/loader"
	"helm.sh/helm/v3/pkg/chartutil"
	"helm.sh/helm/v3/pkg/engine"
	"helm.sh/helm/v3/pkg/storage/driver"
	"k8s.io/client-go/discovery/cached/memory"
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

func TestCiliumServiceMonitorsEnabled(t *testing.T) {
	baseDir := t.TempDir()
	valuesDir := filepath.Join(baseDir, "components", "cilium", "values")
	require.NoError(t, os.MkdirAll(valuesDir, 0755))
	require.NoError(t, os.WriteFile(filepath.Join(valuesDir, "base.yaml"), []byte(`cilium:
  prometheus:
    serviceMonitor:
      enabled: false
  operator:
    prometheus:
      serviceMonitor:
        enabled: false
  hubble:
    metrics:
      serviceMonitor:
        enabled: false
`), 0600))
	require.NoError(t, os.WriteFile(filepath.Join(valuesDir, "homelab.yaml"), []byte(`cilium:
  hubble:
    metrics:
      serviceMonitor:
        enabled: true
`), 0600))

	enabled, err := ciliumServiceMonitorsEnabled(baseDir, "homelab")
	require.NoError(t, err)
	assert.True(t, enabled)

	disabled, err := ciliumServiceMonitorsEnabled(baseDir, "missing")
	require.NoError(t, err)
	assert.False(t, disabled)
}

func TestPrometheusOperatorCRDsComponentDoesNotRequireValuesFiles(t *testing.T) {
	assert.Equal(t, "prometheus-operator-crds", prometheusOperatorCRDsComponent.name)
	assert.False(t, prometheusOperatorCRDsComponent.hasValues)

	values, err := loadValuesForComponent(t.TempDir(), "homelab", prometheusOperatorCRDsComponent)
	require.NoError(t, err)
	assert.Empty(t, values)
}

func TestIsServiceMonitorCRDNotFound(t *testing.T) {
	assert.True(t, isServiceMonitorCRDNotFound(memory.ErrCacheNotFound))
	assert.False(t, isServiceMonitorCRDNotFound(errors.New("permission denied")))
}

func TestInstallCiliumInstallsRequiredCRDsBeforeCilium(t *testing.T) {
	baseDir := t.TempDir()
	valuesDir := filepath.Join(baseDir, "components", "cilium", "values")
	require.NoError(t, os.MkdirAll(valuesDir, 0755))
	require.NoError(t, os.WriteFile(filepath.Join(valuesDir, "base.yaml"), []byte(`cilium:
  prometheus:
    serviceMonitor:
      enabled: true
`), 0600))

	previousInstall := installComponentFn
	previousAvailable := serviceMonitorCRDAvailableFn
	previousWait := waitForServiceMonitorCRDFn
	t.Cleanup(func() {
		installComponentFn = previousInstall
		serviceMonitorCRDAvailableFn = previousAvailable
		waitForServiceMonitorCRDFn = previousWait
	})

	calls := []string{}
	installComponentFn = func(_ context.Context, _, _, _, _ string, _, _ bool, _ string, component componentConfig) (bool, error) {
		calls = append(calls, component.name)
		return true, nil
	}
	serviceMonitorCRDAvailableFn = func(_, _ string) (bool, error) { return false, nil }
	waitForServiceMonitorCRDFn = func(_ context.Context, _, _ string) error {
		calls = append(calls, "wait-for-servicemonitor-crd")
		return nil
	}

	_, err := InstallCilium(context.Background(), "", "", "homelab", baseDir, false, false, "")
	require.NoError(t, err)
	assert.Equal(t, []string{"prometheus-operator-crds", "wait-for-servicemonitor-crd", "cilium"}, calls)
}

func TestInstallCiliumSkipsCRDInstallWhenServiceMonitorCRDExists(t *testing.T) {
	baseDir := t.TempDir()
	valuesDir := filepath.Join(baseDir, "components", "cilium", "values")
	require.NoError(t, os.MkdirAll(valuesDir, 0755))
	require.NoError(t, os.WriteFile(filepath.Join(valuesDir, "base.yaml"), []byte(`cilium:
  prometheus:
    serviceMonitor:
      enabled: true
`), 0600))

	previousInstall := installComponentFn
	previousAvailable := serviceMonitorCRDAvailableFn
	previousWait := waitForServiceMonitorCRDFn
	t.Cleanup(func() {
		installComponentFn = previousInstall
		serviceMonitorCRDAvailableFn = previousAvailable
		waitForServiceMonitorCRDFn = previousWait
	})

	calls := []string{}
	installComponentFn = func(_ context.Context, _, _, _, _ string, _, _ bool, _ string, component componentConfig) (bool, error) {
		calls = append(calls, component.name)
		return true, nil
	}
	serviceMonitorCRDAvailableFn = func(_, _ string) (bool, error) { return true, nil }
	waitForServiceMonitorCRDFn = func(_ context.Context, _, _ string) error {
		calls = append(calls, "wait-for-servicemonitor-crd")
		return nil
	}

	_, err := InstallCilium(context.Background(), "", "", "homelab", baseDir, false, false, "")
	require.NoError(t, err)
	assert.Equal(t, []string{"cilium"}, calls)
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

func TestRenderComponentTemplatesIncludesOnlyEnabledResources(t *testing.T) {
	componentDir := filepath.Join(t.TempDir(), "cilium")
	require.NoError(t, os.MkdirAll(filepath.Join(componentDir, "templates"), 0755))
	require.NoError(t, os.WriteFile(filepath.Join(componentDir, "Chart.yaml"), []byte("apiVersion: v2\nname: cilium\nversion: 1.20.1\n"), 0600))
	require.NoError(t, os.WriteFile(filepath.Join(componentDir, "templates", "servicemonitor.yaml"), []byte(`{{- if .Values.prometheus.serviceMonitor.enabled }}
apiVersion: monitoring.coreos.com/v1
kind: ServiceMonitor
metadata:
  name: cilium-agent
{{- end }}
`), 0600))

	componentChart, err := loader.Load(componentDir)
	require.NoError(t, err)

	disabled, err := renderComponentTemplates(componentChart, map[string]interface{}{
		"prometheus": map[string]interface{}{"serviceMonitor": map[string]interface{}{"enabled": false}},
	}, ciliumComponent, true)
	require.NoError(t, err)
	assert.NotContains(t, disabled, "ServiceMonitor")

	enabled, err := renderComponentTemplates(componentChart, map[string]interface{}{
		"prometheus": map[string]interface{}{"serviceMonitor": map[string]interface{}{"enabled": true}},
	}, ciliumComponent, true)
	require.NoError(t, err)
	assert.True(t, strings.Contains(enabled, "kind: ServiceMonitor"))
	assert.Contains(t, enabled, "name: cilium-agent")
}
