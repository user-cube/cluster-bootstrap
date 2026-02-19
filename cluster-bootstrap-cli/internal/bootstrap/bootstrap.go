package bootstrap

import (
	"context"
	"fmt"
	"os"
	"path/filepath"
	"strings"

	"github.com/user-cube/cluster-bootstrap/cluster-bootstrap-cli/internal/config"
	"github.com/user-cube/cluster-bootstrap/cluster-bootstrap-cli/internal/helm"
	"github.com/user-cube/cluster-bootstrap/cluster-bootstrap-cli/internal/k8s"
	"github.com/user-cube/cluster-bootstrap/cluster-bootstrap-cli/internal/sops"
)

// Options defines the configuration for the bootstrap process
type Options struct {
	Env               string
	BaseDir           string
	AppPath           string
	SecretsFile       string
	Encryption        string
	Kubeconfig        string
	KubeContext       string
	AgeKeyFile        string
	GitCryptKeyFile   string
	DryRun            bool
	SkipArgoCDInstall bool
	Verbose           bool
}

// Result holds the outcome of the bootstrap process
type Result struct {
	EnvSecrets    *config.EnvironmentSecrets
	SecretsPath   string
	ArgoCDAppPath string
	LocalAppPath  string
	SubfolderPath string
}

// Manager handles the bootstrap lifecycle
type Manager struct {
	opts Options
}

// NewManager creates a new bootstrap manager
func NewManager(opts Options) *Manager {
	return &Manager{opts: opts}
}

// ResolvePaths determines the paths to use based on the environment and current directory
func (m *Manager) ResolvePaths() (*Result, error) {
	res := &Result{
		ArgoCDAppPath: m.opts.AppPath,
	}

	if m.opts.BaseDir == "." {
		detected, relPath := DetectGitSubdirectory()
		if detected && relPath != "" {
			res.SubfolderPath = relPath
			if strings.HasPrefix(m.opts.AppPath, relPath+"/") {
				res.ArgoCDAppPath = m.opts.AppPath
			} else {
				res.ArgoCDAppPath = relPath + "/" + m.opts.AppPath
			}
		}
	}

	localPath, err := m.ValidateInputs(res.ArgoCDAppPath)
	if err != nil {
		return nil, err
	}
	res.LocalAppPath = localPath

	return res, nil
}

// ValidateInputs checks if the provided options are valid
func (m *Manager) ValidateInputs(argoCDAppPath string) (string, error) {
	if m.opts.Env == "" {
		return "", fmt.Errorf("environment is required")
	}

	baseInfo, statErr := os.Stat(m.opts.BaseDir)
	if statErr != nil {
		return "", fmt.Errorf("base-dir %s is not accessible: %w", m.opts.BaseDir, statErr)
	}
	if !baseInfo.IsDir() {
		return "", fmt.Errorf("base-dir %s is not a directory", m.opts.BaseDir)
	}

	if filepath.IsAbs(argoCDAppPath) {
		return "", fmt.Errorf("app-path must be relative")
	}

	localAppPath := argoCDAppPath

	if m.opts.BaseDir == "." {
		detected, relPath := DetectGitSubdirectory()
		if detected && relPath != "" && strings.HasPrefix(argoCDAppPath, relPath+"/") {
			localAppPath = strings.TrimPrefix(argoCDAppPath, relPath+"/")
		}
	} else if m.opts.BaseDir != "." {
		cleanBase := filepath.Clean(m.opts.BaseDir)
		baseComponents := strings.Split(cleanBase, string(filepath.Separator))
		pathComponents := strings.Split(argoCDAppPath, "/")
		baseLastComponent := baseComponents[len(baseComponents)-1]

		if len(pathComponents) > 0 && pathComponents[0] == baseLastComponent {
			localAppPath = strings.Join(pathComponents[1:], "/")
			if localAppPath == "" {
				localAppPath = "."
			}
		}
	}

	appFullPath := filepath.Join(m.opts.BaseDir, localAppPath)
	if _, statErr := os.Stat(appFullPath); statErr != nil {
		if argoCDAppPath == "apps" {
			detected, detectErr := AutoDetectAppPath(m.opts.BaseDir)
			if detectErr != nil {
				return "", fmt.Errorf("app-path %s does not exist: %w\n  hint: use --app-path to specify the full path from repository root (e.g., 'k8s/apps')", argoCDAppPath, statErr)
			}
			localAppPath = detected
		} else {
			return "", fmt.Errorf("app-path %s does not exist: %w\n  hint: verify the path exists and try using --base-dir if working with subfolders", argoCDAppPath, statErr)
		}
	}

	if m.opts.SecretsFile != "" {
		isEnc := strings.HasSuffix(m.opts.SecretsFile, ".enc.yaml")
		isYaml := strings.HasSuffix(m.opts.SecretsFile, ".yaml")
		switch m.opts.Encryption {
		case "sops":
			if !isEnc {
				return "", fmt.Errorf("secrets-file must end with .enc.yaml when encryption is sops")
			}
		case "git-crypt":
			if !isYaml || isEnc {
				return "", fmt.Errorf("secrets-file must end with .yaml (not .enc.yaml) when encryption is git-crypt")
			}
		}
	}

	return localAppPath, nil
}

// LoadSecrets loads secrets based on the configured encryption backend
func (m *Manager) LoadSecrets() (*config.EnvironmentSecrets, string, error) {
	var envSecrets *config.EnvironmentSecrets
	var secretsPath string
	var err error

	switch m.opts.Encryption {
	case "git-crypt":
		sf := m.opts.SecretsFile
		if sf == "" {
			sf = filepath.Join(m.opts.BaseDir, config.SecretsFileNamePlain(m.opts.Env))
		}
		secretsPath = sf
		if err := validateSecretsFileExists(secretsPath); err != nil {
			return nil, "", err
		}
		envSecrets, err = config.LoadSecretsPlaintext(sf)
		if err != nil {
			return nil, "", err
		}
	case "sops":
		sf := m.opts.SecretsFile
		if sf == "" {
			sf = filepath.Join(m.opts.BaseDir, config.SecretsFileName(m.opts.Env))
		}
		secretsPath = sf
		if err := validateSecretsFileExists(secretsPath); err != nil {
			return nil, "", err
		}
		sopsOpts := &sops.Options{AgeKeyFile: m.opts.AgeKeyFile}
		envSecrets, err = config.LoadSecrets(sf, sopsOpts)
		if err != nil {
			return nil, "", err
		}
	default:
		return nil, "", fmt.Errorf("unsupported encryption backend: %s (use sops or git-crypt)", m.opts.Encryption)
	}

	return envSecrets, secretsPath, nil
}

// RunBootstrapResources applies the bootstrap resources to the cluster
func (m *Manager) RunBootstrapResources(ctx context.Context, client k8s.ClientInterface, envSecrets *config.EnvironmentSecrets, argoCDAppPath string) (map[string]interface{}, error) {
	// Namespace
	_, err := client.EnsureNamespace(ctx, "argocd")
	if err != nil {
		return nil, err
	}

	// Repo Secret
	_, _, err = client.CreateRepoSSHSecret(ctx, envSecrets.Repo.URL, envSecrets.Repo.SSHPrivateKey, false)
	if err != nil {
		return nil, err
	}

	// Git-crypt Secret
	if m.opts.GitCryptKeyFile != "" {
		keyData, err := os.ReadFile(m.opts.GitCryptKeyFile) // #nosec G304
		if err != nil {
			return nil, fmt.Errorf("failed to read git-crypt key file: %w", err)
		}
		_, err = client.CreateGitCryptKeySecret(ctx, keyData)
		if err != nil {
			return nil, err
		}
	}

	// ArgoCD Install
	if !m.opts.SkipArgoCDInstall {
		helmClient := helm.NewClient()
		_, err = helmClient.InstallArgoCD(ctx, m.opts.Kubeconfig, m.opts.KubeContext, m.opts.Env, m.opts.BaseDir, m.opts.Verbose)
		if err != nil {
			return nil, fmt.Errorf("failed to install ArgoCD: %w", err)
		}
	}

	// App of Apps
	_, _, err = client.ApplyAppOfApps(ctx, envSecrets.Repo.URL, envSecrets.Repo.TargetRevision, m.opts.Env, argoCDAppPath, false)
	if err != nil {
		return nil, err
	}

	return nil, nil
}

// Helper functions moved from cmd/bootstrap.go

func DetectGitSubdirectory() (bool, string) {
	cwd, err := os.Getwd()
	if err != nil {
		return false, ""
	}

	dir := cwd
	for {
		gitPath := filepath.Join(dir, ".git")
		if _, err := os.Stat(gitPath); err == nil {
			if dir == cwd {
				return false, ""
			}
			relPath, err := filepath.Rel(dir, cwd)
			if err != nil {
				return false, ""
			}
			return true, filepath.ToSlash(relPath)
		}

		parent := filepath.Dir(dir)
		if parent == dir {
			return false, ""
		}
		dir = parent
	}
}

func AutoDetectAppPath(base string) (string, error) {
	var candidates []string
	_ = filepath.WalkDir(base, func(path string, d os.DirEntry, walkErr error) error {
		if walkErr != nil {
			return nil
		}
		if d.IsDir() {
			return nil
		}
		if d.Name() != "Chart.yaml" {
			return nil
		}
		dir := filepath.Dir(path)
		if _, err := os.Stat(filepath.Join(dir, "templates", "application.yaml")); err != nil {
			return nil
		}
		rel, err := filepath.Rel(base, dir)
		if err != nil {
			return nil
		}
		candidates = append(candidates, rel)
		return nil
	})

	if len(candidates) == 0 {
		return "", fmt.Errorf("no app chart found under base-dir")
	}

	for _, candidate := range candidates {
		if filepath.Base(candidate) == "apps" {
			return candidate, nil
		}
	}

	return candidates[0], nil
}

func validateSecretsFileExists(path string) error {
	if path == "" {
		return nil
	}
	if _, err := os.Stat(path); err != nil {
		return fmt.Errorf("secrets file not found: %s", path)
	}
	return nil
}
