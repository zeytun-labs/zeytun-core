//go:build darwin && cgo

package systemdns

/*
#cgo LDFLAGS: -framework CoreFoundation -framework SystemConfiguration
#include <CoreFoundation/CoreFoundation.h>
#include <SystemConfiguration/SystemConfiguration.h>
#include <stdlib.h>
#include <string.h>

static SCDynamicStoreRef zeytun_dns_open(const char *key, const char *addresses, const char *domain, int *error) {
	const void *optionKeys[] = { kSCDynamicStoreUseSessionKeys };
	const void *optionValues[] = { kCFBooleanTrue };
	CFDictionaryRef options = CFDictionaryCreate(NULL, optionKeys, optionValues, 1,
		&kCFTypeDictionaryKeyCallBacks, &kCFTypeDictionaryValueCallBacks);
	SCDynamicStoreRef store = SCDynamicStoreCreateWithOptions(NULL, CFSTR("Zeytun TUN DNS"), options, NULL, NULL);
	CFRelease(options);
	if (!store) { *error = SCError(); return NULL; }
	CFMutableArrayRef servers = CFArrayCreateMutable(NULL, 0, &kCFTypeArrayCallBacks);
	char *copy = strdup(addresses);
	char *cursor = copy;
	char *part;
	while ((part = strsep(&cursor, ",")) != NULL) {
		CFStringRef server = CFStringCreateWithCString(NULL, part, kCFStringEncodingUTF8);
		CFArrayAppendValue(servers, server);
		CFRelease(server);
	}
	free(copy);
	CFStringRef match = CFStringCreateWithCString(NULL, domain, kCFStringEncodingUTF8);
	const void *domainValues[] = { match };
	CFArrayRef domains = CFArrayCreate(NULL, domainValues, 1, &kCFTypeArrayCallBacks);
	int orderValue = 100000;
	CFNumberRef order = CFNumberCreate(NULL, kCFNumberIntType, &orderValue);
	int noSearchValue = 1;
	CFNumberRef noSearch = CFNumberCreate(NULL, kCFNumberIntType, &noSearchValue);
	const void *orderValues[] = { order };
	CFArrayRef orders = CFArrayCreate(NULL, orderValues, 1, &kCFTypeArrayCallBacks);
	const void *dnsKeys[] = { kSCPropNetDNSServerAddresses, kSCPropNetDNSSupplementalMatchDomains,
		kSCPropNetDNSSupplementalMatchOrders, CFSTR("SupplementalMatchDomainsNoSearch") };
	const void *dnsValues[] = { servers, domains, orders, noSearch };
	CFDictionaryRef dns = CFDictionaryCreate(NULL, dnsKeys, dnsValues, 4,
		&kCFTypeDictionaryKeyCallBacks, &kCFTypeDictionaryValueCallBacks);
	CFStringRef cfKey = CFStringCreateWithCString(NULL, key, kCFStringEncodingUTF8);
	Boolean success = SCDynamicStoreSetValue(store, cfKey, dns);
	if (!success) { *error = SCError(); }
	CFRelease(cfKey);
	CFRelease(dns);
	CFRelease(orders);
	CFRelease(noSearch);
	CFRelease(order);
	CFRelease(domains);
	CFRelease(match);
	CFRelease(servers);
	if (!success) { CFRelease(store); return NULL; }
	return store;
}

static int zeytun_dns_close(SCDynamicStoreRef store, const char *key) {
	CFStringRef cfKey = CFStringCreateWithCString(NULL, key, kCFStringEncodingUTF8);
	Boolean success = SCDynamicStoreRemoveValue(store, cfKey);
	int error = success ? 0 : SCError();
	CFRelease(cfKey);
	CFRelease(store); // Session keys are also removed if this process crashes.
	return error == kSCStatusNoKey ? 0 : error;
}
*/
import "C"

import (
	"crypto/rand"
	"fmt"
	"io"
	"net/netip"
	"os"
	"strings"
	"sync"
	"unsafe"
)

// Open registers a session-owned catch-all resolver only after TUN is ready.
// It never rewrites persistent Wi-Fi, Ethernet, or VPN DNS settings.
func Open(name string, addresses []netip.Addr) (io.Closer, error) {
	return openDomain(name, addresses, "")
}

func openDomain(name string, addresses []netip.Addr, domain string) (io.Closer, error) {
	if err := validateAddresses(addresses); err != nil {
		return nil, err
	}
	if os.Geteuid() != 0 {
		return nil, fmt.Errorf("native TUN DNS requires administrator privileges")
	}
	var nonce [12]byte
	if _, err := rand.Read(nonce[:]); err != nil {
		return nil, err
	}
	key := fmt.Sprintf("State:/Network/Service/ZeytunTUN-%d-%x/DNS", os.Getpid(), nonce)
	stringsAddresses := make([]string, len(addresses))
	for i, address := range addresses {
		stringsAddresses[i] = address.String()
	}
	ckey := C.CString(key)
	caddresses := C.CString(strings.Join(stringsAddresses, ","))
	cdomain := C.CString(domain)
	defer C.free(unsafe.Pointer(ckey))
	defer C.free(unsafe.Pointer(caddresses))
	defer C.free(unsafe.Pointer(cdomain))
	release := retainAddresses(addresses)
	var status C.int
	store := C.zeytun_dns_open(ckey, caddresses, cdomain, &status)
	if store == 0 {
		release()
		return nil, fmt.Errorf("register native TUN DNS for %s: SystemConfiguration status %d", name, status)
	}
	return &resolver{key: key, close: func() error {
		ckey := C.CString(key)
		defer C.free(unsafe.Pointer(ckey))
		status := C.zeytun_dns_close(store, ckey)
		release()
		if status != 0 {
			return fmt.Errorf("remove native TUN DNS: SystemConfiguration status %d", status)
		}
		return nil
	}}, nil
}

type resolver struct {
	key   string
	once  sync.Once
	close func() error
	err   error
}

func (r *resolver) Close() error {
	r.once.Do(func() { r.err = r.close() })
	return r.err
}
