package configcmd

import (
	"os"
	"path/filepath"
	"testing"
)

func TestAzureConfigTenantIDUsesAzureConfigDirProfile(t *testing.T) {
	home := t.TempDir()
	t.Setenv("HOME", home)
	azureConfigDir := filepath.Join(t.TempDir(), "azure-profile")
	if err := os.MkdirAll(azureConfigDir, 0o755); err != nil {
		t.Fatalf("mkdir azure config dir: %v", err)
	}
	if err := os.WriteFile(filepath.Join(azureConfigDir, "azureProfile.json"), []byte(`{"subscriptions":[{"id":"sub","isDefault":true,"tenantId":"tenant-123"}]}`), 0o600); err != nil {
		t.Fatalf("write azureProfile.json: %v", err)
	}
	t.Setenv("AZURE_CONFIG_DIR", azureConfigDir)

	if got := azureConfigTenantID(); got != "tenant-123" {
		t.Fatalf("unexpected tenant id %q", got)
	}
}
