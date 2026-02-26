package ipsecvpn

func (p *Peering) Redact() {
	p.GetSpec().GetIpsec().Redact()
}
