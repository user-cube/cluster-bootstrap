package cmd

import (
	"fmt"
	"net/url"
	"os"
	"path/filepath"
	"regexp"
	"strings"

	"github.com/charmbracelet/huh"
	"github.com/spf13/cobra"
	"gopkg.in/yaml.v3"

	"github.com/user-cube/cluster-bootstrap/cluster-bootstrap/internal/config"
	"github.com/user-cube/cluster-bootstrap/cluster-bootstrap/internal/sops"
)

var (
	provider   string
	ageKeyFile string
	kmsARN     string
	gcpKMSKey  string
	outputDir  string
)

var initCmd = &cobra.Command{
	Use:   "init [environments...]",
	Short: "Interactive setup to create .sops.yaml and per-environment secrets files",
	Long: `Interactively configure encryption and create per-environment secrets files.
Prompts for the encryption provider, encryption key, and per-environment secrets.

Supported providers:
  - age, aws-kms, gcp-kms: uses SOPS encryption (secrets.<env>.enc.yaml)
  - git-crypt: uses git-crypt transparent encryption (secrets.<env>.yaml)`,
	RunE: runInit,
}

func init() {
	initCmd.Flags().StringVar(&provider, "provider", "", "encryption provider (age|aws-kms|gcp-kms|git-crypt)")
	initCmd.Flags().StringVar(&ageKeyFile, "age-key-file", "", "path to age public key file (for age provider)")
	initCmd.Flags().StringVar(&kmsARN, "kms-arn", "", "AWS KMS key ARN (for aws-kms provider)")
	initCmd.Flags().StringVar(&gcpKMSKey, "gcp-kms-key", "", "GCP KMS key resource ID (for gcp-kms provider)")
	initCmd.Flags().StringVar(&outputDir, "output-dir", ".", "directory for secrets files")

	rootCmd.AddCommand(initCmd)
}

func runInit(cmd *cobra.Command, args []string) error {
	// Use baseDir as fallback for outputDir when outputDir is default
	effectiveOutputDir := outputDir
	if outputDir == "." && baseDir != "." {
		effectiveOutputDir = baseDir
	}

	sopsConfigPath := filepath.Join(effectiveOutputDir, ".sops.yaml")

	// Determine environment names: from positional args or interactive prompt
	var environments []string
	if len(args) > 0 {
		environments = args
	} else {
		for {
			var env string
			err := huh.NewForm(
				huh.NewGroup(
					huh.NewInput().
						Title("Environment name").
						Description("Name for this environment (e.g. dev, staging, prod)").
						Validate(requiredValidator("environment name is required")).
						Value(&env),
				),
			).Run()
			if err != nil {
				return fmt.Errorf("prompt failed: %w", err)
			}
			environments = append(environments, env)
			fmt.Printf("  Added environment: %s\n", env)

			var addMore bool
			err = huh.NewForm(
				huh.NewGroup(
					huh.NewConfirm().
						Title("Add another environment?").
						Value(&addMore),
				),
			).Run()
			if err != nil {
				return fmt.Errorf("prompt failed: %w", err)
			}
			if !addMore {
				break
			}
		}
	}

	// Create per-environment secrets files
	created := 0
	var createdFiles []string
	sopsConfigUpdated := false

	for _, env := range environments {
		fmt.Printf("\n--- Environment: %s ---\n", env)

		// Select encryption provider for this environment
		envProvider := provider
		if envProvider == "" {
			err := huh.NewSelect[string]().
				Title(fmt.Sprintf("Select encryption provider for %s", env)).
				Options(
					huh.NewOption("age", "age"),
					huh.NewOption("AWS KMS", "aws-kms"),
					huh.NewOption("GCP KMS", "gcp-kms"),
					huh.NewOption("git-crypt", "git-crypt"),
				).
				Value(&envProvider).
				Run()
			if err != nil {
				return fmt.Errorf("prompt failed: %w", err)
			}
		}

		if envProvider == "git-crypt" {
			count, createdFile, err := initGitCrypt(effectiveOutputDir, env)
			if err != nil {
				return err
			}
			if createdFile != "" {
				createdFiles = append(createdFiles, createdFile)
			}
			created += count
			continue
		}

		// SOPS path
		key, err := getProviderKey(envProvider)
		if err != nil {
			return err
		}

		// Upsert SOPS creation rule for this environment
		if err := config.UpsertSopsRule(sopsConfigPath, envProvider, key, env); err != nil {
			return err
		}
		fmt.Printf("Updated %s with rule for %s\n", sopsConfigPath, env)
		sopsConfigUpdated = true

		// Create encrypted secrets file
		outputFile := filepath.Join(effectiveOutputDir, config.SecretsFileName(env))
		if _, statErr := os.Stat(outputFile); statErr == nil {
			var overwrite bool
			err := huh.NewConfirm().
				Title(fmt.Sprintf("%s already exists. Overwrite?", config.SecretsFileName(env))).
				Value(&overwrite).
				Run()
			if err != nil {
				return fmt.Errorf("prompt failed: %w", err)
			}
			if !overwrite {
				continue
			}
		}

		envSecrets, err := promptEnvironmentSecrets(env)
		if err != nil {
			return err
		}

		plaintextData, err := yaml.Marshal(envSecrets)
		if err != nil {
			return fmt.Errorf("failed to marshal secrets: %w", err)
		}

		tmpFile := filepath.Join(effectiveOutputDir, ".tmp.enc.yaml")
		if err := os.WriteFile(tmpFile, plaintextData, 0600); err != nil {
			return fmt.Errorf("failed to write temp file: %w", err)
		}

		encrypted, err := sops.EncryptWithTarget(tmpFile, outputFile, nil)
		_ = os.Remove(tmpFile)
		if err != nil {
			return fmt.Errorf("failed to encrypt secrets for %s: %w", env, err)
		}

		if err := os.WriteFile(outputFile, encrypted, 0600); err != nil {
			return fmt.Errorf("failed to write %s: %w", outputFile, err)
		}

		fmt.Printf("Created %s (encrypted)\n", outputFile)
		createdFiles = append(createdFiles, outputFile)
		created++
	}

	if created == 0 {
		// Check for pre-existing environment files before erroring
		existing, _ := filepath.Glob(filepath.Join(effectiveOutputDir, "secrets.*.enc.yaml"))
		existingPlain, _ := filepath.Glob(filepath.Join(effectiveOutputDir, "secrets.*.yaml"))
		existing = append(existing, existingPlain...)
		if len(existing) > 0 {
			fmt.Println("\nExisting environment secrets files found:")
			for _, f := range existing {
				fmt.Printf("  %s\n", filepath.Base(f))
			}
		} else {
			return fmt.Errorf("no environments configured")
		}
	}

	if sopsConfigUpdated || len(createdFiles) > 0 {
		fmt.Println("\nSummary:")
		if sopsConfigUpdated {
			fmt.Printf("  Updated config: %s\n", sopsConfigPath)
		}
		if len(createdFiles) > 0 {
			fmt.Println("  Created secrets files:")
			for _, f := range createdFiles {
				fmt.Printf("    - %s\n", f)
			}
		}
	}

	fmt.Println("\nYou can now run: cluster-bootstrap bootstrap <environment>")

	return nil
}

// initGitCrypt handles the git-crypt provider path for a single environment.
func initGitCrypt(outputDir, env string) (int, string, error) {
	// Verify git-crypt is initialised in the repo
	gitCryptDir := filepath.Join(outputDir, ".git", "git-crypt")
	if _, err := os.Stat(gitCryptDir); os.IsNotExist(err) {
		return 0, "", fmt.Errorf("git-crypt not initialised: %s not found. Run 'git-crypt init' first", gitCryptDir)
	}

	// Ensure .gitattributes has the git-crypt pattern
	if err := config.EnsureGitCryptAttributes(outputDir); err != nil {
		return 0, "", err
	}
	fmt.Printf("Ensured .gitattributes has git-crypt pattern\n")

	// Create plaintext secrets file (git-crypt encrypts on commit)
	outputFile := filepath.Join(outputDir, config.SecretsFileNamePlain(env))
	if _, statErr := os.Stat(outputFile); statErr == nil {
		var overwrite bool
		err := huh.NewConfirm().
			Title(fmt.Sprintf("%s already exists. Overwrite?", config.SecretsFileNamePlain(env))).
			Value(&overwrite).
			Run()
		if err != nil {
			return 0, "", fmt.Errorf("prompt failed: %w", err)
		}
		if !overwrite {
			return 0, "", nil
		}
	}

	envSecrets, err := promptEnvironmentSecrets(env)
	if err != nil {
		return 0, "", err
	}

	plaintextData, err := yaml.Marshal(envSecrets)
	if err != nil {
		return 0, "", fmt.Errorf("failed to marshal secrets: %w", err)
	}

	if err := os.WriteFile(outputFile, plaintextData, 0600); err != nil {
		return 0, "", fmt.Errorf("failed to write %s: %w", outputFile, err)
	}

	fmt.Printf("Created %s (plaintext — git-crypt encrypts on commit)\n", outputFile)
	return 1, outputFile, nil
}

func getProviderKey(provider string) (string, error) {
	switch provider {
	case "age":
		if ageKeyFile != "" {
			data, err := os.ReadFile(ageKeyFile) //nolint:gosec // safe: user-provided age key file from flag
			if err != nil {
				return "", fmt.Errorf("failed to read age key file: %w", err)
			}
			return string(data), nil
		}
		var key string
		err := huh.NewInput().
			Title("Enter age public key (age1...)").
			Value(&key).
			Validate(requiredValidator("age public key is required")).
			Run()
		if err != nil {
			return "", fmt.Errorf("prompt failed: %w", err)
		}
		return key, nil

	case "aws-kms":
		if kmsARN != "" {
			return kmsARN, nil
		}
		var key string
		err := huh.NewInput().
			Title("Enter AWS KMS key ARN").
			Value(&key).
			Validate(requiredValidator("KMS ARN is required")).
			Run()
		if err != nil {
			return "", fmt.Errorf("prompt failed: %w", err)
		}
		return key, nil

	case "gcp-kms":
		if gcpKMSKey != "" {
			return gcpKMSKey, nil
		}
		var key string
		err := huh.NewInput().
			Title("Enter GCP KMS key resource ID").
			Value(&key).
			Validate(requiredValidator("GCP KMS key is required")).
			Run()
		if err != nil {
			return "", fmt.Errorf("prompt failed: %w", err)
		}
		return key, nil

	default:
		return "", fmt.Errorf("unsupported provider: %s", provider)
	}
}

func promptEnvironmentSecrets(env string) (*config.EnvironmentSecrets, error) {
	var repoURL string
	targetRevision := "main"
	var sshKeyPath string

	form := huh.NewForm(
		huh.NewGroup(
			huh.NewInput().
				Title("Repository SSH URL").
				Value(&repoURL).
				Validate(validateRepoURL),
			huh.NewInput().
				Title("Target revision (branch/tag)").
				Value(&targetRevision).
				Validate(validateTargetRevision),
			huh.NewInput().
				Title("Path to SSH private key file").
				Value(&sshKeyPath).
				Validate(requiredValidator("SSH key path is required")),
		),
	)

	if err := form.Run(); err != nil {
		return nil, fmt.Errorf("prompt failed: %w", err)
	}

	// Read SSH key from filesystem
	sshKeyData, err := os.ReadFile(sshKeyPath) //nolint:gosec // safe: user-provided SSH key from flag
	if err != nil {
		return nil, fmt.Errorf("failed to read SSH key at %s: %w", sshKeyPath, err)
	}

	envSecrets := &config.EnvironmentSecrets{
		Repo: config.RepoSecrets{
			URL:            repoURL,
			TargetRevision: targetRevision,
			SSHPrivateKey:  string(sshKeyData),
		},
	}

	return envSecrets, nil
}

func requiredValidator(msg string) func(s string) error {
	return func(s string) error {
		if s == "" {
			return fmt.Errorf("%s", msg)
		}
		return nil
	}
}

var scpLikeRepoURL = regexp.MustCompile(`^[^@\s]+@[^:\s]+:.+`)

func validateRepoURL(value string) error {
	value = strings.TrimSpace(value)
	if value == "" {
		return fmt.Errorf("repository URL is required")
	}
	if strings.Contains(value, " ") {
		return fmt.Errorf("repository URL must not contain spaces")
	}
	if strings.Contains(value, "://") {
		parsed, err := url.Parse(value)
		if err != nil || parsed.Scheme == "" || parsed.Host == "" {
			return fmt.Errorf("repository URL must be a valid URL")
		}
		return nil
	}
	if scpLikeRepoURL.MatchString(value) {
		return nil
	}
	return fmt.Errorf("repository URL must be an ssh or https URL")
}

func validateTargetRevision(value string) error {
	value = strings.TrimSpace(value)
	if value == "" {
		return fmt.Errorf("target revision is required")
	}
	if strings.Contains(value, " ") {
		return fmt.Errorf("target revision must not contain spaces")
	}
	return nil
}
