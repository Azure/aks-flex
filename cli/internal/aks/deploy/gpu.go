package deploy

import (
	"context"
	_ "embed"
	"fmt"
	"log"
	"os"
	"os/exec"
)

//go:embed assets/dra-driver-values.yaml
var draDriverValuesYAML []byte

func preflightDRADriver() error {
	_, err := exec.LookPath("helm")
	if err != nil {
		return fmt.Errorf("helm not found in PATH, please install Helm to use --dra-driver: %w", err)
	}

	return nil
}

func preflightGPUOperator() error {
	_, err := exec.LookPath("helm")
	if err != nil {
		return fmt.Errorf("helm not found in PATH, please install Helm to use --gpu-operator: %w", err)
	}

	return nil
}

func preflightGPUDevicePlugin() error {
	_, err := exec.LookPath("helm")
	if err != nil {
		return fmt.Errorf("helm not found in PATH, please install Helm to use --gpu-device-plugin: %w", err)
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

// installGPUDevicePlugin installs the NVIDIA GPU Device Plugin via Helm.
func installGPUDevicePlugin(ctx context.Context) error {
	log.Print("Installing NVIDIA GPU Device Plugin...")

	commands := []struct {
		name string
		args []string
	}{
		{"helm", []string{"repo", "add", "nvdp", "https://nvidia.github.io/k8s-device-plugin"}},
		{"helm", []string{"repo", "update"}},
		{"helm", []string{"upgrade", "--install", "--wait", "nvidia-device-plugin", "-n", "nvidia-device-plugin", "--create-namespace", "nvdp/nvidia-device-plugin", "--set", "failOnInitError=false", "--set", "affinity=null"}},
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

	log.Print("NVIDIA GPU Device Plugin installed successfully")
	return nil
}

// installDRADriver installs the NVIDIA DRA Driver via Helm.
func installDRADriver(ctx context.Context) error {
	log.Print("Installing NVIDIA DRA Driver...")

	valuesFile, err := os.CreateTemp("", "dra-driver-values-*.yaml")
	if err != nil {
		return fmt.Errorf("failed to create temp values file: %w", err)
	}
	defer os.Remove(valuesFile.Name())

	if _, err := valuesFile.Write(draDriverValuesYAML); err != nil {
		return fmt.Errorf("failed to write DRA driver values: %w", err)
	}
	if err := valuesFile.Close(); err != nil {
		return fmt.Errorf("failed to close DRA driver values file: %w", err)
	}

	commands := []struct {
		name string
		args []string
	}{
		{"helm", []string{"repo", "add", "nvidia", "https://helm.ngc.nvidia.com/nvidia"}},
		{"helm", []string{"repo", "update"}},
		{"helm", []string{"upgrade", "--install", "dra-driver", "nvidia/nvidia-dra-driver-gpu",
			"--version", "25.12.0",
			"--create-namespace",
			"--namespace", "nvidia",
			"-f", valuesFile.Name(),
			"--wait",
		}},
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

	log.Print("NVIDIA DRA Driver installed successfully")
	return nil
}
