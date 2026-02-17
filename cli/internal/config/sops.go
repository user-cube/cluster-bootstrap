package config

import (
	"fmt"
	"os"

	"gopkg.in/yaml.v3"
)

// SOPSConfig represents the .sops.yaml configuration file.
type SOPSConfig struct {
	CreationRules []CreationRule `yaml:"creation_rules"`
}

// CreationRule defines a SOPS creation rule.
type CreationRule struct {
	PathRegex string `yaml:"path_regex,omitempty"`
	Age       string `yaml:"age,omitempty"`
	KMS       string `yaml:"kms,omitempty"`
	GCPKMS    string `yaml:"gcp_kms,omitempty"`
}

// ReadSopsConfig reads and parses an existing .sops.yaml file.
func ReadSopsConfig(path string) (*SOPSConfig, error) {
	data, err := os.ReadFile(path) //nolint:gosec // user-provided file path from flag/config
	if err != nil {
		return nil, fmt.Errorf("failed to read %s: %w", path, err)
	}

	var cfg SOPSConfig
	if err := yaml.Unmarshal(data, &cfg); err != nil {
		return nil, fmt.Errorf("failed to parse %s: %w", path, err)
	}

	return &cfg, nil
}

// EnvPathRegex returns the path_regex pattern for a given environment name.
func EnvPathRegex(envName string) string {
	return fmt.Sprintf(`secrets\.%s\.enc\.yaml$`, envName)
}

// UpsertSopsRule ensures a creation rule exists in .sops.yaml for the given environment.
// If the file doesn't exist, it creates it. If a rule with a matching path_regex
// already exists, it updates the provider/key. Otherwise it appends a new rule.
func UpsertSopsRule(outputPath, provider, key, envName string) error {
	pathRegex := EnvPathRegex(envName)

	rule := CreationRule{
		PathRegex: pathRegex,
	}

	switch provider {
	case "age":
		rule.Age = key
	case "aws-kms":
		rule.KMS = key
	case "gcp-kms":
		rule.GCPKMS = key
	default:
		return fmt.Errorf("unsupported SOPS provider: %s", provider)
	}

	var cfg SOPSConfig

	if _, err := os.Stat(outputPath); err == nil {
		existing, err := ReadSopsConfig(outputPath)
		if err != nil {
			return err
		}
		cfg = *existing
	}

	// Find existing rule with matching path_regex and update, or append
	found := false
	for i, r := range cfg.CreationRules {
		if r.PathRegex == pathRegex {
			cfg.CreationRules[i] = rule
			found = true
			break
		}
	}
	if !found {
		cfg.CreationRules = append(cfg.CreationRules, rule)
	}

	data, err := yaml.Marshal(cfg)
	if err != nil {
		return fmt.Errorf("failed to marshal SOPS config: %w", err)
	}

	if err := os.WriteFile(outputPath, data, 0600); err != nil {
		return fmt.Errorf("failed to write %s: %w", outputPath, err)
	}

	return nil
}

// WriteSopsConfig writes the .sops.yaml file with the given provider configuration.
func WriteSopsConfig(outputPath, provider, key string) error {
	rule := CreationRule{
		PathRegex: `\.enc\.yaml$`,
	}

	switch provider {
	case "age":
		rule.Age = key
	case "aws-kms":
		rule.KMS = key
	case "gcp-kms":
		rule.GCPKMS = key
	default:
		return fmt.Errorf("unsupported SOPS provider: %s", provider)
	}

	config := SOPSConfig{
		CreationRules: []CreationRule{rule},
	}

	data, err := yaml.Marshal(config)
	if err != nil {
		return fmt.Errorf("failed to marshal SOPS config: %w", err)
	}

	if err := os.WriteFile(outputPath, data, 0600); err != nil {
		return fmt.Errorf("failed to write %s: %w", outputPath, err)
	}

	return nil
}
