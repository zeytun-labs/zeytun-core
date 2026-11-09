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
