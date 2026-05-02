package k8s

import (
	"fmt"
	"os"
	"path/filepath"

	"k8s.io/client-go/dynamic"
	"k8s.io/client-go/rest"
	"k8s.io/client-go/tools/clientcmd"
)

// getKubeConfig implements the standard client-go cascading fallback
func getKubeConfig() (*rest.Config, error) {
	// 1. Try In-Cluster Config (This succeeds if running inside a K8s Pod)
	config, err := rest.InClusterConfig()
	if err == nil {
		return config, nil
	}

	// 2. Try the KUBECONFIG environment variable
	kubeconfigPath := os.Getenv("KUBECONFIG")

	// 3. Fallback to the default ~/.kube/config
	if kubeconfigPath == "" {
		homeDir, err := os.UserHomeDir()
		if err != nil {
			return nil, fmt.Errorf("could not find user home directory: %w", err)
		}
		kubeconfigPath = filepath.Join(homeDir, ".kube", "config")
	}

	// Build the out-of-cluster config
	return clientcmd.BuildConfigFromFlags("", kubeconfigPath)
}

func GetDynamicClient() (dynamic.Interface, error) {
	config, err := getKubeConfig()
	if err != nil {
		return nil, fmt.Errorf("failed to get kubeconfig: %w", err)
	}

	dynClient, err := dynamic.NewForConfig(config)
	if err != nil {
		return nil, fmt.Errorf("failed to create dynamic client: %w", err)
	}

	return dynClient, nil
}
