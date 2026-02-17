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
	warningColor = color.New(color.FgYellow, color.Bold).SprintFunc()
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

func stepf(format string, args ...interface{}) {
	fmt.Printf("%s %s\n", stepColor("==>"), fmt.Sprintf(format, args...))
}

func successf(format string, args ...interface{}) {
	fmt.Printf("%s %s\n", successColor("==>"), fmt.Sprintf(format, args...))
}

func warnf(format string, args ...interface{}) {
	fmt.Printf("%s %s\n", warningColor("⚠ "), fmt.Sprintf(format, args...))
}

func errorf(format string, args ...interface{}) {
	fmt.Printf("%s %s\n", errorColor("✗"), fmt.Sprintf(format, args...))
}

func init() {
	rootCmd.PersistentFlags().BoolVarP(&verbose, "verbose", "v", false, "enable verbose output")
	rootCmd.PersistentFlags().StringVar(&baseDir, "base-dir", ".", "base directory for repo content")
}
