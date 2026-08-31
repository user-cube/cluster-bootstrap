package cmd

import (
	"bytes"
	"context"
	"fmt"
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/user-cube/cluster-bootstrap/cluster-bootstrap-cli/internal/k8s"
)

func TestGuardExistingAppOfApps_NoExistingApp(t *testing.T) {
	mockClient := k8s.NewMockClient()

	err := guardExistingAppOfApps(context.Background(), mockClient, "kind-dev", false)
	require.NoError(t, err)
}

func TestGuardExistingAppOfApps_AbortsWhenAppExists(t *testing.T) {
	mockClient := k8s.NewMockClient()
	_, _, err := mockClient.ApplyAppOfApps(context.Background(), "git@github.com:acme/repo.git", "main", "dev", "apps", false, false)
	require.NoError(t, err)

	err = guardExistingAppOfApps(context.Background(), mockClient, "kind-dev", false)
	require.Error(t, err)
	assert.Contains(t, err.Error(), "cluster already bootstrapped")
	assert.Contains(t, err.Error(), "app-of-apps")
	assert.Contains(t, err.Error(), "kind-dev")
	assert.Contains(t, err.Error(), "--force")
}

func TestGuardExistingAppOfApps_ForceOverwrites(t *testing.T) {
	mockClient := k8s.NewMockClient()
	_, _, err := mockClient.ApplyAppOfApps(context.Background(), "git@github.com:acme/repo.git", "main", "dev", "apps", false, false)
	require.NoError(t, err)

	err = guardExistingAppOfApps(context.Background(), mockClient, "kind-dev", true)
	require.NoError(t, err, "--force must allow bootstrap to continue")
}

func TestGuardExistingAppOfApps_PropagatesLookupError(t *testing.T) {
	mockClient := k8s.NewMockClient()
	mockClient.GetAppOfAppsErr = fmt.Errorf("permission denied: cannot read applications")

	err := guardExistingAppOfApps(context.Background(), mockClient, "kind-dev", false)
	require.Error(t, err)
	assert.Contains(t, err.Error(), "permission denied")
}

func TestPrintExistingAppOfApps_ShowsSourceDetails(t *testing.T) {
	var out bytes.Buffer
	printExistingAppOfApps(&out, ArgoCDAppInfo{
		Name:           "app-of-apps",
		Namespace:      "argocd",
		RepoURL:        "git@github.com:acme/repo.git",
		TargetRevision: "main",
		Path:           "apps",
		SyncStatus:     "Synced",
	})

	output := out.String()
	assert.Contains(t, output, "app-of-apps (namespace argocd)")
	assert.Contains(t, output, "git@github.com:acme/repo.git")
	assert.Contains(t, output, "main")
	assert.Contains(t, output, "apps")
	assert.Contains(t, output, "Synced / Unknown", "missing health status should render as Unknown")
}

func TestAnnounceTargetContext_NonInteractiveSkipsCountdown(t *testing.T) {
	var out bytes.Buffer

	start := time.Now()
	announceTargetContext(&out, "kind-dev", 10, false)
	elapsed := time.Since(start)

	assert.Less(t, elapsed, time.Second, "non-interactive runs must not wait")
	assert.Contains(t, out.String(), "kind-dev")
	assert.NotContains(t, out.String(), "Starting in")
}

func TestAnnounceTargetContext_InteractiveCountsDown(t *testing.T) {
	original := countdownInterval
	countdownInterval = time.Millisecond
	defer func() { countdownInterval = original }()

	var out bytes.Buffer
	announceTargetContext(&out, "kind-dev", 3, true)

	output := out.String()
	assert.Contains(t, output, "kind-dev")
	assert.Contains(t, output, "Starting in  3s")
	assert.Contains(t, output, "Starting in  1s")
	assert.Contains(t, output, "Ctrl+C to abort")
	assert.Contains(t, output, "Starting now")
}
