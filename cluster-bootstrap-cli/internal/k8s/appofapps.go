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

const ciliumNamespace = "kube-system"

// ApplyAppOfApps creates or updates the App of Apps root Application CR.
// Returns a boolean indicating if it was created (true) or updated (false) when not in dry-run mode.
// NOTE: This function's signature was changed to return an additional boolean value, which is a
// breaking API change. External callers must be updated to handle the extra return value.
func (c *Client) ApplyAppOfApps(ctx context.Context, repoURL, targetRevision, env, appPath string, dryRun bool) (string, bool, error) {
	app := buildAppOfApps(repoURL, targetRevision, env, appPath)
	return c.applyApplication(ctx, app, dryRun, "App of Apps")
}

func buildAppOfApps(repoURL, targetRevision, env, appPath string) *unstructured.Unstructured {
	return &unstructured.Unstructured{
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
}

// BuildCiliumApplication creates the Git-backed Application that takes over
// management of the release installed during bootstrap.
func BuildCiliumApplication(repoURL, targetRevision, env, componentPath string) *unstructured.Unstructured {
	return &unstructured.Unstructured{
		Object: map[string]interface{}{
			"apiVersion": "argoproj.io/v1alpha1",
			"kind":       "Application",
			"metadata": map[string]interface{}{
				"name":      "cilium",
				"namespace": argoCDNamespace,
			},
			"spec": map[string]interface{}{
				"project": "default",
				"source": map[string]interface{}{
					"repoURL":        repoURL,
					"targetRevision": targetRevision,
					"path":           componentPath,
					"helm": map[string]interface{}{
						"releaseName": "cilium",
						"valueFiles": []interface{}{
							"values/base.yaml",
							fmt.Sprintf("values/%s.yaml", env),
						},
					},
				},
				"destination": map[string]interface{}{
					"server":    "https://kubernetes.default.svc",
					"namespace": ciliumNamespace,
				},
				"syncPolicy": map[string]interface{}{
					"automated": map[string]interface{}{
						"prune":    true,
						"selfHeal": true,
					},
					"syncOptions": []interface{}{
						"CreateNamespace=true",
						"ServerSideApply=true",
					},
				},
			},
		},
	}
}

// ApplyCiliumApplication creates or updates the Cilium Application CR.
func (c *Client) ApplyCiliumApplication(ctx context.Context, repoURL, targetRevision, env, componentPath string, dryRun bool) (string, bool, error) {
	app := BuildCiliumApplication(repoURL, targetRevision, env, componentPath)
	return c.applyApplication(ctx, app, dryRun, "Cilium Application")
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

	gvr := schema.GroupVersionResource{
		Group:    "argoproj.io",
		Version:  "v1alpha1",
		Resource: "applications",
	}

	// Check if Application already exists
	_, err := c.DynamicClient.Resource(gvr).Namespace(argoCDNamespace).Get(ctx, name, metav1.GetOptions{})
	exists := err == nil

	_, err = c.DynamicClient.Resource(gvr).Namespace(argoCDNamespace).Apply(
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
