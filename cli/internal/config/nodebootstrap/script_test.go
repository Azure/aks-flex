package nodebootstrap

import (
	"strings"
	"testing"

	"github.com/Azure/aks-flex/plugin/pkg/util/cloudinit"
)

func Test_marshalScript_basic(t *testing.T) {
	ud := &cloudinit.UserData{
		PackageUpdate: true,
		Packages:      []string{"curl"},
		WriteFiles: []*cloudinit.WriteFile{
			{
				Path:        "/tmp/config.json",
				Content:     `{"key":"value"}`,
				Permissions: "0644",
			},
		},
		RunCmd: []any{
			[]string{"set", "-e"},
			"echo hello",
		},
	}

	out, err := marshalScript(ud)
	if err != nil {
		t.Fatalf("marshalScript returned error: %v", err)
	}

	script := string(out)
	t.Log(script)

	for _, want := range []string{
		"#!/bin/bash",
		"set -euo pipefail",
		"apt-get update -y",
		"apt-get install -y --allow-change-held-packages curl",
		"mkdir -p '/tmp'",
		`cat <<'EOF' > '/tmp/config.json'`,
		`{"key":"value"}`,
		"chmod 0644 '/tmp/config.json'",
		"set -e",
		"echo hello",
		"Node bootstrap script completed.",
	} {
		if !strings.Contains(script, want) {
			t.Errorf("output missing expected string: %q", want)
		}
	}
}

func Test_marshalScript_aptSource(t *testing.T) {
	ud := &cloudinit.UserData{
		APT: &cloudinit.APT{
			Sources: map[string]*cloudinit.APTSource{
				"myrepo": {
					Source: "deb https://example.com/repo /",
					KeyID:  "AABBCCDD",
				},
			},
		},
	}

	out, err := marshalScript(ud)
	if err != nil {
		t.Fatalf("marshalScript returned error: %v", err)
	}

	script := string(out)
	t.Log(script)

	for _, want := range []string{
		"apt-key adv --recv-keys --keyserver keyserver.ubuntu.com",
		"AABBCCDD",
		"/etc/apt/sources.list.d/myrepo.list",
	} {
		if !strings.Contains(script, want) {
			t.Errorf("output missing expected string: %q", want)
		}
	}
}

func Test_marshalScript_empty(t *testing.T) {
	ud := &cloudinit.UserData{}

	out, err := marshalScript(ud)
	if err != nil {
		t.Fatalf("marshalScript returned error: %v", err)
	}

	script := string(out)
	if !strings.HasPrefix(script, "#!/bin/bash") {
		t.Errorf("expected shebang header, got: %q", script[:40])
	}
	if !strings.Contains(script, "Node bootstrap script completed.") {
		t.Error("output missing footer")
	}
}

func Test_pickDelimiter(t *testing.T) {
	if d := pickDelimiter("no conflict"); d != "EOF" {
		t.Errorf("expected EOF, got %q", d)
	}
	if d := pickDelimiter("contains EOF in text"); d != "_EOF" {
		t.Errorf("expected _EOF, got %q", d)
	}
	if d := pickDelimiter("has EOF and _EOF"); d != "__EOF" {
		t.Errorf("expected __EOF, got %q", d)
	}
}

func Test_shellJoin(t *testing.T) {
	got := shellJoin([]string{"echo", "hello world", "foo"})
	want := "echo 'hello world' foo"
	if got != want {
		t.Errorf("shellJoin = %q, want %q", got, want)
	}
}
