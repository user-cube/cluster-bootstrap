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

func TestExtractVersionFromImage(t *testing.T) {
	testCases := []struct {
		image    string
		expected string
	}{
		{"ghcr.io/argoproj/argocd:v2.8.0", "v2.8.0"},
		{"quay.io/vault/vault:1.15.4", "1.15.4"},
		{"myregistry.com/image:latest", "latest"},
		{"image-without-version", "unknown"},
		{"quay.io/vault/vault", "unknown"},
	}

	for _, tc := range testCases {
		result := extractVersionFromImage(tc.image)
		assert.Equal(t, tc.expected, result, "image: %s", tc.image)
	}
}

func TestCheckArgoCDInfo_NotInstalled(t *testing.T) {
	//nolint:staticcheck
	clientset := fake.NewSimpleClientset()

	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()

	info := checkArgoCDInfo(ctx, clientset)
	assert.Equal(t, "ArgoCD", info.Name)
	assert.Equal(t, "argocd", info.Namespace)
	assert.Equal(t, "NotInstalled", info.Status)
	assert.False(t, info.Installed)
}

func TestCheckArgoCDInfo_Ready(t *testing.T) {
	//nolint:staticcheck
	clientset := fake.NewSimpleClientset(
		&corev1.Namespace{
			ObjectMeta: metav1.ObjectMeta{Name: "argocd"},
		},
		&appsv1.Deployment{
			ObjectMeta: metav1.ObjectMeta{
				Name:      "argocd-server",
				Namespace: "argocd",
			},
			Spec: appsv1.DeploymentSpec{
				Replicas: int32Ptr(1),
				Template: corev1.PodTemplateSpec{
					Spec: corev1.PodSpec{
						Containers: []corev1.Container{
							{
								Name:  "argocd-server",
								Image: "ghcr.io/argoproj/argocd:v2.8.0",
							},
						},
					},
				},
			},
			Status: appsv1.DeploymentStatus{
				Replicas:        1,
				UpdatedReplicas: 1,
				ReadyReplicas:   1,
			},
		},
	)

	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()

	info := checkArgoCDInfo(ctx, clientset)
	assert.Equal(t, "ArgoCD", info.Name)
	assert.Equal(t, "Ready", info.Status)
	assert.True(t, info.Installed)
	assert.Equal(t, 1, info.ReadyReplicas)
	assert.Equal(t, 1, info.DesiredReplicas)
	assert.Equal(t, "v2.8.0", info.Version)
}

func TestCheckArgoCDInfo_Progressing(t *testing.T) {
	//nolint:staticcheck
	clientset := fake.NewSimpleClientset(
		&corev1.Namespace{
			ObjectMeta: metav1.ObjectMeta{Name: "argocd"},
		},
		&appsv1.Deployment{
			ObjectMeta: metav1.ObjectMeta{
				Name:      "argocd-server",
				Namespace: "argocd",
			},
			Spec: appsv1.DeploymentSpec{
				Replicas: int32Ptr(2),
				Template: corev1.PodTemplateSpec{
					Spec: corev1.PodSpec{
						Containers: []corev1.Container{
							{
								Name:  "argocd-server",
								Image: "ghcr.io/argoproj/argocd:v2.8.0",
							},
						},
					},
				},
			},
			Status: appsv1.DeploymentStatus{
				Replicas:        2,
				UpdatedReplicas: 2,
				ReadyReplicas:   1,
			},
		},
	)

	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()

	info := checkArgoCDInfo(ctx, clientset)
	assert.Equal(t, "Progressing", info.Status)
	assert.Equal(t, 1, info.ReadyReplicas)
	assert.Equal(t, 2, info.DesiredReplicas)
}

func TestCheckComponentInfo_Deployment_Ready(t *testing.T) {
	//nolint:staticcheck
	clientset := fake.NewSimpleClientset(
		&corev1.Namespace{
			ObjectMeta: metav1.ObjectMeta{Name: "external-secrets"},
		},
		&appsv1.Deployment{
			ObjectMeta: metav1.ObjectMeta{
				Name:      "external-secrets",
				Namespace: "external-secrets",
			},
			Spec: appsv1.DeploymentSpec{
				Replicas: int32Ptr(1),
				Template: corev1.PodTemplateSpec{
					Spec: corev1.PodSpec{
						Containers: []corev1.Container{
							{
								Name:  "external-secrets",
								Image: "ghcr.io/external-secrets/external-secrets:v0.9.0",
							},
						},
					},
				},
			},
			Status: appsv1.DeploymentStatus{
				Replicas:        1,
				UpdatedReplicas: 1,
				ReadyReplicas:   1,
			},
		},
	)

	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()

	info := checkComponentInfo(ctx, clientset, "external-secrets", "external-secrets", "External Secrets", false)
	assert.Equal(t, "External Secrets", info.Name)
	assert.Equal(t, "Ready", info.Status)
	assert.True(t, info.Installed)
	assert.Equal(t, "v0.9.0", info.Version)
}

func TestCheckComponentInfo_StatefulSet_Ready(t *testing.T) {
	//nolint:staticcheck
	clientset := fake.NewSimpleClientset(
		&corev1.Namespace{
			ObjectMeta: metav1.ObjectMeta{Name: "vault"},
		},
		&appsv1.StatefulSet{
			ObjectMeta: metav1.ObjectMeta{
				Name:      "vault",
				Namespace: "vault",
			},
			Spec: appsv1.StatefulSetSpec{
				Replicas: int32Ptr(1),
				Template: corev1.PodTemplateSpec{
					Spec: corev1.PodSpec{
						Containers: []corev1.Container{
							{
								Name:  "vault",
								Image: "hashicorp/vault:1.15.4",
							},
						},
					},
				},
			},
			Status: appsv1.StatefulSetStatus{
				Replicas:        1,
				UpdatedReplicas: 1,
				ReadyReplicas:   1,
			},
		},
	)

	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()

	info := checkComponentInfo(ctx, clientset, "vault", "vault", "Vault", true)
	assert.Equal(t, "Vault", info.Name)
	assert.Equal(t, "Ready", info.Status)
	assert.True(t, info.Installed)
	assert.Equal(t, "1.15.4", info.Version)
}

func TestCheckComponentInfo_NotInstalled(t *testing.T) {
	//nolint:staticcheck
	clientset := fake.NewSimpleClientset()

	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()

	info := checkComponentInfo(ctx, clientset, "vault", "vault", "Vault", true)
	assert.Equal(t, "Vault", info.Name)
	assert.Equal(t, "NotInstalled", info.Status)
	assert.False(t, info.Installed)
}

func TestCheckComponentInfo_Pending(t *testing.T) {
	//nolint:staticcheck
	clientset := fake.NewSimpleClientset(
		&corev1.Namespace{
			ObjectMeta: metav1.ObjectMeta{Name: "monitoring"},
		},
		&appsv1.Deployment{
			ObjectMeta: metav1.ObjectMeta{
				Name:      "kube-prometheus-stack",
				Namespace: "monitoring",
			},
			Spec: appsv1.DeploymentSpec{
				Replicas: int32Ptr(1),
				Template: corev1.PodTemplateSpec{
					Spec: corev1.PodSpec{
						Containers: []corev1.Container{
							{
								Name:  "prometheus",
								Image: "prom/prometheus:latest",
							},
						},
					},
				},
			},
			Status: appsv1.DeploymentStatus{
				Replicas:        1,
				UpdatedReplicas: 0,
				ReadyReplicas:   0,
			},
		},
	)

	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()

	info := checkComponentInfo(ctx, clientset, "monitoring", "kube-prometheus-stack", "Kube Prometheus Stack", false)
	assert.Equal(t, "Pending", info.Status)
	assert.Equal(t, 0, info.ReadyReplicas)
}

func TestInfoResult_Creation(t *testing.T) {
	info := &InfoResult{
		Environment:    "dev",
		ClusterVersion: "v1.27.0",
		ArgoCDVersion:  "v2.8.0",
		Timestamp:      time.Now(),
		Components: []ComponentInfo{
			{
				Name:      "ArgoCD",
				Namespace: "argocd",
				Installed: true,
				Status:    "Ready",
				Version:   "v2.8.0",
			},
		},
	}

	assert.Equal(t, "dev", info.Environment)
	assert.Equal(t, "v2.8.0", info.ArgoCDVersion)
	assert.Len(t, info.Components, 1)
	assert.Equal(t, "Ready", info.Components[0].Status)
}

func TestComponentInfo_StatusIndicators(t *testing.T) {
	testCases := []struct {
		status    string
		ready     int
		desired   int
		installed bool
	}{
		{"Ready", 2, 2, true},
		{"Progressing", 1, 2, true},
		{"Pending", 0, 1, true},
		{"NotInstalled", 0, 0, false},
	}

	for _, tc := range testCases {
		comp := ComponentInfo{
			Name:            "TestComponent",
			Status:          tc.status,
			ReadyReplicas:   tc.ready,
			DesiredReplicas: tc.desired,
			Installed:       tc.installed,
		}

		assert.Equal(t, tc.status, comp.Status)
		assert.Equal(t, tc.ready, comp.ReadyReplicas)
		assert.Equal(t, tc.desired, comp.DesiredReplicas)
		assert.Equal(t, tc.installed, comp.Installed)
	}
}
