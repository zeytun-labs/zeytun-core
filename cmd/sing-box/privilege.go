package main

import (
	"os"
	"strings"

	E "github.com/sagernet/sing/common/exceptions"
)

// Zeytun ships zeytun-core as a privileged binary on macOS (TUN needs root).
// Whenever the core runs with euid 0 it must not trust anything a normal
// user can write: a crafted config alone (log.output, cache_file, rule-set
// paths, ...) is enough to turn "run as root" into "write any file as root".
//
// Rules enforced when euid == 0:
//   - every config file (and the directory holding it) must be owned by root
//     and not group- or world-writable;
//   - config is never read from stdin;
//   - when the process is setuid (real uid != effective uid) only a small set
//     of subcommands is reachable, `-D` is rejected and SUDO_* is ignored.

// privilegedCommands lists the top-level subcommands a setuid invocation may
// reach. Everything else (format -w, merge, rule-set compile, tools, ...) can
// write files or reach the network on the caller's behalf, so it is refused.
var privilegedCommands = map[string]bool{
	"run":     true,
	"check":   true,
	"version": true,
	"help":    true,
}

func privilegedCommandAllowed(name string) bool {
	return privilegedCommands[name]
}

// topLevelCommandName resolves argv (without the program name) to the
// top-level subcommand cobra would dispatch to, e.g. ["tools", "fetch"] ->
// "tools". Returns "" for the root command itself.
func topLevelCommandName(args []string) string {
	cmd, _, err := mainCommand.Find(args)
	if err != nil || cmd == nil {
		return ""
	}
	for cmd.HasParent() && cmd.Parent() != mainCommand {
		cmd = cmd.Parent()
	}
	if cmd == mainCommand {
		return ""
	}
	return cmd.Name()
}

// validateTrustedOwner is the ownership rule for config files and the
// directories that hold them when running as root.
func validateTrustedOwner(kind string, path string, uid uint32, mode os.FileMode) error {
	if uid != 0 {
		return E.New("refusing to run as root: ", kind, " ", path, " is owned by uid ", uid, ", not root")
	}
	if mode.Perm()&0o022 != 0 {
		return E.New("refusing to run as root: ", kind, " ", path, " is group- or world-writable (", strings.TrimPrefix(mode.Perm().String(), "-"), ")")
	}
	return nil
}
