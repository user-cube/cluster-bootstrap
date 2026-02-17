package k8s

import (
	"context"
	"fmt"

	corev1 "k8s.io/api/core/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/apis/meta/v1/unstructured"
)

// MockClient is a mock Kubernetes client for testing. It allows simulating errors and state.
type MockClient struct {
	// Namespaces created in this mock.
	Namespaces map[string]bool
	// Secrets created in this mock.
	Secrets map[string]map[string]*corev1.Secret
	// Applications created in this mock.
	Applications map[string]*unstructured.Unstructured
	// Simulate errors for specific operations.
	EnsureNamespaceErr       error
	CreateRepoSSHSecretErr   error
	CreateGitCryptKeyErr     error
	ApplyAppOfAppsErr        error
	EnsureNamespaceForbidden bool
	CreateSecretForbidden    bool
}

// NewMockClient creates a new mock client for testing.
func NewMockClient() *MockClient {
	return &MockClient{
		Namespaces:   make(map[string]bool),
		Secrets:      make(map[string]map[string]*corev1.Secret),
		Applications: make(map[string]*unstructured.Unstructured),
	}
}

// EnsureNamespace simulates namespace creation with mock state.
func (m *MockClient) EnsureNamespace(ctx context.Context, name string) error {
	if m.EnsureNamespaceErr != nil {
		return m.EnsureNamespaceErr
	}
	if m.EnsureNamespaceForbidden {
		return fmt.Errorf("permission denied: cannot create namespace %s: Forbidden", name)
	}
	m.Namespaces[name] = true
	return nil
}

// CreateRepoSSHSecret simulates secret creation with mock state.
func (m *MockClient) CreateRepoSSHSecret(ctx context.Context, repoURL, sshPrivateKey string, dryRun bool) (*corev1.Secret, bool, error) {
	if m.CreateRepoSSHSecretErr != nil {
		return nil, false, m.CreateRepoSSHSecretErr
	}
	if m.CreateSecretForbidden {
		return nil, false, fmt.Errorf("permission denied: cannot create secrets in argocd namespace: Forbidden")
	}

	secret := &corev1.Secret{
		ObjectMeta: metav1.ObjectMeta{
			Name:      "repo-ssh-key",
			Namespace: "argocd",
		},
		Type: corev1.SecretTypeOpaque,
		StringData: map[string]string{
			"type":          "git",
			"url":           repoURL,
			"sshPrivateKey": sshPrivateKey,
		},
	}

	// Check if secret already exists to determine created vs updated
	created := true
	if m.Secrets["argocd"] != nil {
		if _, exists := m.Secrets["argocd"]["repo-ssh-key"]; exists {
			created = false
		}
	}

	if !dryRun {
		if m.Secrets["argocd"] == nil {
			m.Secrets["argocd"] = make(map[string]*corev1.Secret)
		}
		m.Secrets["argocd"]["repo-ssh-key"] = secret
	}

	return secret, created, nil
}

// CreateGitCryptKeySecret simulates git-crypt key secret creation.
func (m *MockClient) CreateGitCryptKeySecret(ctx context.Context, keyData []byte) (bool, error) {
	if m.CreateGitCryptKeyErr != nil {
		return false, m.CreateGitCryptKeyErr
	}
	if m.CreateSecretForbidden {
		return false, fmt.Errorf("permission denied: cannot create secrets in argocd namespace: Forbidden")
	}

	secret := &corev1.Secret{
		ObjectMeta: metav1.ObjectMeta{
			Name:      "git-crypt-key",
			Namespace: "argocd",
		},
		Type: corev1.SecretTypeOpaque,
		Data: map[string][]byte{
			"key": keyData,
		},
	}

	// Check if secret already exists to determine created vs updated
	created := true
	if m.Secrets["argocd"] != nil {
		if _, exists := m.Secrets["argocd"]["git-crypt-key"]; exists {
			created = false
		}
	}

	if m.Secrets["argocd"] == nil {
		m.Secrets["argocd"] = make(map[string]*corev1.Secret)
	}
	m.Secrets["argocd"]["git-crypt-key"] = secret
	return created, nil
}

// ApplyAppOfApps simulates Application CR creation.
func (m *MockClient) ApplyAppOfApps(ctx context.Context, repoURL, targetRevision, env, appPath string, dryRun bool) (string, bool, error) {
	if m.ApplyAppOfAppsErr != nil {
		return "", false, m.ApplyAppOfAppsErr
	}
	if m.CreateSecretForbidden {
		return "", false, fmt.Errorf("permission denied: cannot apply Application CRD: Forbidden")
	}

	app := &unstructured.Unstructured{
		Object: map[string]interface{}{
			"apiVersion": "argoproj.io/v1alpha1",
			"kind":       "Application",
			"metadata": map[string]interface{}{
				"name":      "app-of-apps",
				"namespace": "argocd",
			},
			"spec": map[string]interface{}{
				"source": map[string]interface{}{
					"repoURL":        repoURL,
					"targetRevision": targetRevision,
					"path":           appPath,
				},
			},
		},
	}

	// Check if application already exists to determine created vs updated
	created := true
	if _, exists := m.Applications["app-of-apps"]; exists {
		created = false
	}

	if !dryRun {
		m.Applications["app-of-apps"] = app
	}

	return "", created, nil
}

// GetSecret retrieves a stored secret from the mock (for testing verification).
func (m *MockClient) GetSecret(namespace, name string) *corev1.Secret {
	if m.Secrets[namespace] == nil {
		return nil
	}
	return m.Secrets[namespace][name]
}

// GetApplication retrieves a stored application from the mock (for testing verification).
func (m *MockClient) GetApplication(name string) *unstructured.Unstructured {
	return m.Applications[name]
}

// ClientInterface defines the interface that both Client and MockClient implement.
// This is useful for testing code that uses a K8s client.
type ClientInterface interface {
	EnsureNamespace(ctx context.Context, name string) error
	CreateRepoSSHSecret(ctx context.Context, repoURL, sshPrivateKey string, dryRun bool) (*corev1.Secret, bool, error)
	CreateGitCryptKeySecret(ctx context.Context, keyData []byte) (bool, error)
	ApplyAppOfApps(ctx context.Context, repoURL, targetRevision, env, appPath string, dryRun bool) (string, bool, error)
}
