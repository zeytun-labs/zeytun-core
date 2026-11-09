//go:build !darwin || !cgo

package systemdns

import (
	"fmt"
	"io"
	"net/netip"
	"runtime"
)

func Open(_ string, _ []netip.Addr) (io.Closer, error) {
	if runtime.GOOS == "darwin" {
		return nil, fmt.Errorf("native macOS TUN DNS requires a CGO-enabled core build")
	}
	return nil, nil
}
