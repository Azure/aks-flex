package config

import (
	"os"
	"path/filepath"
	"runtime"
	"testing"
)

func TestDefaultSubscriptionIDUsesAZCLI(t *testing.T) {
	if runtime.GOOS == "windows" {
		t.Skip("skipping shell-script fake az on Windows")
	}
	t.Setenv("AZURE_SUBSCRIPTION_ID", "")

	dir := t.TempDir()
	fakeAZ := filepath.Join(dir, "az")
	if err := os.WriteFile(fakeAZ, []byte("#!/bin/sh\necho '11111111-2222-3333-4444-555555555555'\n"), 0o755); err != nil {
		t.Fatalf("write fake az: %v", err)
	}
	t.Setenv("PATH", dir+string(os.PathListSeparator)+os.Getenv("PATH"))

	got := defaultSubscriptionID()
	if got != "11111111-2222-3333-4444-555555555555" {
		t.Fatalf("unexpected subscription id %q", got)
	}
}

func TestAzureTenantIDUsesAZCLI(t *testing.T) {
	if runtime.GOOS == "windows" {
		t.Skip("skipping shell-script fake az on Windows")
	}

	dir := t.TempDir()
	fakeAZ := filepath.Join(dir, "az")
	if err := os.WriteFile(fakeAZ, []byte("#!/bin/sh\necho 'tenant-from-az'\n"), 0o755); err != nil {
		t.Fatalf("write fake az: %v", err)
	}
	t.Setenv("PATH", dir+string(os.PathListSeparator)+os.Getenv("PATH"))

	if got := AzureTenantID(); got != "tenant-from-az" {
		t.Fatalf("unexpected tenant id %q", got)
	}
}
