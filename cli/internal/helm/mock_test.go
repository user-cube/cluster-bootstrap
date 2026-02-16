package helm

import (
	"context"
	"fmt"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

// TestMockHelmAction_SuccessfulInstall tests successful Helm install simulation.
func TestMockHelmAction_SuccessfulInstall(t *testing.T) {
	mock := NewMockHelmAction()
	ctx := context.Background()

	err := mock.SimulateInstall(ctx)
	require.NoError(t, err)
	assert.Equal(t, 1, mock.GetCallCount())
}

// TestMockHelmAction_TimeoutInstall tests Helm install timeout simulation.
func TestMockHelmAction_TimeoutInstall(t *testing.T) {
	mock := NewMockHelmAction()
	mock.SimulateTimeout = true
	ctx := context.Background()

	err := mock.SimulateInstall(ctx)
	require.Error(t, err)
	assert.Contains(t, err.Error(), "timed out")
}

// TestMockHelmAction_PermissionDenied tests permission denied error simulation.
func TestMockHelmAction_PermissionDenied(t *testing.T) {
	mock := NewMockHelmAction()
	mock.SimulatePermissionErr = true
	ctx := context.Background()

	err := mock.SimulateInstall(ctx)
	require.Error(t, err)
	assert.Contains(t, err.Error(), "forbidden")
}

// TestMockHelmAction_ImagePullFailure tests image pull failure simulation.
func TestMockHelmAction_ImagePullFailure(t *testing.T) {
	mock := NewMockHelmAction()
	mock.SimulateImagePullErr = true
	ctx := context.Background()

	err := mock.SimulateInstall(ctx)
	require.Error(t, err)
	assert.Contains(t, err.Error(), "ImagePullBackOff")
}

// TestMockHelmAction_ReleaseConflict tests release already exists error.
func TestMockHelmAction_ReleaseConflict(t *testing.T) {
	mock := NewMockHelmAction()
	mock.SimulateConflict = true
	ctx := context.Background()

	err := mock.SimulateInstall(ctx)
	require.Error(t, err)
	assert.Contains(t, err.Error(), "already exists")
}

// TestMockHelmAction_CallCount tests that call count increments correctly.
func TestMockHelmAction_CallCount(t *testing.T) {
	mock := NewMockHelmAction()
	ctx := context.Background()

	assert.Equal(t, 0, mock.GetCallCount())

	mock.SimulateInstall(ctx)
	assert.Equal(t, 1, mock.GetCallCount())

	mock.SimulateInstall(ctx)
	assert.Equal(t, 2, mock.GetCallCount())
}

// TestAnalyzeError_Timeout tests error analysis for timeout errors.
func TestAnalyzeError_Timeout(t *testing.T) {
	tests := []struct {
		name     string
		errMsg   string
		contains string
	}{
		{"timeout error", "timed out", "timed out"},
		{"deadline error", "deadline exceeded", "deadline exceeded"},
		{"permission error", "forbidden", "Permission denied"},
		{"image pull error", "ImagePullBackOff", "Image pull"},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			err := fmt.Errorf("%s", tt.errMsg)
			hint := AnalyzeError(err)
			assert.NotEmpty(t, hint, "hint should not be empty for error: "+tt.errMsg)
		})
	}
}

// TestMockHelmAction_RetryScenario tests a retry scenario using the mock.
func TestMockHelmAction_RetryScenario(t *testing.T) {
	mock := NewMockHelmAction()
	ctx := context.Background()

	// First attempt: simulate timeout
	mock.SimulateTimeout = true
	err := mock.SimulateInstall(ctx)
	require.Error(t, err)
	assert.Equal(t, 1, mock.GetCallCount())

	// Second attempt: clear timeout flag and retry
	mock.SimulateTimeout = false
	err = mock.SimulateInstall(ctx)
	require.NoError(t, err)
	assert.Equal(t, 2, mock.GetCallCount())
}
