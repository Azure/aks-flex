package flex

import (
	"bytes"
	_ "embed"
	"encoding/json"
	"fmt"
	"maps"
	"strings"
	"text/template"

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

//go:embed assets/bootstrap.sh.tmpl
var bootstrapTmpl string

var bootstrapTemplate = template.Must(template.New("bootstrap.sh").Parse(bootstrapTmpl))

const (
	flexNodeVersion = "v0.0.14-docker2"
	defaultArch     = "amd64"
	defaultKubeVer  = "1.34.2"
)

// Options configures how the flex node userdata is generated.
type Options struct {
	EnableNvidiaGPURuntime bool
	KubeVersion            string
	Arch                   string
	KubeadmConfig          *kubeadmapi.Config
}

// Option is a functional option for [UserData].
type Option func(*Options)

// WithEnableNvidiaGPURuntime configures the containerd Nvidia GPU runtime.
func WithEnableNvidiaGPURuntime(enable bool) Option {
	return func(o *Options) { o.EnableNvidiaGPURuntime = enable }
}

// WithKubeVersion sets the Kubernetes version for the downloaded binaries.
func WithKubeVersion(v string) Option {
	return func(o *Options) { o.KubeVersion = v }
}

// WithArch sets the CPU architecture for the flex node binary (e.g. "amd64", "arm64").
func WithArch(arch string) Option {
	return func(o *Options) { o.Arch = arch }
}

// WithKubeadmConfig sets the kubeadm join configuration.
func WithKubeadmConfig(cfg *kubeadmapi.Config) Option {
	return func(o *Options) { o.KubeadmConfig = cfg }
}

func defaultOptions() *Options {
	return &Options{
		KubeVersion: defaultKubeVer,
		Arch:        defaultArch,
	}
}

// supportedArchs is the set of CPU architectures for which flex node binaries
// are published.
var supportedArchs = map[string]bool{
	"amd64": true,
	"arm64": true,
}

// validate performs least-effort validation on the options. This is intentionally
// minimal to catch obvious mistakes for ad-hoc values; callers should perform
// more thorough validation beforehand.
func (o *Options) validate() error {
	if !supportedArchs[o.Arch] {
		return fmt.Errorf("unsupported arch %q, supported: amd64, arm64", o.Arch)
	}
	o.KubeVersion = strings.TrimPrefix(o.KubeVersion, "v")
	if o.KubeVersion == "" {
		return fmt.Errorf("kube version must not be empty")
	}
	return nil
}

// bootstrapParams holds the template parameters for the bootstrap script.
type bootstrapParams struct {
	Arch    string
	Version string
}

func flexMetadata[T proto.Message](name string) *api.Metadata {
	var zero T
	typeName := string(zero.ProtoReflect().Descriptor().FullName())
	return api.Metadata_builder{
		Type: proto.String(typeName),
		Name: proto.String(name),
	}.Build()
}

func resolveFlexComponentConfigs(
	enableNvidiaGPURuntime bool,
	kubeVersion string,
	kubeadmConfig *kubeadmapi.Config,
) ([]byte, error) {
	startCRISpecBuilder := cri.StartContainerdServiceSpec_builder{}
	if enableNvidiaGPURuntime {
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
		linux.DisableDocker_builder{
			Metadata: flexMetadata[*linux.DisableDocker]("disable-docker"),
			Spec:     linux.DisableDockerSpec_builder{}.Build(),
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
		linux.ConfigureIPTables_builder{
			Metadata: flexMetadata[*linux.ConfigureIPTables]("configure-iptables"),
			Spec:     linux.ConfigureIPTablesSpec_builder{}.Build(),
		}.Build(),
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

func renderBootstrapScript(arch string) (string, error) {
	var buf bytes.Buffer
	if err := bootstrapTemplate.Execute(&buf, bootstrapParams{
		Arch:    arch,
		Version: flexNodeVersion,
	}); err != nil {
		return "", fmt.Errorf("rendering bootstrap script: %w", err)
	}
	return buf.String(), nil
}

func UserData(opts ...Option) (*cloudinit.UserData, error) {
	o := defaultOptions()
	for _, opt := range opts {
		opt(o)
	}

	if err := o.validate(); err != nil {
		return nil, err
	}

	componentConfigsJSON, err := resolveFlexComponentConfigs(o.EnableNvidiaGPURuntime, o.KubeVersion, o.KubeadmConfig)
	if err != nil {
		return nil, err
	}

	bootstrapScript, err := renderBootstrapScript(o.Arch)
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
			bootstrapScript,
		},
	}

	return userdata, nil
}
