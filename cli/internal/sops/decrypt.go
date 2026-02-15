package sops

import (
	"fmt"
	"os"
	"os/exec"
)

// Options configures SOPS operations.
type Options struct {
	AgeKeyFile string
}

func buildEnv(opts *Options) []string {
	env := os.Environ()
	if opts != nil && opts.AgeKeyFile != "" {
		env = append(env, "SOPS_AGE_KEY_FILE="+opts.AgeKeyFile)
	}
	return env
}

// Decrypt runs `sops --decrypt` on the given file and returns the plaintext bytes.
func Decrypt(filePath string, opts *Options) ([]byte, error) {
	cmd := exec.Command("sops", "--decrypt", filePath)
	cmd.Env = buildEnv(opts)
	out, err := cmd.Output()
	if err != nil {
		if exitErr, ok := err.(*exec.ExitError); ok {
			return nil, fmt.Errorf("sops decrypt failed: %s", string(exitErr.Stderr))
		}
		return nil, fmt.Errorf("sops decrypt failed: %w", err)
	}
	return out, nil
}

// Encrypt runs `sops --encrypt` on the given file and returns the ciphertext bytes.
func Encrypt(filePath string, opts *Options) ([]byte, error) {
	cmd := exec.Command("sops", "--encrypt", filePath)
	cmd.Env = buildEnv(opts)
	out, err := cmd.Output()
	if err != nil {
		if exitErr, ok := err.(*exec.ExitError); ok {
			return nil, fmt.Errorf("sops encrypt failed: %s", string(exitErr.Stderr))
		}
		return nil, fmt.Errorf("sops encrypt failed: %w", err)
	}
	return out, nil
}
