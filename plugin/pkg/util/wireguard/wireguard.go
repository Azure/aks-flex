package wireguard

import (
	"crypto/rand"
	"encoding/base64"
	"fmt"
	"strings"

	"golang.org/x/crypto/curve25519"
)

// KeyPair represents a WireGuard key pair.
type KeyPair struct {
	PrivateKey string
	PublicKey  string
}

// GenerateKeyPair generates a new WireGuard key pair.
// ref: https://github.com/WireGuard/wireguard-tools/blob/0b7d9821f2815973a2930ace28a3f73c205d0e5c/src/genkey.c#L75
func GenerateKeyPair() (*KeyPair, error) {
	// Generate 32 random bytes for private key
	var privateKey [32]byte
	if _, err := rand.Read(privateKey[:]); err != nil {
		return nil, fmt.Errorf("failed to generate random bytes: %w", err)
	}

	// Clamp the private key per WireGuard spec (Curve25519)
	privateKey[0] &= 248
	privateKey[31] &= 127
	privateKey[31] |= 64

	// Derive public key from private key
	var publicKey [32]byte
	curve25519.ScalarBaseMult(&publicKey, &privateKey)

	return &KeyPair{
		PrivateKey: base64.StdEncoding.EncodeToString(privateKey[:]),
		PublicKey:  base64.StdEncoding.EncodeToString(publicKey[:]),
	}, nil
}

// Peer represents a WireGuard peer configuration.
type Peer struct {
	PublicKey           string
	Endpoint            string // empty for server waiting for client
	AllowedIPs          []string
	PersistentKeepalive int // seconds, 0 to disable
}

// Config holds WireGuard configuration for an interface.
type Config struct {
	// Interface settings
	Address    string // e.g., "100.96.0.1/32"
	ListenPort int    // e.g., 51820 (0 for client mode)
	PrivateKey string

	// Peers configuration
	Peers []Peer

	// Routes to add via PostUp (e.g., ["172.16.0.0/16", "172.18.0.0/16"])
	Routes []string
}

// GenerateConfig generates a WireGuard configuration file content.
func GenerateConfig(cfg *Config) string {
	var sb strings.Builder

	sb.WriteString(fmt.Sprintf("[Interface]\nAddress = %s\nPrivateKey = %s\n", cfg.Address, cfg.PrivateKey))

	if cfg.ListenPort > 0 {
		sb.WriteString(fmt.Sprintf("ListenPort = %d\n", cfg.ListenPort))
	}

	// Add IP forwarding for gateway mode
	if cfg.ListenPort > 0 {
		sb.WriteString(`PostUp = sysctl -w net.ipv4.ip_forward=1
PostUp = iptables -A FORWARD -i wg0 -j ACCEPT
PostUp = iptables -A FORWARD -o wg0 -j ACCEPT
PostDown = iptables -D FORWARD -i wg0 -j ACCEPT
PostDown = iptables -D FORWARD -o wg0 -j ACCEPT
`)
	}

	// Add custom routes via PostUp/PostDown
	for _, route := range cfg.Routes {
		sb.WriteString(fmt.Sprintf("PostUp = ip route add %s dev wg0\n", route))
		sb.WriteString(fmt.Sprintf("PostDown = ip route del %s dev wg0 || true\n", route))
	}

	// Add peers
	for _, peer := range cfg.Peers {
		sb.WriteString(fmt.Sprintf("\n[Peer]\nPublicKey = %s\n", peer.PublicKey))

		if peer.Endpoint != "" {
			sb.WriteString(fmt.Sprintf("Endpoint = %s\n", peer.Endpoint))
		}

		if len(peer.AllowedIPs) > 0 {
			sb.WriteString("AllowedIPs = ")
			sb.WriteString(strings.Join(peer.AllowedIPs, ", "))
			sb.WriteString("\n")
		}

		if peer.PersistentKeepalive > 0 {
			sb.WriteString(fmt.Sprintf("PersistentKeepalive = %d\n", peer.PersistentKeepalive))
		}
	}

	return sb.String()
}
