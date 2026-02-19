package bootstrap

import (
	"os"
	"path/filepath"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestValidateInputs(t *testing.T) {
	tmpDir := t.TempDir()
	require.NoError(t, os.MkdirAll(filepath.Join(tmpDir, "apps"), 0755))

	t.Run("valid sops config", func(t *testing.T) {
		mgr := NewManager(Options{
			Env:         "dev",
			BaseDir:     tmpDir,
			AppPath:     "apps",
			Encryption:  "sops",
			SecretsFile: filepath.Join(tmpDir, "secrets.dev.enc.yaml"),
		})

		_, err := mgr.ValidateInputs("apps")
		require.NoError(t, err)
	})

	t.Run("invalid sops extension", func(t *testing.T) {
		mgr := NewManager(Options{
			Env:         "dev",
			BaseDir:     tmpDir,
			AppPath:     "apps",
			Encryption:  "sops",
			SecretsFile: filepath.Join(tmpDir, "secrets.dev.yaml"),
		})

		_, err := mgr.ValidateInputs("apps")
		assert.ErrorContains(t, err, "must end with .enc.yaml")
	})

	t.Run("invalid git-crypt extension", func(t *testing.T) {
		mgr := NewManager(Options{
			Env:         "dev",
			BaseDir:     tmpDir,
			AppPath:     "apps",
			Encryption:  "git-crypt",
			SecretsFile: filepath.Join(tmpDir, "secrets.dev.enc.yaml"),
		})

		_, err := mgr.ValidateInputs("apps")
		assert.ErrorContains(t, err, "not .enc.yaml")
	})

	t.Run("absolute app path", func(t *testing.T) {
		mgr := NewManager(Options{
			Env:        "dev",
			BaseDir:    tmpDir,
			AppPath:    "/abs/path",
			Encryption: "sops",
		})

		_, err := mgr.ValidateInputs("/abs/path")
		assert.ErrorContains(t, err, "app-path must be relative")
	})

	t.Run("auto-detect app path", func(t *testing.T) {
		require.NoError(t, os.RemoveAll(filepath.Join(tmpDir, "apps")))
		require.NoError(t, os.MkdirAll(filepath.Join(tmpDir, "k8s", "apps", "templates"), 0755))
		require.NoError(t, os.WriteFile(filepath.Join(tmpDir, "k8s", "apps", "Chart.yaml"), []byte("apiVersion: v2\n"), 0644))
		require.NoError(t, os.WriteFile(filepath.Join(tmpDir, "k8s", "apps", "templates", "application.yaml"), []byte("kind: Application\n"), 0644))

		// Test with baseDir pointing to k8s subfolder (simulating --base-dir ./k8s)
		mgr := NewManager(Options{
			Env:         "dev",
			BaseDir:     filepath.Join(tmpDir, "k8s"),
			AppPath:     "k8s/apps",
			Encryption:  "sops",
			SecretsFile: filepath.Join(tmpDir, "secrets.dev.enc.yaml"),
		})

		localPath, err := mgr.ValidateInputs("k8s/apps")
		require.NoError(t, err)
		assert.Equal(t, "apps", localPath)
	})
}
