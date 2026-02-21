package cloudinit

import (
	"bytes"
	"compress/gzip"

	"go.yaml.in/yaml/v3"
)

// https://cloudinit.readthedocs.io/en/latest/reference/modules.html

type UserData struct {
	APT                    *APT         `yaml:"apt,omitempty"`
	PackageUpdate          bool         `yaml:"package_update,omitempty"`
	PackageUpgrade         bool         `yaml:"package_upgrade,omitempty"`
	Packages               []string     `yaml:"packages,omitempty"`
	FQDN                   string       `yaml:"fqdn,omitempty"`
	PreferFQDNOverHostname bool         `yaml:"prefer_fqdn_over_hostname,omitempty"`
	SSHAuthorizedKeys      []string     `yaml:"ssh_authorized_keys,omitempty"`
	WriteFiles             []*WriteFile `yaml:"write_files,omitempty"`
	RunCmd                 []any        `yaml:"runcmd,omitempty"`
}

type APT struct {
	Sources map[string]*APTSource `yaml:"sources,omitempty"`
}

type APTSource struct {
	Source string `yaml:"source,omitempty"`
	Key    string `yaml:"key,omitempty"`
	KeyID  string `yaml:"keyid,omitempty"`
}

type WriteFile struct {
	Path        string `yaml:"path,omitempty"`
	Content     string `yaml:"content,omitempty"`
	Permissions string `yaml:"permissions,omitempty"`
	Append      bool   `yaml:"append,omitempty"`
}

func Unmarshal(b []byte) (ud *UserData, err error) {
	d := yaml.NewDecoder(bytes.NewReader(b))
	d.KnownFields(true)

	err = d.Decode(&ud)
	if err != nil {
		return nil, err
	}

	return ud, nil
}

func (userData *UserData) Marshal() ([]byte, error) {
	b, err := yaml.Marshal(userData)
	if err != nil {
		return nil, err
	}

	return append([]byte("#cloud-config\n"), b...), nil
}

func (userData *UserData) Gzip() ([]byte, error) {
	b, err := userData.Marshal()
	if err != nil {
		return nil, err
	}

	buf := &bytes.Buffer{}
	w := gzip.NewWriter(buf)

	if _, err := w.Write(b); err != nil {
		return nil, err
	}

	if err := w.Close(); err != nil {
		return nil, err
	}

	return buf.Bytes(), nil
}
