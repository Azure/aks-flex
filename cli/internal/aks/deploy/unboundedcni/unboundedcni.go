package unboundedcni

import (
	"context"
	"embed"
	"fmt"
	"io/fs"
	"os"
	"os/exec"
	"path/filepath"

	"github.com/Azure/aks-flex/plugin/pkg/util/config"
)

//go:embed assets/*
var assets embed.FS

const bundledFolder = "unbounded-cni-0.5.2"

func Preflight() error {
	// check for kubectl
	if _, err := exec.LookPath("kubectl"); err != nil {
		return fmt.Errorf("kubectl not found in PATH, please install kubectl to use --unbounded-cni: %w", err)
	}
	return nil
}

func Deploy(
	ctx context.Context,
	kubeconfigFile string,
	cfg *config.Config,
) error {
	// extract the unbounded CNI assets to a temp directory
	tempDir, err := extractAssets()
	if err != nil {
		return err
	}
	defer os.RemoveAll(tempDir) //nolint:errcheck // best-effort cleanup of temp directory

	baseDir := filepath.Join(tempDir, bundledFolder)

	// deploy the unbounded CNI operator resources in dependency order
	// using kubectl apply -f instead of kustomize
	steps := []struct {
		name string
		path string
	}{
		{"CRDs", filepath.Join(baseDir, "crds")},
		{"namespace", filepath.Join(baseDir, "00-namespace.yaml")},
		{"configmap", filepath.Join(baseDir, "01-configmap.yaml")},
		{"controller", filepath.Join(baseDir, "controller")},
		{"node", filepath.Join(baseDir, "node")},
	}
	for _, step := range steps {
		if err := kubectlApply(ctx, kubeconfigFile, step.path); err != nil {
			return fmt.Errorf("failed to apply unbounded CNI %s: %w", step.name, err)
		}
	}

	// deploy the AKS site and gateway pool resources
	aksDir := filepath.Join(tempDir, "aks")
	if err := kubectlApply(ctx, kubeconfigFile, aksDir); err != nil {
		return fmt.Errorf("failed to apply AKS unbounded CNI resources: %w", err)
	}

	return nil
}

// kubectlApply runs kubectl apply -f against the given path (file or directory)
// using the provided kubeconfig.
func kubectlApply(ctx context.Context, kubeconfigFile, path string) error {
	cmd := exec.CommandContext(ctx, "kubectl", "apply", "-f", path)
	cmd.Stdout = os.Stdout
	cmd.Stderr = os.Stderr
	cmd.Env = append(cmd.Environ(),
		"KUBECONFIG="+kubeconfigFile,
	)
	return cmd.Run()
}

// extractAssets extracts the embedded unbounded CNI assets to a temporary
// directory and returns its path. The caller is responsible for cleaning up
// the returned directory.
func extractAssets() (string, error) {
	tempDir, err := os.MkdirTemp("", "unbounded-cni-*")
	if err != nil {
		return "", fmt.Errorf("failed to create temp directory: %w", err)
	}

	root := "assets"
	if err := fs.WalkDir(assets, root, func(path string, d fs.DirEntry, err error) error {
		if err != nil {
			return err
		}

		relPath, err := filepath.Rel(root, path)
		if err != nil {
			return err
		}
		targetPath := filepath.Join(tempDir, relPath)

		if d.IsDir() {
			return os.MkdirAll(targetPath, 0o755)
		}

		data, err := assets.ReadFile(path)
		if err != nil {
			return fmt.Errorf("failed to read embedded file %s: %w", path, err)
		}

		return os.WriteFile(targetPath, data, 0o644)
	}); err != nil {
		os.RemoveAll(tempDir) //nolint:errcheck // best-effort cleanup on extraction failure
		return "", fmt.Errorf("failed to extract unbounded CNI assets: %w", err)
	}

	return tempDir, nil
}
