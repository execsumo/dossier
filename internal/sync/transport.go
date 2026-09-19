package sync

import (
	"net"
	"net/http"
	"time"

	"github.com/go-git/go-git/v5/plumbing/transport/client"
	githttp "github.com/go-git/go-git/v5/plumbing/transport/http"
)

// ConnectTimeout bounds how long any git HTTP(S) operation waits to establish
// a connection. Go's default dialer waits 30 s, which made a manual
// `dossier sync` against an unreachable remote (off VPN, wrong host) sit for
// half a minute before failing. Only the connect and TLS handshake are
// bounded: a large push on a slow but working link is never cut off.
const ConnectTimeout = 10 * time.Second

func init() {
	c := githttp.NewClient(&http.Client{Transport: newHTTPTransport()})
	client.InstallProtocol("https", c)
	client.InstallProtocol("http", c)
}

// newHTTPTransport is http.DefaultTransport with bounded connect phases.
func newHTTPTransport() *http.Transport {
	t := http.DefaultTransport.(*http.Transport).Clone()
	t.DialContext = (&net.Dialer{
		Timeout:   ConnectTimeout,
		KeepAlive: 30 * time.Second,
	}).DialContext
	t.TLSHandshakeTimeout = ConnectTimeout
	return t
}
