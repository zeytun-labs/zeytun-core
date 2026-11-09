package listener

import (
	"errors"
	"io"
	"net"
	"os"
	"testing"
)

func TestAbortiveCloseWhenRootOnlyWrapsForRoot(t *testing.T) {
	inner, err := net.Listen("tcp", "127.0.0.1:0")
	if err != nil {
		t.Fatal(err)
	}
	defer inner.Close()
	wrapped := AbortiveCloseWhenRoot(inner)
	_, isAbortive := wrapped.(*abortiveListener)
	if isAbortive != (os.Geteuid() == 0) {
		t.Fatalf("wrapped=%v, want wrapping only when euid is 0", isAbortive)
	}
}

func TestAbortiveListenerAcceptsAndSetsLinger(t *testing.T) {
	inner, err := net.Listen("tcp", "127.0.0.1:0")
	if err != nil {
		t.Fatal(err)
	}
	defer inner.Close()
	wrapped := &abortiveListener{Listener: inner}
	accepted := make(chan net.Conn, 1)
	go func() {
		conn, err := wrapped.Accept()
		if err != nil {
			t.Error(err)
			accepted <- nil
			return
		}
		accepted <- conn
	}()
	client, err := net.Dial("tcp", inner.Addr().String())
	if err != nil {
		t.Fatal(err)
	}
	defer client.Close()
	server := <-accepted
	if server == nil {
		t.FailNow()
	}
	// Closing with linger 0 sends RST: the client sees a reset, not a clean EOF.
	if err := server.Close(); err != nil {
		t.Fatal(err)
	}
	buf := make([]byte, 1)
	_, err = client.Read(buf)
	if err == nil || errors.Is(err, io.EOF) {
		t.Fatalf("expected connection reset, got %v", err)
	}
}
