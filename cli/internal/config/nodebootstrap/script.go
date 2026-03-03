package nodebootstrap

import (
	"bytes"
	_ "embed"
	"fmt"
	"strings"
	"text/template"

	"github.com/Azure/aks-flex/plugin/pkg/util/cloudinit"
)

//go:embed assets/script.sh.tmpl
var scriptTmpl string

var scriptTemplate = template.Must(
	template.New("script.sh").Funcs(template.FuncMap{
		"join":          strings.Join,
		"writeFile":     renderWriteFile,
		"shellCmd":      renderShellCmd,
		"aptSourceLine": aptSourceLine,
	}).Parse(scriptTmpl),
)

// marshalScript converts cloud-init UserData into an equivalent standalone bash
// script by rendering the embedded template.
func marshalScript(userData *cloudinit.UserData) ([]byte, error) {
	var buf bytes.Buffer
	if err := scriptTemplate.Execute(&buf, userData); err != nil {
		return nil, fmt.Errorf("rendering script template: %w", err)
	}
	return buf.Bytes(), nil
}

// aptSourceLine returns the APT source line, adding a signed-by clause when an
// inline GPG key is used.
func aptSourceLine(name string, src *cloudinit.APTSource) string {
	if src.Key != "" && !strings.Contains(src.Source, "signed-by") {
		return strings.Replace(src.Source, "deb ",
			fmt.Sprintf("deb [signed-by=/usr/share/keyrings/%s.gpg] ", name), 1)
	}
	return src.Source
}

// renderWriteFile emits the shell commands that recreate a single cloud-init
// write_files entry: mkdir, heredoc cat, and optional chmod.
func renderWriteFile(wf *cloudinit.WriteFile) string {
	var buf bytes.Buffer

	dir := wf.Path[:strings.LastIndex(wf.Path, "/")]
	if dir != "" {
		fmt.Fprintf(&buf, "mkdir -p '%s'\n", dir)
	}

	delimiter := pickDelimiter(wf.Content)
	operator := ">"
	if wf.Append {
		operator = ">>"
	}

	fmt.Fprintf(&buf, "cat <<'%s' %s '%s'\n", delimiter, operator, wf.Path)
	buf.WriteString(wf.Content)
	if !strings.HasSuffix(wf.Content, "\n") {
		buf.WriteString("\n")
	}
	buf.WriteString(delimiter)

	if wf.Permissions != "" {
		fmt.Fprintf(&buf, "\nchmod %s '%s'", wf.Permissions, wf.Path)
	}

	return buf.String()
}

// renderShellCmd converts a single cloud-init runcmd entry to a shell line.
func renderShellCmd(cmd any) string {
	switch v := cmd.(type) {
	case string:
		return v
	case []string:
		return shellJoin(v)
	case []any:
		parts := make([]string, 0, len(v))
		for _, elem := range v {
			parts = append(parts, fmt.Sprintf("%v", elem))
		}
		return shellJoin(parts)
	default:
		return fmt.Sprintf("%v", v)
	}
}

// pickDelimiter returns a heredoc delimiter that does not appear in content.
func pickDelimiter(content string) string {
	candidate := "EOF"
	for strings.Contains(content, candidate) {
		candidate = "_" + candidate
	}
	return candidate
}

// shellJoin quotes arguments that contain special shell characters.
func shellJoin(args []string) string {
	quoted := make([]string, 0, len(args))
	for _, a := range args {
		if needsQuoting(a) {
			quoted = append(quoted, "'"+strings.ReplaceAll(a, "'", "'\\''")+"'")
		} else {
			quoted = append(quoted, a)
		}
	}
	return strings.Join(quoted, " ")
}

// needsQuoting returns true if the string contains characters that require
// shell quoting.
func needsQuoting(s string) bool {
	for _, c := range s {
		if !((c >= 'a' && c <= 'z') || (c >= 'A' && c <= 'Z') || (c >= '0' && c <= '9') ||
			c == '-' || c == '_' || c == '/' || c == '.' || c == ',' || c == '+' || c == ':' || c == '=') {
			return true
		}
	}
	return false
}
