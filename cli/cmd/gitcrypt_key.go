package cmd

import (
	"context"
	"fmt"
	"os"

	"github.com/spf13/cobra"

	"github.com/user-cube/cluster-bootstrap/cluster-bootstrap/internal/k8s"
)

var gitCryptKeyFile string

var gitCryptKeyCmd = &cobra.Command{
	Use:   "gitcrypt-key",
	Short: "Store a git-crypt symmetric key as a Kubernetes secret",
	Long: `Reads a git-crypt symmetric key file and stores it as the
git-crypt-key secret in the argocd namespace. This allows ArgoCD
to decrypt git-crypt encrypted repositories.`,
	RunE: runGitCryptKey,
}

func init() {
	gitCryptKeyCmd.Flags().StringVar(&gitCryptKeyFile, "key-file", "", "path to git-crypt symmetric key file (required)")
	gitCryptKeyCmd.Flags().StringVar(&kubeconfig, "kubeconfig", "", "path to kubeconfig file")
	gitCryptKeyCmd.Flags().StringVar(&kubeContext, "context", "", "kubeconfig context to use")
	_ = gitCryptKeyCmd.MarkFlagRequired("key-file")

	rootCmd.AddCommand(gitCryptKeyCmd)
}

func runGitCryptKey(cmd *cobra.Command, args []string) error {
	keyData, err := os.ReadFile(gitCryptKeyFile)
	if err != nil {
		return fmt.Errorf("failed to read key file %s: %w", gitCryptKeyFile, err)
	}

	client, err := k8s.NewClient(kubeconfig, kubeContext)
	if err != nil {
		return fmt.Errorf("failed to create kubernetes client: %w", err)
	}

	ctx := context.Background()

	fmt.Println("==> Creating git-crypt-key secret in argocd namespace...")
	if err := client.CreateGitCryptKeySecret(ctx, keyData); err != nil {
		return err
	}

	fmt.Println("Created secret argocd/git-crypt-key")
	return nil
}
