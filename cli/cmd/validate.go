package cmd

import (
	"fmt"
	"os"
	"path/filepath"
	"strings"

	"github.com/spf13/cobra"

	"github.com/user-cube/cluster-bootstrap/cli/internal/config"
	"github.com/user-cube/cluster-bootstrap/cli/internal/sops"
)

type validateResult struct {
	name string
	note string
	err  error
	warn bool
}

var (
	validateEncryption       string
	validateSecretsFile      string
	validateAgeKeyFile       string
	validateAppPath          string
	validateKubeconfig       string
	validateContext          string
	validateSkipClusterCheck bool
)

var validateCmd = &cobra.Command{
	Use:   "validate <environment>",
	Short: "Validate local config and cluster readiness",
	Long: `Validate local configuration, secrets, and optional cluster access.

This command performs deeper checks than doctor, including reading secrets
files, validating .sops.yaml rules, and verifying repo credentials.`,
	Args: cobra.ExactArgs(1),
	RunE: runValidate,
}

func init() {
	validateCmd.Flags().StringVar(&validateEncryption, "encryption", "sops", "encryption backend (sops|git-crypt)")
	validateCmd.Flags().StringVar(&validateSecretsFile, "secrets-file", "", "path to secrets file (default: secrets.<env>.enc.yaml or secrets.<env>.yaml)")
	validateCmd.Flags().StringVar(&validateAgeKeyFile, "age-key-file", "", "path to age private key file for SOPS decryption")
	validateCmd.Flags().StringVar(&validateAppPath, "app-path", "apps", "path inside the Git repo for the App of Apps source")
	validateCmd.Flags().StringVar(&validateKubeconfig, "kubeconfig", "", "path to kubeconfig file")
	validateCmd.Flags().StringVar(&validateContext, "context", "", "kubeconfig context to use")
	validateCmd.Flags().BoolVar(&validateSkipClusterCheck, "skip-cluster-check", false, "skip kubectl cluster access checks")

	rootCmd.AddCommand(validateCmd)
}

func runValidate(cmd *cobra.Command, args []string) error {
	env := args[0]
	logger := NewLogger(verbose)
	stage := logger.Stage("Validation")

	if validateEncryption != "sops" && validateEncryption != "git-crypt" {
		return fmt.Errorf("unsupported encryption backend: %s (use sops or git-crypt)", validateEncryption)
	}

	results := make([]validateResult, 0, 12)

	results = append(results, runValidateCheck(stage, "base directory", func() (string, error) {
		info, err := os.Stat(baseDir)
		if err != nil {
			return "", fmt.Errorf("base-dir %s not accessible: %w", baseDir, err)
		}
		if !info.IsDir() {
			return "", fmt.Errorf("base-dir %s is not a directory", baseDir)
		}
		return baseDir, nil
	}))

	resolvedAppPath, appErr := resolveAppPath(baseDir, validateAppPath)
	if appErr != nil {
		results = append(results, validateResult{name: "app path", err: appErr})
	} else {
		results = append(results, validateResult{name: "app path", note: resolvedAppPath})
	}

	results = append(results, runValidateCheck(stage, "helm available", func() (string, error) {
		return "", CheckHelm()
	}))

	results = append(results, runValidateCheck(stage, "kubectl available", func() (string, error) {
		return "", CheckKubectlAvailable(true)
	}))

	results = append(results, runValidateCheck(stage, "kubectl current context", func() (string, error) {
		return getKubectlCurrentContext(validateKubeconfig)
	}))

	if !validateSkipClusterCheck {
		results = append(results, runValidateCheck(stage, "kubectl cluster access", func() (string, error) {
			return "", CheckKubectlClusterAccessWithConfig(validateKubeconfig, validateContext)
		}))
	}

	results = append(results, runValidateCheck(stage, "encryption tooling", func() (string, error) {
		switch validateEncryption {
		case "sops":
			if err := CheckSOPS("sops"); err != nil {
				return "", err
			}
			return "sops", CheckAge("sops", validateAgeKeyFile)
		case "git-crypt":
			return "git-crypt", CheckGitCrypt("git-crypt")
		default:
			return "", fmt.Errorf("unsupported encryption backend: %s", validateEncryption)
		}
	}))

	secretsPath := validateSecretsFile
	if secretsPath == "" {
		switch validateEncryption {
		case "sops":
			secretsPath = filepath.Join(baseDir, config.SecretsFileName(env))
		case "git-crypt":
			secretsPath = filepath.Join(baseDir, config.SecretsFileNamePlain(env))
		}
	}

	results = append(results, runValidateCheck(stage, "secrets file", func() (string, error) {
		if _, err := os.Stat(secretsPath); err != nil {
			return "", fmt.Errorf("secrets file not found: %s", secretsPath)
		}
		if err := CheckFilePermissions(secretsPath, true); err != nil {
			return "", err
		}
		return secretsPath, nil
	}))

	results = append(results, runValidateCheck(stage, "secrets content", func() (string, error) {
		var secrets *config.EnvironmentSecrets
		var err error
		switch validateEncryption {
		case "sops":
			secrets, err = config.LoadSecrets(secretsPath, &sops.Options{AgeKeyFile: validateAgeKeyFile})
		case "git-crypt":
			secrets, err = config.LoadSecretsPlaintext(secretsPath)
		}
		if err != nil {
			return "", err
		}
		if strings.TrimSpace(secrets.Repo.URL) == "" {
			return "", fmt.Errorf("repo.url is required in secrets file")
		}
		if strings.TrimSpace(secrets.Repo.SSHPrivateKey) == "" {
			return "", fmt.Errorf("repo.sshPrivateKey is required in secrets file")
		}
		return "repo credentials validated", nil
	}))

	results = append(results, validateSecretsWarnings(secretsPath))

	results = append(results, validateSopsConfig(env))
	results = append(results, validateGitCryptAttributes())

	stage.Done()

	printValidateReport(env, results)
	if countValidateErrors(results) > 0 {
		return fmt.Errorf("validate found %d issue(s)", countValidateErrors(results))
	}

	successf("Validation passed")
	return nil
}

func resolveAppPath(base, appPath string) (string, error) {
	if filepath.IsAbs(appPath) {
		return "", fmt.Errorf("app-path must be relative to base-dir")
	}
	appFullPath := filepath.Join(base, appPath)
	if _, err := os.Stat(appFullPath); err != nil {
		if appPath == "apps" {
			detected, detectErr := autoDetectAppPath(base)
			if detectErr != nil {
				return "", fmt.Errorf("app-path %s does not exist under base-dir: %w", appPath, err)
			}
			return detected, nil
		}
		return "", fmt.Errorf("app-path %s does not exist under base-dir: %w", appPath, err)
	}
	return appPath, nil
}

func validateSopsConfig(env string) validateResult {
	if validateEncryption != "sops" {
		return validateResult{name: ".sops.yaml", note: "skipped", warn: true}
	}

	sopsPath := filepath.Join(baseDir, ".sops.yaml")
	cfg, err := config.ReadSopsConfig(sopsPath)
	if err != nil {
		return validateResult{name: ".sops.yaml", note: "missing or unreadable", warn: true}
	}

	expected := config.EnvPathRegex(env)
	for _, rule := range cfg.CreationRules {
		if rule.PathRegex == expected {
			return validateResult{name: ".sops.yaml", note: "creation rule found"}
		}
	}

	return validateResult{name: ".sops.yaml", note: "missing creation rule for environment", warn: true}
}

func validateGitCryptAttributes() validateResult {
	if validateEncryption != "git-crypt" {
		return validateResult{name: ".gitattributes", note: "skipped", warn: true}
	}

	attrsPath := filepath.Join(baseDir, ".gitattributes")
	data, err := os.ReadFile(attrsPath) // #nosec G304
	if err != nil {
		return validateResult{name: ".gitattributes", err: fmt.Errorf("failed to read %s: %w", attrsPath, err)}
	}

	content := string(data)
	if !strings.Contains(content, config.GitCryptAttributesPattern) {
		return validateResult{name: ".gitattributes", note: "missing git-crypt pattern", warn: true}
	}

	return validateResult{name: ".gitattributes", note: "pattern found"}
}

func runValidateCheck(stage *StageLogger, name string, fn func() (string, error)) validateResult {
	note, err := fn()
	if err != nil {
		stage.Detail("FAIL: %s", name)
		return validateResult{name: name, note: note, err: err}
	}
	stage.Detail("OK: %s", name)
	return validateResult{name: name, note: note}
}

func printValidateReport(env string, results []validateResult) {
	fmt.Println()
	fmt.Println("Validate report:")
	for _, result := range results {
		status := "OK"
		if result.err != nil {
			status = "FAIL"
		} else if result.warn {
			status = "WARN"
		}
		fmt.Printf("  - %s: %s", status, result.name)
		if result.note != "" {
			fmt.Printf(" (%s)", result.note)
		}
		fmt.Println()
		if result.err != nil {
			printDoctorError(result.err)
		}
		if result.warn && result.note == "missing creation rule for environment" {
			fmt.Printf("      hint: run 'cluster-bootstrap init %s' or update .sops.yaml\n", env)
		}
		if result.warn && result.note == "missing git-crypt pattern" {
			fmt.Printf("      hint: run 'cluster-bootstrap init --provider git-crypt' or update .gitattributes\n")
		}
		if result.warn && result.note == "repo.targetRevision is empty" {
			fmt.Printf("      hint: set repo.targetRevision in %s\n", secretsFileForEnv(env))
		}
	}
}

func countValidateErrors(results []validateResult) int {
	count := 0
	for _, result := range results {
		if result.err != nil {
			count++
		}
	}
	return count
}

func validateSecretsWarnings(secretsPath string) validateResult {
	if validateEncryption == "sops" {
		secrets, err := config.LoadSecrets(secretsPath, &sops.Options{AgeKeyFile: validateAgeKeyFile})
		if err != nil {
			return validateResult{name: "secrets warnings", note: "skipped", warn: true}
		}
		if strings.TrimSpace(secrets.Repo.TargetRevision) == "" {
			return validateResult{name: "secrets warnings", note: "repo.targetRevision is empty", warn: true}
		}
		return validateResult{name: "secrets warnings", note: "none"}
	}

	secrets, err := config.LoadSecretsPlaintext(secretsPath)
	if err != nil {
		return validateResult{name: "secrets warnings", note: "skipped", warn: true}
	}
	if strings.TrimSpace(secrets.Repo.TargetRevision) == "" {
		return validateResult{name: "secrets warnings", note: "repo.targetRevision is empty", warn: true}
	}
	return validateResult{name: "secrets warnings", note: "none"}
}

func secretsFileForEnv(env string) string {
	if validateSecretsFile != "" {
		return validateSecretsFile
	}
	if validateEncryption == "git-crypt" {
		return filepath.Join(baseDir, config.SecretsFileNamePlain(env))
	}
	return filepath.Join(baseDir, config.SecretsFileName(env))
}
