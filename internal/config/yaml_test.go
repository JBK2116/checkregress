package config

import (
	"fmt"
	"os"
	"path/filepath"
	"strings"
	"testing"
)

// writeConfigFile creates a fresh temporary directory, optionally writes the
// given file into it, and returns the directory so that loadYamlFromDir can
// resolve the config file relative to it. A zero fileName means no file is
// written at all.
func writeConfigFile(t *testing.T, fileName, content string) string {
	t.Helper()
	dir := t.TempDir()
	if fileName != "" {
		fullPath := filepath.Join(dir, fileName)
		if err := os.WriteFile(fullPath, []byte(content), 0o644); err != nil {
			t.Fatalf("writing %s: %v", fileName, err)
		}
	}
	return dir
}

// catchPanic runs fn and returns the panic value as a string, failing the test
// if no panic occurs or the panic value is not a string.
func catchPanic(t *testing.T, fn func()) string {
	t.Helper()
	var recovered any
	func() {
		defer func() {
			recovered = recover()
		}()
		fn()
	}()
	if recovered == nil {
		t.Fatal("expected panic, but no panic occurred")
	}
	msg, ok := recovered.(string)
	if !ok {
		t.Fatalf("expected string panic, got %T: %v", recovered, recovered)
	}
	return msg
}

// validConfig returns a minimal but fully valid configuration document.
func validConfig() string {
	return `
listen: "127.0.0.1:8080"
admin_listen: "127.0.0.1:8081"
routes:
  - name: "primary"
    legacy: "https://legacy.example.com"
    candidate: "https://candidate.example.com"
`
}

// wrapConfig returns a valid configuration document with the given field
// injected between the required scalar fields and the routes block.
func wrapConfig(field string) string {
	return fmt.Sprintf(`
listen: "127.0.0.1:8080"
admin_listen: "127.0.0.1:8081"
%s
routes:
  - name: "primary"
    legacy: "https://legacy.example.com"
    candidate: "https://candidate.example.com"
`, field)
}

// wrapListenAddr returns a valid configuration document with the given listen
// and admin_listen values substituted in.
func wrapListenAddr(listen, adminListen string) string {
	return fmt.Sprintf(`
listen: %q
admin_listen: %q
routes:
  - name: "primary"
    legacy: "https://legacy.example.com"
    candidate: "https://candidate.example.com"
`, listen, adminListen)
}

func TestLoadYamlFileNotFound(t *testing.T) {
	t.Parallel()
	dir := writeConfigFile(t, "", "")

	msg := catchPanic(t, func() { loadYamlFromDir(dir) })
	if !strings.Contains(msg, "error reading yaml configuration file") {
		t.Fatalf("panic message %q does not mention the read failure", msg)
	}
}

func TestLoadYamlFileNotFoundWrongName(t *testing.T) {
	t.Parallel()
	// Only a file named exactly "config.yml" is picked up; a similarly named
	// file must be treated as missing.
	dir := writeConfigFile(t, "conf.yml", validConfig())

	msg := catchPanic(t, func() { loadYamlFromDir(dir) })
	if !strings.Contains(msg, "error reading yaml configuration file") {
		t.Fatalf("panic message %q does not mention the read failure", msg)
	}
}

func TestLoadYamlMissingFields(t *testing.T) {
	t.Parallel()
	tests := []struct {
		name    string
		content string
		want    string
	}{
		{
			name: "missing listen",
			content: `
admin_listen: "127.0.0.1:8081"
routes:
  - name: "primary"
    legacy: "https://legacy.example.com"
    candidate: "https://candidate.example.com"
`,
			want: "listen",
		},
		{
			name: "missing admin_listen",
			content: `
listen: "127.0.0.1:8080"
routes:
  - name: "primary"
    legacy: "https://legacy.example.com"
    candidate: "https://candidate.example.com"
`,
			want: "admin_listen",
		},
		{
			name: "missing routes",
			content: `
listen: "127.0.0.1:8080"
admin_listen: "127.0.0.1:8081"
`,
			want: "routes",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()
			dir := writeConfigFile(t, "config.yml", tt.content)

			msg := catchPanic(t, func() { loadYamlFromDir(dir) })
			if !strings.Contains(msg, "missing required field") {
				t.Fatalf("panic message %q does not mention a missing field", msg)
			}
			if !strings.Contains(msg, tt.want) {
				t.Fatalf("panic message %q does not mention %q", msg, tt.want)
			}
		})
	}
}

func TestLoadYamlInvalidFieldValues(t *testing.T) {
	t.Parallel()
	tests := []struct {
		name  string
		field string
		want  string
	}{
		{
			name:  "max_body_bytes exceeds limit",
			field: "max_body_bytes: 10485761", // 10 MiB + 1
			want:  "max_body_bytes",
		},
		{
			name:  "max_body_bytes negative",
			field: "max_body_bytes: -1",
			want:  "max_body_bytes",
		},
		{
			name:  "shadow_timeout_ms exceeds limit",
			field: "shadow_timeout_ms: 30001", // 30 s + 1
			want:  "shadow_timeout_ms",
		},
		{
			name:  "shadow_timeout_ms negative",
			field: "shadow_timeout_ms: -1",
			want:  "shadow_timeout_ms",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()
			dir := writeConfigFile(t, "config.yml", wrapConfig(tt.field))

			msg := catchPanic(t, func() { loadYamlFromDir(dir) })
			if !strings.Contains(msg, "improperly configured") {
				t.Fatalf("panic message %q does not mention an invalid field", msg)
			}
			if !strings.Contains(msg, tt.want) {
				t.Fatalf("panic message %q does not mention %q", msg, tt.want)
			}
		})
	}
}

func TestLoadYamlRouteValidation(t *testing.T) {
	t.Parallel()
	tests := []struct {
		name    string
		content string
		want    string
	}{
		{
			name: "route missing name",
			content: `
listen: "127.0.0.1:8080"
admin_listen: "127.0.0.1:8081"
routes:
  - legacy: "https://legacy.example.com"
    candidate: "https://candidate.example.com"
`,
			want: "name",
		},
		{
			name: "route missing legacy",
			content: `
listen: "127.0.0.1:8080"
admin_listen: "127.0.0.1:8081"
routes:
  - name: "primary"
    candidate: "https://candidate.example.com"
`,
			want: "legacy",
		},
		{
			name: "route missing candidate",
			content: `
listen: "127.0.0.1:8080"
admin_listen: "127.0.0.1:8081"
routes:
  - name: "primary"
    legacy: "https://legacy.example.com"
`,
			want: "candidate",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()
			dir := writeConfigFile(t, "config.yml", tt.content)

			msg := catchPanic(t, func() { loadYamlFromDir(dir) })
			if !strings.Contains(msg, "missing required field") {
				t.Fatalf("panic message %q does not mention a missing field", msg)
			}
			if !strings.Contains(msg, tt.want) {
				t.Fatalf("panic message %q does not mention %q", msg, tt.want)
			}
		})
	}
}

func TestLoadYamlDuplicateRouteValues(t *testing.T) {
	t.Parallel()
	tests := []struct {
		name    string
		content string
		want    string
	}{
		{
			name: "duplicate route name",
			content: `
listen: "127.0.0.1:8080"
admin_listen: "127.0.0.1:8081"
routes:
  - name: "primary"
    legacy: "https://legacy.example.com"
    candidate: "https://candidate.example.com"
  - name: "primary"
    legacy: "https://legacy2.example.com"
    candidate: "https://candidate2.example.com"
`,
			want: "Name (primary)",
		},
		{
			name: "duplicate route legacy",
			content: `
listen: "127.0.0.1:8080"
admin_listen: "127.0.0.1:8081"
routes:
  - name: "primary"
    legacy: "https://legacy.example.com"
    candidate: "https://candidate.example.com"
  - name: "secondary"
    legacy: "https://legacy.example.com"
    candidate: "https://candidate2.example.com"
`,
			want: "Legacy (https://legacy.example.com)",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()
			dir := writeConfigFile(t, "config.yml", tt.content)

			msg := catchPanic(t, func() { loadYamlFromDir(dir) })
			if !strings.Contains(msg, "field value already exists") {
				t.Fatalf("panic message %q does not mention a duplicate field", msg)
			}
			if !strings.Contains(msg, tt.want) {
				t.Fatalf("panic message %q does not mention %q", msg, tt.want)
			}
		})
	}
}

func TestLoadYamlInvalidRouteURLs(t *testing.T) {
	t.Parallel()
	tests := []struct {
		name    string
		content string
		want    string
	}{
		{
			name: "legacy missing scheme",
			content: `
listen: "127.0.0.1:8080"
admin_listen: "127.0.0.1:8081"
routes:
  - name: "primary"
    legacy: "legacy.example.com"
    candidate: "https://candidate.example.com"
`,
			want: "legacy",
		},
		{
			name: "legacy unsupported scheme",
			content: `
listen: "127.0.0.1:8080"
admin_listen: "127.0.0.1:8081"
routes:
  - name: "primary"
    legacy: "ftp://legacy.example.com"
    candidate: "https://candidate.example.com"
`,
			want: "legacy",
		},
		{
			name: "legacy missing host",
			content: `
listen: "127.0.0.1:8080"
admin_listen: "127.0.0.1:8081"
routes:
  - name: "primary"
    legacy: "https://"
    candidate: "https://candidate.example.com"
`,
			want: "legacy",
		},
		{
			name: "candidate missing scheme",
			content: `
listen: "127.0.0.1:8080"
admin_listen: "127.0.0.1:8081"
routes:
  - name: "primary"
    legacy: "https://legacy.example.com"
    candidate: "candidate.example.com"
`,
			want: "candidate",
		},
		{
			name: "secondary unsupported scheme",
			content: `
listen: "127.0.0.1:8080"
admin_listen: "127.0.0.1:8081"
routes:
  - name: "primary"
    legacy: "https://legacy.example.com"
    secondary: "ftp://shadow.example.com"
    candidate: "https://candidate.example.com"
`,
			want: "secondary",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()
			dir := writeConfigFile(t, "config.yml", tt.content)

			msg := catchPanic(t, func() { loadYamlFromDir(dir) })
			if !strings.Contains(msg, "improperly configured") {
				t.Fatalf("panic message %q does not mention an invalid field", msg)
			}
			if !strings.Contains(msg, tt.want) {
				t.Fatalf("panic message %q does not mention %q", msg, tt.want)
			}
		})
	}
}

func TestLoadYamlLegacyEqualsCandidate(t *testing.T) {
	t.Parallel()
	content := `
listen: "127.0.0.1:8080"
admin_listen: "127.0.0.1:8081"
routes:
  - name: "primary"
    legacy: "https://same.example.com"
    candidate: "https://same.example.com"
`
	dir := writeConfigFile(t, "config.yml", content)

	msg := catchPanic(t, func() { loadYamlFromDir(dir) })
	if !strings.Contains(msg, "legacy cannot equal candidate") {
		t.Fatalf("panic message %q does not mention the legacy/candidate mismatch", msg)
	}
}

func TestLoadYamlSecondaryEqualsCandidate(t *testing.T) {
	t.Parallel()
	content := `
listen: "127.0.0.1:8080"
admin_listen: "127.0.0.1:8081"
routes:
  - name: "primary"
    legacy: "https://legacy.example.com"
    secondary: "https://candidate.example.com"
    candidate: "https://candidate.example.com"
`
	dir := writeConfigFile(t, "config.yml", content)

	msg := catchPanic(t, func() { loadYamlFromDir(dir) })
	if !strings.Contains(msg, "secondary cannot equal candidate") {
		t.Fatalf("panic message %q does not mention the secondary/candidate mismatch", msg)
	}
}

func TestLoadYamlInvalidListenAddresses(t *testing.T) {
	t.Parallel()
	tests := []struct {
		name        string
		listen      string
		adminListen string
		want        string
	}{
		{
			name:        "listen missing port",
			listen:      "127.0.0.1",
			adminListen: "127.0.0.1:8081",
			want:        "listen must be in host:port form",
		},
		{
			name:        "listen non-numeric port",
			listen:      ":abc",
			adminListen: "127.0.0.1:8081",
			want:        "listen must contain a valid port",
		},
		{
			name:        "listen port out of range",
			listen:      ":65536",
			adminListen: "127.0.0.1:8081",
			want:        "listen must contain a valid port",
		},
		{
			name:        "listen port zero",
			listen:      ":0",
			adminListen: "127.0.0.1:8081",
			want:        "listen must contain a valid port",
		},
		{
			name:        "admin_listen missing port",
			listen:      "127.0.0.1:8080",
			adminListen: "127.0.0.1",
			want:        "admin_listen must be in host:port form",
		},
		{
			name:        "admin_listen non-numeric port",
			listen:      "127.0.0.1:8080",
			adminListen: "127.0.0.1:abc",
			want:        "admin_listen must contain a valid port",
		},
		{
			name:        "admin_listen invalid IP",
			listen:      "127.0.0.1:8080",
			adminListen: "localhost:8081",
			want:        "admin_listen must contain a valid IP address",
		},
		{
			name:        "admin_listen empty host",
			listen:      "127.0.0.1:8080",
			adminListen: ":8081",
			want:        "admin_listen must contain a valid IP address",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()
			dir := writeConfigFile(t, "config.yml", wrapListenAddr(tt.listen, tt.adminListen))

			msg := catchPanic(t, func() { loadYamlFromDir(dir) })
			if !strings.Contains(msg, "improperly configured") {
				t.Fatalf("panic message %q does not mention an invalid field", msg)
			}
			if !strings.Contains(msg, tt.want) {
				t.Fatalf("panic message %q does not contain %q", msg, tt.want)
			}
		})
	}
}

func TestLoadYamlListenEqualsAdminListen(t *testing.T) {
	t.Parallel()
	content := `
listen: "127.0.0.1:8080"
admin_listen: "127.0.0.1:8080"
routes:
  - name: "primary"
    legacy: "https://legacy.example.com"
    candidate: "https://candidate.example.com"
`
	dir := writeConfigFile(t, "config.yml", content)

	msg := catchPanic(t, func() { loadYamlFromDir(dir) })
	if !strings.Contains(msg, "listen cannot equal admin_listen") {
		t.Fatalf("panic message %q does not mention the listen/admin_listen mismatch", msg)
	}
}

func TestLoadYamlDefaults(t *testing.T) {
	t.Parallel()
	dir := writeConfigFile(t, "config.yml", validConfig())

	conf := loadYamlFromDir(dir)

	// Optional fields fall back to their defaults. The typed comparisons below
	// also prove the values unmarshaled as ints, not strings.
	if conf.MaxBodyBytes != defaultMaxBodyBytes {
		t.Errorf("MaxBodyBytes = %d, want %d", conf.MaxBodyBytes, defaultMaxBodyBytes)
	}
	if _, ok := any(conf.MaxBodyBytes).(int); !ok {
		t.Errorf("MaxBodyBytes has type %T, want int", conf.MaxBodyBytes)
	}
	if conf.ShadowTimeoutMS != defaultShadowTimeoutMS {
		t.Errorf("ShadowTimeoutMS = %d, want %d", conf.ShadowTimeoutMS, defaultShadowTimeoutMS)
	}
	if _, ok := any(conf.ShadowTimeoutMS).(int); !ok {
		t.Errorf("ShadowTimeoutMS has type %T, want int", conf.ShadowTimeoutMS)
	}

	if conf.Listen != "127.0.0.1:8080" {
		t.Errorf("Listen = %q, want %q", conf.Listen, "127.0.0.1:8080")
	}
	if conf.AdminListen != "127.0.0.1:8081" {
		t.Errorf("AdminListen = %q, want %q", conf.AdminListen, "127.0.0.1:8081")
	}

	// Route-level default: secondary falls back to legacy.
	if len(conf.Routes) != 1 {
		t.Fatalf("len(Routes) = %d, want 1", len(conf.Routes))
	}
	legacy := conf.Routes[0].Legacy
	secondary := conf.Routes[0].Secondary
	if secondary.String() != legacy.String() {
		t.Errorf("Secondary = %q, want %q", secondary, legacy)
	}
	// The defaulted secondary must be an independent copy, not the same
	// *url.URL pointer, so mutating one does not affect the other.
	if secondary == legacy {
		t.Error("Secondary and Legacy share a *url.URL pointer, want independent values")
	}
}

func TestLoadYamlExplicitValues(t *testing.T) {
	const usingMaxBodyBytes = 2097152
	const usingShadowTimeoutMS = 15000
	t.Parallel()
	content := fmt.Sprintf(`
listen: "0.0.0.0:9090"
admin_listen: "127.0.0.1:9091"
max_body_bytes: %d
shadow_timeout_ms: %d
routes:
  - name: "primary"
    legacy: "https://legacy.example.com"
    secondary: "https://shadow.example.com"
    candidate: "https://candidate.example.com"
`, usingMaxBodyBytes, usingShadowTimeoutMS)
	dir := writeConfigFile(t, "config.yml", content)

	conf := loadYamlFromDir(dir)

	if conf.Listen != "0.0.0.0:9090" {
		t.Errorf("Listen = %q, want %q", conf.Listen, "0.0.0.0:9090")
	}
	if conf.AdminListen != "127.0.0.1:9091" {
		t.Errorf("AdminListen = %q, want %q", conf.AdminListen, "127.0.0.1:9091")
	}
	if conf.MaxBodyBytes != usingMaxBodyBytes {
		t.Errorf("MaxBodyBytes = %d, want %d", conf.MaxBodyBytes, usingMaxBodyBytes)
	}
	if conf.ShadowTimeoutMS != usingShadowTimeoutMS {
		t.Errorf("ShadowTimeoutMS = %d, want %d", conf.ShadowTimeoutMS, usingShadowTimeoutMS)
	}

	if len(conf.Routes) != 1 {
		t.Fatalf("len(Routes) = %d, want 1", len(conf.Routes))
	}
	r := conf.Routes[0]
	if r.Name != "primary" {
		t.Errorf("Name = %q, want %q", r.Name, "primary")
	}
	if r.Legacy.String() != "https://legacy.example.com" {
		t.Errorf("Legacy = %q, want %q", r.Legacy, "https://legacy.example.com")
	}
	if r.Secondary.String() != "https://shadow.example.com" {
		t.Errorf("Secondary = %q, want %q", r.Secondary, "https://shadow.example.com")
	}
	if r.Candidate.String() != "https://candidate.example.com" {
		t.Errorf("Candidate = %q, want %q", r.Candidate, "https://candidate.example.com")
	}
}
