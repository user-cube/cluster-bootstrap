package sops

import (
	"os"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestParseAgeRecipientFromConfig(t *testing.T) {
	tests := []struct {
		name      string
		input     string
		want      string
		wantErr   bool
		errSubstr string
	}{
		{
			name: "standard format",
			input: `creation_rules:
  - path_regex: secrets\.dev\.enc\.yaml$
    age: age1wj3m2ayk4a8nwxc8r678l06q4h4xxa0gqa2l6eyqf037wcdgxaqqla9fr8`,
			want: "age1wj3m2ayk4a8nwxc8r678l06q4h4xxa0gqa2l6eyqf037wcdgxaqqla9fr8",
		},
		{
			name: "with double quotes",
			input: `creation_rules:
  - path_regex: secrets\.dev\.enc\.yaml$
    age: "age1quoted000000000000000000000000000000000000000000000000000000"`,
			want: "age1quoted000000000000000000000000000000000000000000000000000000",
		},
		{
			name: "with single quotes",
			input: `creation_rules:
  - path_regex: secrets\.dev\.enc\.yaml$
    age: 'age1single000000000000000000000000000000000000000000000000000000'`,
			want: "age1single000000000000000000000000000000000000000000000000000000",
		},
		{
			name:      "no age key",
			input:     `creation_rules:\n  - kms: arn:aws:kms:us-east-1:123456789:key/test`,
			wantErr:   true,
			errSubstr: "no age recipient found",
		},
		{
			name:      "empty config",
			input:     ``,
			wantErr:   true,
			errSubstr: "no age recipient found",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got, err := parseAgeRecipientFromConfig([]byte(tt.input))
			if tt.wantErr {
				require.Error(t, err)
				assert.Contains(t, err.Error(), tt.errSubstr)
				return
			}
			require.NoError(t, err)
			assert.Equal(t, tt.want, got)
		})
	}
}

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
