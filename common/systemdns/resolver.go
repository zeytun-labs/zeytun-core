// Package systemdns owns temporary operating-system DNS routing for native TUNs.
package systemdns

import (
	"fmt"
	"net/netip"
	"sync"
)

var managed = struct {
	sync.RWMutex
	addresses map[netip.Addr]int
}{addresses: make(map[netip.Addr]int)}

// IsManaged prevents the local/upstream resolver from querying our own TUN.
func IsManaged(address netip.Addr) bool {
	managed.RLock()
	defer managed.RUnlock()
	return managed.addresses[address.Unmap()] > 0
}

func retainAddresses(addresses []netip.Addr) func() {
	managed.Lock()
	for _, address := range addresses {
		managed.addresses[address.Unmap()]++
	}
	managed.Unlock()
	var once sync.Once
	return func() {
		once.Do(func() {
			managed.Lock()
			defer managed.Unlock()
			for _, address := range addresses {
				key := address.Unmap()
				if managed.addresses[key] <= 1 {
					delete(managed.addresses, key)
				} else {
					managed.addresses[key]--
				}
			}
		})
	}
}

func validateAddresses(addresses []netip.Addr) error {
	if len(addresses) == 0 {
		return fmt.Errorf("native TUN DNS requires at least one DNS address")
	}
	for _, address := range addresses {
		if !address.IsValid() || address.IsUnspecified() || address.IsMulticast() {
			return fmt.Errorf("invalid native TUN DNS address")
		}
	}
	return nil
}
