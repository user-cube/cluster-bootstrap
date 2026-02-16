package cmd

import (
	"context"
	"fmt"
	"io"
	"os"
	"strings"

	"github.com/spf13/cobra"

	"github.com/user-cube/cluster-bootstrap/cluster-bootstrap/internal/k8s"

	"golang.org/x/term"
	corev1 "k8s.io/api/core/v1"
	apierrors "k8s.io/apimachinery/pkg/api/errors"
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
	vaultTokenCmd.Flags().StringVar(&vaultToken, "token", "", "Vault root token (optional; can be read from stdin or prompt)")
	vaultTokenCmd.Flags().StringVar(&kubeconfig, "kubeconfig", "", "path to kubeconfig file")
	vaultTokenCmd.Flags().StringVar(&kubeContext, "context", "", "kubeconfig context to use")

	rootCmd.AddCommand(vaultTokenCmd)
}

func runVaultToken(cmd *cobra.Command, args []string) error {
	token := strings.TrimSpace(vaultToken)
	if token == "" {
		var err error
		token, err = readVaultToken()
		if err != nil {
			return err
		}
	}
	if token == "" {
		return fmt.Errorf("vault token is required (use --token, pipe via stdin, or run interactively)")
	}

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
			"token": token,
		},
	}

	existing, err := client.Clientset.CoreV1().Secrets("vault").Get(ctx, "vault-root-token", metav1.GetOptions{})
	if err != nil {
		if !apierrors.IsNotFound(err) {
			return fmt.Errorf("failed to get vault-root-token secret: %w", err)
		}
		_, err = client.Clientset.CoreV1().Secrets("vault").Create(ctx, secret, metav1.CreateOptions{})
		if err != nil {
			return fmt.Errorf("failed to create vault-root-token secret: %w", err)
		}
		successf("Created secret vault/vault-root-token")
	} else {
		existing.StringData = secret.StringData
		_, err = client.Clientset.CoreV1().Secrets("vault").Update(ctx, existing, metav1.UpdateOptions{})
		if err != nil {
			return fmt.Errorf("failed to update vault-root-token secret: %w", err)
		}
		successf("Updated secret vault/vault-root-token")
	}

	return nil
}

func readVaultToken() (string, error) {
	stdinIsTerminal := term.IsTerminal(int(os.Stdin.Fd()))
	if !stdinIsTerminal {
		data, err := io.ReadAll(os.Stdin)
		if err != nil {
			return "", fmt.Errorf("failed to read vault token from stdin: %w", err)
		}
		token := strings.TrimSpace(string(data))
		if token != "" {
			return token, nil
		}
		return "", fmt.Errorf("vault token is required (use --token or pipe via stdin)")
	}

	fmt.Fprint(os.Stderr, "Vault root token: ")
	bytes, err := term.ReadPassword(int(os.Stdin.Fd()))
	fmt.Fprintln(os.Stderr)
	if err != nil {
		return "", fmt.Errorf("failed to read vault token from prompt: %w", err)
	}
	return strings.TrimSpace(string(bytes)), nil
}
