package helm

import (
	"context"
	"fmt"
	"time"
)

// MockHelmAction is a mock for testing Helm install/upgrade scenarios.
type MockHelmAction struct {
	// Simulate errors for testing
	SimulateTimeout       bool
	SimulatePermissionErr bool
	SimulateImagePullErr  bool
	SimulateConflict      bool

	// Track call count for retry logic
	CallCount int
}

// NewMockHelmAction creates a new mock Helm action for testing.
func NewMockHelmAction() *MockHelmAction {
	return &MockHelmAction{
		CallCount: 0,
	}
}

// SimulateInstall simulates a Helm install with configurable failure modes.
func (m *MockHelmAction) SimulateInstall(ctx context.Context) error {
	m.CallCount++

	if m.SimulateTimeout {
		select {
		case <-ctx.Done():
			return fmt.Errorf("helm install timed out: context deadline exceeded")
		case <-time.After(500 * time.Millisecond):
			break
		}
		return fmt.Errorf("helm install: Helm install timed out waiting for pods to be ready")
	}

	if m.SimulatePermissionErr {
		return fmt.Errorf("error: create pods is forbidden: User \"system:serviceaccount:default:default\" cannot create resource \"pods\" in API group \"\" in the namespace \"argocd\"")
	}

	if m.SimulateImagePullErr {
		return fmt.Errorf("helm install: release argocd failed: pod argocd-server-0 failed due to ImagePullBackOff")
	}

	if m.SimulateConflict {
		return fmt.Errorf("helm install: release argocd already exists. Use --force to force")
	}

	return nil
}

// SimulateUpgrade simulates a Helm upgrade with configurable failure modes.
func (m *MockHelmAction) SimulateUpgrade(ctx context.Context) error {
	m.CallCount++

	if m.SimulateTimeout {
		select {
		case <-ctx.Done():
			return fmt.Errorf("helm upgrade timed out: context deadline exceeded")
		case <-time.After(500 * time.Millisecond):
			break
		}
		return fmt.Errorf("helm upgrade: upgrade timed out waiting for pods to be ready")
	}

	if m.SimulatePermissionErr {
		return fmt.Errorf("error: patch deployment is forbidden: cannot patch resource")
	}

	if m.SimulateImagePullErr {
		return fmt.Errorf("helm upgrade: pod failed due to ImagePullBackOff")
	}

	return nil
}

// GetCallCount returns the number of times the mock was called (useful for retry testing).
func (m *MockHelmAction) GetCallCount() int {
	return m.CallCount
}

// AnalyzeError returns a human-readable hint based on the error type.
func AnalyzeError(err error) string {
	if err == nil {
		return ""
	}

	errMsg := err.Error()

	if contains(errMsg, "timed out") || contains(errMsg, "deadline exceeded") {
		return "Helm operation timed out. Check cluster resources and pod status: kubectl get pods -n argocd -w"
	} else if contains(errMsg, "forbidden") || contains(errMsg, "Forbidden") {
		return "Permission denied. Verify your cluster role has permission to create/modify resources in the argocd namespace"
	} else if contains(errMsg, "ImagePull") {
		return "Image pull failed. Verify container images are accessible and image pull secrets are configured"
	} else if contains(errMsg, "already exists") {
		return "Release already exists. Use --force to reinstall or skip ArgoCD install with --skip-argocd-install"
	} else if contains(errMsg, "repo index") {
		return "Helm repository index download failed. Verify the repository is accessible: helm repo update"
	}

	return "Verify all prerequisites are met and cluster is accessible"
}

// contains is a simple helper to check if a string contains a substring.
func contains(str, substr string) bool {
	for i := 0; i+len(substr) <= len(str); i++ {
		if str[i:i+len(substr)] == substr {
			return true
		}
	}
	return false
}
