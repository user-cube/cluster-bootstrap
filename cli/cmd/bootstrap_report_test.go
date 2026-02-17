package cmd

import (
	"encoding/json"
	"os"
	"path/filepath"
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestNewBootstrapReport(t *testing.T) {
	env := "dev"
	report := NewBootstrapReport(env)

	assert.Equal(t, env, report.Environment)
	assert.False(t, report.StartTime.IsZero())
	assert.NotNil(t, report.Stages)
	assert.Empty(t, report.Stages)
	assert.NotNil(t, report.Resources.Secrets)
	assert.Empty(t, report.Resources.Secrets)
}

func TestBootstrapReport_Complete(t *testing.T) {
	report := NewBootstrapReport("test")
	time.Sleep(10 * time.Millisecond) // Ensure some duration

	report.Complete(true, nil)

	assert.True(t, report.Success)
	assert.False(t, report.EndTime.IsZero())
	assert.NotEmpty(t, report.Duration)
	assert.Greater(t, report.DurationMs, int64(0))
	assert.Empty(t, report.Error)
}

func TestBootstrapReport_CompleteWithError(t *testing.T) {
	report := NewBootstrapReport("test")
	testErr := assert.AnError

	report.Complete(false, testErr)

	assert.False(t, report.Success)
	assert.Equal(t, testErr.Error(), report.Error)
}

func TestBootstrapReport_AddStage(t *testing.T) {
	report := NewBootstrapReport("test")

	stage := StageReport{
		Name:     "Test Stage",
		Duration: "100ms",
		Success:  true,
	}

	report.AddStage(stage)

	require.Len(t, report.Stages, 1)
	assert.Equal(t, "Test Stage", report.Stages[0].Name)
	assert.True(t, report.Stages[0].Success)
}

func TestBootstrapReport_ToJSON(t *testing.T) {
	report := NewBootstrapReport("dev")
	report.Configuration = ConfigReport{
		BaseDir:    "/test/path",
		Encryption: "sops",
	}
	report.Complete(true, nil)

	jsonStr, err := report.ToJSON()
	require.NoError(t, err)
	assert.NotEmpty(t, jsonStr)

	// Verify it's valid JSON
	var decoded map[string]interface{}
	err = json.Unmarshal([]byte(jsonStr), &decoded)
	require.NoError(t, err)
	assert.Equal(t, "dev", decoded["environment"])
	assert.Equal(t, true, decoded["success"])
}

func TestBootstrapReport_WriteToFile(t *testing.T) {
	report := NewBootstrapReport("test")
	report.Complete(true, nil)

	tmpDir := t.TempDir()
	reportPath := filepath.Join(tmpDir, "report.json")

	err := report.WriteToFile(reportPath)
	require.NoError(t, err)

	// Verify file exists and contains valid JSON
	data, err := os.ReadFile(reportPath)
	require.NoError(t, err)

	var decoded BootstrapReport
	err = json.Unmarshal(data, &decoded)
	require.NoError(t, err)
	assert.Equal(t, "test", decoded.Environment)
}

func TestBootstrapReport_PrintSummary(t *testing.T) {
	report := NewBootstrapReport("dev")
	report.Configuration = ConfigReport{
		Encryption: "sops",
	}
	report.Resources = ResourceReport{
		Namespace: NamespaceReport{
			Name:    "argocd",
			Created: true,
		},
		Secrets: []SecretReport{
			{Name: "repo-ssh-key", Namespace: "argocd", Created: true},
		},
		ArgoCDRelease: HelmReleaseReport{
			Name:      "argocd",
			Namespace: "argocd",
			Installed: true,
		},
		AppOfApps: ApplicationReport{
			Name:    "app-of-apps",
			Created: true,
		},
	}
	report.AddStage(StageReport{
		Name:     "Test Stage",
		Duration: "100ms",
		Success:  true,
	})
	report.Complete(true, nil)

	// Just verify it doesn't panic
	assert.NotPanics(t, func() {
		report.PrintSummary()
	})
}

func TestBootstrapReport_PrintSummaryWithHealth(t *testing.T) {
	report := NewBootstrapReport("dev")
	report.Configuration = ConfigReport{
		Encryption: "sops",
	}
	report.Resources = ResourceReport{
		Namespace: NamespaceReport{Name: "argocd", Created: true},
		Secrets:   []SecretReport{{Name: "test", Namespace: "argocd", Created: true}},
		ArgoCDRelease: HelmReleaseReport{
			Name:      "argocd",
			Installed: true,
		},
		AppOfApps: ApplicationReport{Name: "app-of-apps", Created: true},
	}
	report.Health = &HealthReport{
		Checked: true,
		Healthy: true,
		Components: []ComponentHealth{
			{Name: "ArgoCD", Status: "Ready"},
			{Name: "Vault", Status: "NotInstalled"},
		},
	}
	report.Complete(true, nil)

	assert.NotPanics(t, func() {
		report.PrintSummary()
	})
}

func TestBootstrapReport_PrintSummaryWithError(t *testing.T) {
	report := NewBootstrapReport("dev")
	report.Configuration = ConfigReport{
		Encryption: "sops",
	}
	report.Resources = ResourceReport{
		Namespace: NamespaceReport{Name: "argocd", Created: true},
	}
	report.Complete(false, assert.AnError)

	assert.NotPanics(t, func() {
		report.PrintSummary()
	})
}

func TestStageTimer(t *testing.T) {
	timer := startStage("Test Stage")
	assert.Equal(t, "Test Stage", timer.name)
	assert.False(t, timer.startTime.IsZero())

	timer.addDetail("detail 1")
	timer.addDetail("detail 2")
	assert.Len(t, timer.details, 2)

	time.Sleep(10 * time.Millisecond)

	report := timer.complete(true, nil)
	assert.Equal(t, "Test Stage", report.Name)
	assert.True(t, report.Success)
	assert.Empty(t, report.Error)
	assert.Greater(t, report.DurationMs, int64(0))
	assert.Len(t, report.Details, 2)
}

func TestStageTimer_WithError(t *testing.T) {
	timer := startStage("Failed Stage")
	timer.addDetail("started processing")

	testErr := assert.AnError
	report := timer.complete(false, testErr)

	assert.Equal(t, "Failed Stage", report.Name)
	assert.False(t, report.Success)
	assert.Equal(t, testErr.Error(), report.Error)
	assert.Len(t, report.Details, 1)
}

func TestStatusText(t *testing.T) {
	assert.Equal(t, "created", statusText(true, "created", "updated"))
	assert.Equal(t, "updated", statusText(false, "created", "updated"))
}
