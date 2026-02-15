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

	if err := os.WriteFile(outputPath, data, 0644); err != nil {
		return fmt.Errorf("failed to write %s: %w", outputPath, err)
	}

	return nil
}
