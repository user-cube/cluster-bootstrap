package cmd

import (
	"context"
	"fmt"
	"time"

	"github.com/spf13/cobra"
	"github.com/user-cube/cluster-bootstrap/cli/internal/k8s"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/apis/meta/v1/unstructured"
	"k8s.io/apimachinery/pkg/runtime/schema"
	"k8s.io/client-go/kubernetes"
	"k8s.io/client-go/rest"
	"k8s.io/client-go/tools/clientcmd"
	clientcmdapi "k8s.io/client-go/tools/clientcmd/api"
)

// InfoResult holds bootstrap status information
type InfoResult struct {
	Environment    string
	ClusterVersion string
	ArgoCDVersion  string
	Components     []ComponentInfo
	Applications   []ArgoCDAppInfo
	Health         *HealthStatus
	Timestamp      time.Time
}

// ArgoCDAppInfo holds ArgoCD Application information
type ArgoCDAppInfo struct {
	Name         string
	Namespace    string
	SyncStatus   string
	HealthStatus string
	Destination  string
	RepoURL      string
	Path         string
	SyncWave     string
}

// ComponentInfo holds information about a component
type ComponentInfo struct {
	Name            string
	Namespace       string
	Installed       bool
	Status          string
	ReadyReplicas   int
	DesiredReplicas int
	Version         string
	SyncWave        string
	Message         string
}

var (
	infoKubeconfig    string
	infoContext       string
	infoWaitHealth    bool
	infoHealthTimeout int
)

var infoCmd = &cobra.Command{
	Use:   "info <environment>",
	Short: "Show bootstrap status and component information",
	Long: `Display bootstrap status including installed components, ArgoCD sync state,
and cluster health. Useful for diagnosing issues without re-running bootstrap.`,
	Args: cobra.ExactArgs(1),
	RunE: runInfo,
}

func init() {
	infoCmd.Flags().StringVar(&infoKubeconfig, "kubeconfig", "", "path to kubeconfig file")
	infoCmd.Flags().StringVar(&infoContext, "context", "", "kubeconfig context to use")
	infoCmd.Flags().BoolVar(&infoWaitHealth, "wait-for-health", false, "include health check results")
	infoCmd.Flags().IntVar(&infoHealthTimeout, "health-timeout", 180, "timeout in seconds for health checks")

	rootCmd.AddCommand(infoCmd)
}

func runInfo(cmd *cobra.Command, args []string) error {
	env := args[0]

	ctx := context.Background()

	// Create Kubernetes client
	config, err := buildClientConfig(infoKubeconfig, infoContext)
	if err != nil {
		return fmt.Errorf("failed to build kubeconfig: %w", err)
	}

	clientset, err := kubernetes.NewForConfig(config)
	if err != nil {
		return fmt.Errorf("failed to connect to cluster: %w", err)
	}

	// Create k8s client with dynamic client for CRDs
	k8sClient, err := k8s.NewClient(infoKubeconfig, infoContext)
	if err != nil {
		return fmt.Errorf("failed to create k8s client: %w", err)
	}

	// Gather bootstrap info
	stepf("Gathering bootstrap information for environment: %s", env)

	info := &InfoResult{
		Environment: env,
		Timestamp:   time.Now(),
		Components:  []ComponentInfo{},
	}

	// Get cluster version
	if version, err := clientset.Discovery().ServerVersion(); err == nil {
		info.ClusterVersion = version.GitVersion
	}

	// Check ArgoCD
	argoCDInfo := checkArgoCDInfo(ctx, clientset)
	info.Components = append(info.Components, argoCDInfo)
	info.ArgoCDVersion = argoCDInfo.Version

	// Check Vault
	vaultInfo := checkComponentInfo(ctx, clientset, "vault", "vault", "Vault", true)
	info.Components = append(info.Components, vaultInfo)

	// Check External Secrets
	esInfo := checkComponentInfo(ctx, clientset, "external-secrets", "external-secrets", "External Secrets", false)
	info.Components = append(info.Components, esInfo)

	// Check other common components
	prometheusInfo := checkComponentInfo(ctx, clientset, "monitoring", "kube-prometheus-stack-operator", "Kube Prometheus Stack", false)
	info.Components = append(info.Components, prometheusInfo)

	trivyInfo := checkComponentInfo(ctx, clientset, "trivy-system", "trivy-operator", "Trivy Operator", false)
	info.Components = append(info.Components, trivyInfo)

	// Get ArgoCD Applications
	if apps, err := getArgoCDApplications(ctx, k8sClient); err == nil {
		info.Applications = apps
	} else if verbose {
		warnf("Failed to list ArgoCD Applications: %v", err)
	}

	// Optional health check
	if infoWaitHealth {
		fmt.Println()
		stepf("Running health checks...")
		healthStatus, err := WaitForHealth(ctx, infoKubeconfig, infoContext, env, infoHealthTimeout)
		if err != nil {
			warnf("Health check failed: %v", err)
		} else {
			info.Health = healthStatus
		}
	}

	// Print results
	printInfoResults(info)

	return nil
}

// checkArgoCDInfo gathers ArgoCD-specific information
func checkArgoCDInfo(ctx context.Context, clientset kubernetes.Interface) ComponentInfo {
	info := ComponentInfo{
		Name:      "ArgoCD",
		Namespace: "argocd",
		Status:    "NotInstalled",
	}

	// Check namespace
	_, err := clientset.CoreV1().Namespaces().Get(ctx, "argocd", metav1.GetOptions{})
	if err != nil {
		return info
	}

	info.Installed = true

	// Check deployment
	deploy, err := clientset.AppsV1().Deployments("argocd").Get(ctx, "argocd-server", metav1.GetOptions{})
	if err == nil {
		info.ReadyReplicas = int(deploy.Status.ReadyReplicas)
		info.DesiredReplicas = int(deploy.Status.Replicas)
		if deploy.Status.ReadyReplicas > 0 && deploy.Status.ReadyReplicas == deploy.Status.Replicas {
			info.Status = "Ready"
		} else if deploy.Status.ReadyReplicas > 0 {
			info.Status = "Progressing"
		} else {
			info.Status = "Pending"
		}

		// Extract version from image
		if len(deploy.Spec.Template.Spec.Containers) > 0 {
			image := deploy.Spec.Template.Spec.Containers[0].Image
			info.Version = extractVersionFromImage(image)
		}
	}

	return info
}

// checkComponentInfo gathers information about a component deployment
func checkComponentInfo(ctx context.Context, clientset kubernetes.Interface, namespace, resourceName, displayName string, isStatefulSet bool) ComponentInfo {
	info := ComponentInfo{
		Name:      displayName,
		Namespace: namespace,
		Status:    "NotInstalled",
	}

	// Check namespace
	_, err := clientset.CoreV1().Namespaces().Get(ctx, namespace, metav1.GetOptions{})
	if err != nil {
		return info
	}

	info.Installed = true

	// For statefulsets (Vault)
	if isStatefulSet {
		ss, err := clientset.AppsV1().StatefulSets(namespace).Get(ctx, resourceName, metav1.GetOptions{})
		if err == nil {
			info.ReadyReplicas = int(ss.Status.ReadyReplicas)
			info.DesiredReplicas = int(ss.Status.Replicas)
			if ss.Status.ReadyReplicas > 0 && ss.Status.ReadyReplicas == ss.Status.Replicas {
				info.Status = "Ready"
			} else if ss.Status.ReadyReplicas > 0 {
				info.Status = "Progressing"
			} else {
				info.Status = "Pending"
			}
			if len(ss.Spec.Template.Spec.Containers) > 0 {
				info.Version = extractVersionFromImage(ss.Spec.Template.Spec.Containers[0].Image)
			}
		}
		return info
	}

	// For deployments
	deploy, err := clientset.AppsV1().Deployments(namespace).Get(ctx, resourceName, metav1.GetOptions{})
	if err == nil {
		info.ReadyReplicas = int(deploy.Status.ReadyReplicas)
		info.DesiredReplicas = int(deploy.Status.Replicas)
		if deploy.Status.ReadyReplicas > 0 && deploy.Status.ReadyReplicas == deploy.Status.Replicas {
			info.Status = "Ready"
		} else if deploy.Status.ReadyReplicas > 0 {
			info.Status = "Progressing"
		} else {
			info.Status = "Pending"
		}
		if len(deploy.Spec.Template.Spec.Containers) > 0 {
			info.Version = extractVersionFromImage(deploy.Spec.Template.Spec.Containers[0].Image)
		}
	}

	return info
}

// extractVersionFromImage extracts version from image string
// e.g., "ghcr.io/argoproj/argocd:v2.8.0" -> "v2.8.0"
func extractVersionFromImage(image string) string {
	for i := len(image) - 1; i >= 0; i-- {
		if image[i] == ':' {
			return image[i+1:]
		}
	}
	return "unknown"
}

// printInfoResults prints the bootstrap info results
func printInfoResults(info *InfoResult) {
	fmt.Println()
	fmt.Println("═══════════════════════════════════════════════════════════════")
	successf("Bootstrap Information")
	fmt.Println("═══════════════════════════════════════════════════════════════")
	fmt.Printf("Environment: %s\n", info.Environment)
	fmt.Printf("Timestamp: %s\n", info.Timestamp.Format("2006-01-02 15:04:05"))
	if info.ClusterVersion != "" {
		fmt.Printf("Cluster: Kubernetes %s\n", info.ClusterVersion)
	}
	fmt.Println()

	fmt.Println("Components:")
	for _, comp := range info.Components {
		if comp.Installed {
			statusColor := "⚠ "
			switch comp.Status {
			case "Ready":
				statusColor = "✓"
			case "Pending":
				statusColor = "⏳"
			case "Progressing":
				statusColor = "↻"
			}

			versionStr := ""
			if comp.Version != "" && comp.Version != "unknown" {
				versionStr = fmt.Sprintf(" (%s)", comp.Version)
			}

			replicaStr := ""
			if comp.Status != "NotInstalled" {
				replicaStr = fmt.Sprintf(" - %d/%d replicas", comp.ReadyReplicas, comp.DesiredReplicas)
			}

			fmt.Printf("  %s %-20s [%-12s]%s%s\n", statusColor, comp.Name, comp.Status, versionStr, replicaStr)
			fmt.Printf("     Namespace: %s\n", comp.Namespace)
		} else {
			fmt.Printf("  ○ %-20s [NotInstalled]\n", comp.Name)
		}
	}

	if len(info.Applications) > 0 {
		fmt.Println()
		fmt.Println("ArgoCD Applications:")
		for _, app := range info.Applications {
			syncIcon := "○"
			if app.SyncStatus == "Synced" {
				syncIcon = "✓"
			} else if app.SyncStatus == "OutOfSync" {
				syncIcon = "✗"
			} else if app.SyncStatus != "" {
				syncIcon = "↻"
			}

			healthIcon := "○"
			switch app.HealthStatus {
			case "Healthy":
				healthIcon = "✓"
			case "Degraded", "Missing":
				healthIcon = "✗"
			case "Progressing":
				healthIcon = "↻"
			case "Suspended":
				healthIcon = "⏸"
			}

			fmt.Printf("  %s %s %-20s [Sync: %-10s Health: %-10s]\n", syncIcon, healthIcon, app.Name, app.SyncStatus, app.HealthStatus)
			if app.Destination != "" {
				fmt.Printf("     Destination: %s\n", app.Destination)
			}
			if app.Path != "" {
				fmt.Printf("     Path: %s\n", app.Path)
			}
		}
	}

	if info.Health != nil {
		fmt.Println()
		PrintHealthStatus(info.Health)
	}

	fmt.Println()
	fmt.Println("Diagnostics:")
	fmt.Println("  • Run verbose mode for more details: cluster-bootstrap info " + info.Environment + " -v")
	fmt.Println("  • Check component logs: kubectl logs -n <namespace> -l app=<name>")
	fmt.Println("  • Verify ArgoCD sync: kubectl -n argocd get applications")
	fmt.Println("  • View cluster events: kubectl get events -A --sort-by='.lastTimestamp'")
}

// buildClientConfig creates a REST config from kubeconfig and context
func buildClientConfig(kubeconfig, context string) (*rest.Config, error) {
	loadingRules := clientcmd.NewDefaultClientConfigLoadingRules()
	if kubeconfig != "" {
		loadingRules.ExplicitPath = kubeconfig
	}

	configOverrides := &clientcmd.ConfigOverrides{}
	if context != "" {
		configOverrides.CurrentContext = context
	}

	clientConfig := clientcmd.NewNonInteractiveClientConfig(
		*getMergedConfig(loadingRules),
		context,
		configOverrides,
		loadingRules,
	)

	return clientConfig.ClientConfig()
}

// getMergedConfig loads and merges kubeconfig files
func getMergedConfig(loadingRules *clientcmd.ClientConfigLoadingRules) *clientcmdapi.Config {
	if loadingRules.ExplicitPath != "" {
		// Try to load explicit kubeconfig
		cfg, err := clientcmd.LoadFromFile(loadingRules.ExplicitPath)
		if err == nil {
			return cfg
		}
	}
	// Fall back to default
	cfg, _ := clientcmd.LoadFromFile(clientcmd.RecommendedHomeFile)
	if cfg != nil {
		return cfg
	}
	return clientcmdapi.NewConfig()
}

// getArgoCDApplications retrieves all ArgoCD Applications from the cluster
func getArgoCDApplications(ctx context.Context, client *k8s.Client) ([]ArgoCDAppInfo, error) {
	gvr := schema.GroupVersionResource{
		Group:    "argoproj.io",
		Version:  "v1alpha1",
		Resource: "applications",
	}

	list, err := client.DynamicClient.Resource(gvr).Namespace("argocd").List(ctx, metav1.ListOptions{})
	if err != nil {
		return nil, fmt.Errorf("failed to list applications: %w", err)
	}

	apps := make([]ArgoCDAppInfo, 0, len(list.Items))
	for _, item := range list.Items {
		app := parseArgoCDApplication(&item)
		apps = append(apps, app)
	}

	return apps, nil
}

// parseArgoCDApplication extracts relevant info from an ArgoCD Application unstructured object
func parseArgoCDApplication(obj *unstructured.Unstructured) ArgoCDAppInfo {
	app := ArgoCDAppInfo{
		Name:      obj.GetName(),
		Namespace: obj.GetNamespace(),
	}

	// Extract sync status
	if status, found, _ := unstructured.NestedMap(obj.Object, "status"); found {
		if sync, ok := status["sync"].(map[string]interface{}); ok {
			if syncStatus, ok := sync["status"].(string); ok {
				app.SyncStatus = syncStatus
			}
		}
		if health, ok := status["health"].(map[string]interface{}); ok {
			if healthStatus, ok := health["status"].(string); ok {
				app.HealthStatus = healthStatus
			}
		}
	}

	// Extract spec info
	if spec, found, _ := unstructured.NestedMap(obj.Object, "spec"); found {
		if destination, ok := spec["destination"].(map[string]interface{}); ok {
			if namespace, ok := destination["namespace"].(string); ok {
				app.Destination = namespace
			}
		}
		if source, ok := spec["source"].(map[string]interface{}); ok {
			if repoURL, ok := source["repoURL"].(string); ok {
				app.RepoURL = repoURL
			}
			if path, ok := source["path"].(string); ok {
				app.Path = path
			}
		}
	}

	// Extract sync wave annotation
	if annotations := obj.GetAnnotations(); annotations != nil {
		if wave, ok := annotations["argocd.argoproj.io/sync-wave"]; ok {
			app.SyncWave = wave
		}
	}

	return app
}
