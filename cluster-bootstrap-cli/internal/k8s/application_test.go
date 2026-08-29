package k8s

import (
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestBuildCiliumApplication(t *testing.T) {
	app := BuildCiliumApplication(
		"ssh://git@example.com/repo.git",
		"main",
		"dev",
		"k8s/components/cilium",
	)

	metadata := app.Object["metadata"].(map[string]interface{})
	assert.Equal(t, "cilium", metadata["name"])
	assert.Equal(t, "argocd", metadata["namespace"])
	assert.NotContains(t, metadata, "finalizers")

	spec := app.Object["spec"].(map[string]interface{})
	source := spec["source"].(map[string]interface{})
	assert.Equal(t, "ssh://git@example.com/repo.git", source["repoURL"])
	assert.Equal(t, "main", source["targetRevision"])
	assert.Equal(t, "k8s/components/cilium", source["path"])
	helm := source["helm"].(map[string]interface{})
	assert.Equal(t, "cilium", helm["releaseName"])
	assert.Equal(t, []interface{}{"values/base.yaml", "values/dev.yaml"}, helm["valueFiles"])

	destination := spec["destination"].(map[string]interface{})
	assert.Equal(t, "kube-system", destination["namespace"])

	syncPolicy := spec["syncPolicy"].(map[string]interface{})
	automated := syncPolicy["automated"].(map[string]interface{})
	assert.Equal(t, true, automated["prune"])
	assert.Equal(t, true, automated["selfHeal"])
	require.ElementsMatch(t, []interface{}{"CreateNamespace=true", "ServerSideApply=true"}, syncPolicy["syncOptions"])
}
