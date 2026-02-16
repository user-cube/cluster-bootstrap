package config

import (
	"testing"

	"github.com/stretchr/testify/assert"
)

func TestSecretsFileName(t *testing.T) {
	tests := []struct {
		env      string
		expected string
	}{
		{"dev", "secrets.dev.enc.yaml"},
		{"prod", "secrets.prod.enc.yaml"},
		{"staging", "secrets.staging.enc.yaml"},
		{"us-east-1", "secrets.us-east-1.enc.yaml"},
	}

	for _, tt := range tests {
		t.Run(tt.env, func(t *testing.T) {
			assert.Equal(t, tt.expected, SecretsFileName(tt.env))
		})
	}
}
