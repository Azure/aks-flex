package ubuntu2404vmss

func (ap *AgentPool) Redact() {
	ap.GetSpec().GetKubeadm().Redact()
}

func (i *Instance) Redact() {
}
