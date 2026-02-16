package cmd

import (
	"fmt"
	"os"

	"github.com/fatih/color"
	"github.com/spf13/cobra"
)

var (
	verbose bool
	baseDir string
)

var (
	stepColor    = color.New(color.FgCyan, color.Bold).SprintFunc()
	successColor = color.New(color.FgGreen, color.Bold).SprintFunc()
	errorColor   = color.New(color.FgRed, color.Bold).SprintFunc()
)

var rootCmd = &cobra.Command{
	Use:   "cluster-bootstrap",
	Short: "CLI tool for bootstrapping Kubernetes clusters with ArgoCD",
	Long: `cluster-bootstrap is a CLI tool that replaces the manual bootstrap process.
It uses SOPS-encrypted secrets to configure ArgoCD, create Kubernetes secrets,
and deploy the App of Apps pattern.`,
}

func Execute() {
	if err := rootCmd.Execute(); err != nil {
		fmt.Fprintf(os.Stderr, "%s %v\n", errorColor("ERROR"), err)
		os.Exit(1)
	}
}

// wrapKubeconfigError enhances error messages for kubeconfig issues.
func wrapKubeconfigError(err error, kubeconfig, context string) error {
	errMsg := err.Error()
	if kubeconfig != "" && (len(errMsg) > 0 && errMsg != "") {
		return fmt.Errorf("failed to load kubeconfig %s: %w\n  hint: verify the file exists and is readable", kubeconfig, err)
	}
	if context != "" {
		return fmt.Errorf("context %s not found in kubeconfig: %w\n  hint: verify the context with: kubectl config get-contexts", context, err)
	}
	return fmt.Errorf("failed to load kubeconfig: %w\n  hint: ensure kubectl is configured. Check: kubectl config view", err)
}

// wrapClusterConnectionError enhances error messages for cluster connection issues.
func wrapClusterConnectionError(err error) error {
	return fmt.Errorf("failed to connect to cluster: %w\n  hint: verify cluster is accessible and kubeconfig credentials are valid\n  tip: try: kubectl cluster-info", err)
}

// wrapPermissionError enhances error messages for permission issues.
func wrapPermissionError(operation, resource string, err error) error {
	return fmt.Errorf("%s failed for %s: permission denied or resource conflict\n  error: %w\n  hint: verify your cluster role has the required permissions", operation, resource, err)
}

func stepf(format string, args ...interface{}) {
	fmt.Printf("%s %s\n", stepColor("==>"), fmt.Sprintf(format, args...))
}

func successf(format string, args ...interface{}) {
	fmt.Printf("%s %s\n", successColor("==>"), fmt.Sprintf(format, args...))
}

func init() {
	rootCmd.PersistentFlags().BoolVarP(&verbose, "verbose", "v", false, "enable verbose output")
	rootCmd.PersistentFlags().StringVar(&baseDir, "base-dir", ".", "base directory for repo content")
}
