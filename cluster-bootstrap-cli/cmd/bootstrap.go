package cmd

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
	"strings"
	"time"

	"github.com/spf13/cobra"
	"github.com/user-cube/cluster-bootstrap/cluster-bootstrap-cli/internal/config"
	"github.com/user-cube/cluster-bootstrap/cluster-bootstrap-cli/internal/helm"
	"github.com/user-cube/cluster-bootstrap/cluster-bootstrap-cli/internal/k8s"
	"github.com/user-cube/cluster-bootstrap/cluster-bootstrap-cli/internal/sops"
)

var (
	secretsFile       string
	dryRun            bool
	dryRunOutput      string
	skipArgoCDInstall bool
	enableCilium      bool
	kubeconfig        string
	kubeContext       string
	bootstrapAgeKey   string
	storeSopsAgeKey   bool
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
	Short: "Bootstrap a Kubernetes cluster with optional Cilium, ArgoCD, and secrets",
	Long: `Decrypts the secrets file, optionally installs Cilium, installs ArgoCD,
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
	bootstrapCmd.Flags().BoolVar(&enableCilium, "enable-cilium", false, "install Cilium before ArgoCD and configure it for ArgoCD management")
	bootstrapCmd.Flags().StringVar(&kubeconfig, "kubeconfig", "", "path to kubeconfig file")
	bootstrapCmd.Flags().StringVar(&kubeContext, "context", "", "kubeconfig context to use")
	bootstrapCmd.Flags().StringVar(&bootstrapAgeKey, "age-key-file", "", "path to age private key file for SOPS decryption")
	bootstrapCmd.Flags().BoolVar(&storeSopsAgeKey, "store-sops-age-key", false, "store the SOPS age key as the sops-age-key Kubernetes Secret in argocd")
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

	// Detect if we're running from a subdirectory and adjust paths accordingly
	var argoCDAppPath string
	var subfolderPath string

	if baseDir == "." {
		// Check if we're in a subdirectory of a Git repository
		detected, relPath := detectGitSubdirectory()
		if detected && relPath != "" {
			subfolderPath = relPath

			// Handle different appPath scenarios:
			// 1. appPath="apps" -> convert to "k8s/apps"
			// 2. appPath="k8s/apps" (user specified full path) -> strip to "apps" for local validation, keep "k8s/apps" for ArgoCD
			if strings.HasPrefix(appPath, relPath+"/") {
				// User provided full path (e.g., "k8s/apps" while in k8s/)
				// This is valid, keep it for ArgoCD
				argoCDAppPath = appPath
				if verbose {
					fmt.Printf("  📁 Detected running from subdirectory: %s\n", relPath)
					fmt.Printf("  📍 Using full path for ArgoCD: %s\n", argoCDAppPath)
				}
			} else {
				// User provided relative path (e.g., "apps")
				// Convert to full path for ArgoCD
				argoCDAppPath = relPath + "/" + appPath
				if verbose {
					fmt.Printf("  📁 Detected running from subdirectory: %s\n", relPath)
					fmt.Printf("  📍 Local path: %s -> ArgoCD path: %s\n", appPath, argoCDAppPath)
				}
			}
		} else {
			argoCDAppPath = appPath
		}
	} else {
		// baseDir is explicitly set, use the original logic
		argoCDAppPath = appPath
	}

	// Initialize bootstrap report
	report := NewBootstrapReport(env)
	report.Configuration = ConfigReport{
		BaseDir:           baseDir,
		AppPath:           argoCDAppPath,
		Encryption:        encryption,
		SecretsFile:       secretsFile,
		Kubeconfig:        kubeconfig,
		Context:           kubeContext,
		DryRun:            dryRun,
		SkipArgoCDInstall: skipArgoCDInstall,
		EnableCilium:      enableCilium,
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
	// Only require kubectl if we're going to use wait-for-health
	preflightTimer := startStage("Preflight Checks")
	if err := PreflightChecks(encryption, bootstrapAgeKey, verbose, waitForHealth); err != nil {
		bootstrapErr = err
		report.AddStage(preflightTimer.complete(false, err))
		return err
	}
	report.AddStage(preflightTimer.complete(true, nil))

	stepf("Bootstrapping cluster for environment: %s", env)

	// Validation
	validationTimer := startStage("Validation")
	localAppPath, err := validateBootstrapInputs(env, argoCDAppPath)
	if err != nil {
		bootstrapErr = fmt.Errorf("validation failed: %w", err)
		report.AddStage(validationTimer.complete(false, err))
		return bootstrapErr
	}
	if enableCilium {
		if err := validateCiliumEnvironment(env); err != nil {
			bootstrapErr = err
			report.AddStage(validationTimer.complete(false, err))
			return err
		}
	}
	report.AddStage(validationTimer.complete(true, nil))

	// Log configuration
	configStage := logger.Stage("Configuration")
	configStage.Detail("Environment: %s", env)
	configStage.Detail("Base directory: %s", baseDir)
	if subfolderPath != "" {
		configStage.Detail("Subfolder context: %s", subfolderPath)
	}
	configStage.Detail("App path (ArgoCD): %s", argoCDAppPath)
	if localAppPath != argoCDAppPath {
		configStage.Detail("App path (local): %s", localAppPath)
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
	if enableCilium {
		configStage.Detail("Cilium: enabled")
	}
	configStage.Done()

	// Load secrets based on encryption backend
	secretsTimer := startStage("Loading Secrets")
	secretsStage := logger.Stage("Loading Secrets")
	var envSecrets *config.EnvironmentSecrets

	var secretsPath string
	switch encryption {
	case "git-crypt":
		sf := secretsFile
		if sf == "" {
			sf = filepath.Join(baseDir, config.SecretsFileNamePlain(env))
		}
		secretsPath = sf
		report.Configuration.SecretsFile = secretsPath
		if err := validateSecretsFileExists(secretsPath); err != nil {
			bootstrapErr = err
			report.AddStage(secretsTimer.complete(false, err))
			return err
		}
		secretsStage.Detail("Loading plaintext secrets from %s", sf)
		stepf("Loading plaintext secrets from %s...", sf)
		envSecrets, err = config.LoadSecretsPlaintext(sf)
		if err != nil {
			bootstrapErr = err
			report.AddStage(secretsTimer.complete(false, err))
			return err
		}
		secretsStage.Detail("✓ Secrets loaded successfully")
	case "sops":
		sf := secretsFile
		if sf == "" {
			sf = filepath.Join(baseDir, config.SecretsFileName(env))
		}
		secretsPath = sf
		report.Configuration.SecretsFile = secretsPath
		if err := validateSecretsFileExists(secretsPath); err != nil {
			bootstrapErr = err
			report.AddStage(secretsTimer.complete(false, err))
			return err
		}
		secretsStage.Detail("Decrypting secrets from %s", sf)
		stepf("Decrypting secrets from %s...", sf)
		sopsOpts := &sops.Options{AgeKeyFile: bootstrapAgeKey}
		envSecrets, err = config.LoadSecrets(sf, sopsOpts)
		if err != nil {
			bootstrapErr = err
			report.AddStage(secretsTimer.complete(false, err))
			return err
		}
		secretsStage.Detail("✓ Secrets decrypted successfully")
	default:
		bootstrapErr = fmt.Errorf("unsupported encryption backend: %s (use sops or git-crypt)", encryption)
		report.AddStage(secretsTimer.complete(false, bootstrapErr))
		return bootstrapErr
	}

	secretsStage.Detail("Repository: %s", envSecrets.Repo.URL)
	secretsStage.Detail("Target revision: %s", envSecrets.Repo.TargetRevision)
	secretsStage.Done()
	report.AddStage(secretsTimer.complete(true, nil))

	if verbose {
		fmt.Printf("  Repo URL: %s\n", envSecrets.Repo.URL)
		fmt.Printf("  Target revision: %s\n", envSecrets.Repo.TargetRevision)
	}

	if dryRun {
		bootstrapErr = printDryRun(envSecrets, env, argoCDAppPath, enableCilium)
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

	// Create Kubernetes secrets (before Helm install, as the chart may reference them)
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

	// Optionally store the SOPS age identity for ArgoCD repo-server instances
	// that decrypt SOPS-encrypted Helm values in-cluster.
	if storeSopsAgeKey {
		ageKeyPath := bootstrapAgeKey
		if ageKeyPath == "" {
			ageKeyPath = os.Getenv("SOPS_AGE_KEY_FILE")
		}
		if ageKeyPath == "" {
			bootstrapErr = fmt.Errorf("--store-sops-age-key requires --age-key-file or SOPS_AGE_KEY_FILE")
			report.AddStage(secretsK8sTimer.complete(false, bootstrapErr))
			return bootstrapErr
		}

		// The path is deliberately provided by the operator through a CLI flag or
		// SOPS_AGE_KEY_FILE; bootstrap must support age keys stored outside the repo.
		keyData, err := os.ReadFile(ageKeyPath) // #nosec G304,G703 -- explicit operator-controlled key path
		if err != nil {
			bootstrapErr = fmt.Errorf("failed to read SOPS age key file: %w", err)
			report.AddStage(secretsK8sTimer.complete(false, bootstrapErr))
			return bootstrapErr
		}

		stepf("Creating sops-age-key secret...")
		sopsAgeKeyCreated, err := client.CreateSopsAgeKeySecret(ctx, keyData)
		if err != nil {
			bootstrapErr = err
			report.AddStage(secretsK8sTimer.complete(false, err))
			return err
		}
		report.Resources.Secrets = append(report.Resources.Secrets, SecretReport{
			Name:      "sops-age-key",
			Namespace: "argocd",
			Created:   sopsAgeKeyCreated,
		})
		if sopsAgeKeyCreated {
			secretsK8sStage.SecretDetail("Created", "sops-age-key", "argocd")
		} else {
			secretsK8sStage.SecretDetail("Updated", "sops-age-key", "argocd")
		}
	}
	secretsK8sStage.Done()
	report.AddStage(secretsK8sTimer.complete(true, nil))

	bootstrapErr = runBootstrapComponents(
		ctx,
		enableCilium,
		skipArgoCDInstall,
		func(ctx context.Context) error {
			helmTimer := startStage("Installing Cilium")
			helmStage := logger.Stage("Installing Cilium via Helm")
			stepf("Installing Cilium via Helm and waiting for it to become healthy...")
			installed, installErr := helm.InstallCilium(ctx, kubeconfig, kubeContext, env, baseDir, verbose)
			if installErr != nil {
				report.AddStage(helmTimer.complete(false, installErr))
				return installErr
			}
			report.Resources.CiliumRelease = &HelmReleaseReport{
				Name:      "cilium",
				Namespace: "kube-system",
				Installed: installed,
			}
			if installed {
				helmStage.Detail("✓ Cilium installed and healthy")
			} else {
				helmStage.Detail("✓ Cilium upgraded and healthy")
			}
			helmStage.Done()
			report.AddStage(helmTimer.complete(true, nil))
			return nil
		},
		func(ctx context.Context) error {
			helmTimer := startStage("Installing ArgoCD")
			helmStage := logger.Stage("Installing ArgoCD via Helm")
			stepf("Installing ArgoCD via Helm...")
			installed, installErr := helm.InstallArgoCD(ctx, kubeconfig, kubeContext, env, baseDir, verbose)
			if installErr != nil {
				report.AddStage(helmTimer.complete(false, installErr))
				return installErr
			}
			report.Resources.ArgoCDRelease = HelmReleaseReport{
				Name:      "argocd",
				Namespace: "argocd",
				Installed: installed,
			}
			if installed {
				helmStage.Detail("✓ ArgoCD installed successfully")
			} else {
				helmStage.Detail("✓ ArgoCD upgraded successfully")
			}
			helmStage.Done()
			report.AddStage(helmTimer.complete(true, nil))
			return nil
		},
	)
	if bootstrapErr != nil {
		return bootstrapErr
	}
	if skipArgoCDInstall {
		report.Resources.ArgoCDRelease = HelmReleaseReport{
			Name:      "argocd",
			Namespace: "argocd",
			Skipped:   true,
		}
	}

	bootstrapErr = ensureArgoCDReadyForCilium(ctx, enableCilium, skipArgoCDInstall, func(ctx context.Context) error {
		healthTimer := startStage("Waiting for Existing ArgoCD")
		stepf("Waiting for the existing ArgoCD installation to become ready...")
		waitCtx, cancel := context.WithTimeout(ctx, time.Duration(healthTimeout)*time.Second)
		defer cancel()
		if waitErr := waitForDeployment(waitCtx, client.Clientset, "argocd", "argocd-server"); waitErr != nil {
			report.AddStage(healthTimer.complete(false, waitErr))
			return waitErr
		}
		report.AddStage(healthTimer.complete(true, nil))
		return nil
	})
	if bootstrapErr != nil {
		return bootstrapErr
	}

	// Apply App of Apps
	appTimer := startStage("Deploying App of Apps")
	appStage := logger.Stage("Deploying App of Apps")
	stepf("Applying App of Apps for environment: %s", env)
	_, appCreated, err := client.ApplyAppOfApps(ctx, envSecrets.Repo.URL, envSecrets.Repo.TargetRevision, env, argoCDAppPath, enableCilium, false)
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
	if enableCilium {
		appStage.Detail("Cilium enabled through the App of Apps")
		report.Resources.CiliumApplication = &ApplicationReport{
			Name:      "cilium",
			Namespace: "argocd",
			ManagedBy: "app-of-apps",
		}
	}
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
			// Don't fail bootstrap if health checks don't complete, just warn
		} else {
			PrintHealthStatus(healthStatus)
			report.Health.Healthy = healthStatus.Healthy

			// Convert health status results to component health
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

	// Print access instructions (only if not using JSON report format)
	if reportFormat != "json" {
		fmt.Println()
		successf("Done! ArgoCD is installed and the app-of-apps root Application has been created.")
		logger.PrintStageSummary()
		printBootstrapSummary(env, secretsPath, argoCDAppPath)
		fmt.Println("    Access the ArgoCD UI:")
		fmt.Println("      kubectl port-forward svc/argocd-server -n argocd 8080:443")
		fmt.Println("    Get the initial admin password:")
		fmt.Println("      kubectl -n argocd get secret argocd-initial-admin-secret -o jsonpath='{.data.password}' | base64 -d")
	}

	return nil
}

func printDryRun(envSecrets *config.EnvironmentSecrets, env, appPath string, ciliumEnabled bool) error {
	output, err := renderDryRunOutputWithOptions(envSecrets, env, appPath, ciliumEnabled)
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
	return renderDryRunOutputWithOptions(envSecrets, env, appPath, false)
}

func renderDryRunOutputWithOptions(envSecrets *config.EnvironmentSecrets, env, appPath string, ciliumEnabled bool) (string, error) {
	repoSecret, appOfApps := buildDryRunObjectsWithOptions(envSecrets, env, appPath, ciliumEnabled)

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

func validateCiliumEnvironment(env string) error {
	if env == "." || env == ".." || strings.ContainsAny(env, `/\\`) {
		return fmt.Errorf("environment must be a single path-safe name when Cilium is enabled")
	}
	return nil
}

func runBootstrapComponents(
	ctx context.Context,
	ciliumEnabled bool,
	skipArgoCD bool,
	installCilium func(context.Context) error,
	installArgoCD func(context.Context) error,
) error {
	if ciliumEnabled {
		if err := installCilium(ctx); err != nil {
			return fmt.Errorf("failed to bootstrap Cilium and wait for health: %w", err)
		}
	}
	if skipArgoCD {
		return nil
	}
	if err := installArgoCD(ctx); err != nil {
		return fmt.Errorf("failed to install ArgoCD: %w", err)
	}
	return nil
}

func ensureArgoCDReadyForCilium(
	ctx context.Context,
	ciliumEnabled bool,
	skipArgoCD bool,
	waitForArgoCD func(context.Context) error,
) error {
	if !ciliumEnabled || !skipArgoCD {
		return nil
	}
	if err := waitForArgoCD(ctx); err != nil {
		return fmt.Errorf("existing ArgoCD is not ready for Cilium ownership handoff: %w", err)
	}
	return nil
}

func buildDryRunObjects(envSecrets *config.EnvironmentSecrets, env, appPath string) (map[string]interface{}, map[string]interface{}) {
	return buildDryRunObjectsWithOptions(envSecrets, env, appPath, false)
}

func buildDryRunObjectsWithOptions(envSecrets *config.EnvironmentSecrets, env, appPath string, enableCilium bool) (map[string]interface{}, map[string]interface{}) {
	repoSecret := map[string]interface{}{
		"apiVersion": "v1",
		"kind":       "Secret",
		"metadata": map[string]interface{}{
			"name":      "repo-ssh-key",
			"namespace": "argocd",
			"labels": map[string]string{ // #nosec G101
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

	helmValues := map[string]interface{}{
		"valueFiles": []string{
			fmt.Sprintf("values/%s.yaml", env),
		},
	}
	if enableCilium {
		helmValues["parameters"] = []map[string]string{
			{
				"name":  "components.cilium.enabled",
				"value": "true",
			},
		}
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
				"helm":           helmValues,
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

func validateBootstrapInputs(env string, argoCDAppPath string) (localPath string, err error) {
	if env == "" {
		return "", fmt.Errorf("environment is required")
	}

	baseInfo, statErr := os.Stat(baseDir)
	if statErr != nil {
		return "", fmt.Errorf("base-dir %s is not accessible: %w", baseDir, statErr)
	}
	if !baseInfo.IsDir() {
		return "", fmt.Errorf("base-dir %s is not a directory", baseDir)
	}

	if filepath.IsAbs(argoCDAppPath) {
		return "", fmt.Errorf("app-path must be relative")
	}

	// Determine the local path to validate
	// The argoCDAppPath is the full path from repository root (e.g., "k8s/apps")
	// We need to determine what part to validate locally based on baseDir or current directory
	localAppPath := argoCDAppPath

	if baseDir == "." {
		// Check if we're in a Git subdirectory
		detected, relPath := detectGitSubdirectory()
		if detected && relPath != "" && strings.HasPrefix(argoCDAppPath, relPath+"/") {
			// We're in a subdirectory and argoCDAppPath includes that prefix
			// Strip it for local validation
			// Example: In k8s/, argoCDAppPath="k8s/apps" -> localAppPath="apps"
			localAppPath = strings.TrimPrefix(argoCDAppPath, relPath+"/")
		}
	} else if baseDir != "." {
		// When baseDir is set (e.g., "./k8s"), we need to strip the matching prefix from argoCDAppPath
		// Example: baseDir="./k8s", argoCDAppPath="k8s/apps" -> localAppPath="apps"
		cleanBase := filepath.Clean(baseDir)
		baseComponents := strings.Split(cleanBase, string(filepath.Separator))
		pathComponents := strings.Split(argoCDAppPath, "/")

		// Find the last component of baseDir (e.g., "k8s" from "./k8s")
		baseLastComponent := baseComponents[len(baseComponents)-1]

		// If argoCDAppPath starts with the same component, strip it
		if len(pathComponents) > 0 && pathComponents[0] == baseLastComponent {
			// Strip the first component for local validation
			localAppPath = strings.Join(pathComponents[1:], "/")
			if localAppPath == "" {
				localAppPath = "."
			}
		}
	}

	appFullPath := filepath.Join(baseDir, localAppPath)
	if _, statErr := os.Stat(appFullPath); statErr != nil {
		if argoCDAppPath == "apps" {
			detected, detectErr := autoDetectAppPath(baseDir)
			if detectErr != nil {
				return "", fmt.Errorf("app-path %s does not exist: %w\n  hint: use --app-path to specify the full path from repository root (e.g., 'k8s/apps')", argoCDAppPath, statErr)
			}
			localAppPath = detected
			if verbose {
				fmt.Printf("  App path auto-detected: %s\n", localAppPath)
			}
		} else {
			return "", fmt.Errorf("app-path %s does not exist: %w\n  hint: verify the path exists and try using --base-dir if working with subfolders", argoCDAppPath, statErr)
		}
	}

	if secretsFile != "" {
		isEnc := strings.HasSuffix(secretsFile, ".enc.yaml")
		isYaml := strings.HasSuffix(secretsFile, ".yaml")
		switch encryption {
		case "sops":
			if !isEnc {
				return "", fmt.Errorf("secrets-file must end with .enc.yaml when encryption is sops")
			}
		case "git-crypt":
			if !isYaml || isEnc {
				return "", fmt.Errorf("secrets-file must end with .yaml (not .enc.yaml) when encryption is git-crypt")
			}
		}
	}

	return localAppPath, nil
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

func validateSecretsFileExists(path string) error {
	if path == "" {
		return nil
	}
	if _, err := os.Stat(path); err != nil {
		return fmt.Errorf("secrets file not found: %s", path)
	}
	return nil
}

// detectGitSubdirectory checks if we're running from a subdirectory of a Git repository
// Returns (detected bool, relative path from repo root)
func detectGitSubdirectory() (bool, string) {
	cwd, err := os.Getwd()
	if err != nil {
		return false, ""
	}

	// Walk up the directory tree looking for .git
	dir := cwd
	for {
		gitPath := filepath.Join(dir, ".git")
		if _, err := os.Stat(gitPath); err == nil {
			// Found .git directory - this is the repo root
			if dir == cwd {
				// We're at the repo root
				return false, ""
			}

			// Calculate relative path from repo root to current directory
			relPath, err := filepath.Rel(dir, cwd)
			if err != nil {
				return false, ""
			}

			// Normalize path separators to forward slashes (for consistency with Git paths)
			relPath = filepath.ToSlash(relPath)

			return true, relPath
		}

		// Move up one directory
		parent := filepath.Dir(dir)
		if parent == dir {
			// Reached filesystem root without finding .git
			return false, ""
		}
		dir = parent
	}
}
