package arc

func (config *Config) Redact() {
	config.ClearServicePrincipalSecret()
}
