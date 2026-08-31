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

// AppOfAppsName is the name of the root Application deployed by bootstrap.
const AppOfAppsName = "app-of-apps"

var applicationGVR = schema.GroupVersionResource{
	Group:    "argoproj.io",
	Version:  "v1alpha1",
	Resource: "applications",
}

// GetAppOfApps returns the App of Apps root Application already present in the
// cluster, or nil when it does not exist. A missing ArgoCD Application CRD is
// also reported as absent, since no App of Apps can exist without it.
func (c *Client) GetAppOfApps(ctx context.Context) (*unstructured.Unstructured, error) {
	app, err := c.DynamicClient.Resource(applicationGVR).Namespace(argoCDNamespace).Get(ctx, AppOfAppsName, metav1.GetOptions{})
	if err != nil {
		if apierrors.IsNotFound(err) {
			return nil, nil
		}
		return nil, fmt.Errorf("failed to look up the existing App of Apps: %w\n  hint: verify your role can read applications.argoproj.io in the argocd namespace\n  tip: try: kubectl -n argocd get application app-of-apps", err)
	}
	return app, nil
}

// ApplyAppOfApps creates or updates the App of Apps root Application CR.
// Returns a boolean indicating if it was created (true) or updated (false) when not in dry-run mode.
func (c *Client) ApplyAppOfApps(ctx context.Context, repoURL, targetRevision, env, appPath string, enableCilium, dryRun bool) (string, bool, error) {
	app := buildAppOfApps(repoURL, targetRevision, env, appPath, enableCilium)
	return c.applyApplication(ctx, app, dryRun, "App of Apps")
}

func buildAppOfApps(repoURL, targetRevision, env, appPath string, enableCilium bool) *unstructured.Unstructured {
	helm := map[string]interface{}{
		"valueFiles": []interface{}{
			fmt.Sprintf("values/%s.yaml", env),
		},
	}
	if enableCilium {
		helm["parameters"] = []interface{}{
			map[string]interface{}{
				"name":  "components.cilium.enabled",
				"value": "true",
			},
		}
	}

	return &unstructured.Unstructured{
		Object: map[string]interface{}{
			"apiVersion": "argoproj.io/v1alpha1",
			"kind":       "Application",
			"metadata": map[string]interface{}{
				"name":      AppOfAppsName,
				"namespace": argoCDNamespace,
			},
			"spec": map[string]interface{}{
				"project": "default",
				"source": map[string]interface{}{
					"repoURL":        repoURL,
					"targetRevision": targetRevision,
					"path":           appPath,
					"helm":           helm,
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
}

func (c *Client) applyApplication(ctx context.Context, app *unstructured.Unstructured, dryRun bool, description string) (string, bool, error) {
	name := app.GetName()

	if dryRun {
		data, err := json.MarshalIndent(app.Object, "", "  ")
		if err != nil {
			return fmt.Sprintf("%+v", app.Object), true, nil
		}
		return string(data), true, nil
	}

	// Check if Application already exists
	_, err := c.DynamicClient.Resource(applicationGVR).Namespace(argoCDNamespace).Get(ctx, name, metav1.GetOptions{})
	exists := err == nil

	_, err = c.DynamicClient.Resource(applicationGVR).Namespace(argoCDNamespace).Apply(
		ctx, name, app, metav1.ApplyOptions{FieldManager: "cluster-bootstrap"},
	)
	if err != nil {
		if apierrors.IsForbidden(err) {
			return "", false, fmt.Errorf("permission denied: cannot apply Application CRD: %w\n  hint: verify ArgoCD CRDs are installed and your role has permission to apply them\n  tip: check: kubectl api-resources | grep Application", err)
		}
		if apierrors.IsNotFound(err) {
			return "", false, fmt.Errorf("ArgoCD CRD not found: %w\n  hint: ensure ArgoCD is installed before creating Applications\n  tip: try: kubectl get crd applications.argoproj.io", err)
		}
		return "", false, fmt.Errorf("failed to apply %s: %w\n  hint: verify the Application CR is valid and ArgoCD is running", description, err)
	}

	return "", !exists, nil
}
