package flex

import (
	"encoding/json"
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
