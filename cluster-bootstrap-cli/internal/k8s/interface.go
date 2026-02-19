package k8s

import (
	"context"

	v1 "k8s.io/api/core/v1"
)

// ClientInterface defines the methods for interacting with Kubernetes.
type ClientInterface interface {
	EnsureNamespace(ctx context.Context, name string) (bool, error)
	CreateRepoSSHSecret(ctx context.Context, repoURL, sshPrivateKey string, dryRun bool) (*v1.Secret, bool, error)
	CreateGitCryptKeySecret(ctx context.Context, keyData []byte) (bool, error)
	ApplyAppOfApps(ctx context.Context, repoURL, targetRevision, env, appPath string, dryRun bool) (string, bool, error)
}
