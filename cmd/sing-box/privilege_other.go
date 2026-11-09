//go:build !unix

package main

import "os"

func runningAsRoot() bool {
	return false
}

func runningSetuid() bool {
	return false
}

func checkPrivilegedInvocation(args []string) error {
	return nil
}

func readConfigFile(path string) ([]byte, error) {
	return os.ReadFile(path)
}
