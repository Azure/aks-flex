package k8s

import (
	"fmt"
	"os"

	"k8s.io/client-go/rest"
)

func retrieveClusterCAInsidePod() ([]byte, error) {
	ca, err := os.ReadFile("/var/run/secrets/kubernetes.io/serviceaccount/ca.crt")
	if err != nil {
		return nil, fmt.Errorf("reading cluster CA from service account: %w", err)
	}
	return ca, nil
}

// RetrieveClusterCA retrieves the cluster CA certificate in the following order:
// 1. from the provided optional rest.Config
// 2. from the service account volume mount (if running inside a pod)
func RetrieveClusterCA(restConfig *rest.Config) ([]byte, error) {
	if restConfig != nil && restConfig.CAData != nil {
		return restConfig.CAData, nil
	}
	return retrieveClusterCAInsidePod()
}
