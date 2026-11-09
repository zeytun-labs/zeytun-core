//go:build unix

package main

import (
	"io"
	"os"
	"path/filepath"
	"syscall"

	E "github.com/sagernet/sing/common/exceptions"
)

func runningAsRoot() bool {
	return os.Geteuid() == 0
}

// runningSetuid reports a setuid/setgid invocation: the caller is someone
// else and everything it controls (argv, env, cwd) is untrusted.
func runningSetuid() bool {
	return os.Getuid() != os.Geteuid() || os.Getgid() != os.Getegid()
}

// checkPrivilegedInvocation runs before cobra parses anything, so subcommands
// that override PersistentPreRun cannot skip it.
func checkPrivilegedInvocation(args []string) error {
	if !runningAsRoot() || !runningSetuid() {
		return nil
	}
	name := topLevelCommandName(args)
	if !privilegedCommandAllowed(name) {
		return E.New("refusing to run `", name, "` with setuid privileges; only run, check and version are allowed")
	}
	return nil
}

// enterPrivilegedWorkdir pins a setuid run to the directory of its single
// config file. Without this, a relative path in a trusted config (the
// default `cache_file` is "cache.db") resolves against the caller's cwd, and
// a symlink planted there makes root create files wherever the caller wants.
func enterPrivilegedWorkdir() error {
	if len(configDirectories) != 0 || len(configPaths) != 1 || configPaths[0] == "stdin" {
		return E.New("refusing setuid run: exactly one -c config file is required")
	}
	absPath, err := filepath.Abs(configPaths[0])
	if err != nil {
		return err
	}
	err = os.Chdir(filepath.Dir(absPath))
	if err != nil {
		return E.Cause(err, "enter config directory")
	}
	// Validate the directory we are now in by inode, not by path, so it
	// cannot be swapped between the check and later relative file creation.
	err = validateWorkdir(".")
	if err != nil {
		return err
	}
	configPaths[0] = filepath.Base(absPath)
	resetPrivilegedEnv()
	return nil
}

func validateWorkdir(dir string) error {
	info, err := os.Stat(dir)
	if err != nil {
		return err
	}
	stat, ok := info.Sys().(*syscall.Stat_t)
	if !ok {
		return E.New("refusing to run as root: cannot read owner of working directory")
	}
	cwd, _ := os.Getwd()
	return validateTrustedOwner("working directory", cwd, uint32(stat.Uid), info.Mode())
}

// resetPrivilegedEnv drops the caller's environment (HOME, TMPDIR, proxy and
// TLS variables, ...) so none of it steers what the root process does.
func resetPrivilegedEnv() {
	os.Clearenv()
	_ = os.Setenv("PATH", "/usr/bin:/bin:/usr/sbin:/sbin")
}

// readConfigFile reads a config file, enforcing the root-ownership rule when
// running as root.
func readConfigFile(path string) ([]byte, error) {
	return readConfigFileChecked(path, runningAsRoot())
}

// readConfigFileChecked opens the file once and validates the descriptor it
// will read from, so the file cannot be swapped between check and read.
func readConfigFileChecked(path string, enforceRootOwned bool) ([]byte, error) {
	if !enforceRootOwned {
		return os.ReadFile(path)
	}
	file, err := os.Open(path)
	if err != nil {
		return nil, err
	}
	defer file.Close()
	info, err := file.Stat()
	if err != nil {
		return nil, err
	}
	if !info.Mode().IsRegular() {
		return nil, E.New("refusing to run as root: config ", path, " is not a regular file")
	}
	stat, ok := info.Sys().(*syscall.Stat_t)
	if !ok {
		return nil, E.New("refusing to run as root: cannot read owner of config ", path)
	}
	err = validateTrustedOwner("config", path, uint32(stat.Uid), info.Mode())
	if err != nil {
		return nil, err
	}
	// A root-owned file in a user-writable directory can be renamed away and
	// replaced, so the containing directory must be trusted too.
	absPath, err := filepath.Abs(path)
	if err != nil {
		return nil, err
	}
	dir := filepath.Dir(absPath)
	dirInfo, err := os.Stat(dir)
	if err != nil {
		return nil, err
	}
	dirStat, ok := dirInfo.Sys().(*syscall.Stat_t)
	if !ok {
		return nil, E.New("refusing to run as root: cannot read owner of config directory ", dir)
	}
	err = validateTrustedOwner("config directory", dir, uint32(dirStat.Uid), dirInfo.Mode())
	if err != nil {
		return nil, err
	}
	return io.ReadAll(file)
}
