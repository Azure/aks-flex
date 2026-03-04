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

	// deploy the unbounded CNI operator through kubectl apply -k
	kustomizeDir := filepath.Join(tempDir, "unbounded-cni-0.0.4")
	cmd := exec.CommandContext(ctx, "kubectl", "apply", "-k", kustomizeDir)
	cmd.Stdout = os.Stdout
	cmd.Stderr = os.Stderr
	cmd.Env = append(cmd.Environ(),
		"KUBECONFIG="+kubeconfigFile,
	)
	if err := cmd.Run(); err != nil {
		return fmt.Errorf("failed to apply unbounded CNI manifests: %w", err)
	}

	// deploy the AKS site and gateway pool resources
	aksDir := filepath.Join(tempDir, "aks")
	cmd = exec.CommandContext(ctx, "kubectl", "apply", "-f", aksDir)
	cmd.Stdout = os.Stdout
	cmd.Stderr = os.Stderr
	cmd.Env = append(cmd.Environ(),
		"KUBECONFIG="+kubeconfigFile,
	)
	if err := cmd.Run(); err != nil {
		return fmt.Errorf("failed to apply AKS unbounded CNI resources: %w", err)
	}

	return nil
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
