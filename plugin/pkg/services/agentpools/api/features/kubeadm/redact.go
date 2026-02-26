package kubeadm

func (config *Config) Redact() {
	config.ClearToken()
}
