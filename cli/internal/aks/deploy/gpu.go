package deploy

import (
	"context"
	"fmt"
	"log"
	"os/exec"
)

func preflightGPUOperator() error {
	_, err := exec.LookPath("helm")
	if err != nil {
		return fmt.Errorf("helm not found in PATH, please install Helm to use --enable-gpu-operator: %w", err)
	}

	return nil
}

// installGPUOperator installs the NVIDIA GPU Operator via Helm.
func installGPUOperator(ctx context.Context) error {
	log.Print("Installing NVIDIA GPU Operator...")

	commands := []struct {
		name string
		args []string
	}{
		{"helm", []string{"repo", "add", "nvidia", "https://helm.ngc.nvidia.com/nvidia"}},
		{"helm", []string{"repo", "update"}},
		{"helm", []string{"upgrade", "--install", "--wait", "gpu-operator", "-n", "gpu-operator", "--create-namespace", "nvidia/gpu-operator"}},
	}

	for _, c := range commands {
		cmd := exec.CommandContext(ctx, c.name, c.args...)
		cmd.Stdout = log.Writer()
		cmd.Stderr = log.Writer()
		log.Printf("  Running: %s %v", c.name, c.args)
		if err := cmd.Run(); err != nil {
			return fmt.Errorf("failed to run %s %v: %w", c.name, c.args, err)
		}
	}

	log.Print("NVIDIA GPU Operator installed successfully")
	return nil
}
