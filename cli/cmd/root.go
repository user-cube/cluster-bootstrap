package cmd

import (
	"fmt"
	"os"

	"github.com/spf13/cobra"
)

var (
	verbose bool
	baseDir string
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
		fmt.Fprintln(os.Stderr, err)
		os.Exit(1)
	}
}

func init() {
	rootCmd.PersistentFlags().BoolVarP(&verbose, "verbose", "v", false, "enable verbose output")
	rootCmd.PersistentFlags().StringVar(&baseDir, "base-dir", ".", "base directory for repo content")
}
