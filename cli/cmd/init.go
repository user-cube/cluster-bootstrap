package cmd

import (
	"fmt"
	"os"
	"path/filepath"

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
	Long: `Interactively configure SOPS encryption and create encrypted secrets files.
Prompts for the SOPS provider, encryption key, and per-environment secrets.
Each environment gets its own file: secrets.<env>.enc.yaml`,
	RunE: runInit,
}

func init() {
	initCmd.Flags().StringVar(&provider, "provider", "", "SOPS provider (age|aws-kms|gcp-kms)")
	initCmd.Flags().StringVar(&ageKeyFile, "age-key-file", "", "path to age public key file (for age provider)")
	initCmd.Flags().StringVar(&kmsARN, "kms-arn", "", "AWS KMS key ARN (for aws-kms provider)")
	initCmd.Flags().StringVar(&gcpKMSKey, "gcp-kms-key", "", "GCP KMS key resource ID (for gcp-kms provider)")
	initCmd.Flags().StringVar(&outputDir, "output-dir", ".", "directory for encrypted secrets files")

	rootCmd.AddCommand(initCmd)
}

func runInit(cmd *cobra.Command, args []string) error {
	sopsConfigPath := filepath.Join(outputDir, ".sops.yaml")

	// Determine environment names: from positional args or interactive prompt
	var environments []string
	if len(args) > 0 {
		environments = args
	} else {
		for {
			var env string
			err := huh.NewInput().
				Title("Environment name (leave empty to finish)").
				Value(&env).
				Run()
			if err != nil {
				return fmt.Errorf("prompt failed: %w", err)
			}
			if env == "" {
				break
			}
			environments = append(environments, env)
		}
	}

	// Create per-environment secrets files
	created := 0

	for _, env := range environments {
		fmt.Printf("\n--- Environment: %s ---\n", env)

		// Select SOPS provider for this environment
		envProvider := provider
		if envProvider == "" {
			err := huh.NewSelect[string]().
				Title(fmt.Sprintf("Select SOPS provider for %s", env)).
				Options(
					huh.NewOption("age", "age"),
					huh.NewOption("AWS KMS", "aws-kms"),
					huh.NewOption("GCP KMS", "gcp-kms"),
				).
				Value(&envProvider).
				Run()
			if err != nil {
				return fmt.Errorf("prompt failed: %w", err)
			}
		}

		// Get encryption key for this environment
		key, err := getProviderKey(envProvider)
		if err != nil {
			return err
		}

		// Upsert SOPS creation rule for this environment
		if err := config.UpsertSopsRule(sopsConfigPath, envProvider, key, env); err != nil {
			return err
		}
		fmt.Printf("Updated %s with rule for %s\n", sopsConfigPath, env)

		// Create encrypted secrets file
		outputFile := filepath.Join(outputDir, config.SecretsFileName(env))
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

		tmpFile := filepath.Join(outputDir, ".tmp.enc.yaml")
		if err := os.WriteFile(tmpFile, plaintextData, 0600); err != nil {
			return fmt.Errorf("failed to write temp file: %w", err)
		}

		encrypted, err := sops.Encrypt(tmpFile, nil)
		_ = os.Remove(tmpFile)
		if err != nil {
			return fmt.Errorf("failed to encrypt secrets for %s: %w", env, err)
		}

		if err := os.WriteFile(outputFile, encrypted, 0600); err != nil {
			return fmt.Errorf("failed to write %s: %w", outputFile, err)
		}

		fmt.Printf("Created %s (encrypted)\n", outputFile)
		created++
	}

	if created == 0 {
		// Check for pre-existing environment files before erroring
		existing, _ := filepath.Glob(filepath.Join(outputDir, "secrets.*.enc.yaml"))
		if len(existing) > 0 {
			fmt.Println("\nExisting environment secrets files found:")
			for _, f := range existing {
				fmt.Printf("  %s\n", filepath.Base(f))
			}
		} else {
			return fmt.Errorf("no environments configured")
		}
	}

	fmt.Println("\nYou can now run: cluster-bootstrap bootstrap <environment>")

	return nil
}

func getProviderKey(provider string) (string, error) {
	switch provider {
	case "age":
		if ageKeyFile != "" {
			data, err := os.ReadFile(ageKeyFile)
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
				Validate(requiredValidator("repository URL is required")),
			huh.NewInput().
				Title("Target revision (branch/tag)").
				Value(&targetRevision).
				Validate(requiredValidator("target revision is required")),
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
	sshKeyData, err := os.ReadFile(sshKeyPath)
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
