package helm

import "context"

// ClientInterface defines the methods for Helm operations.
type ClientInterface interface {
	InstallArgoCD(ctx context.Context, kubeconfig, kubeContext, env, baseDir string, verbose bool) (bool, error)
}
