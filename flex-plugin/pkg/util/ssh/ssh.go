package ssh

import (
	"errors"
	"os"
	"path/filepath"
)

func PublicKey() ([]byte, error) {
	for _, name := range []string{
		filepath.Join(os.Getenv("HOME"), ".ssh/id_ed25519.pub"),
		filepath.Join(os.Getenv("HOME"), ".ssh/id_rsa.pub"),
	} {
		b, err := os.ReadFile(name)
		if err == nil {
			return b, nil
		}
	}

	return nil, errors.New("public key not found")
}
