package cmd

import (
	"context"
	"fmt"

	"github.com/spf13/cobra"

	"github.com/user-cube/cluster-bootstrap/cluster-bootstrap/internal/k8s"

	corev1 "k8s.io/api/core/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
)

var vaultToken string

var vaultTokenCmd = &cobra.Command{
	Use:   "vault-token",
	Short: "Store the Vault root token as a Kubernetes secret",
	Long: `Creates or updates the vault-root-token secret in the vault namespace.
This is required for non-dev Vault instances where the root token
is obtained from 'vault operator init'.`,
	RunE: runVaultToken,
}

func init() {
	vaultTokenCmd.Flags().StringVar(&vaultToken, "token", "", "Vault root token (required)")
	vaultTokenCmd.Flags().StringVar(&kubeconfig, "kubeconfig", "", "path to kubeconfig file")
	vaultTokenCmd.Flags().StringVar(&kubeContext, "context", "", "kubeconfig context to use")
	_ = vaultTokenCmd.MarkFlagRequired("token")

	rootCmd.AddCommand(vaultTokenCmd)
}

func runVaultToken(cmd *cobra.Command, args []string) error {
	client, err := k8s.NewClient(kubeconfig, kubeContext)
	if err != nil {
		return fmt.Errorf("failed to create kubernetes client: %w", err)
	}

	ctx := context.Background()

	if err := client.EnsureNamespace(ctx, "vault"); err != nil {
		return err
	}

	secret := &corev1.Secret{
		ObjectMeta: metav1.ObjectMeta{
			Name:      "vault-root-token",
			Namespace: "vault",
		},
		Type: corev1.SecretTypeOpaque,
		StringData: map[string]string{
			"token": vaultToken,
		},
	}

	existing, err := client.Clientset.CoreV1().Secrets("vault").Get(ctx, "vault-root-token", metav1.GetOptions{})
	if err != nil {
		_, err = client.Clientset.CoreV1().Secrets("vault").Create(ctx, secret, metav1.CreateOptions{})
		if err != nil {
			return fmt.Errorf("failed to create vault-root-token secret: %w", err)
		}
		fmt.Println("Created secret vault/vault-root-token")
	} else {
		existing.StringData = secret.StringData
		_, err = client.Clientset.CoreV1().Secrets("vault").Update(ctx, existing, metav1.UpdateOptions{})
		if err != nil {
			return fmt.Errorf("failed to update vault-root-token secret: %w", err)
		}
		fmt.Println("Updated secret vault/vault-root-token")
	}

	return nil
}
