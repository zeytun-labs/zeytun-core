package main

import (
	"os"
	"testing"
)

func TestValidateTrustedOwner(t *testing.T) {
	cases := []struct {
		name  string
		uid   uint32
		mode  os.FileMode
		trust bool
	}{
		{"root 0600", 0, 0o600, true},
		{"root 0644", 0, 0o644, true},
		{"root 0755 dir", 0, os.ModeDir | 0o755, true},
		{"user owned", 501, 0o600, false},
		{"root group-writable", 0, 0o664, false},
		{"root world-writable", 0, 0o646, false},
		{"root dir world-writable", 0, os.ModeDir | 0o777, false},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			err := validateTrustedOwner("config", "/x/config.json", tc.uid, tc.mode)
			if tc.trust && err != nil {
				t.Fatalf("expected trusted, got %v", err)
			}
			if !tc.trust && err == nil {
				t.Fatal("expected rejection")
			}
		})
	}
}

func TestPrivilegedCommandAllowList(t *testing.T) {
	cases := map[string]string{
		"run":       "run",
		"check":     "check",
		"version":   "version",
		"merge":     "",
		"format":    "",
		"tools":     "",
		"rule-set":  "",
		"generate":  "",
		"geoip":     "",
		"geosite":   "",
		"(unknown)": "",
	}
	for command, want := range cases {
		if got := privilegedCommandAllowed(command); got != (want != "") {
			t.Errorf("privilegedCommandAllowed(%q) = %v", command, got)
		}
	}
}

func TestTopLevelCommandName(t *testing.T) {
	cases := []struct {
		args []string
		want string
	}{
		{[]string{"run", "-c", "config.json"}, "run"},
		{[]string{"-c", "config.json", "run"}, "run"},
		{[]string{"tools", "fetch", "https://example.com"}, "tools"},
		{[]string{"rule-set", "compile", "x.json"}, "rule-set"},
		{[]string{"merge", "out.json"}, "merge"},
		{nil, ""},
	}
	for _, tc := range cases {
		if got := topLevelCommandName(tc.args); got != tc.want {
			t.Errorf("topLevelCommandName(%v) = %q, want %q", tc.args, got, tc.want)
		}
	}
}
