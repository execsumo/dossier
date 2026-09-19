package sync

import (
	"context"
	"net"
	"testing"
	"time"
)

func TestHTTPTransportBoundsConnectPhase(t *testing.T) {
	tr := newHTTPTransport()
	if tr.TLSHandshakeTimeout != ConnectTimeout {
		t.Fatalf("TLS handshake timeout = %v, want %v", tr.TLSHandshakeTimeout, ConnectTimeout)
	}
	if tr.DialContext == nil {
		t.Fatal("DialContext not set")
	}
	if ConnectTimeout > 15*time.Second {
		t.Fatalf("ConnectTimeout %v is too long for an interactive sync", ConnectTimeout)
	}
	// Dialing a closed local port must fail fast rather than hang.
	ln, err := net.Listen("tcp", "127.0.0.1:0")
	if err != nil {
		t.Fatal(err)
	}
	addr := ln.Addr().String()
	_ = ln.Close()
	start := time.Now()
	if _, err := tr.DialContext(context.Background(), "tcp", addr); err == nil {
		t.Fatal("dial to a closed port succeeded")
	}
	if elapsed := time.Since(start); elapsed > ConnectTimeout {
		t.Fatalf("dial took %v", elapsed)
	}
}
