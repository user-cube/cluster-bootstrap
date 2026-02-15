package k8s

import (
	"context"
	"fmt"

	corev1 "k8s.io/api/core/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
)

// EnsureNamespace creates a namespace if it does not already exist.
func (c *Client) EnsureNamespace(ctx context.Context, name string) error {
	_, err := c.Clientset.CoreV1().Namespaces().Get(ctx, name, metav1.GetOptions{})
	if err == nil {
		return nil
	}

	ns := &corev1.Namespace{
		ObjectMeta: metav1.ObjectMeta{
			Name: name,
		},
	}
	_, err = c.Clientset.CoreV1().Namespaces().Create(ctx, ns, metav1.CreateOptions{})
	if err != nil {
		return fmt.Errorf("failed to create namespace %s: %w", name, err)
	}
	return nil
}

// CreateRepoSSHSecret creates or updates the repo-ssh-key secret in the argocd namespace.
// This matches the exact labels/annotations from the original install.sh.
func (c *Client) CreateRepoSSHSecret(ctx context.Context, repoURL, sshPrivateKey string, dryRun bool) (*corev1.Secret, error) {
	secret := &corev1.Secret{
		ObjectMeta: metav1.ObjectMeta{
			Name:      "repo-ssh-key",
			Namespace: "argocd",
			Labels: map[string]string{
				"argocd.argoproj.io/secret-type": "repo-creds",
			},
			Annotations: map[string]string{
				"managed-by":                   "argocd.argoproj.io",
				"cluster-bootstrap/origin":     "bootstrap",
				"cluster-bootstrap/managed-by": "external-secrets",
			},
		},
		Type: corev1.SecretTypeOpaque,
		StringData: map[string]string{
			"type":          "git",
			"url":           repoURL,
			"sshPrivateKey": sshPrivateKey,
		},
	}

	if dryRun {
		return secret, nil
	}

	// Try to update, create if not exists
	existing, err := c.Clientset.CoreV1().Secrets("argocd").Get(ctx, "repo-ssh-key", metav1.GetOptions{})
	if err != nil {
		_, err = c.Clientset.CoreV1().Secrets("argocd").Create(ctx, secret, metav1.CreateOptions{})
		if err != nil {
			return nil, fmt.Errorf("failed to create repo-ssh-key secret: %w", err)
		}
		return secret, nil
	}

	existing.Labels = secret.Labels
	existing.Annotations = secret.Annotations
	existing.StringData = secret.StringData
	_, err = c.Clientset.CoreV1().Secrets("argocd").Update(ctx, existing, metav1.UpdateOptions{})
	if err != nil {
		return nil, fmt.Errorf("failed to update repo-ssh-key secret: %w", err)
	}
	return secret, nil
}

// CreateVaultTokenSecret creates or updates the vault-token secret in the vault namespace.
func (c *Client) CreateVaultTokenSecret(ctx context.Context, address, token string, dryRun bool) (*corev1.Secret, error) {
	secret := &corev1.Secret{
		ObjectMeta: metav1.ObjectMeta{
			Name:      "vault-token",
			Namespace: "vault",
			Annotations: map[string]string{
				"cluster-bootstrap/origin":     "bootstrap",
				"cluster-bootstrap/managed-by": "external-secrets",
			},
		},
		Type: corev1.SecretTypeOpaque,
		StringData: map[string]string{
			"address": address,
			"token":   token,
		},
	}

	if dryRun {
		return secret, nil
	}

	if err := c.EnsureNamespace(ctx, "vault"); err != nil {
		return nil, err
	}

	existing, err := c.Clientset.CoreV1().Secrets("vault").Get(ctx, "vault-token", metav1.GetOptions{})
	if err != nil {
		_, err = c.Clientset.CoreV1().Secrets("vault").Create(ctx, secret, metav1.CreateOptions{})
		if err != nil {
			return nil, fmt.Errorf("failed to create vault-token secret: %w", err)
		}
		return secret, nil
	}

	existing.Annotations = secret.Annotations
	existing.StringData = secret.StringData
	_, err = c.Clientset.CoreV1().Secrets("vault").Update(ctx, existing, metav1.UpdateOptions{})
	if err != nil {
		return nil, fmt.Errorf("failed to update vault-token secret: %w", err)
	}
	return secret, nil
}
