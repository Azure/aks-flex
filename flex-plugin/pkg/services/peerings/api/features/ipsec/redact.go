package ipsec

func (config *Config) Redact() {
	config.ClearPreSharedKey()
}
