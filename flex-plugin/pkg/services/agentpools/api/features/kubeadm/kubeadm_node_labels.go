package kubeadm

import "maps"

func (x *Config) AddNodeLabels(extra map[string]string) {
	labels := x.GetNodeLabels()
	if labels == nil {
		labels = map[string]string{}
	}
	maps.Copy(labels, extra)
	x.SetNodeLabels(labels)
}
