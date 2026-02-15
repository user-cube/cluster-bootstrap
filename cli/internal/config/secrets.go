package config

import (
	"fmt"

	"gopkg.in/yaml.v3"

	"github.com/user-cube/cluster-bootstrap/cli/internal/sops"
)

// SecretsFile represents the top-level structure of secrets.enc.yaml.
type SecretsFile struct {
	Environments map[string]EnvironmentSecrets `yaml:"environments"`
}

// EnvironmentSecrets holds the secrets for a single environment.
type EnvironmentSecrets struct {
	Repo  RepoSecrets  `yaml:"repo"`
	Vault VaultSecrets `yaml:"vault,omitempty"`
}

// RepoSecrets holds git repository credentials.
type RepoSecrets struct {
	URL            string `yaml:"url"`
	TargetRevision string `yaml:"targetRevision"`
	SSHPrivateKey  string `yaml:"sshPrivateKey"`
}

// VaultSecrets holds Vault connection details.
type VaultSecrets struct {
	Address string `yaml:"address,omitempty"`
	Token   string `yaml:"token,omitempty"`
}

// ValidEnvironments lists the allowed environment names.
var ValidEnvironments = []string{"dev", "staging", "prod"}

// IsValidEnvironment checks if the given environment name is valid.
func IsValidEnvironment(env string) bool {
	for _, e := range ValidEnvironments {
		if e == env {
			return true
		}
	}
	return false
}

// LoadSecrets decrypts and parses the SOPS-encrypted secrets file.
func LoadSecrets(filePath string, sopsOpts *sops.Options) (*SecretsFile, error) {
	plaintext, err := sops.Decrypt(filePath, sopsOpts)
	if err != nil {
		return nil, fmt.Errorf("failed to decrypt secrets: %w", err)
	}

	var secrets SecretsFile
	if err := yaml.Unmarshal(plaintext, &secrets); err != nil {
		return nil, fmt.Errorf("failed to parse secrets: %w", err)
	}

	return &secrets, nil
}

// GetEnvironment returns the secrets for a specific environment.
func (s *SecretsFile) GetEnvironment(env string) (*EnvironmentSecrets, error) {
	envSecrets, ok := s.Environments[env]
	if !ok {
		return nil, fmt.Errorf("environment %q not found in secrets file", env)
	}
	return &envSecrets, nil
}
