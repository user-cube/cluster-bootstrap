package cmd

import (
	"context"
	"fmt"
	"time"

	"github.com/user-cube/cluster-bootstrap/cluster-bootstrap/internal/k8s"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/client-go/kubernetes"
	"k8s.io/client-go/tools/clientcmd"
)

// HealthCheckResult holds the result of a health check
type HealthCheckResult struct {
	Component string
	Status    string
	Message   string
	Duration  time.Duration
}

// HealthStatus represents overall health
type HealthStatus struct {
	Healthy     bool
	StartTime   time.Time
	EndTime     time.Time
	Results     []HealthCheckResult
	CheckedAt   time.Time
	Environment string
}

// WaitForHealth waits for critical components to be ready after bootstrap.
// Timeout is in seconds. Returns detailed health status and any errors.
func WaitForHealth(ctx context.Context, kubeconfig, kubeContext, environment string, timeoutSecs int) (*HealthStatus, error) {
	status := &HealthStatus{
		StartTime:   time.Now(),
		Environment: environment,
	}

	// Build config using standard kubectl loading rules
	loadingRules := &clientcmd.ClientConfigLoadingRules{
		ExplicitPath: kubeconfig,
	}
	if kubeconfig == "" {
		// Use default kubeconfig locations (~/.kube/config, KUBECONFIG env, etc)
		loadingRules = clientcmd.NewDefaultClientConfigLoadingRules()
	}

	// Load the kubeconfig
	kubeCfg, err := loadingRules.Load()
	if err != nil {
		return status, fmt.Errorf("failed to load kubeconfig: %w", err)
	}

	// Create a context override if kubeContext is specified
	configOverrides := &clientcmd.ConfigOverrides{}
	if kubeContext != "" {
		configOverrides.CurrentContext = kubeContext
	}

	// Create the client config
	clientConfig := clientcmd.NewNonInteractiveClientConfig(*kubeCfg, kubeContext, configOverrides, loadingRules)
	config, err := clientConfig.ClientConfig()
	if err != nil {
		return status, fmt.Errorf("failed to build kubeconfig: %w", err)
	}

	clientset, err := kubernetes.NewForConfig(config)
	if err != nil {
		return status, fmt.Errorf("failed to create kubernetes client: %w", err)
	}

	timeout := time.Duration(timeoutSecs) * time.Second
	ctx, cancel := context.WithTimeout(ctx, timeout)
	defer cancel()

	// Check ArgoCD
	result := checkArgoCDReady(ctx, clientset)
	status.Results = append(status.Results, result)
	if result.Status != "Ready" {
		status.Healthy = false
	}

	// Check Vault (if installed)
	result = checkVaultReady(ctx, clientset)
	status.Results = append(status.Results, result)

	// Check External Secrets (if installed)
	result = checkExternalSecretsReady(ctx, clientset)
	status.Results = append(status.Results, result)

	// Determine overall health
	status.Healthy = true
	for _, r := range status.Results {
		if r.Status == "Error" || r.Status == "Timeout" {
			status.Healthy = false
			break
		}
	}

	status.EndTime = time.Now()
	status.CheckedAt = time.Now()

	return status, nil
}

// checkArgoCDReady verifies ArgoCD deployment is ready
func checkArgoCDReady(ctx context.Context, clientset kubernetes.Interface) HealthCheckResult {
	start := time.Now()
	result := HealthCheckResult{
		Component: "ArgoCD",
	}

	// Wait for argocd-server deployment to be ready
	err := waitForDeployment(ctx, clientset, "argocd", "argocd-server")
	if err != nil {
		result.Status = "Timeout"
		result.Message = fmt.Sprintf("argocd-server not ready: %v", err)
	} else {
		result.Status = "Ready"
		result.Message = "argocd-server deployment is running"
	}

	result.Duration = time.Since(start)
	return result
}

// checkVaultReady verifies Vault statefulset is ready
func checkVaultReady(ctx context.Context, clientset kubernetes.Interface) HealthCheckResult {
	start := time.Now()
	result := HealthCheckResult{
		Component: "Vault",
	}

	// Check if vault namespace exists
	_, err := clientset.CoreV1().Namespaces().Get(ctx, "vault", metav1.GetOptions{})
	if err != nil {
		result.Status = "NotInstalled"
		result.Message = "Vault namespace not found"
		result.Duration = time.Since(start)
		return result
	}

	// Wait for vault statefulset
	err = waitForStatefulSet(ctx, clientset, "vault", "vault")
	if err != nil {
		result.Status = "Timeout"
		result.Message = fmt.Sprintf("vault statefulset not ready: %v", err)
	} else {
		result.Status = "Ready"
		result.Message = "vault statefulset is running"
	}

	result.Duration = time.Since(start)
	return result
}

// checkExternalSecretsReady verifies External Secrets operator is ready
func checkExternalSecretsReady(ctx context.Context, clientset kubernetes.Interface) HealthCheckResult {
	start := time.Now()
	result := HealthCheckResult{
		Component: "External Secrets",
	}

	// Check if external-secrets namespace exists
	_, err := clientset.CoreV1().Namespaces().Get(ctx, "external-secrets", metav1.GetOptions{})
	if err != nil {
		result.Status = "NotInstalled"
		result.Message = "External Secrets namespace not found"
		result.Duration = time.Since(start)
		return result
	}

	// Wait for external-secrets deployment
	err = waitForDeployment(ctx, clientset, "external-secrets", "external-secrets")
	if err != nil {
		result.Status = "Timeout"
		result.Message = fmt.Sprintf("external-secrets deployment not ready: %v", err)
	} else {
		result.Status = "Ready"
		result.Message = "external-secrets deployment is running"
	}

	result.Duration = time.Since(start)
	return result
}

// waitForDeployment waits for a deployment to have at least 1 ready replica
func waitForDeployment(ctx context.Context, clientset kubernetes.Interface, namespace, name string) error {
	ticker := time.NewTicker(2 * time.Second)
	defer ticker.Stop()

	for {
		select {
		case <-ctx.Done():
			return fmt.Errorf("timeout waiting for %s/%s", namespace, name)
		case <-ticker.C:
			deploy, err := clientset.AppsV1().Deployments(namespace).Get(ctx, name, metav1.GetOptions{})
			if err != nil {
				continue
			}
			// Check if at least 1 replica is ready
			if deploy.Status.ReadyReplicas > 0 && deploy.Status.UpdatedReplicas > 0 {
				if deploy.Status.ReadyReplicas == deploy.Status.Replicas {
					return nil
				}
			}
		}
	}
}

// waitForStatefulSet waits for a statefulset to have at least 1 ready replica
func waitForStatefulSet(ctx context.Context, clientset kubernetes.Interface, namespace, name string) error {
	ticker := time.NewTicker(2 * time.Second)
	defer ticker.Stop()

	for {
		select {
		case <-ctx.Done():
			return fmt.Errorf("timeout waiting for %s/%s", namespace, name)
		case <-ticker.C:
			ss, err := clientset.AppsV1().StatefulSets(namespace).Get(ctx, name, metav1.GetOptions{})
			if err != nil {
				continue
			}
			// Check if at least 1 replica is ready
			if ss.Status.ReadyReplicas > 0 && ss.Status.UpdatedReplicas > 0 {
				if ss.Status.ReadyReplicas == ss.Status.Replicas {
					return nil
				}
			}
		}
	}
}

// CheckArgoCDSync checks if ArgoCD applications are syncing
func CheckArgoCDSync(ctx context.Context, client k8s.ClientInterface) (int, int, error) {
	// Note: This would require applications.argoproj.io CRD access
	// For now, this is a placeholder for future Argo CD sync checking
	// The actual implementation would list Application CRs and check their sync status
	return 0, 0, nil
}

// PrintHealthStatus prints the health check results in a formatted way
func PrintHealthStatus(status *HealthStatus) {
	fmt.Println()
	fmt.Println("═══════════════════════════════════════════════════════════════")
	if status.Healthy {
		successf("✓ Cluster Health Check - PASSED")
	} else {
		errorf("✗ Cluster Health Check - FAILED")
	}
	fmt.Println("═══════════════════════════════════════════════════════════════")
	fmt.Printf("Environment: %s\n", status.Environment)
	fmt.Printf("Checked at: %s\n", status.CheckedAt.Format("2006-01-02 15:04:05"))
	fmt.Printf("Total duration: %s\n", status.EndTime.Sub(status.StartTime).String())
	fmt.Println()
	fmt.Println("Components:")
	for _, result := range status.Results {
		statusStr := result.Status
		if result.Status == "Ready" {
			statusStr = fmt.Sprintf("\033[32m%s\033[0m", "Ready") // Green
		} else if result.Status == "Timeout" || result.Status == "Error" {
			statusStr = fmt.Sprintf("\033[31m%s\033[0m", result.Status) // Red
		} else if result.Status == "NotInstalled" {
			statusStr = fmt.Sprintf("\033[33m%s\033[0m", "NotInstalled") // Yellow
		}
		fmt.Printf("  • %-20s %s (%s)\n", result.Component, statusStr, result.Duration.String())
		if result.Message != "" {
			fmt.Printf("    Message: %s\n", result.Message)
		}
	}
	fmt.Println()
}
