//go:build darwin && cgo

package systemdns

import (
	"bufio"
	"fmt"
	"net/netip"
	"os"
	"os/exec"
	"strings"
	"testing"
	"time"
)

// This opt-in test touches only a unique .invalid DNS scope. It does not alter
// the system's default resolver or require a real TUN or internet connection.
func TestSessionResolverCleanup(t *testing.T) {
	if os.Getenv("ZEYTUN_SYSTEMDNS_TEST") != "1" || os.Geteuid() != 0 {
		t.Skip("requires ZEYTUN_SYSTEMDNS_TEST=1 and administrator privileges")
	}
	for _, crash := range []bool{false, true} {
		t.Run(fmt.Sprintf("crash=%v", crash), func(t *testing.T) {
			cmd := exec.Command(os.Args[0], "-test.run=^TestSystemDNSChild$")
			cmd.Env = append(os.Environ(), "ZEYTUN_SYSTEMDNS_CHILD=1")
			stdout, err := cmd.StdoutPipe()
			if err != nil {
				t.Fatal(err)
			}
			stdin, err := cmd.StdinPipe()
			if err != nil {
				t.Fatal(err)
			}
			if err := cmd.Start(); err != nil {
				t.Fatal(err)
			}
			t.Cleanup(func() { _ = cmd.Process.Kill(); _ = cmd.Wait() })
			ready := make(chan string, 1)
			go func() {
				scanner := bufio.NewScanner(stdout)
				if scanner.Scan() {
					ready <- scanner.Text()
				} else {
					ready <- ""
				}
			}()
			var key string
			select {
			case key = <-ready:
			case <-time.After(5 * time.Second):
				t.Fatal("helper did not register its resolver")
			}
			if !strings.HasPrefix(key, "State:/Network/Service/ZeytunTUN-") {
				t.Fatalf("unexpected helper response %q", key)
			}
			if !storeHasKey(t, key) {
				t.Fatal("resolver was not published")
			}
			if crash {
				_ = cmd.Process.Kill()
			} else {
				_ = stdin.Close()
			}
			_ = cmd.Wait()
			deadline := time.Now().Add(5 * time.Second)
			for storeHasKey(t, key) {
				if time.Now().After(deadline) {
					t.Fatal("resolver survived the owning process")
				}
				time.Sleep(50 * time.Millisecond)
			}
		})
	}
}

func storeHasKey(t *testing.T, key string) bool {
	t.Helper()
	cmd := exec.Command("/usr/sbin/scutil")
	cmd.Stdin = strings.NewReader("show " + key + "\nquit\n")
	output, err := cmd.CombinedOutput()
	if err != nil {
		t.Fatal(err)
	}
	return strings.Contains(string(output), "ServerAddresses")
}

func TestSystemDNSChild(t *testing.T) {
	if os.Getenv("ZEYTUN_SYSTEMDNS_CHILD") != "1" {
		t.Skip("subprocess helper")
	}
	r, err := openDomain("integration test", []netip.Addr{netip.MustParseAddr("192.0.2.53")}, fmt.Sprintf("zeytun-dns-%d.invalid", os.Getpid()))
	if err != nil {
		t.Fatal(err)
	}
	defer r.Close()
	fmt.Println(r.(*resolver).key)
	var value [1]byte
	_, _ = os.Stdin.Read(value[:])
	if err := r.Close(); err != nil {
		t.Fatal(err)
	}
	if err := r.Close(); err != nil {
		t.Fatal(err)
	}
}
