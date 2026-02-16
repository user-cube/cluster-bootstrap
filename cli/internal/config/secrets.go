package config

import (
	"bytes"
	"fmt"
	"os"

	"gopkg.in/yaml.v3"

	"github.com/user-cube/cluster-bootstrap/cluster-bootstrap/internal/sops"
)

// gitCryptMagic is the magic header written by git-crypt to encrypted files.
var gitCryptMagic = []byte("\x00GITCRYPT")

// EnvironmentSecrets holds the secrets for a single environment.
// Each environment has its own secrets file: secrets.<env>.enc.yaml
type EnvironmentSecrets struct {
	Repo RepoSecrets `yaml:"repo"`
}

// RepoSecrets holds git repository credentials.
type RepoSecrets struct {
	URL            string `yaml:"url"`
	TargetRevision string `yaml:"targetRevision"`
	SSHPrivateKey  string `yaml:"sshPrivateKey"`
}

// SecretsFileName returns the SOPS-encrypted secrets file name for the given environment.
func SecretsFileName(env string) string {
	return fmt.Sprintf("secrets.%s.enc.yaml", env)
}

// SecretsFileNamePlain returns the plaintext secrets file name for git-crypt environments.
func SecretsFileNamePlain(env string) string {
	return fmt.Sprintf("secrets.%s.yaml", env)
}

// LoadSecrets decrypts and parses a per-environment SOPS-encrypted secrets file.
func LoadSecrets(filePath string, sopsOpts *sops.Options) (*EnvironmentSecrets, error) {
	plaintext, err := sops.Decrypt(filePath, sopsOpts)
	if err != nil {
		return nil, fmt.Errorf("failed to decrypt secrets: %w", err)
	}

	var secrets EnvironmentSecrets
	if err := yaml.Unmarshal(plaintext, &secrets); err != nil {
		return nil, fmt.Errorf("failed to parse secrets: %w", err)
	}

	return &secrets, nil
}

// LoadSecretsPlaintext reads a plaintext (git-crypt managed) secrets file.
// It returns an error if the file still contains the git-crypt magic header,
// which means it has not been decrypted (git-crypt unlock has not been run).
func LoadSecretsPlaintext(filePath string) (*EnvironmentSecrets, error) {
	data, err := os.ReadFile(filePath)
	if err != nil {
		return nil, fmt.Errorf("failed to read secrets file: %w", err)
	}

	if bytes.HasPrefix(data, gitCryptMagic) {
		return nil, fmt.Errorf("file %s is still encrypted by git-crypt; run 'git-crypt unlock' first", filePath)
	}

	var secrets EnvironmentSecrets
	if err := yaml.Unmarshal(data, &secrets); err != nil {
		return nil, fmt.Errorf("failed to parse secrets: %w", err)
	}

	return &secrets, nil
}
