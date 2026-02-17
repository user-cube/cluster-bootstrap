package cmd

import (
	"fmt"

	"github.com/spf13/cobra"
)

// These variables are populated at build time via -ldflags.
// Defaults are for local dev builds (go run / go build).
var (
	Version = "dev"
	Commit  = "unknown"
	Date    = "unknown"
)

var versionCmd = &cobra.Command{
	Use:   "version",
	Short: "Print the cluster-bootstrap-cli version",
	Long:  "Print the cluster-bootstrap-cli version, git commit, and build date.",
	Run: func(cmd *cobra.Command, args []string) {
		isDevBuild := Version == "dev"

		fmt.Println()
		fmt.Println(stepColor("cluster-bootstrap-cli"))
		fmt.Println("────────────────────────────────────────")
		fmt.Printf("  Version: %s\n", Version)
		fmt.Printf("  Commit:  %s\n", Commit)
		fmt.Printf("  Date:    %s\n", Date)

		if isDevBuild {
			fmt.Println()
			fmt.Println("  (local dev build – not from a tagged release)")
		}

		fmt.Println()
	},
}

func init() {
	rootCmd.AddCommand(versionCmd)
}
