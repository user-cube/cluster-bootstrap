package helm

import (
	"context"
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"sort"
	"strings"
	"time"

	"gopkg.in/yaml.v3"
	"helm.sh/helm/v3/pkg/action"
	"helm.sh/helm/v3/pkg/chart"
	"helm.sh/helm/v3/pkg/chart/loader"
	"helm.sh/helm/v3/pkg/chartutil"
	"helm.sh/helm/v3/pkg/cli"
	"helm.sh/helm/v3/pkg/engine"
	"helm.sh/helm/v3/pkg/getter"
	"helm.sh/helm/v3/pkg/repo"
	"helm.sh/helm/v3/pkg/storage/driver"
	apierrors "k8s.io/apimachinery/pkg/api/errors"
	"k8s.io/apimachinery/pkg/api/meta"
	"k8s.io/client-go/discovery"
	"k8s.io/client-go/discovery/cached/memory"
	"k8s.io/client-go/rest"
	"k8s.io/client-go/restmapper"
	"k8s.io/client-go/tools/clientcmd"
)

const (
	argoCDNamespace  = "argocd"
	argoCDRelease    = "argocd"
	argoCDChartDep   = "argo-cd"
	ciliumNamespace  = "kube-system"
	ciliumRelease    = "cilium"
	ciliumChartDep   = "cilium"
	componentTimeout = 5 * time.Minute
)

type componentConfig struct {
	name             string
	releaseName      string
	namespace        string
	dependencyName   string
	installWrapper   bool
	envOverridesBase bool
	hasValues        bool
	wait             bool
	timeout          time.Duration
}

var (
	installComponentFn           = installComponent
	serviceMonitorCRDAvailableFn = serviceMonitorCRDAvailable
	waitForServiceMonitorCRDFn   = waitForServiceMonitorCRD
	argoCDComponent              = componentConfig{
		name:           "argocd",
		releaseName:    argoCDRelease,
		namespace:      argoCDNamespace,
		dependencyName: argoCDChartDep,
		hasValues:      true,
		wait:           true,
		timeout:        componentTimeout,
	}
	ciliumComponent = componentConfig{
		name:             "cilium",
		releaseName:      ciliumRelease,
		namespace:        ciliumNamespace,
		dependencyName:   ciliumChartDep,
		installWrapper:   true,
		envOverridesBase: true,
		hasValues:        true,
		wait:             true,
		timeout:          componentTimeout,
	}
	prometheusOperatorCRDsComponent = componentConfig{
		name:           "prometheus-operator-crds",
		releaseName:    "prometheus-operator-crds",
		namespace:      "monitoring",
		dependencyName: "prometheus-operator-crds",
		wait:           true,
		timeout:        componentTimeout,
	}
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

// loadChartConfig reads a component Chart.yaml and returns the named dependency's
// chart name, version, and repository URL.
func loadChartConfig(baseDir, componentName, dependencyName string) (name, version, repoURL string, err error) {
	chartPath := filepath.Join(baseDir, "components", componentName, "Chart.yaml")
	data, err := os.ReadFile(chartPath) // #nosec G304
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

	var depNames []string
	for _, dep := range cf.Dependencies {
		depNames = append(depNames, dep.Name)
		if dep.Name == dependencyName {
			return dep.Name, dep.Version, dep.Repository, nil
		}
	}

	return "", "", "", fmt.Errorf("dependency %s not found in %s (found: %s)", dependencyName, chartPath, strings.Join(depNames, ", "))
}

// InstallArgoCD installs or upgrades ArgoCD using the Helm SDK.
// It loads values from components/argocd/values/base.yaml and values/<env>.yaml,
// then runs helm upgrade --install with --wait.
// Returns helpful error messages for common failure scenarios.
// Returns a boolean indicating if it was installed (true) or upgraded (false).
func InstallArgoCD(ctx context.Context, kubeconfig, kubeContext, env, baseDir string, verbose, verboseWithTemplates bool, templateOutputPath string) (bool, error) {
	return installComponent(ctx, kubeconfig, kubeContext, env, baseDir, verbose, verboseWithTemplates, templateOutputPath, argoCDComponent)
}

// InstallCilium installs or upgrades Cilium using the same Helm bootstrap path as ArgoCD.
// Helm wait is always enabled, so a successful return is the health barrier before ArgoCD.
func InstallCilium(ctx context.Context, kubeconfig, kubeContext, env, baseDir string, verbose, verboseWithTemplates bool, templateOutputPath string) (bool, error) {
	serviceMonitorsEnabled, err := ciliumServiceMonitorsEnabled(baseDir, env)
	if err != nil {
		return false, fmt.Errorf("failed to inspect Cilium monitoring configuration: %w", err)
	}
	if serviceMonitorsEnabled {
		available, err := serviceMonitorCRDAvailableFn(kubeconfig, kubeContext)
		if err != nil {
			return false, fmt.Errorf("failed to check for Prometheus Operator ServiceMonitor CRD: %w", err)
		}
		if !available {
			fmt.Println("  Cilium ServiceMonitors are enabled; installing Prometheus Operator CRDs before Cilium...")
			if _, err := installComponentFn(ctx, kubeconfig, kubeContext, env, baseDir, verbose, verboseWithTemplates, templateOutputPath, prometheusOperatorCRDsComponent); err != nil {
				return false, fmt.Errorf("failed to install Prometheus Operator CRDs required by Cilium: %w", err)
			}
			if err := waitForServiceMonitorCRDFn(ctx, kubeconfig, kubeContext); err != nil {
				return false, err
			}
		} else if verbose {
			fmt.Println("  Prometheus Operator ServiceMonitor CRD is already installed")
		}
	}
	return installComponentFn(ctx, kubeconfig, kubeContext, env, baseDir, verbose, verboseWithTemplates, templateOutputPath, ciliumComponent)
}

func installComponent(ctx context.Context, kubeconfig, kubeContext, env, baseDir string, verbose, verboseWithTemplates bool, templateOutputPath string, component componentConfig) (bool, error) {
	verbose = verbose || verboseWithTemplates
	settings := cli.New()
	settings.SetNamespace(component.namespace)
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

	restClientGetter := newRESTClientGetter(kubeconfig, kubeContext, component.namespace)
	if err := actionConfig.Init(restClientGetter, component.namespace, "secret", logFunc); err != nil {
		return false, fmt.Errorf("failed to init helm action config: %w", err)
	}

	chartName, chartVersion, repoURL, err := loadChartConfig(baseDir, component.name, component.dependencyName)
	if err != nil {
		return false, fmt.Errorf("failed to load chart config: %w\n  hint: ensure components/%s/Chart.yaml exists and has the %s dependency defined", err, component.name, component.dependencyName)
	}

	// Download the chart
	chartPath, err := fetchChart(settings, chartName, chartVersion, repoURL, verbose)
	if err != nil {
		return false, fmt.Errorf("%w\n  hint: verify the Helm repository is accessible and the chart version exists\n  tip: try: helm repo add %s %s && helm repo update", err, component.name, repoURL)
	}

	installChart, err := loadInstallChart(baseDir, chartPath, component)
	if err != nil {
		return false, err
	}

	// Load and merge values
	vals, err := loadValuesForComponent(baseDir, env, component)
	if err != nil {
		return false, fmt.Errorf("failed to load values: %w", err)
	}

	if verbose {
		fmt.Printf("  Chart: %s-%s\n", installChart.Metadata.Name, installChart.Metadata.Version)
		if component.hasValues {
			fmt.Printf("  Values: %s\n", filepath.Join(baseDir, "components", component.name, "values", "base.yaml"))
			fmt.Printf("  Values override: %s\n", filepath.Join(baseDir, "components", component.name, "values", fmt.Sprintf("%s.yaml", env)))
		}
	}

	// Check if release exists; if not, install; otherwise upgrade
	histClient := action.NewHistory(actionConfig)
	histClient.Max = 1
	_, err = histClient.Run(component.releaseName)
	releaseExists, historyErr := helmReleaseExists(err)
	if historyErr != nil {
		return false, fmt.Errorf("failed to check Helm release %s in namespace %s: %w", component.releaseName, component.namespace, historyErr)
	}
	if verboseWithTemplates {
		manifest, renderErr := renderComponentTemplates(installChart, vals, component, !releaseExists)
		if renderErr != nil {
			return false, fmt.Errorf("failed to render %s templates for verbose output: %w", component.name, renderErr)
		}
		if err := appendRenderedTemplates(templateOutputPath, component.name, manifest); err != nil {
			return false, err
		}
	}

	if !releaseExists {
		install := action.NewInstall(actionConfig)
		install.ReleaseName = component.releaseName
		install.Namespace = component.namespace
		install.Wait = component.wait
		install.Timeout = component.timeout
		install.CreateNamespace = true

		rel, err := install.RunWithContext(ctx, installChart, vals)
		if err != nil {
			errMsg := err.Error()
			hint := fmt.Sprintf("verify %s is not already installed and chart values are valid", component.name)
			if strings.Contains(errMsg, "timeout") {
				hint = fmt.Sprintf("Helm install timed out. Check cluster resources and pod status: kubectl get pods -n %s -w", component.namespace)
			} else if strings.Contains(errMsg, "permission denied") || strings.Contains(errMsg, "Forbidden") {
				hint = fmt.Sprintf("permission denied. Verify your cluster role permissions to create resources in the %s namespace", component.namespace)
			} else if strings.Contains(errMsg, "imagePull") || strings.Contains(errMsg, "ErrImagePull") {
				hint = "image pull failed. Verify container images are accessible and image pull secrets are configured"
			}
			return false, fmt.Errorf("failed to install %s: %w\n  hint: %s", component.name, err, hint)
		}
		if verbose {
			fmt.Printf("  Release %s installed, status: %s\n", rel.Name, rel.Info.Status)
		}
		return true, nil
	}

	upgrade := action.NewUpgrade(actionConfig)
	upgrade.Wait = component.wait
	upgrade.Timeout = component.timeout
	upgrade.Namespace = component.namespace

	rel, err := upgrade.RunWithContext(ctx, component.releaseName, installChart, vals)
	if err != nil {
		errMsg := err.Error()
		hint := fmt.Sprintf("verify %s release configuration and chart values", component.name)
		if strings.Contains(errMsg, "timeout") {
			hint = fmt.Sprintf("Helm upgrade timed out. Check pod status: kubectl get pods -n %s", component.namespace)
		} else if strings.Contains(errMsg, "permission denied") || strings.Contains(errMsg, "Forbidden") {
			hint = fmt.Sprintf("permission denied. Verify your cluster role permissions to upgrade resources in the %s namespace", component.namespace)
		}
		return false, fmt.Errorf("failed to upgrade %s: %w\n  hint: %s", component.name, err, hint)
	}

	if verbose {
		fmt.Printf("  Release %s upgraded, status: %s\n", rel.Name, rel.Info.Status)
	}

	return false, nil
}

func appendRenderedTemplates(path, componentName, manifest string) error {
	if path == "" {
		return fmt.Errorf("template output path is required when verbose templates are enabled")
	}
	file, err := os.OpenFile(path, os.O_CREATE|os.O_APPEND|os.O_WRONLY, 0600) // #nosec G304 -- bootstrap creates this absolute path beneath --base-dir.
	if err != nil {
		return fmt.Errorf("failed to open rendered template output %s: %w", path, err)
	}
	if _, err := fmt.Fprintf(file, "# BEGIN RENDERED %s HELM MANIFESTS\n%s# END RENDERED %s HELM MANIFESTS\n", strings.ToUpper(componentName), manifest, strings.ToUpper(componentName)); err != nil {
		_ = file.Close()
		return fmt.Errorf("failed to write rendered templates to %s: %w", path, err)
	}
	if err := file.Close(); err != nil {
		return fmt.Errorf("failed to close rendered template output %s: %w", path, err)
	}
	return nil
}

// renderComponentTemplates renders the same chart and merged values that will be
// passed to Helm. It intentionally does not contact or modify the cluster.
func renderComponentTemplates(componentChart *chart.Chart, vals map[string]interface{}, component componentConfig, isInstall bool) (string, error) {
	release := chartutil.ReleaseOptions{
		Name:      component.releaseName,
		Namespace: component.namespace,
		IsInstall: isInstall,
		IsUpgrade: !isInstall,
	}
	renderValues, err := chartutil.ToRenderValues(componentChart, vals, release, chartutil.DefaultCapabilities)
	if err != nil {
		return "", fmt.Errorf("failed to prepare template values: %w", err)
	}
	rendered, err := engine.Render(componentChart, renderValues)
	if err != nil {
		return "", fmt.Errorf("failed to render chart: %w", err)
	}

	files := make([]string, 0, len(rendered))
	for name := range rendered {
		files = append(files, name)
	}
	sort.Strings(files)

	var manifest strings.Builder
	for _, name := range files {
		contents := strings.TrimSpace(rendered[name])
		if contents == "" || strings.HasSuffix(name, "NOTES.txt") {
			continue
		}
		fmt.Fprintf(&manifest, "---\n# Source: %s\n%s\n", name, contents)
	}
	return manifest.String(), nil
}

func helmReleaseExists(historyErr error) (bool, error) {
	if historyErr == nil {
		return true, nil
	}
	if errors.Is(historyErr, driver.ErrReleaseNotFound) {
		return false, nil
	}
	return false, historyErr
}

func loadInstallChart(baseDir, dependencyChartPath string, component componentConfig) (*chart.Chart, error) {
	dependencyChart, err := loader.Load(dependencyChartPath)
	if err != nil {
		return nil, fmt.Errorf("failed to load chart: %w\n  hint: verify the downloaded chart is not corrupted", err)
	}
	if !component.installWrapper {
		return dependencyChart, nil
	}

	wrapperPath := filepath.Join(baseDir, "components", component.name)
	wrapper, err := loader.Load(wrapperPath)
	if err != nil {
		return nil, fmt.Errorf("failed to load component wrapper %s: %w", wrapperPath, err)
	}

	dependencies := make([]*chart.Chart, 0, len(wrapper.Dependencies())+1)
	for _, existing := range wrapper.Dependencies() {
		if existing.Name() != component.dependencyName {
			dependencies = append(dependencies, existing)
		}
	}
	dependencies = append(dependencies, dependencyChart)
	wrapper.SetDependencies(dependencies...)
	return wrapper, nil
}

// fetchChart downloads the given chart from a Helm repository.
func fetchChart(settings *cli.EnvSettings, chartName, chartVersion, repoURL string, verbose bool) (string, error) {
	entry := &repo.Entry{
		Name: chartName + "-repo",
		URL:  repoURL,
	}

	providers := getter.All(settings)
	chartRepo, err := repo.NewChartRepository(entry, providers)
	if err != nil {
		return "", fmt.Errorf("failed to create chart repository: %w", err)
	}

	const maxAttempts = 3
	var chartPath string
	var lastErr error
	for attempt := 1; attempt <= maxAttempts; attempt++ {
		// Download the repo index
		_, err = chartRepo.DownloadIndexFile()
		if err != nil {
			lastErr = fmt.Errorf("failed to download repo index: %w", err)
		} else {
			// Locate/download the chart
			chartPathOpts := action.ChartPathOptions{
				RepoURL: repoURL,
				Version: chartVersion,
			}
			chartPath, err = chartPathOpts.LocateChart(chartName, settings)
			if err == nil {
				if verbose {
					fmt.Printf("  Downloaded chart %s-%s to %s\n", chartName, chartVersion, chartPath)
				}
				return chartPath, nil
			}
			lastErr = fmt.Errorf("failed to locate chart: %w", err)
		}

		if attempt < maxAttempts {
			time.Sleep(time.Duration(attempt) * time.Second)
		}
	}

	return "", fmt.Errorf("failed to fetch chart from %s after %d attempts: %w", repoURL, maxAttempts, lastErr)
}

// loadValues reads base.yaml and the environment-specific values file, then merges them.
func loadValues(baseDir, env string) (map[string]interface{}, error) {
	return loadComponentValuesWithOrder(baseDir, "argocd", env, false)
}

func loadComponentValues(baseDir, componentName, env string) (map[string]interface{}, error) {
	return loadComponentValuesWithOrder(baseDir, componentName, env, true)
}

func loadValuesForComponent(baseDir, env string, component componentConfig) (map[string]interface{}, error) {
	if !component.hasValues {
		return map[string]interface{}{}, nil
	}
	return loadComponentValuesWithOrder(baseDir, component.name, env, component.envOverridesBase)
}

func ciliumServiceMonitorsEnabled(baseDir, env string) (bool, error) {
	values, err := loadValuesForComponent(baseDir, env, ciliumComponent)
	if err != nil {
		return false, err
	}
	for _, path := range [][]string{
		{"cilium", "prometheus", "serviceMonitor", "enabled"},
		{"cilium", "operator", "prometheus", "serviceMonitor", "enabled"},
		{"cilium", "hubble", "metrics", "serviceMonitor", "enabled"},
	} {
		if valueAtPathIsTrue(values, path) {
			return true, nil
		}
	}
	return false, nil
}

func valueAtPathIsTrue(values map[string]interface{}, path []string) bool {
	var current interface{} = values
	for _, key := range path {
		mapValue, ok := current.(map[string]interface{})
		if !ok {
			return false
		}
		current = mapValue[key]
	}
	enabled, ok := current.(bool)
	return ok && enabled
}

func serviceMonitorCRDAvailable(kubeconfig, kubeContext string) (bool, error) {
	discoveryClient, err := newRESTClientGetter(kubeconfig, kubeContext, "").ToDiscoveryClient()
	if err != nil {
		return false, err
	}
	resources, err := discoveryClient.ServerResourcesForGroupVersion("monitoring.coreos.com/v1")
	if isServiceMonitorCRDNotFound(err) {
		return false, nil
	}
	if err != nil {
		return false, err
	}
	for _, resource := range resources.APIResources {
		if resource.Kind == "ServiceMonitor" {
			return true, nil
		}
	}
	return false, nil
}

func isServiceMonitorCRDNotFound(err error) bool {
	return apierrors.IsNotFound(err) || errors.Is(err, memory.ErrCacheNotFound)
}

func waitForServiceMonitorCRD(ctx context.Context, kubeconfig, kubeContext string) error {
	deadline := time.NewTimer(componentTimeout)
	defer deadline.Stop()
	ticker := time.NewTicker(500 * time.Millisecond)
	defer ticker.Stop()

	for {
		available, err := serviceMonitorCRDAvailable(kubeconfig, kubeContext)
		if err == nil && available {
			return nil
		}
		select {
		case <-ctx.Done():
			return fmt.Errorf("waiting for Prometheus Operator ServiceMonitor CRD canceled: %w", ctx.Err())
		case <-deadline.C:
			return fmt.Errorf("timed out waiting for Prometheus Operator ServiceMonitor CRD after %s", componentTimeout)
		case <-ticker.C:
		}
	}
}

func loadComponentValuesWithOrder(baseDir, componentName, env string, envOverridesBase bool) (map[string]interface{}, error) {
	baseFile := filepath.Join(baseDir, "components", componentName, "values", "base.yaml")
	envFile := filepath.Join(baseDir, "components", componentName, "values", fmt.Sprintf("%s.yaml", env))

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

	if !envOverridesBase {
		// Preserve the established ArgoCD bootstrap merge behavior.
		return chartutil.MergeTables(baseVals.AsMap(), envVals.AsMap()), nil
	}
	// MergeTables keeps values from its first argument, so environment values go first.
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
