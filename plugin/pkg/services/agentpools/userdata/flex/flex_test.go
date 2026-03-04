package flex

import (
	"encoding/json"
	"strings"
	"testing"

	kubeadm "github.com/Azure/aks-flex/plugin/pkg/services/agentpools/api/features/kubeadm"
)

func Test_resolveFlexComponentConfigs_basic(t *testing.T) {
	kubeadmSpec := kubeadm.Config_builder{}.Build()

	b, err := resolveFlexComponentConfigs(true, "1.33.3", kubeadmSpec)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	t.Log(string(b))

	// do a round trip to verify the generated config is valid
	var x []map[string]any
	if err := json.Unmarshal(b, &x); err != nil {
		t.Fatalf("failed to unmarshal generated config: %v", err)
	}
}

func TestUserData_defaults(t *testing.T) {
	kubeadmSpec := kubeadm.Config_builder{}.Build()

	ud, err := UserData(WithKubeadmConfig(kubeadmSpec))
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	b, err := ud.Marshal()
	if err != nil {
		t.Fatalf("failed to marshal userdata: %v", err)
	}

	content := string(b)
	// defaults should produce amd64 binary URL
	if !strings.Contains(content, "amd64") {
		t.Error("expected default arch amd64 in userdata")
	}
}

func TestUserData_arm64(t *testing.T) {
	kubeadmSpec := kubeadm.Config_builder{}.Build()

	ud, err := UserData(
		WithArch("arm64"),
		WithKubeadmConfig(kubeadmSpec),
	)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	b, err := ud.Marshal()
	if err != nil {
		t.Fatalf("failed to marshal userdata: %v", err)
	}

	content := string(b)
	if !strings.Contains(content, "arm64") {
		t.Error("expected arm64 in userdata")
	}
	if strings.Contains(content, "amd64") {
		t.Error("unexpected amd64 in userdata when arm64 was specified")
	}
}

func TestUserData_invalidArch(t *testing.T) {
	kubeadmSpec := kubeadm.Config_builder{}.Build()

	_, err := UserData(
		WithArch("mips64"),
		WithKubeadmConfig(kubeadmSpec),
	)
	if err == nil {
		t.Fatal("expected error for unsupported arch")
	}
	if !strings.Contains(err.Error(), "unsupported arch") {
		t.Errorf("unexpected error message: %v", err)
	}
}

func TestUserData_invalidKubeVersion(t *testing.T) {
	kubeadmSpec := kubeadm.Config_builder{}.Build()

	_, err := UserData(
		WithKubeVersion(""),
		WithKubeadmConfig(kubeadmSpec),
	)
	if err == nil {
		t.Fatal("expected error for empty kube version")
	}
	if !strings.Contains(err.Error(), "must not be empty") {
		t.Errorf("unexpected error message: %v", err)
	}
}

func TestUserData_trimsLeadingV(t *testing.T) {
	kubeadmSpec := kubeadm.Config_builder{}.Build()

	ud, err := UserData(
		WithKubeVersion("v1.33.3"),
		WithKubeadmConfig(kubeadmSpec),
	)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	b, err := ud.Marshal()
	if err != nil {
		t.Fatalf("failed to marshal userdata: %v", err)
	}

	content := string(b)
	if strings.Contains(content, "v1.33.3") {
		t.Error("expected leading 'v' to be trimmed from kube version")
	}
	if !strings.Contains(content, "1.33.3") {
		t.Error("expected kube version 1.33.3 in userdata")
	}
}

func TestUserData_preReleaseKubeVersion(t *testing.T) {
	kubeadmSpec := kubeadm.Config_builder{}.Build()

	_, err := UserData(
		WithKubeVersion("1.33.0-rc.1"),
		WithKubeadmConfig(kubeadmSpec),
	)
	if err != nil {
		t.Fatalf("expected pre-release kube version to be valid, got: %v", err)
	}
}
