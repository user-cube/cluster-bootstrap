package cmd

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
	"strings"

	"github.com/spf13/cobra"

	"github.com/user-cube/cluster-bootstrap/cluster-bootstrap/internal/config"
	"github.com/user-cube/cluster-bootstrap/cluster-bootstrap/internal/helm"
	"github.com/user-cube/cluster-bootstrap/cluster-bootstrap/internal/k8s"
	"github.com/user-cube/cluster-bootstrap/cluster-bootstrap/internal/sops"
)

var (
	secretsFile       string
	dryRun            bool
	dryRunOutput      string
	skipArgoCDInstall bool
	kubeconfig        string
	kubeContext       string
	bootstrapAgeKey   string
	encryption        string
	gitcryptKeyFile   string
	appPath           string
	waitForHealth     bool
	healthTimeout     int
)

var bootstrapCmd = &cobra.Command{
	Use:   "bootstrap <environment>",
	Short: "Bootstrap a Kubernetes cluster with ArgoCD and secrets",
	Long: `Decrypts the secrets file, installs ArgoCD,
creates Kubernetes secrets, and applies the App of Apps root Application.

Replaces the manual install.sh process.`,
	Args: cobra.ExactArgs(1),
	RunE: runBootstrap,
}

func init() {
	bootstrapCmd.Flags().StringVar(&secretsFile, "secrets-file", "", "path to secrets file (default: secrets.<env>.enc.yaml or secrets.<env>.yaml)")
	bootstrapCmd.Flags().BoolVar(&dryRun, "dry-run", false, "print manifests without applying")
	bootstrapCmd.Flags().StringVar(&dryRunOutput, "dry-run-output", "", "write dry-run manifests to file")
	bootstrapCmd.Flags().BoolVar(&skipArgoCDInstall, "skip-argocd-install", false, "skip ArgoCD installation")
	bootstrapCmd.Flags().StringVar(&kubeconfig, "kubeconfig", "", "path to kubeconfig file")
	bootstrapCmd.Flags().StringVar(&kubeContext, "context", "", "kubeconfig context to use")
	bootstrapCmd.Flags().StringVar(&bootstrapAgeKey, "age-key-file", "", "path to age private key file for SOPS decryption")
	bootstrapCmd.Flags().StringVar(&encryption, "encryption", "sops", "encryption backend (sops|git-crypt)")
	bootstrapCmd.Flags().StringVar(&gitcryptKeyFile, "gitcrypt-key-file", "", "path to git-crypt symmetric key file (creates K8s secret)")
	bootstrapCmd.Flags().StringVar(&appPath, "app-path", "apps", "path inside the Git repo for the App of Apps source")
	bootstrapCmd.Flags().BoolVar(&waitForHealth, "wait-for-health", false, "wait for cluster components to be ready after bootstrap")
	bootstrapCmd.Flags().IntVar(&healthTimeout, "health-timeout", 180, "timeout in seconds for health checks (default 180)")

	rootCmd.AddCommand(bootstrapCmd)
}

func runBootstrap(cmd *cobra.Command, args []string) error {
	env := args[0]
	logger := NewLogger(verbose)

	// Run preflight checks
	// Only require kubectl if we're going to use wait-for-health
	if err := PreflightChecks(encryption, bootstrapAgeKey, verbose, waitForHealth); err != nil {
		return err
	}

	stepf("Bootstrapping cluster for environment: %s", env)
	if err := validateBootstrapInputs(env); err != nil {
		return fmt.Errorf("validation failed: %w", err)
	}

	// Log configuration
	configStage := logger.Stage("Configuration")
	configStage.Detail("Environment: %s", env)
	configStage.Detail("Base directory: %s", baseDir)
	configStage.Detail("App path: %s", appPath)
	configStage.Detail("Encryption: %s", encryption)
	if kubeconfig != "" {
		configStage.Detail("Kubeconfig: %s", kubeconfig)
	}
	if kubeContext != "" {
		configStage.Detail("Context: %s", kubeContext)
	}
	if dryRun {
		configStage.Detail("⚠ DRY RUN mode - no changes will be applied")
	}
	if skipArgoCDInstall {
		configStage.Detail("⚠ Skipping ArgoCD installation")
	}
	configStage.Done()

	// Load secrets based on encryption backend
	secretsStage := logger.Stage("Loading Secrets")
	var envSecrets *config.EnvironmentSecrets
	var err error

	var secretsPath string
	switch encryption {
	case "git-crypt":
		sf := secretsFile
		if sf == "" {
			sf = filepath.Join(baseDir, config.SecretsFileNamePlain(env))
		}
		secretsPath = sf
		if err := validateSecretsFileExists(secretsPath); err != nil {
			return err
		}
		secretsStage.Detail("Loading plaintext secrets from %s", sf)
		stepf("Loading plaintext secrets from %s...", sf)
		envSecrets, err = config.LoadSecretsPlaintext(sf)
		if err != nil {
			return err
		}
		secretsStage.Detail("✓ Secrets loaded successfully")
	case "sops":
		sf := secretsFile
		if sf == "" {
			sf = filepath.Join(baseDir, config.SecretsFileName(env))
		}
		secretsPath = sf
		if err := validateSecretsFileExists(secretsPath); err != nil {
			return err
		}
		secretsStage.Detail("Decrypting secrets from %s", sf)
		stepf("Decrypting secrets from %s...", sf)
		sopsOpts := &sops.Options{AgeKeyFile: bootstrapAgeKey}
		envSecrets, err = config.LoadSecrets(sf, sopsOpts)
		if err != nil {
			return err
		}
		secretsStage.Detail("✓ Secrets decrypted successfully")
	default:
		return fmt.Errorf("unsupported encryption backend: %s (use sops or git-crypt)", encryption)
	}

	secretsStage.Detail("Repository: %s", envSecrets.Repo.URL)
	secretsStage.Detail("Target revision: %s", envSecrets.Repo.TargetRevision)
	secretsStage.Done()

	if verbose {
		fmt.Printf("  Repo URL: %s\n", envSecrets.Repo.URL)
		fmt.Printf("  Target revision: %s\n", envSecrets.Repo.TargetRevision)
	}

	if dryRun {
		return printDryRun(envSecrets, env, appPath)
	}

	// Create k8s client
	k8sStage := logger.Stage("Kubernetes Client")
	client, err := k8s.NewClient(kubeconfig, kubeContext)
	if err != nil {
		return err
	}
	k8sStage.Detail("✓ Connected to cluster")
	k8sStage.Done()

	ctx := context.Background()

	// Create Kubernetes secrets (before Helm install, as the chart may reference them)
	secretsK8sStage := logger.Stage("Creating K8s Secrets")
	stepf("Creating Kubernetes secrets...")
	if err := client.EnsureNamespace(ctx, "argocd"); err != nil {
		return err
	}
	secretsK8sStage.Detail("✓ Created/verified namespace 'argocd'")

	if _, err := client.CreateRepoSSHSecret(ctx, envSecrets.Repo.URL, envSecrets.Repo.SSHPrivateKey, false); err != nil {
		return err
	}
	secretsK8sStage.SecretDetail("Created", "repo-ssh-key", "argocd")

	// If git-crypt key file provided, store it as a K8s secret
	if gitcryptKeyFile != "" {
		keyData, err := os.ReadFile(gitcryptKeyFile)
		if err != nil {
			return fmt.Errorf("failed to read git-crypt key file: %w", err)
		}
		stepf("Creating git-crypt-key secret...")
		if err := client.CreateGitCryptKeySecret(ctx, keyData); err != nil {
			return err
		}
		secretsK8sStage.SecretDetail("Created", "git-crypt-key", "argocd")
	}
	secretsK8sStage.Done()

	// Install ArgoCD via Helm
	if !skipArgoCDInstall {
		helmStage := logger.Stage("Installing ArgoCD via Helm")
		stepf("Installing ArgoCD via Helm...")
		if err := helm.InstallArgoCD(ctx, kubeconfig, kubeContext, env, baseDir, verbose); err != nil {
			return fmt.Errorf("failed to install ArgoCD: %w", err)
		}
		helmStage.Detail("✓ ArgoCD installed successfully")
		helmStage.Done()
	}

	// Apply App of Apps
	appStage := logger.Stage("Deploying App of Apps")
	stepf("Applying App of Apps for environment: %s", env)
	if _, err := client.ApplyAppOfApps(ctx, envSecrets.Repo.URL, envSecrets.Repo.TargetRevision, env, appPath, false); err != nil {
		return err
	}
	appStage.Detail("✓ App of Apps created successfully")
	appStage.Detail("ArgoCD will automatically sync enabled components")
	appStage.Done()

	// Wait for health checks if requested
	if waitForHealth {
		fmt.Println()
		stepf("Waiting for cluster components to be ready...")
		healthStatus, err := WaitForHealth(ctx, kubeconfig, kubeContext, env, healthTimeout)
		if err != nil {
			warnf("Health check failed: %v", err)
			// Don't fail bootstrap if health checks don't complete, just warn
		} else {
			PrintHealthStatus(healthStatus)
			if !healthStatus.Healthy {
				warnf("Some components are not ready yet. Bootstrap completed, but you may want to wait a bit longer for everything to be ready.")
			}
		}
	}

	// Print access instructions
	fmt.Println()
	successf("Done! ArgoCD is installed and the app-of-apps root Application has been created.")
	logger.PrintStageSummary()
	printBootstrapSummary(env, secretsPath)
	fmt.Println("    Access the ArgoCD UI:")
	fmt.Println("      kubectl port-forward svc/argocd-server -n argocd 8080:443")
	fmt.Println("    Get the initial admin password:")
	fmt.Println("      kubectl -n argocd get secret argocd-initial-admin-secret -o jsonpath='{.data.password}' | base64 -d")

	return nil
}

func printDryRun(envSecrets *config.EnvironmentSecrets, env, appPath string) error {
	output, err := renderDryRunOutput(envSecrets, env, appPath)
	if err != nil {
		return err
	}
	if dryRunOutput != "" {
		if err := os.WriteFile(dryRunOutput, []byte(output), 0644); err != nil {
			return fmt.Errorf("failed to write dry-run output: %w", err)
		}
	}
	fmt.Print(output)
	return nil
}

func renderDryRunOutput(envSecrets *config.EnvironmentSecrets, env, appPath string) (string, error) {
	repoSecret, appOfApps := buildDryRunObjects(envSecrets, env, appPath)

	repoJSON, err := json.MarshalIndent(repoSecret, "", "  ")
	if err != nil {
		return "", fmt.Errorf("failed to marshal repo secret: %w", err)
	}
	appJSON, err := json.MarshalIndent(appOfApps, "", "  ")
	if err != nil {
		return "", fmt.Errorf("failed to marshal app of apps: %w", err)
	}

	var out bytes.Buffer
	out.WriteString("\n--- DRY RUN: Kubernetes Secrets ---\n")
	out.Write(repoJSON)
	out.WriteString("\n---\n")
	out.WriteString("\n--- DRY RUN: App of Apps Application ---\n")
	out.Write(appJSON)
	out.WriteString("\n")

	return out.String(), nil
}

func buildDryRunObjects(envSecrets *config.EnvironmentSecrets, env, appPath string) (map[string]interface{}, map[string]interface{}) {
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
				"path":           appPath,
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

	return repoSecret, appOfApps
}

func validateBootstrapInputs(env string) error {
	if env == "" {
		return fmt.Errorf("environment is required")
	}

	baseInfo, err := os.Stat(baseDir)
	if err != nil {
		return fmt.Errorf("base-dir %s is not accessible: %w", baseDir, err)
	}
	if !baseInfo.IsDir() {
		return fmt.Errorf("base-dir %s is not a directory", baseDir)
	}

	if filepath.IsAbs(appPath) {
		return fmt.Errorf("app-path must be relative to base-dir")
	}
	appFullPath := filepath.Join(baseDir, appPath)
	if _, err := os.Stat(appFullPath); err != nil {
		if appPath == "apps" {
			detected, detectErr := autoDetectAppPath(baseDir)
			if detectErr != nil {
				return fmt.Errorf("app-path %s does not exist under base-dir: %w", appPath, err)
			}
			appPath = detected
			if verbose {
				fmt.Printf("  App path auto-detected: %s\n", appPath)
			}
		} else {
			return fmt.Errorf("app-path %s does not exist under base-dir: %w", appPath, err)
		}
	}

	if secretsFile != "" {
		isEnc := strings.HasSuffix(secretsFile, ".enc.yaml")
		isYaml := strings.HasSuffix(secretsFile, ".yaml")
		switch encryption {
		case "sops":
			if !isEnc {
				return fmt.Errorf("secrets-file must end with .enc.yaml when encryption is sops")
			}
		case "git-crypt":
			if !isYaml || isEnc {
				return fmt.Errorf("secrets-file must end with .yaml (not .enc.yaml) when encryption is git-crypt")
			}
		}
	}

	return nil
}

func autoDetectAppPath(base string) (string, error) {
	var candidates []string
	_ = filepath.WalkDir(base, func(path string, d os.DirEntry, walkErr error) error {
		if walkErr != nil {
			return nil
		}
		if d.IsDir() {
			return nil
		}
		if d.Name() != "Chart.yaml" {
			return nil
		}
		dir := filepath.Dir(path)
		if _, err := os.Stat(filepath.Join(dir, "templates", "application.yaml")); err != nil {
			return nil
		}
		rel, err := filepath.Rel(base, dir)
		if err != nil {
			return nil
		}
		candidates = append(candidates, rel)
		return nil
	})

	if len(candidates) == 0 {
		return "", fmt.Errorf("no app chart found under base-dir")
	}

	// Prefer a directory named "apps" if present.
	for _, candidate := range candidates {
		if filepath.Base(candidate) == "apps" {
			return candidate, nil
		}
	}

	return candidates[0], nil
}

func printYAMLish(obj interface{}) {
	data, err := json.MarshalIndent(obj, "", "  ")
	if err != nil {
		fmt.Fprintf(os.Stderr, "failed to marshal: %v\n", err)
		return
	}
	fmt.Println(string(data))
}

func printBootstrapSummary(env, secretsPath string) {
	fmt.Println("\nSummary:")
	fmt.Printf("  Environment: %s\n", env)
	if secretsPath != "" {
		fmt.Printf("  Secrets file: %s\n", secretsPath)
	}
	fmt.Printf("  App path: %s\n", appPath)
	fmt.Printf("  Encryption: %s\n", encryption)
	if skipArgoCDInstall {
		fmt.Println("  ArgoCD install: skipped")
	} else {
		fmt.Println("  ArgoCD install: attempted")
	}
	if gitcryptKeyFile != "" {
		fmt.Printf("  Git-crypt key: %s\n", gitcryptKeyFile)
	}
}

func validateSecretsFileExists(path string) error {
	if path == "" {
		return nil
	}
	if _, err := os.Stat(path); err != nil {
		return fmt.Errorf("secrets file not found: %s", path)
	}
	return nil
}
