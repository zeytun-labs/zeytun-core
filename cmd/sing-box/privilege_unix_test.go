//go:build unix

package main

import (
	"os"
	"path/filepath"
	"strings"
	"testing"
)

func TestReadConfigFileCheckedRejectsUserOwnedConfig(t *testing.T) {
	if os.Geteuid() == 0 {
		t.Skip("test files would be root-owned")
	}
	path := filepath.Join(t.TempDir(), "config.json")
	if err := os.WriteFile(path, []byte(`{"log":{"output":"/etc/evil"}}`), 0o600); err != nil {
		t.Fatal(err)
	}

	_, err := readConfigFileChecked(path, true)
	if err == nil || !strings.Contains(err.Error(), "not root") {
		t.Fatalf("expected ownership rejection, got %v", err)
	}

	content, err := readConfigFileChecked(path, false)
	if err != nil || !strings.Contains(string(content), "/etc/evil") {
		t.Fatalf("unprivileged read should pass through, got %q, %v", content, err)
	}
}

func TestReadConfigFileCheckedRejectsNonRegularFile(t *testing.T) {
	_, err := readConfigFileChecked(t.TempDir(), true)
	if err == nil {
		t.Fatal("expected directory to be rejected as config")
	}
}

func TestReadConfigFileCheckedAcceptsRootOwnedSystemFile(t *testing.T) {
	// /etc/hosts (via its real path) is root-owned, 0644, in a root-owned
	// directory on every unix we build for; it stands in for a staged config.
	path, err := filepath.EvalSymlinks("/etc/hosts")
	if err != nil {
		t.Skip("no /etc/hosts")
	}
	if _, err := readConfigFileChecked(path, true); err != nil {
		t.Fatalf("expected root-owned file to be trusted, got %v", err)
	}
}

// withConfigFlags sets the -c/-C globals and the cwd for one test, restoring
// them afterwards.
func withConfigFlags(t *testing.T, paths []string, dirs []string) {
	t.Helper()
	oldPaths, oldDirs := configPaths, configDirectories
	oldWd, err := os.Getwd()
	if err != nil {
		t.Fatal(err)
	}
	configPaths, configDirectories = paths, dirs
	t.Cleanup(func() {
		configPaths, configDirectories = oldPaths, oldDirs
		_ = os.Chdir(oldWd)
	})
}

func TestEnterPrivilegedWorkdirRequiresSingleConfigFile(t *testing.T) {
	for _, tc := range []struct {
		name  string
		paths []string
		dirs  []string
	}{
		{"none", nil, nil},
		{"two", []string{"/etc/hosts", "/etc/hosts"}, nil},
		{"stdin", []string{"stdin"}, nil},
		{"directory", []string{"/etc/hosts"}, []string{"/etc"}},
	} {
		t.Run(tc.name, func(t *testing.T) {
			withConfigFlags(t, tc.paths, tc.dirs)
			if err := enterPrivilegedWorkdir(); err == nil {
				t.Fatal("expected rejection")
			}
		})
	}
}

func TestEnterPrivilegedWorkdirRejectsUserOwnedDirectory(t *testing.T) {
	if os.Geteuid() == 0 {
		t.Skip("temp dir would be root-owned")
	}
	// A config in a caller-owned directory would let the caller plant
	// symlinks next to it (e.g. cache.db) before root creates files there.
	dir := t.TempDir()
	withConfigFlags(t, []string{filepath.Join(dir, "config.json")}, nil)
	err := enterPrivilegedWorkdir()
	if err == nil || !strings.Contains(err.Error(), "working directory") {
		t.Fatalf("expected working directory rejection, got %v", err)
	}
}

func TestEnterPrivilegedWorkdirPinsCwdToTrustedConfigDir(t *testing.T) {
	path, err := filepath.EvalSymlinks("/etc/hosts")
	if err != nil {
		t.Skip("no /etc/hosts")
	}
	withConfigFlags(t, []string{path}, nil)
	savedEnv := os.Environ()
	t.Cleanup(func() {
		os.Clearenv()
		for _, kv := range savedEnv {
			if k, v, ok := strings.Cut(kv, "="); ok {
				_ = os.Setenv(k, v)
			}
		}
	})
	_ = os.Setenv("HOME", "/tmp/attacker-home")
	if err := enterPrivilegedWorkdir(); err != nil {
		t.Fatalf("expected root-owned config dir to be accepted, got %v", err)
	}
	wd, _ := os.Getwd()
	if wd != filepath.Dir(path) {
		t.Fatalf("cwd = %q, want %q", wd, filepath.Dir(path))
	}
	if configPaths[0] != filepath.Base(path) {
		t.Fatalf("config path = %q, want base name", configPaths[0])
	}
	if os.Getenv("HOME") != "" {
		t.Fatal("caller environment should be dropped")
	}
}
