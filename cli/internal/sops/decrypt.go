package sops

import (
	"fmt"
	"os"
	"path/filepath"

	sopslib "github.com/getsops/sops/v3"
	"github.com/getsops/sops/v3/aes"
	"github.com/getsops/sops/v3/cmd/sops/common"
	sopsconfig "github.com/getsops/sops/v3/config"
	"github.com/getsops/sops/v3/decrypt"
	"github.com/getsops/sops/v3/keyservice"
	"github.com/getsops/sops/v3/version"
)

// Options configures SOPS operations.
type Options struct {
	AgeKeyFile string
}

type encryptionConfig struct {
	keyGroups               []sopslib.KeyGroup
	shamirThreshold         int
	unencryptedSuffix       string
	encryptedSuffix         string
	unencryptedRegex        string
	encryptedRegex          string
	unencryptedCommentRegex string
	encryptedCommentRegex   string
	macOnlyEncrypted        bool
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
	return EncryptWithTarget(filePath, filePath, opts)
}

// EncryptWithTarget encrypts a plaintext file using config rules for targetPath.
// This allows creation_rules to match the final output filename.
func EncryptWithTarget(filePath, targetPath string, opts *Options) ([]byte, error) {
	restore := setAgeEnv(opts)
	defer restore()

	// Read plaintext
	data, err := os.ReadFile(filePath) //nolint:gosec // safe: user-provided decryption input file
	if err != nil {
		return nil, fmt.Errorf("sops encrypt: failed to read file: %w", err)
	}

	encConfig, err := loadEncryptionConfig(targetPath)
	if err != nil {
		return nil, err
	}

	// Parse plaintext into sops tree branches
	store := common.DefaultStoreForPath(sopsconfig.NewStoresConfig(), targetPath)
	branches, err := store.LoadPlainFile(data)
	if err != nil {
		return nil, fmt.Errorf("sops encrypt: failed to parse plaintext: %w", err)
	}
	if len(branches) < 1 {
		return nil, fmt.Errorf("sops encrypt: file cannot be empty")
	}

	absPath, err := filepath.Abs(targetPath)
	if err != nil {
		return nil, fmt.Errorf("sops encrypt: failed to resolve path: %w", err)
	}

	tree := sopslib.Tree{
		Branches: branches,
		Metadata: sopslib.Metadata{
			KeyGroups:               encConfig.keyGroups,
			ShamirThreshold:         encConfig.shamirThreshold,
			UnencryptedSuffix:       encConfig.unencryptedSuffix,
			EncryptedSuffix:         encConfig.encryptedSuffix,
			UnencryptedRegex:        encConfig.unencryptedRegex,
			EncryptedRegex:          encConfig.encryptedRegex,
			UnencryptedCommentRegex: encConfig.unencryptedCommentRegex,
			EncryptedCommentRegex:   encConfig.encryptedCommentRegex,
			MACOnlyEncrypted:        encConfig.macOnlyEncrypted,
			Version:                 version.Version,
		},
		FilePath: absPath,
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

	// Emit encrypted output in the original format
	out, err := store.EmitEncryptedFile(tree)
	if err != nil {
		return nil, fmt.Errorf("sops encrypt: failed to emit encrypted file: %w", err)
	}

	return out, nil
}

func loadEncryptionConfig(filePath string) (*encryptionConfig, error) {
	configPath, err := findSopsConfigPath(filePath)
	if err != nil {
		return nil, err
	}

	absPath, err := filepath.Abs(filePath)
	if err != nil {
		return nil, fmt.Errorf("sops encrypt: failed to resolve path: %w", err)
	}

	conf, err := sopsconfig.LoadCreationRuleForFile(configPath, absPath, nil)
	if err != nil {
		return nil, fmt.Errorf("sops encrypt: failed to load config: %w", err)
	}
	if conf == nil {
		return nil, fmt.Errorf("sops encrypt: no matching creation rules found in %s", configPath)
	}

	unencryptedSuffix := conf.UnencryptedSuffix
	cryptRuleCount := 0
	if conf.UnencryptedSuffix != "" {
		cryptRuleCount++
	}
	if conf.EncryptedSuffix != "" {
		cryptRuleCount++
	}
	if conf.UnencryptedRegex != "" {
		cryptRuleCount++
	}
	if conf.EncryptedRegex != "" {
		cryptRuleCount++
	}
	if conf.UnencryptedCommentRegex != "" {
		cryptRuleCount++
	}
	if conf.EncryptedCommentRegex != "" {
		cryptRuleCount++
	}
	if cryptRuleCount == 0 {
		unencryptedSuffix = sopslib.DefaultUnencryptedSuffix
	}

	return &encryptionConfig{
		keyGroups:               conf.KeyGroups,
		shamirThreshold:         conf.ShamirThreshold,
		unencryptedSuffix:       unencryptedSuffix,
		encryptedSuffix:         conf.EncryptedSuffix,
		unencryptedRegex:        conf.UnencryptedRegex,
		encryptedRegex:          conf.EncryptedRegex,
		unencryptedCommentRegex: conf.UnencryptedCommentRegex,
		encryptedCommentRegex:   conf.EncryptedCommentRegex,
		macOnlyEncrypted:        conf.MACOnlyEncrypted,
	}, nil
}

func findSopsConfigPath(filePath string) (string, error) {
	result, err := sopsconfig.LookupConfigFile(filePath)
	if err == nil {
		return result.Path, nil
	}
	if result.Warning != "" {
		return "", fmt.Errorf("sops encrypt: %s", result.Warning)
	}
	return "", fmt.Errorf("sops encrypt: %w", err)
}
