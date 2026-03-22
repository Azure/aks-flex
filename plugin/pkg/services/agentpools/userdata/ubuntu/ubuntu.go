package ubuntu

import (
	"strings"

	kubeadmapi "github.com/Azure/aks-flex/plugin/pkg/services/agentpools/api/features/kubeadm"
	"github.com/Azure/aks-flex/plugin/pkg/util/cloudinit"
	"github.com/Azure/aks-flex/plugin/pkg/util/kubeadm"
)

const (
	joinConfigPath = "/root/joinconfig"
	kubeConfigPath = "/root/.kube/config"
)

func UserData(kubeadmConfig *kubeadmapi.Config) (*cloudinit.UserData, error) {
	kubeconfig, err := kubeadm.Kubeconfig(kubeadmConfig)
	if err != nil {
		return nil, err
	}

	joinconfig, err := kubeadm.JoinConfig(kubeadmConfig, kubeConfigPath)
	if err != nil {
		return nil, err
	}

	userdata := &cloudinit.UserData{
		APT: &cloudinit.APT{
			Sources: map[string]*cloudinit.APTSource{
				"kubernetes": {
					Source: "deb https://pkgs.k8s.io/core:/stable:/v1.34/deb/ /",
					KeyID:  "DE15B14486CD377B9E876E1A234654DA9A296436", // curl -sL https://pkgs.k8s.io/core:/stable:/v1.34/deb/Release.key | gpg --show-keys
				},
			},
		},
		PackageUpdate:  true,
		PackageUpgrade: true,
		Packages: []string{
			"containerd",
			"kubeadm",
			"kubelet",
		},
		WriteFiles: []*cloudinit.WriteFile{
			{
				Path:        kubeConfigPath,
				Content:     string(kubeconfig),
				Permissions: "0600",
			}, {
				Path:    joinConfigPath,
				Content: string(joinconfig),
			},
		},
		RunCmd: []any{
			[]string{"set", "-e"},
			strings.Join([]string{
				"mkdir -p /etc/containerd",
				"containerd config default | sed -e '/SystemdCgroup/ s/false/true/' >/etc/containerd/config.toml",
				"systemctl restart containerd.service",
			}, "\n"),
			[]string{"kubeadm", "join", "--config", joinConfigPath},
			[]string{"rm", "-rf", joinConfigPath, kubeConfigPath},
		},
	}

	return userdata, nil
}
