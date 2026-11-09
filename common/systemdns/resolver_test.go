package systemdns

import (
	"net/netip"
	"testing"
)

func TestManagedAddressesAreReferenceCounted(t *testing.T) {
	address := netip.MustParseAddr("172.31.254.2")
	a := retainAddresses([]netip.Addr{address})
	b := retainAddresses([]netip.Addr{address})
	defer a()
	defer b()
	if !IsManaged(address) {
		t.Fatal("managed address was not registered")
	}
	a()
	a()
	if !IsManaged(address) {
		t.Fatal("one owner removed another owner's guard")
	}
	b()
	if IsManaged(address) {
		t.Fatal("last owner did not release address")
	}
}

func TestInvalidDNSAddressesAreRejected(t *testing.T) {
	for _, addresses := range [][]netip.Addr{nil, {netip.Addr{}}, {netip.MustParseAddr("0.0.0.0")}, {netip.MustParseAddr("ff02::1")}} {
		if validateAddresses(addresses) == nil {
			t.Fatalf("accepted %v", addresses)
		}
	}
}
