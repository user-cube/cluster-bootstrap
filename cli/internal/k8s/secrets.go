package k8s

import (
	"context"
	"fmt"

	corev1 "k8s.io/api/core/v1"
	apierrors "k8s.io/apimachinery/pkg/api/errors"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
)

// EnsureNamespace creates a namespace if it does not already exist.
func (c *Client) EnsureNamespace(ctx context.Context, name string) error {
	_, err := c.Clientset.CoreV1().Namespaces().Get(ctx, name, metav1.GetOptions{})
	if err == nil {
		return nil
	}
	if !apierrors.IsNotFound(err) {
		if apierrors.IsForbidden(err) {
			return fmt.Errorf("permission denied: cannot get namespace %s: %w\n  hint: verify your cluster role has permission to get namespaces", name, err)
		}
		return fmt.Errorf("failed to get namespace %s: %w", name, err)
	}

	ns := &corev1.Namespace{
		ObjectMeta: metav1.ObjectMeta{
			Name: name,
		},
	}
	_, err = c.Clientset.CoreV1().Namespaces().Create(ctx, ns, metav1.CreateOptions{})
	if err != nil {
		if apierrors.IsForbidden(err) {
			return fmt.Errorf("permission denied: cannot create namespace %s: %w\n  hint: verify your cluster role has permission to create namespaces", name, err)
		}
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
		if !apierrors.IsNotFound(err) {
			if apierrors.IsForbidden(err) {
				return nil, fmt.Errorf("permission denied: cannot access secrets in argocd namespace: %w\n  hint: verify your cluster role has permission to get secrets", err)
			}
			return nil, fmt.Errorf("failed to get repo-ssh-key secret: %w", err)
		}
		_, err = c.Clientset.CoreV1().Secrets("argocd").Create(ctx, secret, metav1.CreateOptions{})
		if err != nil {
			if apierrors.IsForbidden(err) {
				return nil, fmt.Errorf("permission denied: cannot create secrets in argocd namespace: %w\n  hint: verify your cluster role has permission to create secrets", err)
			}
			return nil, fmt.Errorf("failed to create repo-ssh-key secret: %w", err)
		}
		return secret, nil
	}

	existing.Labels = secret.Labels
	existing.Annotations = secret.Annotations
	existing.StringData = secret.StringData
	_, err = c.Clientset.CoreV1().Secrets("argocd").Update(ctx, existing, metav1.UpdateOptions{})
	if err != nil {
		if apierrors.IsForbidden(err) {
			return nil, fmt.Errorf("permission denied: cannot update secrets in argocd namespace: %w\n  hint: verify your cluster role has permission to update secrets", err)
		}
		return nil, fmt.Errorf("failed to update repo-ssh-key secret: %w", err)
	}
	return secret, nil
}

// CreateGitCryptKeySecret creates or updates the git-crypt-key secret in the argocd namespace.
// The key data is the raw symmetric key used by git-crypt.
func (c *Client) CreateGitCryptKeySecret(ctx context.Context, keyData []byte) error {
	if err := c.EnsureNamespace(ctx, "argocd"); err != nil {
		return err
	}

	secret := &corev1.Secret{
		ObjectMeta: metav1.ObjectMeta{
			Name:      "git-crypt-key",
			Namespace: "argocd",
			Annotations: map[string]string{
				"cluster-bootstrap/origin":     "gitcrypt-key",
				"cluster-bootstrap/managed-by": "cluster-bootstrap",
			},
		},
		Type: corev1.SecretTypeOpaque,
		Data: map[string][]byte{
			"git-crypt-key": keyData,
		},
	}

	existing, err := c.Clientset.CoreV1().Secrets("argocd").Get(ctx, "git-crypt-key", metav1.GetOptions{})
	if err != nil {
		if !apierrors.IsNotFound(err) {
			return fmt.Errorf("failed to get git-crypt-key secret: %w", err)
		}
		_, err = c.Clientset.CoreV1().Secrets("argocd").Create(ctx, secret, metav1.CreateOptions{})
		if err != nil {
			return fmt.Errorf("failed to create git-crypt-key secret: %w", err)
		}
		return nil
	}

	existing.Annotations = secret.Annotations
	existing.Data = secret.Data
	_, err = c.Clientset.CoreV1().Secrets("argocd").Update(ctx, existing, metav1.UpdateOptions{})
	if err != nil {
		return fmt.Errorf("failed to update git-crypt-key secret: %w", err)
	}
	return nil
}
