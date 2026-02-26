package flex

import (
	"encoding/json"
	"strings"

	kubeadmapi "github.com/Azure/aks-flex/plugin/pkg/services/agentpools/api/features/kubeadm"
	"github.com/Azure/aks-flex/plugin/pkg/util/cloudinit"
)

func resolveFlexComponentConfigs(
	kubeVersion string,
	kubeadmConfig *kubeadmapi.Config,
) []any {
	kubletConfig := map[string]any{
		"bootstrap_auth_info": map[string]any{
			"token": kubeadmConfig.GetToken(),
		},
		"node_labels": kubeadmConfig.GetNodeLabels(),
	}
	if kubeadmConfig.HasNodeIp() {
		kubletConfig["node_ip"] = kubeadmConfig.GetNodeIp()
	}

	kubeadmNodeJoin := map[string]any{
		"metadata": map[string]any{
			"type": "aks.flex.components.kubeadm.v20260301.KubadmNodeJoin", // FIXME: typo
			"name": "kubeadm-node-join",
		},
		"spec": map[string]any{
			"control_plane": map[string]any{
				"server":                     kubeadmConfig.GetServer(),
				"certificate_authority_data": kubeadmConfig.GetCertificateAuthorityData(),
			},
			"kubelet": kubletConfig,
		},
	}

	return []any{
		json.RawMessage(`
{
	"metadata": {
		"type": "aks.flex.components.linux.v20260301.ConfigureBaseOS",
		"name": "configure-base-os"
	},
	"spec": {}
}
		`),
		json.RawMessage(`
{
	"metadata": {
		"type": "aks.flex.components.cri.v20260301.DownloadCRIBinaries",
		"name": "download-cri-binaries"
	},
	"spec": {
		"containerd_version": "2.0.4",
		"runc_version": "1.2.5"
	}
}
`),
		map[string]any{
			"metadata": map[string]any{
				"type": "aks.flex.components.kubebins.v20260301.DownloadKubeBinaries",
				"name": "download-kube-binaries",
			},
			"spec": map[string]any{
				"kubernetes_version": kubeVersion,
			},
		},
		json.RawMessage(`
{
	"metadata": {
		"type": "aks.flex.components.cri.v20260301.StartContainerdService",
		"name": "start-containerd-service"
	},
	"spec": {}
}
`),
		kubeadmNodeJoin,
	}
}

func UserData(kubeVersion string, kubeadmConfig *kubeadmapi.Config) (*cloudinit.UserData, error) {
	componentConfigs := resolveFlexComponentConfigs(kubeVersion, kubeadmConfig)
	componentConfigsJSON, err := json.MarshalIndent(componentConfigs, "", "  ")
	if err != nil {
		return nil, err
	}

	userdata := &cloudinit.UserData{
		PackageUpdate: true,
		Packages: []string{
			"curl", // for downloading the initial bootstrap binary
		},
		WriteFiles: []*cloudinit.WriteFile{
			{
				Path:        "/tmp/flex-config.json",
				Content:     string(componentConfigsJSON),
				Permissions: "0644",
			},
		},
		RunCmd: []any{
			[]string{"set", "-e"},
			strings.Join([]string{
				"mkdir -p /tmp/flex",
				"curl -L -o /tmp/flex/aks-flex-node https://bahestoragetest.z13.web.core.windows.net/flex/aks-flex-node-linux-amd64",
				"chmod +x /tmp/flex/aks-flex-node",
				"/tmp/flex/aks-flex-node apply -f /tmp/flex-config.json",
			}, "\n"),
		},
	}

	return userdata, nil
}
