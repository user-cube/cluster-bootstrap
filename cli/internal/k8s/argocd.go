package k8s

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"time"

	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/apis/meta/v1/unstructured"
	"k8s.io/apimachinery/pkg/runtime/schema"
	"k8s.io/apimachinery/pkg/util/yaml"
	"k8s.io/client-go/dynamic"
)

const (
	argoCDManifestURL = "https://raw.githubusercontent.com/argoproj/argo-cd/stable/manifests/install.yaml"
	argoCDNamespace   = "argocd"
)

// InstallArgoCD fetches the ArgoCD manifests and applies them to the cluster.
func (c *Client) InstallArgoCD(ctx context.Context, verbose bool) error {
	if err := c.EnsureNamespace(ctx, argoCDNamespace); err != nil {
		return fmt.Errorf("failed to create argocd namespace: %w", err)
	}

	if verbose {
		fmt.Printf("  Fetching ArgoCD manifests from %s\n", argoCDManifestURL)
	}

	resp, err := http.Get(argoCDManifestURL)
	if err != nil {
		return fmt.Errorf("failed to fetch ArgoCD manifests: %w", err)
	}
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusOK {
		return fmt.Errorf("failed to fetch ArgoCD manifests: HTTP %d", resp.StatusCode)
	}

	body, err := io.ReadAll(resp.Body)
	if err != nil {
		return fmt.Errorf("failed to read ArgoCD manifests: %w", err)
	}

	decoder := yaml.NewYAMLOrJSONDecoder(
		io.NopCloser(bytes.NewReader(body)),
		4096,
	)

	applied := 0
	for {
		var obj unstructured.Unstructured
		err := decoder.Decode(&obj)
		if err != nil {
			if err == io.EOF {
				break
			}
			return fmt.Errorf("failed to decode manifest: %w", err)
		}

		if obj.GetKind() == "" {
			continue
		}

		if obj.GetNamespace() == "" && isNamespacedKind(obj.GetKind()) {
			obj.SetNamespace(argoCDNamespace)
		}

		gvr := gvrFromObject(&obj)
		var resource dynamic.ResourceInterface
		if obj.GetNamespace() != "" {
			resource = c.DynamicClient.Resource(gvr).Namespace(obj.GetNamespace())
		} else {
			resource = c.DynamicClient.Resource(gvr)
		}

		_, err = resource.Apply(ctx, obj.GetName(), &obj, metav1.ApplyOptions{
			FieldManager: "cluster-bootstrap",
		})
		if err != nil {
			if verbose {
				fmt.Printf("  Warning: failed to apply %s/%s: %v\n", obj.GetKind(), obj.GetName(), err)
			}
			continue
		}
		applied++
	}

	if verbose {
		fmt.Printf("  Applied %d ArgoCD resources\n", applied)
	}

	return nil
}

// WaitForArgoCD waits for ArgoCD deployments to be ready.
func (c *Client) WaitForArgoCD(ctx context.Context, verbose bool) error {
	deployments := []string{
		"argocd-server",
		"argocd-repo-server",
		"argocd-applicationset-controller",
	}

	timeout := 5 * time.Minute
	deadline := time.Now().Add(timeout)

	for _, name := range deployments {
		if verbose {
			fmt.Printf("  Waiting for deployment %s...\n", name)
		}

		for {
			if time.Now().After(deadline) {
				return fmt.Errorf("timeout waiting for deployment %s", name)
			}

			deploy, err := c.Clientset.AppsV1().Deployments(argoCDNamespace).Get(ctx, name, metav1.GetOptions{})
			if err != nil {
				time.Sleep(5 * time.Second)
				continue
			}

			if deploy.Status.AvailableReplicas > 0 && deploy.Status.ReadyReplicas == *deploy.Spec.Replicas {
				if verbose {
					fmt.Printf("  Deployment %s is ready\n", name)
				}
				break
			}

			time.Sleep(5 * time.Second)
		}
	}

	return nil
}

// ApplyAppOfApps creates the App of Apps root Application CR.
func (c *Client) ApplyAppOfApps(ctx context.Context, repoURL, targetRevision, env string, dryRun bool) (string, error) {
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
					"path":           "apps",
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

func gvrFromObject(obj *unstructured.Unstructured) schema.GroupVersionResource {
	gvk := obj.GroupVersionKind()
	return schema.GroupVersionResource{
		Group:    gvk.Group,
		Version:  gvk.Version,
		Resource: pluralize(gvk.Kind),
	}
}

func pluralize(kind string) string {
	lower := toLower(kind)
	irregulars := map[string]string{
		"endpoints":                "endpoints",
		"endpointslice":            "endpointslices",
		"ingress":                  "ingresses",
		"networkpolicy":            "networkpolicies",
		"podsecuritypolicy":        "podsecuritypolicies",
		"resourcequota":            "resourcequotas",
		"storageclass":             "storageclasses",
		"priorityclass":            "priorityclasses",
		"ingressclass":             "ingressclasses",
		"runtimeclass":             "runtimeclasses",
		"customresourcedefinition": "customresourcedefinitions",
	}
	if plural, ok := irregulars[lower]; ok {
		return plural
	}
	if len(lower) > 0 && lower[len(lower)-1] == 's' {
		return lower
	}
	return lower + "s"
}

func toLower(s string) string {
	result := make([]byte, len(s))
	for i := 0; i < len(s); i++ {
		c := s[i]
		if c >= 'A' && c <= 'Z' {
			result[i] = c + 32
		} else {
			result[i] = c
		}
	}
	return string(result)
}

func isNamespacedKind(kind string) bool {
	clusterScoped := map[string]bool{
		"Namespace":                true,
		"ClusterRole":              true,
		"ClusterRoleBinding":       true,
		"CustomResourceDefinition": true,
		"PersistentVolume":         true,
		"StorageClass":             true,
		"PriorityClass":            true,
		"IngressClass":             true,
		"RuntimeClass":             true,
		"Node":                     true,
	}
	return !clusterScoped[kind]
}
