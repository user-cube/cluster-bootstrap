package cmd

import (
	"fmt"
	"os/exec"
	"strings"

	"github.com/spf13/cobra"
)

type doctorResult struct {
	name string
	note string
	err  error
}

var (
	doctorEncryption       string
	doctorAgeKeyFile       string
	doctorKubeconfig       string
	doctorContext          string
	doctorSkipClusterCheck bool
)

var doctorCmd = &cobra.Command{
	Use:   "doctor",
	Short: "Check local and cluster prerequisites",
	Long: `Run prerequisite checks for cluster bootstrap.

This validates local tooling (kubectl, helm, encryption tools) and optionally
checks cluster access with the configured kubeconfig/context.`,
	RunE: runDoctor,
}

func init() {
	doctorCmd.Flags().StringVar(&doctorEncryption, "encryption", "sops", "encryption backend (sops|git-crypt)")
	doctorCmd.Flags().StringVar(&doctorAgeKeyFile, "age-key-file", "", "path to age private key file for SOPS decryption")
	doctorCmd.Flags().StringVar(&doctorKubeconfig, "kubeconfig", "", "path to kubeconfig file")
	doctorCmd.Flags().StringVar(&doctorContext, "context", "", "kubeconfig context to use")
	doctorCmd.Flags().BoolVar(&doctorSkipClusterCheck, "skip-cluster-check", false, "skip kubectl cluster access checks")

	rootCmd.AddCommand(doctorCmd)
}

func runDoctor(cmd *cobra.Command, args []string) error {
	logger := NewLogger(verbose)
	stage := logger.Stage("Doctor Checks")

	if doctorEncryption != "sops" && doctorEncryption != "git-crypt" {
		return fmt.Errorf("unsupported encryption backend: %s (use sops or git-crypt)", doctorEncryption)
	}

	results := make([]doctorResult, 0, 8)

	results = append(results, runDoctorCheck(stage, "kubectl available", func() (string, error) {
		return "", CheckKubectlAvailable(true)
	}))

	results = append(results, runDoctorCheck(stage, "kubectl current context", func() (string, error) {
		return getKubectlCurrentContext(doctorKubeconfig)
	}))

	if !doctorSkipClusterCheck {
		results = append(results, runDoctorCheck(stage, "kubectl cluster access", func() (string, error) {
			return "", CheckKubectlClusterAccessWithConfig(doctorKubeconfig, doctorContext)
		}))
	}

	results = append(results, runDoctorCheck(stage, "helm available", func() (string, error) {
		return "", CheckHelm()
	}))

	if doctorEncryption == "sops" {
		results = append(results, runDoctorCheck(stage, "sops available", func() (string, error) {
			return "", CheckSOPS("sops")
		}))
		results = append(results, runDoctorCheck(stage, "age available", func() (string, error) {
			return "", CheckAge("sops", doctorAgeKeyFile)
		}))
	}

	if doctorEncryption == "git-crypt" {
		results = append(results, runDoctorCheck(stage, "git-crypt available", func() (string, error) {
			return "", CheckGitCrypt("git-crypt")
		}))
	}

	stage.Done()

	fmt.Println()
	fmt.Println("Doctor report:")
	failures := 0
	for _, result := range results {
		status := "OK"
		if result.err != nil {
			status = "FAIL"
			failures++
		}
		fmt.Printf("  - %s: %s", status, result.name)
		if result.note != "" {
			fmt.Printf(" (%s)", result.note)
		}
		fmt.Println()
		if result.err != nil {
			printDoctorError(result.err)
		}
	}

	if failures > 0 {
		return fmt.Errorf("doctor found %d issue(s)", failures)
	}

	successf("All checks passed")
	return nil
}

func runDoctorCheck(stage *StageLogger, name string, fn func() (string, error)) doctorResult {
	note, err := fn()
	if err != nil {
		stage.Detail("FAIL: %s", name)
		return doctorResult{name: name, note: note, err: err}
	}
	stage.Detail("OK: %s", name)
	return doctorResult{name: name, note: note, err: nil}
}

func printDoctorError(err error) {
	lines := strings.Split(err.Error(), "\n")
	for _, line := range lines {
		line = strings.TrimSpace(line)
		if line == "" {
			continue
		}
		fmt.Printf("      %s\n", line)
	}
}

func getKubectlCurrentContext(kubeconfig string) (string, error) {
	path, err := exec.LookPath("kubectl")
	if err != nil {
		return "", fmt.Errorf("kubectl not found in PATH: %w", err)
	}

	args := make([]string, 0, 4)
	if kubeconfig != "" {
		args = append(args, "--kubeconfig", kubeconfig)
	}
	args = append(args, "config", "current-context")

	cmd := exec.Command(path, args...) // #nosec G204
	output, err := cmd.CombinedOutput()
	if err != nil {
		return "", fmt.Errorf("failed to read current context: %w\n  output: %s", err, string(output))
	}

	context := strings.TrimSpace(string(output))
	if context == "" {
		return "", fmt.Errorf("no current context set\n  hint: set a kubectl context or pass --context to bootstrap")
	}

	return context, nil
}
