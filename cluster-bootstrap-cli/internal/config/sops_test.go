package config

import (
	"os"
	"path/filepath"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestEnvPathRegex(t *testing.T) {
	tests := []struct {
		name     string
		envName  string
		expected string
	}{
		{"dev", "dev", `secrets\.dev\.enc\.yaml$`},
		{"prod", "prod", `secrets\.prod\.enc\.yaml$`},
		{"staging", "staging", `secrets\.staging\.enc\.yaml$`},
		{"us-east-1", "us-east-1", `secrets\.us-east-1\.enc\.yaml$`},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			assert.Equal(t, tt.expected, EnvPathRegex(tt.envName))
		})
	}
}

func TestReadSopsConfig(t *testing.T) {
	t.Run("valid file", func(t *testing.T) {
		cfg, err := ReadSopsConfig("../../testdata/valid.sops.yaml")
		require.NoError(t, err)
		require.Len(t, cfg.CreationRules, 1)
		assert.Equal(t, `secrets\.dev\.enc\.yaml$`, cfg.CreationRules[0].PathRegex)
		assert.Equal(t, "age1wj3m2ayk4a8nwxc8r678l06q4h4xxa0gqa2l6eyqf037wcdgxaqqla9fr8", cfg.CreationRules[0].Age)
	})

	t.Run("invalid yaml", func(t *testing.T) {
		_, err := ReadSopsConfig("../../testdata/invalid.sops.yaml")
		assert.Error(t, err)
		assert.Contains(t, err.Error(), "failed to parse")
	})

	t.Run("nonexistent file", func(t *testing.T) {
		_, err := ReadSopsConfig("../../testdata/does-not-exist.yaml")
		assert.Error(t, err)
		assert.Contains(t, err.Error(), "failed to read")
	})
}

func TestWriteSopsConfig(t *testing.T) {
	tests := []struct {
		name      string
		provider  string
		key       string
		wantErr   bool
		errSubstr string
		checkAge  string
		checkKMS  string
		checkGCP  string
	}{
		{
			name:     "age provider",
			provider: "age",
			key:      "age1testkey",
			checkAge: "age1testkey",
		},
		{
			name:     "aws-kms provider",
			provider: "aws-kms",
			key:      "arn:aws:kms:us-east-1:123456789:key/test",
			checkKMS: "arn:aws:kms:us-east-1:123456789:key/test",
		},
		{
			name:     "gcp-kms provider",
			provider: "gcp-kms",
			key:      "projects/my-project/locations/global/keyRings/test/cryptoKeys/key",
			checkGCP: "projects/my-project/locations/global/keyRings/test/cryptoKeys/key",
		},
		{
			name:      "unsupported provider",
			provider:  "invalid",
			key:       "somekey",
			wantErr:   true,
			errSubstr: "unsupported SOPS provider",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			tmpDir := t.TempDir()
			outPath := filepath.Join(tmpDir, ".sops.yaml")

			err := WriteSopsConfig(outPath, tt.provider, tt.key)
			if tt.wantErr {
				require.Error(t, err)
				assert.Contains(t, err.Error(), tt.errSubstr)
				return
			}

			require.NoError(t, err)

			cfg, err := ReadSopsConfig(outPath)
			require.NoError(t, err)
			require.Len(t, cfg.CreationRules, 1)

			rule := cfg.CreationRules[0]
			assert.Equal(t, `\.enc\.yaml$`, rule.PathRegex)
			assert.Equal(t, tt.checkAge, rule.Age)
			assert.Equal(t, tt.checkKMS, rule.KMS)
			assert.Equal(t, tt.checkGCP, rule.GCPKMS)
		})
	}
}

func TestUpsertSopsRule(t *testing.T) {
	t.Run("creates new file with rule", func(t *testing.T) {
		tmpDir := t.TempDir()
		outPath := filepath.Join(tmpDir, ".sops.yaml")

		err := UpsertSopsRule(outPath, "age", "age1newkey", "dev")
		require.NoError(t, err)

		cfg, err := ReadSopsConfig(outPath)
		require.NoError(t, err)
		require.Len(t, cfg.CreationRules, 1)
		assert.Equal(t, `secrets\.dev\.enc\.yaml$`, cfg.CreationRules[0].PathRegex)
		assert.Equal(t, "age1newkey", cfg.CreationRules[0].Age)
	})

	t.Run("updates existing rule", func(t *testing.T) {
		tmpDir := t.TempDir()
		outPath := filepath.Join(tmpDir, ".sops.yaml")

		// Create initial rule
		err := UpsertSopsRule(outPath, "age", "age1oldkey", "dev")
		require.NoError(t, err)

		// Update same environment
		err = UpsertSopsRule(outPath, "age", "age1newkey", "dev")
		require.NoError(t, err)

		cfg, err := ReadSopsConfig(outPath)
		require.NoError(t, err)
		require.Len(t, cfg.CreationRules, 1)
		assert.Equal(t, "age1newkey", cfg.CreationRules[0].Age)
	})

	t.Run("appends new environment rule", func(t *testing.T) {
		tmpDir := t.TempDir()
		outPath := filepath.Join(tmpDir, ".sops.yaml")

		err := UpsertSopsRule(outPath, "age", "age1devkey", "dev")
		require.NoError(t, err)

		err = UpsertSopsRule(outPath, "age", "age1prodkey", "prod")
		require.NoError(t, err)

		cfg, err := ReadSopsConfig(outPath)
		require.NoError(t, err)
		require.Len(t, cfg.CreationRules, 2)
		assert.Equal(t, "age1devkey", cfg.CreationRules[0].Age)
		assert.Equal(t, "age1prodkey", cfg.CreationRules[1].Age)
	})

	t.Run("unsupported provider returns error", func(t *testing.T) {
		tmpDir := t.TempDir()
		outPath := filepath.Join(tmpDir, ".sops.yaml")

		err := UpsertSopsRule(outPath, "invalid", "key", "dev")
		require.Error(t, err)
		assert.Contains(t, err.Error(), "unsupported SOPS provider")
	})

	t.Run("invalid existing file returns error", func(t *testing.T) {
		tmpDir := t.TempDir()
		outPath := filepath.Join(tmpDir, ".sops.yaml")
		_ = os.WriteFile(outPath, []byte("not: [valid yaml"), 0644)

		err := UpsertSopsRule(outPath, "age", "age1key", "dev")
		require.Error(t, err)
	})
}
