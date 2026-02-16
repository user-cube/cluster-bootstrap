package k8s

import (
	"context"
	"encoding/json"
	"fmt"

	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/apis/meta/v1/unstructured"
	"k8s.io/apimachinery/pkg/runtime/schema"
)

const argoCDNamespace = "argocd"

// ApplyAppOfApps creates the App of Apps root Application CR.
func (c *Client) ApplyAppOfApps(ctx context.Context, repoURL, targetRevision, env, appPath string, dryRun bool) (string, error) {
	app := &unstructured.Unstructured{
		Object: map[string]interface{}{
			"apiVersion": "argoproj.io/v1alpha1",
			"kind":       "Application",
			"metadata": map[string]interface{}{
				"name":      "app-of-apps",
				"namespace": argoCDNamespace,
			},
			"spec": map[string]interface{}{
				"project": "default",
				"source": map[string]interface{}{
					"repoURL":        repoURL,
					"targetRevision": targetRevision,
					"path":           appPath,
					"helm": map[string]interface{}{
						"valueFiles": []interface{}{
							fmt.Sprintf("values/%s.yaml", env),
						},
					},
				},
				"destination": map[string]interface{}{
					"server":    "https://kubernetes.default.svc",
					"namespace": argoCDNamespace,
				},
				"syncPolicy": map[string]interface{}{
					"automated": map[string]interface{}{
						"prune":    true,
						"selfHeal": true,
					},
				},
			},
		},
	}

	if dryRun {
		data, err := json.MarshalIndent(app.Object, "", "  ")
		if err != nil {
			return fmt.Sprintf("%+v", app.Object), nil
		}
		return string(data), nil
	}

	gvr := schema.GroupVersionResource{
		Group:    "argoproj.io",
		Version:  "v1alpha1",
		Resource: "applications",
	}

	_, err := c.DynamicClient.Resource(gvr).Namespace(argoCDNamespace).Apply(
		ctx, "app-of-apps", app, metav1.ApplyOptions{FieldManager: "cluster-bootstrap"},
	)
	if err != nil {
		return "", fmt.Errorf("failed to apply App of Apps: %w", err)
	}

	return "", nil
}
