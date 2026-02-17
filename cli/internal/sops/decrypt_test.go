package sops

import (
	"os"
	"path/filepath"
	"testing"

	sopslib "github.com/getsops/sops/v3"
	"github.com/getsops/sops/v3/gcpkms"
	"github.com/getsops/sops/v3/kms"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestSetAgeEnv(t *testing.T) {
	t.Run("nil options does nothing", func(t *testing.T) {
		orig, hadOrig := os.LookupEnv("SOPS_AGE_KEY_FILE")
		restore := setAgeEnv(nil)
		after, hadAfter := os.LookupEnv("SOPS_AGE_KEY_FILE")
		restore()

		assert.Equal(t, hadOrig, hadAfter)
		if hadOrig {
			assert.Equal(t, orig, after)
		}
	})

	t.Run("empty key file does nothing", func(t *testing.T) {
		orig, hadOrig := os.LookupEnv("SOPS_AGE_KEY_FILE")
		restore := setAgeEnv(&Options{AgeKeyFile: ""})
		after, hadAfter := os.LookupEnv("SOPS_AGE_KEY_FILE")
		restore()

		assert.Equal(t, hadOrig, hadAfter)
		if hadOrig {
			assert.Equal(t, orig, after)
		}
	})

	t.Run("sets and restores env var", func(t *testing.T) {
		// Clear the env var first
		_ = os.Unsetenv("SOPS_AGE_KEY_FILE")

		restore := setAgeEnv(&Options{AgeKeyFile: "/tmp/test-key.txt"})

		val, ok := os.LookupEnv("SOPS_AGE_KEY_FILE")
		assert.True(t, ok)
		assert.Equal(t, "/tmp/test-key.txt", val)

		restore()

		_, ok = os.LookupEnv("SOPS_AGE_KEY_FILE")
		assert.False(t, ok)
	})

	t.Run("restores previous value", func(t *testing.T) {
		_ = os.Setenv("SOPS_AGE_KEY_FILE", "original-value")
		defer func() { _ = os.Unsetenv("SOPS_AGE_KEY_FILE") }()

		restore := setAgeEnv(&Options{AgeKeyFile: "/tmp/new-key.txt"})

		val, _ := os.LookupEnv("SOPS_AGE_KEY_FILE")
		assert.Equal(t, "/tmp/new-key.txt", val)

		restore()

		val, ok := os.LookupEnv("SOPS_AGE_KEY_FILE")
		assert.True(t, ok)
		assert.Equal(t, "original-value", val)
	})
}

func TestLoadEncryptionConfig_KMSAndGCP(t *testing.T) {
	tmpDir := t.TempDir()
	configPath := filepath.Join(tmpDir, ".sops.yaml")
	config := `creation_rules:
  - path_regex: secrets\.dev\.enc\.yaml$
    kms: arn:aws:kms:us-east-1:123456789:key/test
  - path_regex: secrets\.gcp\.enc\.yaml$
    gcp_kms: projects/p/locations/l/keyRings/r/cryptoKeys/k
  - path_regex: secrets\.age\.enc\.yaml$
    age: age1wj3m2ayk4a8nwxc8r678l06q4h4xxa0gqa2l6eyqf037wcdgxaqqla9fr8
`
	require.NoError(t, os.WriteFile(configPath, []byte(config), 0644))

	devFile := filepath.Join(tmpDir, "secrets.dev.enc.yaml")
	gcpFile := filepath.Join(tmpDir, "secrets.gcp.enc.yaml")
	ageFile := filepath.Join(tmpDir, "secrets.age.enc.yaml")

	devConfig, err := loadEncryptionConfig(devFile)
	require.NoError(t, err)
	require.Len(t, devConfig.keyGroups, 1)
	require.NotEmpty(t, devConfig.keyGroups[0])
	_, isKMS := devConfig.keyGroups[0][0].(*kms.MasterKey)
	assert.True(t, isKMS)

	gcpConfig, err := loadEncryptionConfig(gcpFile)
	require.NoError(t, err)
	require.Len(t, gcpConfig.keyGroups, 1)
	require.NotEmpty(t, gcpConfig.keyGroups[0])
	_, isGCP := gcpConfig.keyGroups[0][0].(*gcpkms.MasterKey)
	assert.True(t, isGCP)

	ageConfig, err := loadEncryptionConfig(ageFile)
	require.NoError(t, err)
	assert.Equal(t, sopslib.DefaultUnencryptedSuffix, ageConfig.unencryptedSuffix)
}
