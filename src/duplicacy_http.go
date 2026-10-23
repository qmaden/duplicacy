// Copyright (c) Acrosync LLC. All rights reserved.
// Free for personal use and commercial trial
// Commercial use requires per-user licenses available from https://duplicacy.com

package duplicacy

import (
	"net"
	"net/http"
	"time"
)

// newHTTPClient returns an HTTP client for a storage backend used by 'threads' threads (0 if unknown).
//
// http.DefaultClient keeps only 2 idle connections per host, so with more threads most requests opened a new
// TCP and TLS connection, and it has no timeouts, so a stalled server could hang a backup forever.  There is no
// overall request timeout here because large chunks can take long to transfer; a server that stops responding is
// caught by the dial, TLS handshake and response header timeouts.
func newHTTPClient(threads int) *http.Client {
	if threads <= 0 {
		threads = 16
	}
	transport := http.DefaultTransport.(*http.Transport).Clone()
	transport.DialContext = (&net.Dialer{Timeout: 30 * time.Second, KeepAlive: 30 * time.Second}).DialContext
	transport.MaxIdleConnsPerHost = threads + 4
	if transport.MaxIdleConns < 2*transport.MaxIdleConnsPerHost {
		transport.MaxIdleConns = 2 * transport.MaxIdleConnsPerHost
	}
	transport.TLSHandshakeTimeout = 30 * time.Second
	transport.ResponseHeaderTimeout = 5 * time.Minute
	transport.ExpectContinueTimeout = 5 * time.Second
	return &http.Client{Transport: transport}
}
