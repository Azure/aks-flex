package flex

import (
	"encoding/json"
	"maps"
	"strings"

	"github.com/Azure/AKSFlexNode/components/api"
	"github.com/Azure/AKSFlexNode/components/cri"
	"github.com/Azure/AKSFlexNode/components/kubeadm"
	"github.com/Azure/AKSFlexNode/components/kubebins"
	"github.com/Azure/AKSFlexNode/components/linux"
	"google.golang.org/protobuf/encoding/protojson"
	"google.golang.org/protobuf/proto"

	kubeadmapi "github.com/Azure/aks-flex/plugin/pkg/services/agentpools/api/features/kubeadm"
	"github.com/Azure/aks-flex/plugin/pkg/util/cloudinit"
)

func flexMetadata[T proto.Message](name string) *api.Metadata {
	var zero T
	typeName := string(zero.ProtoReflect().Descriptor().FullName())
	return api.Metadata_builder{
		Type: proto.String(typeName),
		Name: proto.String(name),
	}.Build()
}

func resolveFlexComponentConfigs(
	hasGPU bool,
	kubeVersion string,
	kubeadmConfig *kubeadmapi.Config,
) ([]byte, error) {
	startCRISpecBuilder := cri.StartContainerdServiceSpec_builder{}
	if hasGPU {
		startCRISpecBuilder.GpuConfig = cri.GPUConfig_builder{
			NvidiaRuntime: cri.NvidiaRuntime_builder{}.Build(),
		}.Build()
	}
	startCRI := cri.StartContainerdService_builder{
		Metadata: flexMetadata[*cri.StartContainerdService]("start-cri"),
		Spec:     startCRISpecBuilder.Build(),
	}.Build()

	kubletConfig := kubeadm.Kubelet_builder{
		BootstrapAuthInfo: kubeadm.NodeAuthInfo_builder{
			Token: proto.String(kubeadmConfig.GetToken()),
		}.Build(),
		NodeLabels: maps.Clone(kubeadmConfig.GetNodeLabels()),
	}.Build()
	if nodeIP := kubeadmConfig.GetNodeIp(); nodeIP != "" {
		kubletConfig.SetNodeIp(nodeIP)
	}
	kubeadmNodeJoin := kubeadm.KubeadmNodeJoin_builder{
		Metadata: flexMetadata[*kubeadm.KubeadmNodeJoin]("kubeadm-node-join"),
		Spec: kubeadm.KubeadmNodeJoinSpec_builder{
			ControlPlane: kubeadm.ControlPlane_builder{
				Server:                   proto.String(kubeadmConfig.GetServer()),
				CertificateAuthorityData: kubeadmConfig.GetCertificateAuthorityData(),
			}.Build(),
			Kubelet: kubletConfig,
		}.Build(),
	}.Build()

	steps := []proto.Message{
		linux.ConfigureBaseOS_builder{
			Metadata: flexMetadata[*linux.ConfigureBaseOS]("configure-base-os"),
			Spec:     linux.ConfigureBaseOSSpec_builder{}.Build(),
		}.Build(),
		cri.DownloadCRIBinaries_builder{
			Metadata: flexMetadata[*cri.DownloadCRIBinaries]("download-cri-binaries"),
			Spec: cri.DownloadCRIBinariesSpec_builder{
				ContainerdVersion: proto.String("2.0.4"),
				RuncVersion:       proto.String("1.2.5"),
			}.Build(),
		}.Build(),
		kubebins.DownloadKubeBinaries_builder{
			Metadata: flexMetadata[*kubebins.DownloadKubeBinaries]("download-kube-binaries"),
			Spec: kubebins.DownloadKubeBinariesSpec_builder{
				KubernetesVersion: proto.String(kubeVersion),
			}.Build(),
		}.Build(),
		startCRI,
		kubeadmNodeJoin,
	}
	marshalOpts := protojson.MarshalOptions{
		Multiline: true,
		Indent:    "  ",
	}
	var xs []json.RawMessage
	for _, step := range steps {
		var err error
		b, err := marshalOpts.Marshal(step)
		if err != nil {
			return nil, err
		}
		xs = append(xs, b)
	}
	b, err := json.Marshal(xs)
	if err != nil {
		return nil, err
	}

	return b, nil
}

func UserData(hasGPU bool, kubeVersion string, kubeadmConfig *kubeadmapi.Config) (*cloudinit.UserData, error) {
	componentConfigsJSON, err := resolveFlexComponentConfigs(hasGPU, kubeVersion, kubeadmConfig)
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
				// TODO: this should be overridable
				"curl -L -o /tmp/flex/aks-flex-node-linux-amd64.tar.gz https://github.com/Azure/AKSFlexNode/releases/download/v0.0.12/aks-flex-node-linux-amd64.tar.gz",
				"tar -xzf /tmp/flex/aks-flex-node-linux-amd64.tar.gz -C /tmp/flex",
				"chmod +x /tmp/flex/aks-flex-node",
				"/tmp/flex/aks-flex-node apply -f /tmp/flex-config.json",
			}, "\n"),
		},
	}

	return userdata, nil
}
