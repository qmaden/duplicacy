// Copyright (c) Acrosync LLC. All rights reserved.
// Free for personal use and commercial trial
// Commercial use requires per-user licenses available from https://duplicacy.com

package duplicacy

import (
	"io"
	"net"
	"net/http"
	"net/http/httptest"
	"sync"
	"sync/atomic"
	"testing"
)

// W1: with http.DefaultClient (2 idle connections per host) most requests from parallel threads opened a new
// connection; the storage client keeps one connection per thread.
func TestHTTPClientReusesConnections(t *testing.T) {
	count := func(client *http.Client) int32 {
		var connections int32
		server := httptest.NewUnstartedServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			w.Write([]byte("ok"))
		}))
		server.Config.ConnState = func(conn net.Conn, state http.ConnState) {
			if state == http.StateNew {
				atomic.AddInt32(&connections, 1)
			}
		}
		server.Start()
		defer server.Close()

		var wg sync.WaitGroup
		for thread := 0; thread < 8; thread++ {
			wg.Add(1)
			go func() {
				defer wg.Done()
				for i := 0; i < 20; i++ {
					response, err := client.Get(server.URL)
					if err != nil {
						t.Error(err)
						return
					}
					io.Copy(io.Discard, response.Body)
					response.Body.Close()
				}
			}()
		}
		wg.Wait()
		return atomic.LoadInt32(&connections)
	}

	tuned := count(newHTTPClient(8))
	defaultClient := count(&http.Client{Transport: http.DefaultTransport.(*http.Transport).Clone()})
	t.Logf("Connections for 160 requests from 8 threads: %d with the storage client, %d with the default client",
		tuned, defaultClient)
	if tuned > 12 {
		t.Errorf("The storage client opened %d connections for 8 threads", tuned)
	}
}
