package config

import (
	"os"
	"path/filepath"
	"testing"
)

func TestDefaultSubscriptionIDHonorsAzureConfigDir(t *testing.T) {
	t.Setenv("AZURE_SUBSCRIPTION_ID", "")
	home := t.TempDir()
	t.Setenv("HOME", home)

	azureConfigDir := filepath.Join(t.TempDir(), "azure-custom")
	if err := os.MkdirAll(azureConfigDir, 0o755); err != nil {
		t.Fatalf("mkdir azureConfigDir: %v", err)
	}
	if err := os.WriteFile(filepath.Join(azureConfigDir, "clouds.config"), []byte("[AzureCloud]\nsubscription = 11111111-2222-3333-4444-555555555555\n"), 0o600); err != nil {
		t.Fatalf("write clouds.config: %v", err)
	}
	t.Setenv("AZURE_CONFIG_DIR", azureConfigDir)

	got := defaultSubscriptionID()
	if got != "11111111-2222-3333-4444-555555555555" {
		t.Fatalf("unexpected subscription id %q", got)
	}
}
