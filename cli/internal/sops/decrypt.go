package sops

import (
	"fmt"
	"os"
	"path/filepath"
	"strings"

	"github.com/getsops/sops/v3"
	"github.com/getsops/sops/v3/aes"
	"github.com/getsops/sops/v3/age"
	"github.com/getsops/sops/v3/cmd/sops/common"
	"github.com/getsops/sops/v3/decrypt"
	"github.com/getsops/sops/v3/keyservice"
	yamlstore "github.com/getsops/sops/v3/stores/yaml"
	"github.com/getsops/sops/v3/version"
)

// Options configures SOPS operations.
type Options struct {
	AgeKeyFile string
}

// setAgeEnv sets SOPS_AGE_KEY_FILE so the sops library can find the key.
func setAgeEnv(opts *Options) (restore func()) {
	restore = func() {}
	if opts == nil || opts.AgeKeyFile == "" {
		return
	}
	prev, hadPrev := os.LookupEnv("SOPS_AGE_KEY_FILE")
	_ = os.Setenv("SOPS_AGE_KEY_FILE", opts.AgeKeyFile)
	restore = func() {
		if hadPrev {
			_ = os.Setenv("SOPS_AGE_KEY_FILE", prev)
		} else {
			_ = os.Unsetenv("SOPS_AGE_KEY_FILE")
		}
	}
	return
}

// Decrypt decrypts a SOPS-encrypted file and returns the plaintext bytes.
func Decrypt(filePath string, opts *Options) ([]byte, error) {
	restore := setAgeEnv(opts)
	defer restore()

	out, err := decrypt.File(filePath, "yaml")
	if err != nil {
		return nil, fmt.Errorf("sops decrypt failed: %w", err)
	}
	return out, nil
}

// Encrypt encrypts a plaintext YAML file using the .sops.yaml config and returns ciphertext bytes.
func Encrypt(filePath string, opts *Options) ([]byte, error) {
	restore := setAgeEnv(opts)
	defer restore()

	// Read plaintext
	data, err := os.ReadFile(filePath)
	if err != nil {
		return nil, fmt.Errorf("sops encrypt: failed to read file: %w", err)
	}

	// Load .sops.yaml creation rules to find the age recipient
	recipient, err := findAgeRecipient(filePath)
	if err != nil {
		return nil, fmt.Errorf("sops encrypt: %w", err)
	}

	// Parse plaintext into sops tree branches
	store := &yamlstore.Store{}
	branches, err := store.LoadPlainFile(data)
	if err != nil {
		return nil, fmt.Errorf("sops encrypt: failed to parse plaintext: %w", err)
	}

	// Build age master key
	ageKey, err := age.MasterKeyFromRecipient(recipient)
	if err != nil {
		return nil, fmt.Errorf("sops encrypt: invalid age recipient: %w", err)
	}
	keyGroup := sops.KeyGroup{ageKey}

	tree := sops.Tree{
		Branches: branches,
		Metadata: sops.Metadata{
			KeyGroups:         []sops.KeyGroup{keyGroup},
			UnencryptedSuffix: "",
			Version:           version.Version,
		},
	}

	// Generate data key
	dataKey, errs := tree.GenerateDataKeyWithKeyServices(
		[]keyservice.KeyServiceClient{keyservice.NewLocalClient()},
	)
	if len(errs) > 0 {
		return nil, fmt.Errorf("sops encrypt: failed to generate data key: %v", errs)
	}

	// Encrypt
	err = common.EncryptTree(common.EncryptTreeOpts{
		DataKey: dataKey,
		Tree:    &tree,
		Cipher:  aes.NewCipher(),
	})
	if err != nil {
		return nil, fmt.Errorf("sops encrypt: failed to encrypt: %w", err)
	}

	// Emit encrypted YAML
	out, err := store.EmitEncryptedFile(tree)
	if err != nil {
		return nil, fmt.Errorf("sops encrypt: failed to emit encrypted file: %w", err)
	}

	return out, nil
}

// findAgeRecipient reads the .sops.yaml config to find the age recipient for the given file.
func findAgeRecipient(filePath string) (string, error) {
	// Walk up from the file to find .sops.yaml
	dir := filepath.Dir(filePath)
	for {
		sopsConfig := filepath.Join(dir, ".sops.yaml")
		data, err := os.ReadFile(sopsConfig)
		if err == nil {
			recipient, err := parseAgeRecipientFromConfig(data)
			if err == nil {
				return recipient, nil
			}
		}
		parent := filepath.Dir(dir)
		if parent == dir {
			break
		}
		dir = parent
	}
	return "", fmt.Errorf("could not find .sops.yaml with age recipient")
}

// parseAgeRecipientFromConfig extracts the age recipient from .sops.yaml content.
// We parse it manually to avoid importing the full sops config package.
func parseAgeRecipientFromConfig(data []byte) (string, error) {
	// Simple extraction: find "age:" field in creation_rules
	for _, line := range strings.Split(string(data), "\n") {
		trimmed := strings.TrimSpace(line)
		if strings.HasPrefix(trimmed, "age:") {
			val := strings.TrimSpace(strings.TrimPrefix(trimmed, "age:"))
			val = strings.Trim(val, "\"'")
			if val != "" {
				return val, nil
			}
		}
	}
	return "", fmt.Errorf("no age recipient found in .sops.yaml")
}
