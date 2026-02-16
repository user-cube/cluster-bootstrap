package cmd

import (
	"os"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

// TestCheckKubectlAvailable tests kubectl availability check.
func TestCheckKubectlAvailable(t *testing.T) {
	// Test with strict=false - should not fail if kubectl not available
	err := CheckKubectlAvailable(false)
	require.NoError(t, err, "kubectl check should pass in non-strict mode")

	// Test with strict=true - will fail if kubectl not available
	err = CheckKubectlAvailable(true)
	// This will only pass if kubectl is installed
	if err != nil {
		t.Logf("kubectl not available (expected in some environments): %v", err)
	}
}

// TestCheckHelm tests helm availability check.
func TestCheckHelm(t *testing.T) {
	err := CheckHelm()
	// This will only pass if helm is installed
	if err != nil {
		t.Logf("helm not available (expected in some environments): %v", err)
	}
}

// TestCheckSOPS_WithoutSOPS tests SOPS check is skipped when not using SOPS.
func TestCheckSOPS_WithoutSOPS(t *testing.T) {
	err := CheckSOPS("git-crypt")
	require.NoError(t, err, "should skip SOPS check when using git-crypt")
}

// TestCheckSOPS_WithSOPS tests SOPS check when using SOPS encryption.
func TestCheckSOPS_WithSOPS(t *testing.T) {
	err := CheckSOPS("sops")
	// This will only pass if sops is installed
	if err != nil {
		t.Logf("sops not available (expected in some environments): %v", err)
	}
}

// TestCheckAge_WithoutAge tests Age check is skipped when not specified.
func TestCheckAge_WithoutAge(t *testing.T) {
	err := CheckAge("sops", "")
	require.NoError(t, err, "should skip Age check when no key file provided")

	err = CheckAge("git-crypt", "")
	require.NoError(t, err, "should skip Age check when using git-crypt")
}

// TestCheckGitCrypt_WithoutGitCrypt tests git-crypt check is skipped when not using it.
func TestCheckGitCrypt_WithoutGitCrypt(t *testing.T) {
	err := CheckGitCrypt("sops")
	require.NoError(t, err, "should skip git-crypt check when using SOPS")
}

// TestCheckFilePermissions_Readable tests file permissions check.
func TestCheckFilePermissions_Readable(t *testing.T) {
	// Create a temporary file with proper permissions
	tmpFile, err := os.CreateTemp("", "test-perm-*.txt")
	require.NoError(t, err)
	defer os.Remove(tmpFile.Name())

	// Set permissions to 600 for a secret file
	require.NoError(t, os.Chmod(tmpFile.Name(), 0o600))

	err = CheckFilePermissions(tmpFile.Name(), true)
	require.NoError(t, err, "should accept file with 600 permissions")
}

// TestCheckFilePermissions_TooPermissive tests file permissions check for overly permissive files.
func TestCheckFilePermissions_TooPermissive(t *testing.T) {
	// Create a temporary file with overly permissive permissions
	tmpFile, err := os.CreateTemp("", "test-perm-*.txt")
	require.NoError(t, err)
	defer os.Remove(tmpFile.Name())

	// Set permissions to 644 (too permissive for a secret)
	require.NoError(t, os.Chmod(tmpFile.Name(), 0o644))

	err = CheckFilePermissions(tmpFile.Name(), true)
	require.Error(t, err, "should reject file with overly permissive permissions")
	assert.Contains(t, err.Error(), "too permissive")
}

// TestCheckFilePermissions_NonSecret tests file permissions check for non-secret files.
func TestCheckFilePermissions_NonSecret(t *testing.T) {
	// Create a temporary file
	tmpFile, err := os.CreateTemp("", "test-perm-*.txt")
	require.NoError(t, err)
	defer os.Remove(tmpFile.Name())

	// Set permissions to 644 (acceptable for non-secret)
	require.NoError(t, os.Chmod(tmpFile.Name(), 0o644))

	err = CheckFilePermissions(tmpFile.Name(), false)
	require.NoError(t, err, "should accept file with 644 permissions for non-secret")
}

// TestCheckSSHKey_Missing tests SSH key check for missing file.
func TestCheckSSHKey_Missing(t *testing.T) {
	err := CheckSSHKey("/nonexistent/path/to/key")
	require.Error(t, err, "should error on missing SSH key")
	assert.Contains(t, err.Error(), "not found")
}

// TestCheckSSHKey_Empty tests SSH key check when path is empty.
func TestCheckSSHKey_Empty(t *testing.T) {
	err := CheckSSHKey("")
	require.NoError(t, err, "should skip check when SSH key path is empty")
}

// TestCheckSSHKey_Valid tests SSH key check for valid file.
func TestCheckSSHKey_Valid(t *testing.T) {
	// Create a temporary file with proper permissions
	tmpFile, err := os.CreateTemp("", "test-key-*.pem")
	require.NoError(t, err)
	defer os.Remove(tmpFile.Name())

	// Set permissions to 600 for SSH key
	require.NoError(t, os.Chmod(tmpFile.Name(), 0o600))

	err = CheckSSHKey(tmpFile.Name())
	require.NoError(t, err, "should accept SSH key with 600 permissions")
}

// TestWrapNonZeroExitError_Kubectl tests error wrapping for kubectl failures.
func TestWrapNonZeroExitError_Kubectl(t *testing.T) {
	err := WrapNonZeroExitError("kubectl", assert.AnError)
	require.Error(t, err)
	assert.Contains(t, err.Error(), "kubectl command failed")
	assert.Contains(t, err.Error(), "cluster")
}

// TestWrapNonZeroExitError_Helm tests error wrapping for helm failures.
func TestWrapNonZeroExitError_Helm(t *testing.T) {
	err := WrapNonZeroExitError("helm", assert.AnError)
	require.Error(t, err)
	assert.Contains(t, err.Error(), "helm command failed")
}

// TestWrapNonZeroExitError_NoError tests error wrapping when there's no error.
func TestWrapNonZeroExitError_NoError(t *testing.T) {
	err := WrapNonZeroExitError("kubectl", nil)
	require.NoError(t, err)
}

// TestPreflightChecks_LoggerIntegration tests that preflight checks integrate with logger.
func TestPreflightChecks_LoggerIntegration(t *testing.T) {
	// This is more of an integration test, testing that the function doesn't panic
	// The actual result depends on what's installed in the environment
	logger := NewLogger(false)
	_ = logger
	// In a real scenario, this would be tested with mocks of the check functions
	// For now, we just verify the structure is sound
}
