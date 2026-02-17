package cmd

import (
	"fmt"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestValidateInputs(t *testing.T) {
	tests := []struct {
		name    string
		org     string
		repo    string
		appPath string
		wantErr bool
	}{
		{
			name:    "valid inputs",
			org:     "mycompany",
			repo:    "k8s-gitops",
			appPath: "apps",
			wantErr: false,
		},
		{
			name:    "valid with hyphens",
			org:     "my-company",
			repo:    "k8s-gitops-prod",
			appPath: "kubernetes/apps",
			wantErr: false,
		},
		{
			name:    "invalid org with spaces",
			org:     "my company",
			repo:    "k8s-gitops",
			appPath: "apps",
			wantErr: true,
		},
		{
			name:    "invalid org starting with hyphen",
			org:     "-mycompany",
			repo:    "k8s-gitops",
			appPath: "apps",
			wantErr: true,
		},
		{
			name:    "invalid repo with special chars",
			org:     "mycompany",
			repo:    "k8s@gitops",
			appPath: "apps",
			wantErr: true,
		},
		{
			name:    "empty app path",
			org:     "mycompany",
			repo:    "k8s-gitops",
			appPath: "",
			wantErr: true,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			orgFlag = tt.org
			repoFlag = tt.repo
			appPathFlag = tt.appPath

			err := validateInputs()
			if tt.wantErr {
				assert.Error(t, err)
			} else {
				assert.NoError(t, err)
			}
		})
	}
}

func TestDetectCurrentValues(t *testing.T) {
	// Create a temporary workspace
	tmpDir := t.TempDir()
	cliDir := filepath.Join(tmpDir, "cli")
	err := os.MkdirAll(cliDir, 0755)
	require.NoError(t, err)

	// Create a go.mod file
	goModContent := `module github.com/test-org/test-repo/cli

go 1.25.0

require (
	github.com/spf13/cobra v1.10.2
)
`
	goModPath := filepath.Join(cliDir, "go.mod")
	err = os.WriteFile(goModPath, []byte(goModContent), 0644)
	require.NoError(t, err)

	// Test detection
	org, repo, err := detectCurrentValues(tmpDir)
	require.NoError(t, err)
	assert.Equal(t, "test-org", org)
	assert.Equal(t, "test-repo", repo)
}

func TestDetectCurrentValues_InvalidFormat(t *testing.T) {
	// Create a temporary workspace
	tmpDir := t.TempDir()
	cliDir := filepath.Join(tmpDir, "cli")
	err := os.MkdirAll(cliDir, 0755)
	require.NoError(t, err)

	// Create a go.mod with invalid format
	goModContent := `module invalid-module-path

go 1.25.0
`
	goModPath := filepath.Join(cliDir, "go.mod")
	err = os.WriteFile(goModPath, []byte(goModContent), 0644)
	require.NoError(t, err)

	// Test detection should fail
	_, _, err = detectCurrentValues(tmpDir)
	assert.Error(t, err)
}

func TestDetectCurrentAppPath(t *testing.T) {
	tests := []struct {
		name         string
		valuesYAML   string
		expectedPath string
		wantErr      bool
	}{
		{
			name: "standard app path",
			valuesYAML: `source:
  repoURL: git@github.com:org/repo.git
  targetRevision: main
  path: apps
destination:
  server: https://kubernetes.default.svc
`,
			expectedPath: "apps",
			wantErr:      false,
		},
		{
			name: "custom app path",
			valuesYAML: `source:
  repoURL: git@github.com:org/repo.git
  targetRevision: main
  path: custom/my-apps
destination:
  server: https://kubernetes.default.svc
`,
			expectedPath: "custom/my-apps",
			wantErr:      false,
		},
		{
			name: "indented path",
			valuesYAML: `spec:
  source:
    repoURL: git@github.com:org/repo.git
    targetRevision: main
    path: nested-apps
`,
			expectedPath: "nested-apps",
			wantErr:      false,
		},
		{
			name: "missing path",
			valuesYAML: `source:
  repoURL: git@github.com:org/repo.git
  targetRevision: main
`,
			wantErr: true,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			tmpDir := t.TempDir()
			appsDir := filepath.Join(tmpDir, "apps")
			err := os.MkdirAll(appsDir, 0755)
			require.NoError(t, err)

			valuesPath := filepath.Join(appsDir, "values.yaml")
			err = os.WriteFile(valuesPath, []byte(tt.valuesYAML), 0644)
			require.NoError(t, err)

			path, err := detectCurrentAppPath(tmpDir)
			if tt.wantErr {
				assert.Error(t, err)
			} else {
				assert.NoError(t, err)
				assert.Equal(t, tt.expectedPath, path)
			}
		})
	}
}

func TestApplyReplacement(t *testing.T) {
	// Create temporary workspace
	tmpDir := t.TempDir()

	// Create test file
	testFile := filepath.Join(tmpDir, "test.yaml")
	originalContent := `repoUrl: git@github.com:old-org/old-repo.git
otherField: value
`
	err := os.WriteFile(testFile, []byte(originalContent), 0644)
	require.NoError(t, err)

	// Apply replacement
	r := replacement{
		name:    "test replacement",
		pattern: "git@github.com:old-org/old-repo.git",
		replace: "git@github.com:new-org/new-repo.git",
		files:   []string{"test.yaml"},
	}

	count, err := applyReplacement(tmpDir, r, false)
	require.NoError(t, err)
	assert.Equal(t, 1, count)

	// Verify file was updated
	newContent, err := os.ReadFile(testFile)
	require.NoError(t, err)
	assert.Contains(t, string(newContent), "git@github.com:new-org/new-repo.git")
	assert.NotContains(t, string(newContent), "old-org/old-repo")
}

func TestApplyReplacement_DryRun(t *testing.T) {
	// Create temporary workspace
	tmpDir := t.TempDir()

	// Create test file
	testFile := filepath.Join(tmpDir, "test.yaml")
	originalContent := `repoUrl: git@github.com:old-org/old-repo.git`
	err := os.WriteFile(testFile, []byte(originalContent), 0644)
	require.NoError(t, err)

	// Apply replacement with dry-run
	r := replacement{
		name:    "test replacement",
		pattern: "git@github.com:old-org/old-repo.git",
		replace: "git@github.com:new-org/new-repo.git",
		files:   []string{"test.yaml"},
	}

	count, err := applyReplacement(tmpDir, r, true)
	require.NoError(t, err)
	assert.Equal(t, 1, count)

	// Verify file was NOT updated
	currentContent, err := os.ReadFile(testFile)
	require.NoError(t, err)
	assert.Equal(t, originalContent, string(currentContent))
}

func TestApplyReplacement_NoMatch(t *testing.T) {
	// Create temporary workspace
	tmpDir := t.TempDir()

	// Create test file
	testFile := filepath.Join(tmpDir, "test.yaml")
	originalContent := `repoUrl: git@github.com:different-org/different-repo.git`
	err := os.WriteFile(testFile, []byte(originalContent), 0644)
	require.NoError(t, err)

	// Apply replacement with pattern that doesn't match
	r := replacement{
		name:    "test replacement",
		pattern: "git@github.com:old-org/old-repo.git",
		replace: "git@github.com:new-org/new-repo.git",
		files:   []string{"test.yaml"},
	}

	count, err := applyReplacement(tmpDir, r, false)
	require.NoError(t, err)
	assert.Equal(t, 0, count) // No files changed
}

func TestApplyReplacement_NonExistentFile(t *testing.T) {
	tmpDir := t.TempDir()

	r := replacement{
		name:    "test replacement",
		pattern: "old",
		replace: "new",
		files:   []string{"nonexistent.yaml"},
	}

	count, err := applyReplacement(tmpDir, r, false)
	require.NoError(t, err)
	assert.Equal(t, 0, count) // Should skip non-existent files gracefully
}

func TestApplyReplacement_Idempotence(t *testing.T) {
	// Test that applying the same replacement twice doesn't change the file
	tmpDir := t.TempDir()

	// Create test file with old pattern
	testFile := filepath.Join(tmpDir, "test.yaml")
	originalContent := `repoUrl: git@github.com:old-org/old-repo.git
otherField: value
`
	err := os.WriteFile(testFile, []byte(originalContent), 0644)
	require.NoError(t, err)

	r := replacement{
		name:    "Repository URLs",
		pattern: "git@github.com:old-org/old-repo.git",
		replace: "git@github.com:myorg/myrepo.git",
		files:   []string{"test.yaml"},
	}

	// First run: pattern matches, replacement made
	count, err := applyReplacement(tmpDir, r, false)
	require.NoError(t, err)
	assert.Equal(t, 1, count, "First run should update the file")

	// Verify content changed
	content, err := os.ReadFile(testFile)
	require.NoError(t, err)
	assert.Contains(t, string(content), "myorg/myrepo")
	assert.NotContains(t, string(content), "old-org/old-repo")

	// Get file info after first change
	info1, err := os.Stat(testFile)
	require.NoError(t, err)

	// Sleep to ensure we can detect modification time changes
	time.Sleep(10 * time.Millisecond)

	// Second run with same replacement: should detect no change needed
	r2 := replacement{
		name:    "Repository URLs",
		pattern: "git@github.com:myorg/myrepo.git",
		replace: "git@github.com:myorg/myrepo.git", // Same as pattern - no-op
		files:   []string{"test.yaml"},
	}

	count, err = applyReplacement(tmpDir, r2, false)
	require.NoError(t, err)
	assert.Equal(t, 0, count, "Second run should not update when content unchanged")

	// Verify content unchanged
	newContent, err := os.ReadFile(testFile)
	require.NoError(t, err)
	assert.Equal(t, string(content), string(newContent), "Content should remain unchanged")

	// Verify file modification time didn't change
	info2, err := os.Stat(testFile)
	require.NoError(t, err)
	assert.Equal(t, info1.ModTime(), info2.ModTime(), "File should not be modified when content unchanged")
}

func TestListGoFiles(t *testing.T) {
	// Create temporary workspace with Go files
	tmpDir := t.TempDir()
	cliDir := filepath.Join(tmpDir, "cli")
	cmdDir := filepath.Join(cliDir, "cmd")
	err := os.MkdirAll(cmdDir, 0755)
	require.NoError(t, err)

	// Create some Go files
	files := []string{
		filepath.Join(cliDir, "main.go"),
		filepath.Join(cmdDir, "root.go"),
		filepath.Join(cmdDir, "bootstrap.go"),
	}
	for _, f := range files {
		err := os.WriteFile(f, []byte("package main"), 0644)
		require.NoError(t, err)
	}

	// Create a non-Go file
	err = os.WriteFile(filepath.Join(cmdDir, "README.md"), []byte("# README"), 0644)
	require.NoError(t, err)

	// List Go files
	goFiles := listGoFiles(tmpDir)

	// Should find 3 Go files
	assert.Len(t, goFiles, 3)

	// Verify paths are relative
	for _, f := range goFiles {
		assert.True(t, strings.HasPrefix(f, "cli/"), "Path should start with cli/: %s", f)
		assert.True(t, strings.HasSuffix(f, ".go"), "Path should end with .go: %s", f)
	}
}

func TestGetWorkspaceRoot_FromCli(t *testing.T) {
	// Create temporary workspace
	tmpDir := t.TempDir()
	appsDir := filepath.Join(tmpDir, "apps")
	cliDir := filepath.Join(tmpDir, "cli")
	err := os.MkdirAll(appsDir, 0755)
	require.NoError(t, err)
	err = os.MkdirAll(cliDir, 0755)
	require.NoError(t, err)

	// Change to cli directory
	originalDir, _ := os.Getwd()
	defer func() { _ = os.Chdir(originalDir) }()
	err = os.Chdir(cliDir)
	require.NoError(t, err)

	// Get workspace root
	root, err := getWorkspaceRoot()
	require.NoError(t, err)

	// Resolve symlinks in expected path (macOS /var -> /private/var)
	expectedRoot, err := filepath.EvalSymlinks(tmpDir)
	require.NoError(t, err)
	assert.Equal(t, expectedRoot, root)
}

func TestGetWorkspaceRoot_FromRoot(t *testing.T) {
	// Create temporary workspace
	tmpDir := t.TempDir()
	appsDir := filepath.Join(tmpDir, "apps")
	cliDir := filepath.Join(tmpDir, "cli")
	err := os.MkdirAll(appsDir, 0755)
	require.NoError(t, err)
	err = os.MkdirAll(cliDir, 0755)
	require.NoError(t, err)

	// Change to workspace root
	originalDir, _ := os.Getwd()
	defer func() { _ = os.Chdir(originalDir) }()
	err = os.Chdir(tmpDir)
	require.NoError(t, err)

	// Get workspace root
	root, err := getWorkspaceRoot()
	require.NoError(t, err)

	// Resolve symlinks in expected path (macOS /var -> /private/var)
	expectedRoot, err := filepath.EvalSymlinks(tmpDir)
	require.NoError(t, err)
	assert.Equal(t, expectedRoot, root)
}

func TestGetWorkspaceRoot_Invalid(t *testing.T) {
	// Create directory without apps/cli structure
	tmpDir := t.TempDir()

	originalDir, _ := os.Getwd()
	defer func() { _ = os.Chdir(originalDir) }()
	err := os.Chdir(tmpDir)
	require.NoError(t, err)

	// Should fail
	_, err = getWorkspaceRoot()
	assert.Error(t, err)
}

func TestEndToEnd_Customization(t *testing.T) {
	// Create a minimal workspace structure
	tmpDir := t.TempDir()

	// Create directories
	appsDir := filepath.Join(tmpDir, "apps")
	cliDir := filepath.Join(tmpDir, "cli")
	cmdDir := filepath.Join(cliDir, "cmd")
	err := os.MkdirAll(appsDir, 0755)
	require.NoError(t, err)
	err = os.MkdirAll(cmdDir, 0755)
	require.NoError(t, err)

	// Create apps/values.yaml
	appsValuesContent := `repo:
  url: git@github.com:user-cube/cluster-bootstrap.git
  path: apps
`
	err = os.WriteFile(filepath.Join(appsDir, "values.yaml"), []byte(appsValuesContent), 0644)
	require.NoError(t, err)

	// Create cli/go.mod
	goModContent := `module github.com/user-cube/cluster-bootstrap/cluster-bootstrap-cli

go 1.25.0
`
	err = os.WriteFile(filepath.Join(cliDir, "go.mod"), []byte(goModContent), 0644)
	require.NoError(t, err)

	// Create a test Go file with imports
	goFileContent := `package cmd

import (
	"github.com/user-cube/cluster-bootstrap/cluster-bootstrap-cli/internal/config"
)
`
	err = os.WriteFile(filepath.Join(cmdDir, "test.go"), []byte(goFileContent), 0644)
	require.NoError(t, err)

	// Set flags
	orgFlag = "mycompany"
	repoFlag = "k8s-gitops"
	appPathFlag = "kubernetes/apps"

	// Detect current values
	currentOrg, currentRepo, err := detectCurrentValues(tmpDir)
	require.NoError(t, err)
	assert.Equal(t, "user-cube", currentOrg)
	assert.Equal(t, "cluster-bootstrap", currentRepo)

	// Detect current CLI module suffix
	currentModuleSuffix, err := detectCurrentModuleSuffix(tmpDir)
	require.NoError(t, err)
	assert.Equal(t, "cluster-bootstrap-cli", currentModuleSuffix)

	// Apply replacements
	replacements := []replacement{
		{
			name:    "Repository URLs",
			pattern: fmt.Sprintf("git@github.com:%s/%s.git", currentOrg, currentRepo),
			replace: fmt.Sprintf("git@github.com:%s/%s.git", orgFlag, repoFlag),
			files:   []string{"apps/values.yaml"},
		},
		{
			name:    "Go module path",
			pattern: fmt.Sprintf("module github.com/%s/%s/%s", currentOrg, currentRepo, currentModuleSuffix),
			replace: fmt.Sprintf("module github.com/%s/%s/%s", orgFlag, repoFlag, currentModuleSuffix),
			files:   []string{"cli/go.mod"},
		},
		{
			name:    "Go imports",
			pattern: fmt.Sprintf(`"github.com/%s/%s/%s/`, currentOrg, currentRepo, currentModuleSuffix),
			replace: fmt.Sprintf(`"github.com/%s/%s/%s/`, orgFlag, repoFlag, currentModuleSuffix),
			files:   []string{"cli/cmd/test.go"},
		},
		{
			name:    "App path",
			pattern: "path: apps",
			replace: "path: kubernetes/apps",
			files:   []string{"apps/values.yaml"},
		},
	}

	for _, r := range replacements {
		count, err := applyReplacement(tmpDir, r, false)
		require.NoError(t, err, "replacement %s failed", r.name)
		if len(r.files) > 0 {
			assert.Greater(t, count, 0, "Expected changes in %s", r.name)
		}
	}

	// Verify changes
	updatedAppsValues, err := os.ReadFile(filepath.Join(appsDir, "values.yaml"))
	require.NoError(t, err)
	assert.Contains(t, string(updatedAppsValues), "git@github.com:mycompany/k8s-gitops.git")
	assert.Contains(t, string(updatedAppsValues), "path: kubernetes/apps")

	updatedGoMod, err := os.ReadFile(filepath.Join(cliDir, "go.mod"))
	require.NoError(t, err)
	assert.Contains(t, string(updatedGoMod), "module github.com/mycompany/k8s-gitops/cluster-bootstrap-cli")

	updatedGoFile, err := os.ReadFile(filepath.Join(cmdDir, "test.go"))
	require.NoError(t, err)
	assert.Contains(t, string(updatedGoFile), `"github.com/mycompany/k8s-gitops/cluster-bootstrap-cli/internal/config"`)
}

func TestReCustomization_AppPath(t *testing.T) {
	// Test that re-customization with a different app-path works correctly
	// This validates the fix for detecting current app path instead of hardcoding "apps"
	tmpDir := t.TempDir()

	// Create directories
	appsDir := filepath.Join(tmpDir, "apps")
	err := os.MkdirAll(appsDir, 0755)
	require.NoError(t, err)

	// Create apps/values.yaml with a custom path (already customized once)
	appsValuesContent := `repoSource:
  url: git@github.com:mycompany/k8s-gitops.git
  path: custom/apps
destination:
  server: https://kubernetes.default.svc
`
	err = os.WriteFile(filepath.Join(appsDir, "values.yaml"), []byte(appsValuesContent), 0644)
	require.NoError(t, err)

	// Detect current app path
	currentAppPath, err := detectCurrentAppPath(tmpDir)
	require.NoError(t, err)
	assert.Equal(t, "custom/apps", currentAppPath)

	// Now re-customize with a different app path
	newAppPath := "kubernetes/applications"

	// Apply replacement using detected current path
	r := replacement{
		name:    "App path",
		pattern: fmt.Sprintf("path: %s", currentAppPath),
		replace: fmt.Sprintf("path: %s", newAppPath),
		files:   []string{"apps/values.yaml"},
	}

	count, err := applyReplacement(tmpDir, r, false)
	require.NoError(t, err)
	assert.Equal(t, 1, count, "Expected 1 file to be updated")

	// Verify the change
	updatedAppsValues, err := os.ReadFile(filepath.Join(appsDir, "values.yaml"))
	require.NoError(t, err)
	assert.Contains(t, string(updatedAppsValues), "path: kubernetes/applications")
	assert.NotContains(t, string(updatedAppsValues), "path: custom/apps")
}
