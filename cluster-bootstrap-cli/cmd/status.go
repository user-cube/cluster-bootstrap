package cmd

import (
	"github.com/spf13/cobra"
)

var statusCmd = &cobra.Command{
	Use:   "status <environment>",
	Short: "Show cluster status and component information",
	Long: `Alias of the info command. Shows cluster status, component readiness,
versions, and optional health checks for the given environment.`,
	Args: cobra.ExactArgs(1),
	RunE: runInfo,
}

func init() {
	statusCmd.Flags().StringVar(&infoKubeconfig, "kubeconfig", "", "path to kubeconfig file")
	statusCmd.Flags().StringVar(&infoContext, "context", "", "kubeconfig context to use")
	statusCmd.Flags().BoolVar(&infoWaitHealth, "wait-for-health", false, "include health check results")
	statusCmd.Flags().IntVar(&infoHealthTimeout, "health-timeout", 180, "timeout in seconds for health checks")

	rootCmd.AddCommand(statusCmd)
}
