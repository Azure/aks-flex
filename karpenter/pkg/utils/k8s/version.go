package k8s

import (
	"fmt"

	"k8s.io/client-go/discovery"
	"k8s.io/client-go/rest"
)

// RetrieveClusterVersion returns the Kubernetes cluster version (e.g. "v1.30.2")
// by querying the API server's /version endpoint via a discovery client.
func RetrieveClusterVersion(restConfig *rest.Config) (string, error) {
	discoveryClient, err := discovery.NewDiscoveryClientForConfig(restConfig)
	if err != nil {
		return "", fmt.Errorf("creating discovery client: %w", err)
	}
	serverVersion, err := discoveryClient.ServerVersion()
	if err != nil {
		return "", fmt.Errorf("retrieving server version: %w", err)
	}
	return serverVersion.GitVersion, nil
}
