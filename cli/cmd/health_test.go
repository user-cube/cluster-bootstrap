package cmd

import (
	"context"
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	appsv1 "k8s.io/api/apps/v1"
	corev1 "k8s.io/api/core/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/client-go/kubernetes/fake"
)

func TestWaitForHealth_ArgoCDReady(t *testing.T) {
	clientset := fake.NewSimpleClientset(
		// Create argocd namespace
		&corev1.Namespace{
			ObjectMeta: metav1.ObjectMeta{Name: "argocd"},
		},
		// Create argocd-server deployment
		&appsv1.Deployment{
			ObjectMeta: metav1.ObjectMeta{
				Name:      "argocd-server",
				Namespace: "argocd",
			},
			Spec: appsv1.DeploymentSpec{
				Replicas: int32Ptr(1),
			},
			Status: appsv1.DeploymentStatus{
				Replicas:        1,
				UpdatedReplicas: 1,
				ReadyReplicas:   1,
			},
		},
	)

	ctx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
	defer cancel()

	result := checkArgoCDReady(ctx, clientset)
	assert.Equal(t, "ArgoCD", result.Component)
	assert.Equal(t, "Ready", result.Status)
	assert.Contains(t, result.Message, "running")
}

func TestWaitForHealth_ArgoCDTimeout(t *testing.T) {
	clientset := fake.NewSimpleClientset(
		// Create argocd namespace
		&corev1.Namespace{
			ObjectMeta: metav1.ObjectMeta{Name: "argocd"},
		},
		// Create argocd-server deployment with NO ready replicas
		&appsv1.Deployment{
			ObjectMeta: metav1.ObjectMeta{
				Name:      "argocd-server",
				Namespace: "argocd",
			},
			Spec: appsv1.DeploymentSpec{
				Replicas: int32Ptr(1),
			},
			Status: appsv1.DeploymentStatus{
				Replicas:        1,
				UpdatedReplicas: 0,
				ReadyReplicas:   0,
			},
		},
	)

	ctx, cancel := context.WithTimeout(context.Background(), 100*time.Millisecond)
	defer cancel()

	result := checkArgoCDReady(ctx, clientset)
	assert.Equal(t, "ArgoCD", result.Component)
	assert.Equal(t, "Timeout", result.Status)
	assert.Contains(t, result.Message, "not ready")
}

func TestWaitForHealth_VaultReady(t *testing.T) {
	clientset := fake.NewSimpleClientset(
		// Create vault namespace
		&corev1.Namespace{
			ObjectMeta: metav1.ObjectMeta{Name: "vault"},
		},
		// Create vault statefulset
		&appsv1.StatefulSet{
			ObjectMeta: metav1.ObjectMeta{
				Name:      "vault",
				Namespace: "vault",
			},
			Spec: appsv1.StatefulSetSpec{
				Replicas: int32Ptr(1),
			},
			Status: appsv1.StatefulSetStatus{
				Replicas:        1,
				UpdatedReplicas: 1,
				ReadyReplicas:   1,
			},
		},
	)

	ctx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
	defer cancel()

	result := checkVaultReady(ctx, clientset)
	assert.Equal(t, "Vault", result.Component)
	assert.Equal(t, "Ready", result.Status)
	assert.Contains(t, result.Message, "running")
}

func TestWaitForHealth_VaultNotInstalled(t *testing.T) {
	clientset := fake.NewSimpleClientset()

	ctx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
	defer cancel()

	result := checkVaultReady(ctx, clientset)
	assert.Equal(t, "Vault", result.Component)
	assert.Equal(t, "NotInstalled", result.Status)
	assert.Contains(t, result.Message, "not found")
}

func TestWaitForHealth_ExternalSecretsReady(t *testing.T) {
	clientset := fake.NewSimpleClientset(
		// Create external-secrets namespace
		&corev1.Namespace{
			ObjectMeta: metav1.ObjectMeta{Name: "external-secrets"},
		},
		// Create external-secrets deployment
		&appsv1.Deployment{
			ObjectMeta: metav1.ObjectMeta{
				Name:      "external-secrets",
				Namespace: "external-secrets",
			},
			Spec: appsv1.DeploymentSpec{
				Replicas: int32Ptr(1),
			},
			Status: appsv1.DeploymentStatus{
				Replicas:        1,
				UpdatedReplicas: 1,
				ReadyReplicas:   1,
			},
		},
	)

	ctx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
	defer cancel()

	result := checkExternalSecretsReady(ctx, clientset)
	assert.Equal(t, "External Secrets", result.Component)
	assert.Equal(t, "Ready", result.Status)
	assert.Contains(t, result.Message, "running")
}

func TestWaitForHealth_ExternalSecretsNotInstalled(t *testing.T) {
	clientset := fake.NewSimpleClientset()

	ctx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
	defer cancel()

	result := checkExternalSecretsReady(ctx, clientset)
	assert.Equal(t, "External Secrets", result.Component)
	assert.Equal(t, "NotInstalled", result.Status)
	assert.Contains(t, result.Message, "not found")
}

func TestHealthCheckResult_Duration(t *testing.T) {
	result := HealthCheckResult{
		Component: "TestComponent",
		Status:    "Ready",
	}

	// Simulate some time passing
	time.Sleep(10 * time.Millisecond)
	result.Duration = 10 * time.Millisecond

	assert.Equal(t, "TestComponent", result.Component)
	assert.Equal(t, "Ready", result.Status)
	assert.GreaterOrEqual(t, result.Duration, 10*time.Millisecond)
}

func TestHealthStatus_OverallHealth(t *testing.T) {
	status := &HealthStatus{
		Environment: "dev",
		Results: []HealthCheckResult{
			{Component: "ArgoCD", Status: "Ready"},
			{Component: "Vault", Status: "NotInstalled"},
			{Component: "External Secrets", Status: "Ready"},
		},
	}

	// Set healthy based on results
	status.Healthy = true
	for _, r := range status.Results {
		if r.Status == "Error" || r.Status == "Timeout" {
			status.Healthy = false
			break
		}
	}

	assert.True(t, status.Healthy)
	assert.Equal(t, "dev", status.Environment)
	assert.Len(t, status.Results, 3)
}

func TestHealthStatus_UnhealthyStatus(t *testing.T) {
	status := &HealthStatus{
		Environment: "prod",
		Results: []HealthCheckResult{
			{Component: "ArgoCD", Status: "Ready"},
			{Component: "Vault", Status: "Timeout"},
		},
	}

	// Set healthy based on results
	status.Healthy = true
	for _, r := range status.Results {
		if r.Status == "Error" || r.Status == "Timeout" {
			status.Healthy = false
			break
		}
	}

	assert.False(t, status.Healthy)
}

// Helper function to create int32 pointers for test
func int32Ptr(i int32) *int32 {
	return &i
}
