package helm

import (
	"context"
	"fmt"
	"os"
	"path/filepath"
	"time"

	"gopkg.in/yaml.v3"
	"helm.sh/helm/v3/pkg/action"
	"helm.sh/helm/v3/pkg/chart/loader"
	"helm.sh/helm/v3/pkg/chartutil"
	"helm.sh/helm/v3/pkg/cli"
	"helm.sh/helm/v3/pkg/getter"
	"helm.sh/helm/v3/pkg/repo"
	"k8s.io/apimachinery/pkg/api/meta"
	"k8s.io/client-go/discovery"
	"k8s.io/client-go/discovery/cached/memory"
	"k8s.io/client-go/rest"
	"k8s.io/client-go/restmapper"
	"k8s.io/client-go/tools/clientcmd"
)

const (
	argoCDNamespace = "argocd"
	argoCDRelease   = "argocd"
)

// chartDependency represents a single entry in Chart.yaml dependencies.
type chartDependency struct {
	Name       string `yaml:"name"`
	Version    string `yaml:"version"`
	Repository string `yaml:"repository"`
}

// chartFile represents the relevant fields of Chart.yaml.
type chartFile struct {
	Dependencies []chartDependency `yaml:"dependencies"`
}

// loadChartConfig reads components/argocd/Chart.yaml and returns the first dependency's
// chart name, version, and repository URL.
func loadChartConfig(baseDir string) (name, version, repoURL string, err error) {
	chartPath := filepath.Join(baseDir, "components/argocd/Chart.yaml")
	data, err := os.ReadFile(chartPath)
	if err != nil {
		return "", "", "", fmt.Errorf("failed to read %s: %w", chartPath, err)
	}

	var cf chartFile
	if err := yaml.Unmarshal(data, &cf); err != nil {
		return "", "", "", fmt.Errorf("failed to parse %s: %w", chartPath, err)
	}

	if len(cf.Dependencies) == 0 {
		return "", "", "", fmt.Errorf("no dependencies found in %s", chartPath)
	}

	dep := cf.Dependencies[0]
	return dep.Name, dep.Version, dep.Repository, nil
}

// InstallArgoCD installs or upgrades ArgoCD using the Helm SDK.
// It loads values from components/argocd/values/base.yaml and values/<env>.yaml,
// then runs helm upgrade --install with --wait.
func InstallArgoCD(ctx context.Context, kubeconfig, kubeContext, env, baseDir string, verbose bool) error {
	settings := cli.New()
	settings.SetNamespace(argoCDNamespace)
	if kubeconfig != "" {
		settings.KubeConfig = kubeconfig
	}

	// Build action configuration
	actionConfig := new(action.Configuration)
	logFunc := func(format string, v ...interface{}) {}
	if verbose {
		logFunc = func(format string, v ...interface{}) {
			fmt.Printf("  [helm] "+format+"\n", v...)
		}
	}

	restClientGetter := newRESTClientGetter(kubeconfig, kubeContext, argoCDNamespace)
	if err := actionConfig.Init(restClientGetter, argoCDNamespace, "secret", logFunc); err != nil {
		return fmt.Errorf("failed to init helm action config: %w", err)
	}

	// Read chart name, version and repo from components/argocd/Chart.yaml
	chartName, chartVersion, repoURL, err := loadChartConfig(baseDir)
	if err != nil {
		return fmt.Errorf("failed to load chart config: %w", err)
	}

	// Download the chart
	chartPath, err := fetchChart(settings, chartName, chartVersion, repoURL, verbose)
	if err != nil {
		return fmt.Errorf("failed to fetch chart: %w", err)
	}

	// Load the chart
	chart, err := loader.Load(chartPath)
	if err != nil {
		return fmt.Errorf("failed to load chart: %w", err)
	}

	// Load and merge values
	vals, err := loadValues(baseDir, env)
	if err != nil {
		return fmt.Errorf("failed to load values: %w", err)
	}

	if verbose {
		fmt.Printf("  Chart: %s-%s\n", chart.Metadata.Name, chart.Metadata.Version)
	}

	// Check if release exists; if not, install; otherwise upgrade
	histClient := action.NewHistory(actionConfig)
	histClient.Max = 1
	_, err = histClient.Run(argoCDRelease)
	releaseExists := err == nil

	if !releaseExists {
		install := action.NewInstall(actionConfig)
		install.ReleaseName = argoCDRelease
		install.Namespace = argoCDNamespace
		install.Wait = true
		install.Timeout = 5 * time.Minute
		install.CreateNamespace = true

		rel, err := install.RunWithContext(ctx, chart, vals)
		if err != nil {
			return fmt.Errorf("failed to install ArgoCD: %w", err)
		}
		if verbose {
			fmt.Printf("  Release %s installed, status: %s\n", rel.Name, rel.Info.Status)
		}
		return nil
	}

	upgrade := action.NewUpgrade(actionConfig)
	upgrade.Wait = true
	upgrade.Timeout = 5 * time.Minute
	upgrade.Namespace = argoCDNamespace

	rel, err := upgrade.RunWithContext(ctx, argoCDRelease, chart, vals)
	if err != nil {
		return fmt.Errorf("failed to upgrade ArgoCD: %w", err)
	}

	if verbose {
		fmt.Printf("  Release %s upgraded, status: %s\n", rel.Name, rel.Info.Status)
	}

	return nil
}

// fetchChart downloads the given chart from a Helm repository.
func fetchChart(settings *cli.EnvSettings, chartName, chartVersion, repoURL string, verbose bool) (string, error) {
	entry := &repo.Entry{
		Name: "argocd-repo",
		URL:  repoURL,
	}

	providers := getter.All(settings)
	chartRepo, err := repo.NewChartRepository(entry, providers)
	if err != nil {
		return "", fmt.Errorf("failed to create chart repository: %w", err)
	}

	// Download the repo index
	_, err = chartRepo.DownloadIndexFile()
	if err != nil {
		return "", fmt.Errorf("failed to download repo index: %w", err)
	}

	// Locate/download the chart
	chartPathOpts := action.ChartPathOptions{
		RepoURL: repoURL,
		Version: chartVersion,
	}

	chartPath, err := chartPathOpts.LocateChart(chartName, settings)
	if err != nil {
		return "", fmt.Errorf("failed to locate chart: %w", err)
	}

	if verbose {
		fmt.Printf("  Downloaded chart %s-%s to %s\n", chartName, chartVersion, chartPath)
	}

	return chartPath, nil
}

// loadValues reads base.yaml and the environment-specific values file, then merges them.
func loadValues(baseDir, env string) (map[string]interface{}, error) {
	baseFile := filepath.Join(baseDir, "components/argocd/values/base.yaml")
	envFile := filepath.Join(baseDir, fmt.Sprintf("components/argocd/values/%s.yaml", env))

	baseVals, err := chartutil.ReadValuesFile(baseFile)
	if err != nil {
		return nil, fmt.Errorf("failed to read base values %s: %w", baseFile, err)
	}

	envVals, err := chartutil.ReadValuesFile(envFile)
	if err != nil {
		if os.IsNotExist(err) {
			return baseVals.AsMap(), nil
		}
		return nil, fmt.Errorf("failed to read env values %s: %w", envFile, err)
	}

	// Merge: env values override base values
	merged := chartutil.MergeTables(envVals.AsMap(), baseVals.AsMap())
	return merged, nil
}

// kubeConfigGetter implements genericclioptions.RESTClientGetter using client-go.
type kubeConfigGetter struct {
	kubeconfig  string
	kubeContext string
	namespace   string
}

func newRESTClientGetter(kubeconfig, kubeContext, namespace string) *kubeConfigGetter {
	return &kubeConfigGetter{
		kubeconfig:  kubeconfig,
		kubeContext: kubeContext,
		namespace:   namespace,
	}
}

func (r *kubeConfigGetter) ToRESTConfig() (*rest.Config, error) {
	return r.toClientConfig().ClientConfig()
}

func (r *kubeConfigGetter) ToDiscoveryClient() (discovery.CachedDiscoveryInterface, error) {
	config, err := r.ToRESTConfig()
	if err != nil {
		return nil, err
	}
	dc, err := discovery.NewDiscoveryClientForConfig(config)
	if err != nil {
		return nil, err
	}
	return memory.NewMemCacheClient(dc), nil
}

func (r *kubeConfigGetter) ToRESTMapper() (meta.RESTMapper, error) {
	dc, err := r.ToDiscoveryClient()
	if err != nil {
		return nil, err
	}
	mapper := restmapper.NewDeferredDiscoveryRESTMapper(dc)
	return mapper, nil
}

func (r *kubeConfigGetter) ToRawKubeConfigLoader() clientcmd.ClientConfig {
	return r.toClientConfig()
}

func (r *kubeConfigGetter) toClientConfig() clientcmd.ClientConfig {
	loadingRules := clientcmd.NewDefaultClientConfigLoadingRules()
	if r.kubeconfig != "" {
		loadingRules.ExplicitPath = r.kubeconfig
	}
	overrides := &clientcmd.ConfigOverrides{}
	if r.kubeContext != "" {
		overrides.CurrentContext = r.kubeContext
	}
	if r.namespace != "" {
		overrides.Context.Namespace = r.namespace
	}
	return clientcmd.NewNonInteractiveDeferredLoadingClientConfig(loadingRules, overrides)
}
