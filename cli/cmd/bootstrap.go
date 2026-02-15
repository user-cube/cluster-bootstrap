package cmd

import (
	"context"
	"encoding/json"
	"fmt"
	"os"
	"strings"

	"github.com/spf13/cobra"

	"github.com/user-cube/cluster-bootstrap/cli/internal/config"
	"github.com/user-cube/cluster-bootstrap/cli/internal/k8s"
	"github.com/user-cube/cluster-bootstrap/cli/internal/sops"
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
	bootstrapCmd.Flags().StringVar(&secretsFile, "secrets-file", "secrets.enc.yaml", "path to SOPS-encrypted secrets file")
	bootstrapCmd.Flags().BoolVar(&dryRun, "dry-run", false, "print manifests without applying")
	bootstrapCmd.Flags().BoolVar(&skipArgoCDInstall, "skip-argocd-install", false, "skip ArgoCD installation")
	bootstrapCmd.Flags().StringVar(&kubeconfig, "kubeconfig", "", "path to kubeconfig file")
	bootstrapCmd.Flags().StringVar(&kubeContext, "context", "", "kubeconfig context to use")
	bootstrapCmd.Flags().StringVar(&bootstrapAgeKey, "age-key-file", "", "path to age private key file for SOPS decryption")

	rootCmd.AddCommand(bootstrapCmd)
}

func runBootstrap(cmd *cobra.Command, args []string) error {
	env := args[0]

	// Step 1: Validate environment
	if !config.IsValidEnvironment(env) {
		return fmt.Errorf("invalid environment %q, must be one of: %s",
			env, strings.Join(config.ValidEnvironments, ", "))
	}

	fmt.Printf("==> Bootstrapping cluster for environment: %s\n", env)

	// Step 2: Decrypt secrets
	fmt.Println("==> Decrypting secrets...")
	sopsOpts := &sops.Options{AgeKeyFile: bootstrapAgeKey}
	secrets, err := config.LoadSecrets(secretsFile, sopsOpts)
	if err != nil {
		return err
	}

	envSecrets, err := secrets.GetEnvironment(env)
	if err != nil {
		return err
	}

	if verbose {
		fmt.Printf("  Repo URL: %s\n", envSecrets.Repo.URL)
		fmt.Printf("  Target revision: %s\n", envSecrets.Repo.TargetRevision)
		if envSecrets.Vault.Address != "" {
			fmt.Printf("  Vault address: %s\n", envSecrets.Vault.Address)
		}
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

	// Step 4: Install ArgoCD
	if !skipArgoCDInstall {
		fmt.Println("==> Installing ArgoCD...")
		if err := client.InstallArgoCD(ctx, verbose); err != nil {
			return fmt.Errorf("failed to install ArgoCD: %w", err)
		}

		fmt.Println("==> Waiting for ArgoCD to be ready...")
		if err := client.WaitForArgoCD(ctx, verbose); err != nil {
			return fmt.Errorf("ArgoCD not ready: %w", err)
		}
	}

	// Step 5: Create Kubernetes secrets
	fmt.Println("==> Creating SSH repo credentials secret...")
	if err := client.EnsureNamespace(ctx, "argocd"); err != nil {
		return err
	}
	if _, err := client.CreateRepoSSHSecret(ctx, envSecrets.Repo.URL, envSecrets.Repo.SSHPrivateKey, false); err != nil {
		return err
	}

	if envSecrets.Vault.Token != "" {
		fmt.Println("==> Creating Vault token secret...")
		if _, err := client.CreateVaultTokenSecret(ctx, envSecrets.Vault.Address, envSecrets.Vault.Token, false); err != nil {
			return err
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
	fmt.Println("\n--- DRY RUN: Kubernetes Secrets ---\n")

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

	// Vault secret
	if envSecrets.Vault.Token != "" {
		fmt.Println("---")
		vaultSecret := map[string]interface{}{
			"apiVersion": "v1",
			"kind":       "Secret",
			"metadata": map[string]interface{}{
				"name":      "vault-token",
				"namespace": "vault",
				"annotations": map[string]string{
					"cluster-bootstrap/origin":     "bootstrap",
					"cluster-bootstrap/managed-by": "external-secrets",
				},
			},
			"type": "Opaque",
			"stringData": map[string]string{
				"address": envSecrets.Vault.Address,
				"token":   envSecrets.Vault.Token,
			},
		}
		printYAMLish(vaultSecret)
	}

	// App of Apps
	fmt.Println("---")
	fmt.Println("\n--- DRY RUN: App of Apps Application ---\n")
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
