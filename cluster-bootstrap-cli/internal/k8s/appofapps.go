package k8s

import (
	"context"
	"encoding/json"
	"fmt"

	apierrors "k8s.io/apimachinery/pkg/api/errors"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/apis/meta/v1/unstructured"
	"k8s.io/apimachinery/pkg/runtime/schema"
)

const argoCDNamespace = "argocd"

// ApplyAppOfApps creates or updates the App of Apps root Application CR.
// Returns a boolean indicating if it was created (true) or updated (false) when not in dry-run mode.
// NOTE: This function's signature was changed to return an additional boolean value, which is a
// breaking API change. External callers must be updated to handle the extra return value.
func (c *Client) ApplyAppOfApps(ctx context.Context, repoURL, targetRevision, env, appPath string, dryRun bool) (string, bool, error) {
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
			return fmt.Sprintf("%+v", app.Object), true, nil
		}
		return string(data), true, nil
	}

	gvr := schema.GroupVersionResource{
		Group:    "argoproj.io",
		Version:  "v1alpha1",
		Resource: "applications",
	}

	// Check if Application already exists
	_, err := c.DynamicClient.Resource(gvr).Namespace(argoCDNamespace).Get(ctx, "app-of-apps", metav1.GetOptions{})
	exists := err == nil

	_, err = c.DynamicClient.Resource(gvr).Namespace(argoCDNamespace).Apply(
		ctx, "app-of-apps", app, metav1.ApplyOptions{FieldManager: "cluster-bootstrap"},
	)
	if err != nil {
		if apierrors.IsForbidden(err) {
			return "", false, fmt.Errorf("permission denied: cannot apply Application CRD: %w\n  hint: verify ArgoCD CRDs are installed and your role has permission to apply them\n  tip: check: kubectl api-resources | grep Application", err)
		}
		if apierrors.IsNotFound(err) {
			return "", false, fmt.Errorf("ArgoCD CRD not found: %w\n  hint: ensure ArgoCD is installed before creating Applications\n  tip: try: kubectl get crd applications.argoproj.io", err)
		}
		return "", false, fmt.Errorf("failed to apply App of Apps: %w\n  hint: verify the Application CR is valid and ArgoCD is running", err)
	}

	return "", !exists, nil
}
