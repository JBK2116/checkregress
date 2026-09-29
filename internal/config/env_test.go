package config

import (
	"os"
	"path/filepath"
	"strings"
	"syscall"
	"testing"
)

// chdirToEnvFile creates a fresh temporary directory, optionally writes the
// given file into it, and changes the working directory to that directory so
// that LoadEnv resolves ".env" relative to a temporary directory instead of
// the repository. A zero fileName means no file is written at all.
func chdirToEnvFile(t *testing.T, fileName, content string) {
	t.Helper()
	dir := t.TempDir()
	if fileName != "" {
		fullPath := filepath.Join(dir, fileName)
		if err := os.WriteFile(fullPath, []byte(content), 0o600); err != nil {
			t.Fatalf("writing %s: %v", fileName, err)
		}
	}
	t.Chdir(dir)
}

// withoutEnvVar removes key from the process environment for the duration of
// fn, restoring the previous value (if any) when the test finishes.
//
// godotenv refuses to overwrite variables that already exist in the process
// environment and loadEnvFromPath reads the loaded value back through
// [os.Getenv] so the env tests need a known-clean environment. Because these
//
// tests mutate process-global state they deliberately do not run in parallel.
func withoutEnvVar(t *testing.T, key string, fn func()) {
	t.Helper()
	prev, existed := os.LookupEnv(key)
	os.Unsetenv(key)
	t.Cleanup(func() {
		if existed {
			func() error {
				err := syscall.Setenv(key, prev)
				if err != nil {
					return os.NewSyscallError("setenv", err)
				}
				return nil
			}()
		} else {
			os.Unsetenv(key)
		}
	})
	fn()
}

// validEnv returns a minimal but fully valid env file document, mirroring the
// layout of the real ".env" file (comment plus quoted value).
func validEnv() string {
	return `
# Database Functionality
DatabaseURL="postgres://user:pass@localhost:5432/checkregress"
`
}

//nolint:paralleltest // The test mutates process-global state (cwd and env), so it must run serially.
func TestLoadEnvFileNotFound(t *testing.T) {
	chdirToEnvFile(t, "", "")

	msg := catchPanic(t, func() { LoadEnv() })
	if !strings.Contains(msg, "error reading env configuration file") {
		t.Fatalf("panic message %q does not mention the read failure", msg)
	}
}

//nolint:paralleltest // The test mutates process-global state (cwd and env), so it must run serially.
func TestLoadEnvFileNotFoundWrongName(t *testing.T) {
	// Only a file named exactly ".env" is picked up; a similarly named file
	// must be treated as missing.
	chdirToEnvFile(t, ".env.bak", validEnv())

	msg := catchPanic(t, func() { LoadEnv() })
	if !strings.Contains(msg, "error reading env configuration file") {
		t.Fatalf("panic message %q does not mention the read failure", msg)
	}
}

//nolint:paralleltest // The test mutates process-global state (cwd and env), so it must run serially.
func TestLoadEnvFileIsDirectory(t *testing.T) {
	// A directory named ".env" cannot be parsed as an env file and must be
	// reported as a read failure.
	dir := t.TempDir()
	if err := os.Mkdir(filepath.Join(dir, ".env"), 0o755); err != nil {
		t.Fatalf("creating .env directory: %v", err)
	}
	t.Chdir(dir)

	msg := catchPanic(t, func() { LoadEnv() })
	if !strings.Contains(msg, "error reading env configuration file") {
		t.Fatalf("panic message %q does not mention the read failure", msg)
	}
}

//nolint:paralleltest // The test mutates process-global state (cwd and env), so it must run serially.
func TestLoadEnvMissingField(t *testing.T) {
	tests := []struct {
		name    string
		content string
	}{
		{
			name:    "empty file",
			content: "",
		},
		{
			name:    "only unrelated variables",
			content: "SOME_OTHER_VAR=value\n",
		},
		{
			name:    "DatabaseURL set to empty value",
			content: "DatabaseURL=\n",
		},
	}

	//nolint:paralleltest // The subtests mutate process-global state (cwd and env), so they must run serially.
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			chdirToEnvFile(t, ".env", tt.content)

			var msg string
			withoutEnvVar(t, "DatabaseURL", func() {
				msg = catchPanic(t, func() { LoadEnv() })
			})

			if !strings.Contains(msg, "missing required field") {
				t.Fatalf("panic message %q does not mention a missing field", msg)
			}
			if !strings.Contains(msg, "DatabaseURL") {
				t.Fatalf("panic message %q does not mention %q", msg, "DatabaseURL")
			}
		})
	}
}

//nolint:paralleltest // The test mutates process-global state (cwd and env), so it must run serially.
func TestLoadEnvValid(t *testing.T) {
	want := "postgres://user:pass@localhost:5432/checkregress"
	chdirToEnvFile(t, ".env", validEnv())

	var conf EnvConfig
	withoutEnvVar(t, "DatabaseURL", func() {
		conf = LoadEnv()
	})

	if conf.DatabaseURL != want {
		t.Errorf("DatabaseURL = %q, want %q", conf.DatabaseURL, want)
	}
}

//nolint:paralleltest // The test mutates process-global state (cwd and env), so it must run serially.
func TestLoadEnvIgnoresUnrelatedVariables(t *testing.T) {
	// Variables other than DatabaseURL may be present in the file; only the
	// DatabaseURL entry feeds the config. An unquoted value is also valid.
	want := "postgres://user:pass@localhost:5432/checkregress"
	content := "\nSOME_OTHER_VAR=\"value\"\nDatabaseURL=" + want + "\n"
	chdirToEnvFile(t, ".env", content)

	var conf EnvConfig
	withoutEnvVar(t, "DatabaseURL", func() {
		conf = LoadEnv()
	})

	if conf.DatabaseURL != want {
		t.Errorf("DatabaseURL = %q, want %q", conf.DatabaseURL, want)
	}
}
