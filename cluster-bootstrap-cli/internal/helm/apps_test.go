package helm

import (
	"path/filepath"
	"strings"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"helm.sh/helm/v3/pkg/chart"
	"helm.sh/helm/v3/pkg/chart/loader"
	"helm.sh/helm/v3/pkg/chartutil"
	"helm.sh/helm/v3/pkg/engine"
	"helm.sh/helm/v3/pkg/strvals"
)

func TestAppOfAppsRendersCiliumOnlyWhenEnabled(t *testing.T) {
	chartDir := filepath.Join("..", "..", "..", "apps")
	appChart, err := loader.Load(chartDir)
	require.NoError(t, err)

	disabledValues, err := chartutil.ReadValuesFile(filepath.Join(chartDir, "values.yaml"))
	require.NoError(t, err)
	disabledOutput := renderAppOfApps(t, appChart, disabledValues)
	assert.NotContains(t, disabledOutput, "name: cilium")

	enabledValues, err := chartutil.ReadValuesFile(filepath.Join(chartDir, "values.yaml"))
	require.NoError(t, err)
	require.NoError(t, strvals.ParseInto("components.cilium.enabled=true", enabledValues))
	enabledOutput := renderAppOfApps(t, appChart, enabledValues)

	start := strings.Index(enabledOutput, "metadata:\n  name: cilium")
	require.NotEqual(t, -1, start)
	application := enabledOutput[start:]
	if end := strings.Index(application, "\n---"); end >= 0 {
		application = application[:end]
	}
	assert.Contains(t, application, "releaseName: cilium")
	assert.Contains(t, application, "namespace: kube-system")
	assert.Contains(t, application, "ServerSideApply=true")
	assert.NotContains(t, application, "finalizers:")
}

func renderAppOfApps(t *testing.T, appChart *chart.Chart, values map[string]interface{}) string {
	t.Helper()
	release := chartutil.ReleaseOptions{Name: "app-of-apps", Namespace: "argocd", IsInstall: true}
	renderValues, err := chartutil.ToRenderValues(appChart, values, release, chartutil.DefaultCapabilities)
	require.NoError(t, err)
	rendered, err := engine.Render(appChart, renderValues)
	require.NoError(t, err)

	var output strings.Builder
	for _, manifest := range rendered {
		output.WriteString(manifest)
	}
	return output.String()
}
