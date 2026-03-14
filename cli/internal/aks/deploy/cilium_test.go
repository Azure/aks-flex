package deploy

import (
	"os"
	"path/filepath"
	"testing"
)

func TestKubeconfigAPIServer(t *testing.T) {
	dir := t.TempDir()
	kubeconfig := filepath.Join(dir, "config")
	if err := os.WriteFile(kubeconfig, []byte(`apiVersion: v1
kind: Config
current-context: test
clusters:
- name: test-cluster
  cluster:
    server: https://example.hcp.eastus2.azmk8s.io:443
contexts:
- name: test
  context:
    cluster: test-cluster
    user: test-user
users:
- name: test-user
  user:
    token: test
`), 0o600); err != nil {
		t.Fatalf("write kubeconfig: %v", err)
	}

	host, port, err := kubeconfigAPIServer(kubeconfig)
	if err != nil {
		t.Fatalf("kubeconfigAPIServer returned error: %v", err)
	}
	if host != "example.hcp.eastus2.azmk8s.io" {
		t.Fatalf("unexpected host %q", host)
	}
	if port != "443" {
		t.Fatalf("unexpected port %q", port)
	}
}
