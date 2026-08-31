package k8s

import (
	"fmt"

	"k8s.io/client-go/dynamic"
	"k8s.io/client-go/kubernetes"
	"k8s.io/client-go/tools/clientcmd"
)

// Client wraps the Kubernetes clientset and dynamic client.
type Client struct {
	Clientset     kubernetes.Interface
	DynamicClient dynamic.Interface
}

// NewClient creates a Kubernetes client from the given kubeconfig and context.
// If kubeconfig is empty, it uses the default loading rules.
// If context is empty, it uses the current context.
func NewClient(kubeconfig, context string) (*Client, error) {
	config, err := clientConfig(kubeconfig, context).ClientConfig()
	if err != nil {
		return nil, wrapKubeconfigError(err, kubeconfig, context)
	}

	clientset, err := kubernetes.NewForConfig(config)
	if err != nil {
		return nil, wrapClusterConnectionError(err)
	}

	dynClient, err := dynamic.NewForConfig(config)
	if err != nil {
		return nil, wrapClusterConnectionError(err)
	}

	return &Client{
		Clientset:     clientset,
		DynamicClient: dynClient,
	}, nil
}

// clientConfig builds the kubeconfig loader shared by NewClient and ResolveContext,
// so both resolve the same context for the same arguments.
func clientConfig(kubeconfig, context string) clientcmd.ClientConfig {
	loadingRules := clientcmd.NewDefaultClientConfigLoadingRules()
	if kubeconfig != "" {
		loadingRules.ExplicitPath = kubeconfig
	}

	configOverrides := &clientcmd.ConfigOverrides{}
	if context != "" {
		configOverrides.CurrentContext = context
	}

	return clientcmd.NewNonInteractiveDeferredLoadingClientConfig(loadingRules, configOverrides)
}

// ResolveContext returns the kubeconfig context that NewClient would connect to
// for the same arguments: the explicit override when given, otherwise the
// current context of the resolved kubeconfig.
func ResolveContext(kubeconfig, context string) (string, error) {
	if context != "" {
		return context, nil
	}

	raw, err := clientConfig(kubeconfig, context).RawConfig()
	if err != nil {
		return "", wrapKubeconfigError(err, kubeconfig, context)
	}
	if raw.CurrentContext == "" {
		return "", fmt.Errorf("no current context set in kubeconfig\n  hint: select a context with: kubectl config use-context <name>\n  tip: or pass --context explicitly")
	}
	return raw.CurrentContext, nil
}

// wrapKubeconfigError enhances error messages for kubeconfig issues.
func wrapKubeconfigError(err error, kubeconfig, context string) error {
	if kubeconfig != "" {
		return fmt.Errorf("failed to load kubeconfig %s: %w\n  hint: verify the file exists and is readable", kubeconfig, err)
	}
	if context != "" {
		return fmt.Errorf("context %s not found in kubeconfig: %w\n  hint: verify the context with: kubectl config get-contexts", context, err)
	}
	return fmt.Errorf("failed to load kubeconfig: %w\n  hint: ensure kubectl is configured. Check: kubectl config view", err)
}

// wrapClusterConnectionError enhances error messages for cluster connection issues.
func wrapClusterConnectionError(err error) error {
	return fmt.Errorf("failed to connect to cluster: %w\n  hint: verify cluster is accessible and kubeconfig credentials are valid\n  tip: try: kubectl cluster-info", err)
}
