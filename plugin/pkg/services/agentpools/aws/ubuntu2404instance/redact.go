package ubuntu2404instance

func (ap *AgentPool) Redact() {
	ap.GetSpec().GetKubeadm().Redact()
}

func (i *Instance) Redact() {
}
