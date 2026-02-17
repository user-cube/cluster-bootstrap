package cmd

import (
	"fmt"
	"os"
	"os/exec"
)

// CheckKubectlAvailable verifies that kubectl is installed and accessible.
// Returns nil if kubectl is available, but only logs a warning if not available
// (unless strict mode is enabled).
func CheckKubectlAvailable(strict bool) error {
	// Use kubectl --version which is simpler and more reliable
	cmd := exec.Command("kubectl", "version")
	err := cmd.Run()
	if err != nil {
		if strict {
			return fmt.Errorf("kubectl not found or not accessible: %w\n  hint: ensure kubectl is installed and in your PATH\n  tip: install from https://kubernetes.io/docs/tasks/tools/", err)
		}
		// In non-strict mode, just warn but don't fail
		return nil
	}
	return nil
}

// CheckKubectlClusterAccess verifies that kubectl can connect to the cluster.
// This checks if the current context is valid and accessible.
func CheckKubectlClusterAccess() error {
	return CheckKubectlClusterAccessWithConfig("", "")
}

// CheckKubectlClusterAccessWithConfig verifies that kubectl can connect to the cluster
// using the provided kubeconfig and context.
func CheckKubectlClusterAccessWithConfig(kubeconfig, kubeContext string) error {
	path, err := exec.LookPath("kubectl")
	if err != nil {
		return fmt.Errorf("kubectl not found in PATH: %w", err)
	}

	args := make([]string, 0, 6)
	if kubeconfig != "" {
		args = append(args, "--kubeconfig", kubeconfig)
	}
	if kubeContext != "" {
		args = append(args, "--context", kubeContext)
	}
	args = append(args, "cluster-info")

	cmd := exec.Command(path, args...) // #nosec G204
	output, err := cmd.CombinedOutput()
	if err != nil {
		return fmt.Errorf("cannot connect to cluster: %w\n  output: %s\n  hint: verify kubeconfig/context and cluster access", err, string(output))
	}
	return nil
}

// CheckHelm verifies that Helm is installed and accessible.
func CheckHelm() error {
	path, err := exec.LookPath("helm")
	if err != nil {
		return fmt.Errorf("helm not found in PATH: %w\n  hint: ensure helm is installed and in your PATH\n  tip: install from https://helm.sh/docs/intro/install/", err)
	}
	cmd := exec.Command(path, "version", "--short") // #nosec G204
	err = cmd.Run()
	if err != nil {
		return fmt.Errorf("helm found at %s but failed to run: %w\n  hint: verify helm is properly configured", path, err)
	}
	return nil
}

// CheckSOPS verifies that sops is installed and accessible (if using SOPS encryption).
func CheckSOPS(encryptionBackend string) error {
	if encryptionBackend != "sops" {
		return nil
	}

	path, err := exec.LookPath("sops")
	if err != nil {
		return fmt.Errorf("sops not found in PATH: %w\n  hint: ensure sops is installed and in your PATH\n  tip: install from https://github.com/mozilla/sops", err)
	}
	cmd := exec.Command(path, "--version") // #nosec G204
	err = cmd.Run()
	if err != nil {
		return fmt.Errorf("sops found at %s but failed to run: %w", path, err)
	}
	return nil
}

// CheckAge verifies that age is installed (if using age encryption).
func CheckAge(encryptionBackend string, ageKeyFile string) error {
	if encryptionBackend != "sops" || ageKeyFile == "" {
		return nil
	}

	// Check if age-keygen is available
	path, err := exec.LookPath("age-keygen")
	if err != nil {
		return fmt.Errorf("age not found in PATH: %w\n  hint: ensure age is installed and in your PATH\n  tip: install from https://github.com/FiloSottile/age", err)
	}

	cmd := exec.Command(path, "--version") // #nosec G204
	err = cmd.Run()
	if err != nil {
		return fmt.Errorf("age found at %s but failed to run: %w", path, err)
	}

	// Check if the age key file is readable
	if _, err := os.Stat(ageKeyFile); err != nil {
		return fmt.Errorf("age key file not accessible: %w\n  hint: verify the path exists and you have read permissions\n  path: %s", err, ageKeyFile)
	}

	return nil
}

// CheckGitCrypt verifies that git-crypt is installed (if using git-crypt encryption).
func CheckGitCrypt(encryptionBackend string) error {
	if encryptionBackend != "git-crypt" {
		return nil
	}

	path, err := exec.LookPath("git-crypt")
	if err != nil {
		return fmt.Errorf("git-crypt not found in PATH: %w\n  hint: ensure git-crypt is installed and in your PATH\n  tip: install from https://github.com/AGWA/git-crypt", err)
	}

	cmd := exec.Command(path, "--version") // #nosec G204
	err = cmd.Run()
	if err != nil {
		return fmt.Errorf("git-crypt found at %s but failed to run: %w", path, err)
	}
	return nil
}

// CheckFilePermissions verifies that critical files have proper permissions.
func CheckFilePermissions(filePath string, isSecret bool) error {
	info, err := os.Stat(filePath)
	if err != nil {
		return fmt.Errorf("file not found: %s: %w", filePath, err)
	}

	if isSecret {
		// For secret files, check that only the owner can read (600)
		mode := info.Mode()
		if mode&0o077 != 0 {
			return fmt.Errorf("file permissions too permissive for secret: %s\n  current: %o (should be 600)\n  hint: run: chmod 600 %s", filePath, mode.Perm(), filePath)
		}
	}

	return nil
}

// CheckSSHKey verifies that SSH key file is readable and has proper permissions.
func CheckSSHKey(keyPath string) error {
	if keyPath == "" {
		// SSH key is optional if SSH is not used
		return nil
	}

	if _, err := os.Stat(keyPath); err != nil {
		return fmt.Errorf("SSH key file not found: %s: %w\n  hint: verify the path and that you have read permissions", keyPath, err)
	}

	return CheckFilePermissions(keyPath, true)
}

// PreflightChecks performs all prerequisite checks before bootstrap.
// strict mode is enabled when --wait-for-health is true, requiring kubectl cluster access.
func PreflightChecks(encryption, ageKeyFile string, verbose bool, requireKubectl bool) error {
	logger := NewLogger(verbose)
	checksStage := logger.Stage("Prerequisite Checks")

	checks := []struct {
		name string
		fn   func() error
	}{
		{"kubectl available", func() error {
			return CheckKubectlAvailable(requireKubectl)
		}},
		{"kubectl cluster access", func() error {
			// Only check cluster access if required (e.g., when using --wait-for-health)
			if !requireKubectl {
				return nil
			}
			return CheckKubectlClusterAccess()
		}},
		{"helm available", CheckHelm},
		{"sops/age for encryption", func() error {
			if err := CheckSOPS(encryption); err != nil {
				return err
			}
			return CheckAge(encryption, ageKeyFile)
		}},
		{"git-crypt for encryption", func() error {
			return CheckGitCrypt(encryption)
		}},
	}

	for _, check := range checks {
		if err := check.fn(); err != nil {
			checksStage.Detail("✗ %s: %v", check.name, err)
			checksStage.Done()
			return err
		}
		checksStage.Detail("✓ %s", check.name)
	}

	checksStage.Done()
	logger.PrintStageSummary()
	return nil
}

// WrapNonZeroExitError wraps errors from external commands with context.
func WrapNonZeroExitError(command string, err error) error {
	if err == nil {
		return nil
	}

	var hint string
	switch command {
	case "kubectl":
		hint = "ensure kubectl is properly configured and the cluster is accessible"
	case "helm":
		hint = "ensure helm is properly configured"
	case "sops":
		hint = "ensure sops is installed and the encryption key is valid"
	default:
		hint = "check the command output above for more details"
	}

	return fmt.Errorf("%s command failed: %w\n  hint: %s", command, err, hint)
}
