//go:build !generate

package main

import (
	"os"

	"github.com/sagernet/sing-box/log"
)

func main() {
	if err := checkPrivilegedInvocation(os.Args[1:]); err != nil {
		log.Fatal(err)
	}
	if err := mainCommand.Execute(); err != nil {
		log.Fatal(err)
	}
}
