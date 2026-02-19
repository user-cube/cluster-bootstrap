package cmd

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"os"

	"github.com/spf13/cobra"

	"github.com/user-cube/cluster-bootstrap/cluster-bootstrap-cli/internal/bootstrap"
	"github.com/user-cube/cluster-bootstrap/cluster-bootstrap-cli/internal/config"
	"github.com/user-cube/cluster-bootstrap/cluster-bootstrap-cli/internal/helm"
	"github.com/user-cube/cluster-bootstrap/cluster-bootstrap-cli/internal/k8s"
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
	reportFormat      string
	reportOutput      string
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
	bootstrapCmd.Flags().StringVar(&appPath, "app-path", "apps", "path to App of Apps (relative to current dir when in subfolder, or full repo path with --base-dir)")
	bootstrapCmd.Flags().BoolVar(&waitForHealth, "wait-for-health", false, "wait for cluster components to be ready after bootstrap")
	bootstrapCmd.Flags().IntVar(&healthTimeout, "health-timeout", 180, "timeout in seconds for health checks (default 180)")
	bootstrapCmd.Flags().StringVar(&reportFormat, "report-format", "summary", "report format: summary, json, none")
	bootstrapCmd.Flags().StringVar(&reportOutput, "report-output", "", "write JSON report to file")

	rootCmd.AddCommand(bootstrapCmd)
}

func runBootstrap(cmd *cobra.Command, args []string) error {
	env := args[0]

	// Validate report format
	if reportFormat != "summary" && reportFormat != "json" && reportFormat != "none" {
		return fmt.Errorf("invalid report format '%s': must be 'summary', 'json', or 'none'", reportFormat)
	}

	logger := NewLogger(verbose)

	// Initialize bootstrap manager
	mgr := bootstrap.NewManager(bootstrap.Options{
		Env:               env,
		BaseDir:           baseDir,
		AppPath:           appPath,
		SecretsFile:       secretsFile,
		Encryption:        encryption,
		Kubeconfig:        kubeconfig,
		KubeContext:       kubeContext,
		AgeKeyFile:        bootstrapAgeKey,
		GitCryptKeyFile:   gitcryptKeyFile,
		DryRun:            dryRun,
		SkipArgoCDInstall: skipArgoCDInstall,
		Verbose:           verbose,
	})

	// Resolve paths
	res, err := mgr.ResolvePaths()
	if err != nil {
		return err
	}

	// Initialize bootstrap report
	report := NewBootstrapReport(env)
	report.Configuration = ConfigReport{
		BaseDir:           baseDir,
		AppPath:           res.ArgoCDAppPath,
		Encryption:        encryption,
		SecretsFile:       secretsFile,
		Kubeconfig:        kubeconfig,
		Context:           kubeContext,
		DryRun:            dryRun,
		SkipArgoCDInstall: skipArgoCDInstall,
		WaitForHealth:     waitForHealth,
	}

	// Defer finalizing the report
	var bootstrapErr error
	defer func() {
		report.Complete(bootstrapErr == nil, bootstrapErr)

		// Generate and display report
		if reportFormat != "none" && !dryRun {
			switch reportFormat {
			case "json":
				jsonReport, err := report.ToJSON()
				if err != nil {
					fmt.Fprintf(os.Stderr, "Failed to generate JSON report: %v\n", err)
				} else {
					fmt.Println(jsonReport)
				}
			case "summary":
				report.PrintSummary()
			}
		}

		// Write report to file if requested
		if reportOutput != "" && !dryRun {
			if err := report.WriteToFile(reportOutput); err != nil {
				warnf("Failed to write report to %s: %v", reportOutput, err)
			} else if reportFormat != "json" {
				fmt.Printf("\n📄 Report saved to: %s\n", reportOutput)
			}
		}
	}()

	// Run preflight checks
	preflightTimer := startStage("Preflight Checks")
	if err := PreflightChecks(encryption, bootstrapAgeKey, verbose, waitForHealth); err != nil {
		bootstrapErr = err
		report.AddStage(preflightTimer.complete(false, err))
		return err
	}
	report.AddStage(preflightTimer.complete(true, nil))

	stepf("Bootstrapping cluster for environment: %s", env)

	// Validation stage in report (mgr.ResolvePaths already did most of it)
	validationTimer := startStage("Validation")
	report.AddStage(validationTimer.complete(true, nil))

	// Log configuration
	configStage := logger.Stage("Configuration")
	configStage.Detail("Environment: %s", env)
	configStage.Detail("Base directory: %s", baseDir)
	if res.SubfolderPath != "" {
		configStage.Detail("Subfolder context: %s", res.SubfolderPath)
	}
	configStage.Detail("App path (ArgoCD): %s", res.ArgoCDAppPath)
	if res.LocalAppPath != res.ArgoCDAppPath {
		configStage.Detail("App path (local): %s", res.LocalAppPath)
	}
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
	secretsTimer := startStage("Loading Secrets")
	secretsStage := logger.Stage("Loading Secrets")
	stepf("Loading secrets from %s...", encryption)

	envSecrets, secretsPath, err := mgr.LoadSecrets()
	if err != nil {
		bootstrapErr = err
		report.AddStage(secretsTimer.complete(false, err))
		return err
	}
	report.Configuration.SecretsFile = secretsPath

	secretsStage.Detail("✓ Secrets loaded successfully from %s", secretsPath)
	secretsStage.Detail("Repository: %s", envSecrets.Repo.URL)
	secretsStage.Detail("Target revision: %s", envSecrets.Repo.TargetRevision)
	secretsStage.Done()
	report.AddStage(secretsTimer.complete(true, nil))

	if verbose {
		fmt.Printf("  Repo URL: %s\n", envSecrets.Repo.URL)
		fmt.Printf("  Target revision: %s\n", envSecrets.Repo.TargetRevision)
	}

	if dryRun {
		bootstrapErr = printDryRun(envSecrets, env, res.ArgoCDAppPath)
		return bootstrapErr
	}

	// Create k8s client
	k8sTimer := startStage("K8s Client Connection")
	k8sStage := logger.Stage("Kubernetes Client")
	client, err := k8s.NewClient(kubeconfig, kubeContext)
	if err != nil {
		bootstrapErr = err
		report.AddStage(k8sTimer.complete(false, err))
		return err
	}
	k8sStage.Detail("✓ Connected to cluster")
	k8sStage.Done()
	report.AddStage(k8sTimer.complete(true, nil))

	ctx := context.Background()

	// Create Kubernetes secrets
	secretsK8sTimer := startStage("Creating K8s Resources")
	secretsK8sStage := logger.Stage("Creating K8s Secrets")
	stepf("Creating Kubernetes secrets...")
	namespaceCreated, err := client.EnsureNamespace(ctx, "argocd")
	if err != nil {
		bootstrapErr = err
		report.AddStage(secretsK8sTimer.complete(false, err))
		return err
	}
	if namespaceCreated {
		secretsK8sStage.Detail("✓ Created namespace 'argocd'")
	} else {
		secretsK8sStage.Detail("✓ Verified existing namespace 'argocd'")
	}
	report.Resources.Namespace = NamespaceReport{
		Name:    "argocd",
		Created: namespaceCreated,
	}

	_, repoSecretCreated, err := client.CreateRepoSSHSecret(ctx, envSecrets.Repo.URL, envSecrets.Repo.SSHPrivateKey, false)
	if err != nil {
		bootstrapErr = err
		report.AddStage(secretsK8sTimer.complete(false, err))
		return err
	}
	report.Resources.Secrets = append(report.Resources.Secrets, SecretReport{
		Name:      "repo-ssh-key",
		Namespace: "argocd",
		Created:   repoSecretCreated,
	})
	if repoSecretCreated {
		secretsK8sStage.SecretDetail("Created", "repo-ssh-key", "argocd")
	} else {
		secretsK8sStage.SecretDetail("Updated", "repo-ssh-key", "argocd")
	}

	// If git-crypt key file provided, store it as a K8s secret
	if gitcryptKeyFile != "" {
		keyData, err := os.ReadFile(gitcryptKeyFile) // #nosec G304
		if err != nil {
			bootstrapErr = fmt.Errorf("failed to read git-crypt key file: %w", err)
			report.AddStage(secretsK8sTimer.complete(false, bootstrapErr))
			return bootstrapErr
		}
		stepf("Creating git-crypt-key secret...")
		gitCryptSecretCreated, err := client.CreateGitCryptKeySecret(ctx, keyData)
		if err != nil {
			bootstrapErr = err
			report.AddStage(secretsK8sTimer.complete(false, err))
			return err
		}
		report.Resources.Secrets = append(report.Resources.Secrets, SecretReport{
			Name:      "git-crypt-key",
			Namespace: "argocd",
			Created:   gitCryptSecretCreated,
		})
		if gitCryptSecretCreated {
			secretsK8sStage.SecretDetail("Created", "git-crypt-key", "argocd")
		} else {
			secretsK8sStage.SecretDetail("Updated", "git-crypt-key", "argocd")
		}
	}
	secretsK8sStage.Done()
	report.AddStage(secretsK8sTimer.complete(true, nil))

	// Install ArgoCD via Helm
	if !skipArgoCDInstall {
		helmTimer := startStage("Installing ArgoCD")
		helmStage := logger.Stage("Installing ArgoCD via Helm")
		helmClient := helm.NewClient()
		stepf("Installing ArgoCD via Helm...")
		installed, err := helmClient.InstallArgoCD(ctx, kubeconfig, kubeContext, env, baseDir, verbose)
		if err != nil {
			bootstrapErr = fmt.Errorf("failed to install ArgoCD: %w", err)
			report.AddStage(helmTimer.complete(false, bootstrapErr))
			return bootstrapErr
		}
		report.Resources.ArgoCDRelease = HelmReleaseReport{
			Name:      "argocd",
			Namespace: "argocd",
			Installed: installed,
			Skipped:   false,
		}
		if installed {
			helmStage.Detail("✓ ArgoCD installed successfully")
		} else {
			helmStage.Detail("✓ ArgoCD upgraded successfully")
		}
		helmStage.Done()
		report.AddStage(helmTimer.complete(true, nil))
	} else {
		report.Resources.ArgoCDRelease = HelmReleaseReport{
			Name:      "argocd",
			Namespace: "argocd",
			Skipped:   true,
		}
	}

	// Apply App of Apps
	appTimer := startStage("Deploying App of Apps")
	appStage := logger.Stage("Deploying App of Apps")
	stepf("Applying App of Apps for environment: %s", env)
	_, appCreated, err := client.ApplyAppOfApps(ctx, envSecrets.Repo.URL, envSecrets.Repo.TargetRevision, env, res.ArgoCDAppPath, false)
	if err != nil {
		bootstrapErr = err
		report.AddStage(appTimer.complete(false, err))
		return err
	}
	report.Resources.AppOfApps = ApplicationReport{
		Name:      "app-of-apps",
		Namespace: "argocd",
		Created:   appCreated,
	}
	if appCreated {
		appStage.Detail("✓ App of Apps created successfully")
	} else {
		appStage.Detail("✓ App of Apps updated successfully")
	}
	appStage.Detail("ArgoCD will automatically sync enabled components")
	appStage.Done()
	report.AddStage(appTimer.complete(true, nil))

	// Wait for health checks if requested
	if waitForHealth {
		healthTimer := startStage("Health Checks")
		fmt.Println()
		stepf("Waiting for cluster components to be ready...")
		healthStatus, err := WaitForHealth(ctx, kubeconfig, kubeContext, env, healthTimeout)

		// Populate health report
		report.Health = &HealthReport{
			Checked: true,
			Timeout: healthTimeout,
		}

		if err != nil {
			warnf("Health check failed: %v", err)
			report.Health.Healthy = false
			report.AddStage(healthTimer.complete(false, err))
		} else {
			PrintHealthStatus(healthStatus)
			report.Health.Healthy = healthStatus.Healthy

			for _, result := range healthStatus.Results {
				report.Health.Components = append(report.Health.Components, ComponentHealth{
					Name:   result.Component,
					Status: result.Status,
				})
			}

			if !healthStatus.Healthy {
				warnf("Some components are not ready yet. Bootstrap completed, but you may want to wait a bit longer for everything to be ready.")
			}
			report.AddStage(healthTimer.complete(healthStatus.Healthy, nil))
		}
	}

	// Print access instructions
	if reportFormat != "json" {
		fmt.Println()
		successf("Done! ArgoCD is installed and the app-of-apps root Application has been created.")
		logger.PrintStageSummary()
		printBootstrapSummary(env, secretsPath, res.ArgoCDAppPath)
		fmt.Println("    Access the ArgoCD UI:")
		fmt.Println("      kubectl port-forward svc/argocd-server -n argocd 8080:443")
		fmt.Println("    Get the initial admin password:")
		fmt.Println("      kubectl -n argocd get secret argocd-initial-admin-secret -o jsonpath='{.data.password}' | base64 -d")
	}

	return nil
}

func printDryRun(envSecrets *config.EnvironmentSecrets, env, appPath string) error {
	output, err := renderDryRunOutput(envSecrets, env, appPath)
	if err != nil {
		return err
	}
	if dryRunOutput != "" {
		if err := os.WriteFile(dryRunOutput, []byte(output), 0600); err != nil {
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

func printBootstrapSummary(env, secretsPath, displayAppPath string) {
	fmt.Println("\nSummary:")
	fmt.Printf("  Environment: %s\n", env)
	if secretsPath != "" {
		fmt.Printf("  Secrets file: %s\n", secretsPath)
	}
	fmt.Printf("  App path: %s\n", displayAppPath)
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
