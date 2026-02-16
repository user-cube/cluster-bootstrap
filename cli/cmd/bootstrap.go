package cmd

import (
	"context"
	"encoding/json"
	"fmt"
	"os"

	"github.com/spf13/cobra"

	"github.com/user-cube/cluster-bootstrap/cluster-bootstrap/internal/config"
	"github.com/user-cube/cluster-bootstrap/cluster-bootstrap/internal/helm"
	"github.com/user-cube/cluster-bootstrap/cluster-bootstrap/internal/k8s"
	"github.com/user-cube/cluster-bootstrap/cluster-bootstrap/internal/sops"
)

var (
	secretsFile       string
	dryRun            bool
	skipArgoCDInstall bool
	kubeconfig        string
	kubeContext       string
	bootstrapAgeKey   string
)

var bootstrapCmd = &cobra.Command{
	Use:   "bootstrap <environment>",
	Short: "Bootstrap a Kubernetes cluster with ArgoCD and secrets",
	Long: `Decrypts the SOPS-encrypted secrets file, installs ArgoCD,
creates Kubernetes secrets, and applies the App of Apps root Application.

Replaces the manual install.sh process.`,
	Args: cobra.ExactArgs(1),
	RunE: runBootstrap,
}

func init() {
	bootstrapCmd.Flags().StringVar(&secretsFile, "secrets-file", "", "path to SOPS-encrypted secrets file (default: secrets.<env>.enc.yaml)")
	bootstrapCmd.Flags().BoolVar(&dryRun, "dry-run", false, "print manifests without applying")
	bootstrapCmd.Flags().BoolVar(&skipArgoCDInstall, "skip-argocd-install", false, "skip ArgoCD installation")
	bootstrapCmd.Flags().StringVar(&kubeconfig, "kubeconfig", "", "path to kubeconfig file")
	bootstrapCmd.Flags().StringVar(&kubeContext, "context", "", "kubeconfig context to use")
	bootstrapCmd.Flags().StringVar(&bootstrapAgeKey, "age-key-file", "", "path to age private key file for SOPS decryption")

	rootCmd.AddCommand(bootstrapCmd)
}

func runBootstrap(cmd *cobra.Command, args []string) error {
	env := args[0]

	fmt.Printf("==> Bootstrapping cluster for environment: %s\n", env)

	// Step 2: Decrypt secrets
	sf := secretsFile
	if sf == "" {
		sf = config.SecretsFileName(env)
	}
	fmt.Printf("==> Decrypting secrets from %s...\n", sf)
	sopsOpts := &sops.Options{AgeKeyFile: bootstrapAgeKey}
	envSecrets, err := config.LoadSecrets(sf, sopsOpts)
	if err != nil {
		return err
	}

	if verbose {
		fmt.Printf("  Repo URL: %s\n", envSecrets.Repo.URL)
		fmt.Printf("  Target revision: %s\n", envSecrets.Repo.TargetRevision)
	}

	if dryRun {
		return printDryRun(envSecrets, env)
	}

	// Step 3: Create k8s client
	client, err := k8s.NewClient(kubeconfig, kubeContext)
	if err != nil {
		return fmt.Errorf("failed to create kubernetes client: %w", err)
	}

	ctx := context.Background()

	// Step 4: Create Kubernetes secrets (before Helm install, as the chart may reference them)
	fmt.Println("==> Creating Kubernetes secrets...")
	if err := client.EnsureNamespace(ctx, "argocd"); err != nil {
		return err
	}
	if _, err := client.CreateRepoSSHSecret(ctx, envSecrets.Repo.URL, envSecrets.Repo.SSHPrivateKey, false); err != nil {
		return err
	}

	// Step 5: Install ArgoCD via Helm
	if !skipArgoCDInstall {
		fmt.Println("==> Installing ArgoCD via Helm...")
		if err := helm.InstallArgoCD(ctx, kubeconfig, kubeContext, env, verbose); err != nil {
			return fmt.Errorf("failed to install ArgoCD: %w", err)
		}
	}

	// Step 6: Apply App of Apps
	fmt.Printf("==> Applying App of Apps for environment: %s\n", env)
	if _, err := client.ApplyAppOfApps(ctx, envSecrets.Repo.URL, envSecrets.Repo.TargetRevision, env, false); err != nil {
		return err
	}

	// Step 7: Print access instructions
	fmt.Println("\n==> Done! ArgoCD is installed and the app-of-apps root Application has been created.")
	fmt.Println("    Access the ArgoCD UI:")
	fmt.Println("      kubectl port-forward svc/argocd-server -n argocd 8080:443")
	fmt.Println("    Get the initial admin password:")
	fmt.Println("      kubectl -n argocd get secret argocd-initial-admin-secret -o jsonpath='{.data.password}' | base64 -d")

	return nil
}

func printDryRun(envSecrets *config.EnvironmentSecrets, env string) error {
	fmt.Println("\n--- DRY RUN: Kubernetes Secrets ---")

	// Repo SSH secret
	repoSecret := map[string]interface{}{
		"apiVersion": "v1",
		"kind":       "Secret",
		"metadata": map[string]interface{}{
			"name":      "repo-ssh-key",
			"namespace": "argocd",
			"labels": map[string]string{
				"argocd.argoproj.io/secret-type": "repo-creds",
			},
			"annotations": map[string]string{
				"managed-by":                   "argocd.argoproj.io",
				"cluster-bootstrap/origin":     "bootstrap",
				"cluster-bootstrap/managed-by": "external-secrets",
			},
		},
		"type": "Opaque",
		"stringData": map[string]string{
			"type":          "git",
			"url":           envSecrets.Repo.URL,
			"sshPrivateKey": envSecrets.Repo.SSHPrivateKey,
		},
	}
	printYAMLish(repoSecret)

	// App of Apps
	fmt.Println("---")
	fmt.Println("\n--- DRY RUN: App of Apps Application ---")
	appOfApps := map[string]interface{}{
		"apiVersion": "argoproj.io/v1alpha1",
		"kind":       "Application",
		"metadata": map[string]interface{}{
			"name":      "app-of-apps",
			"namespace": "argocd",
		},
		"spec": map[string]interface{}{
			"project": "default",
			"source": map[string]interface{}{
				"repoURL":        envSecrets.Repo.URL,
				"targetRevision": envSecrets.Repo.TargetRevision,
				"path":           "apps",
				"helm": map[string]interface{}{
					"valueFiles": []string{
						fmt.Sprintf("values/%s.yaml", env),
					},
				},
			},
			"destination": map[string]interface{}{
				"server":    "https://kubernetes.default.svc",
				"namespace": "argocd",
			},
			"syncPolicy": map[string]interface{}{
				"automated": map[string]interface{}{
					"prune":    true,
					"selfHeal": true,
				},
			},
		},
	}
	printYAMLish(appOfApps)

	return nil
}

func printYAMLish(obj interface{}) {
	data, err := json.MarshalIndent(obj, "", "  ")
	if err != nil {
		fmt.Fprintf(os.Stderr, "failed to marshal: %v\n", err)
		return
	}
	fmt.Println(string(data))
}
