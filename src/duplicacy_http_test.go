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
	"time"
)

// W1: http.DefaultClient keeps only 2 idle connections per host, so after a burst of parallel requests most
// threads had to open a new connection; the storage client keeps one per thread.  The server holds each round of
// requests until all of them have arrived, so a round uses exactly one connection per thread, and the second
// round shows how many of those connections the client kept.
func TestHTTPClientReusesConnections(t *testing.T) {
	const threads = 8
	count := func(client *http.Client) int32 {
		var connections int32
		var lock sync.Mutex
		arrived := 0
		release := make(chan struct{})
		server := httptest.NewUnstartedServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			lock.Lock()
			arrived++
			wait := release
			if arrived == threads {
				close(release)
				arrived = 0
				release = make(chan struct{})
			}
			lock.Unlock()
			select {
			case <-wait:
			case <-time.After(10 * time.Second):
				t.Error("Not all requests of a round arrived")
			}
			w.Write([]byte("ok"))
		}))
		server.Config.ConnState = func(conn net.Conn, state http.ConnState) {
			if state == http.StateNew {
				atomic.AddInt32(&connections, 1)
			}
		}
		server.Start()
		defer server.Close()

		for round := 0; round < 2; round++ {
			var wg sync.WaitGroup
			for thread := 0; thread < threads; thread++ {
				wg.Add(1)
				go func() {
					defer wg.Done()
					response, err := client.Get(server.URL)
					if err != nil {
						t.Error(err)
						return
					}
					io.Copy(io.Discard, response.Body)
					response.Body.Close()
				}()
			}
			wg.Wait()
			// A connection goes back to the idle pool just after its response body is read to the end
			time.Sleep(200 * time.Millisecond)
		}
		return atomic.LoadInt32(&connections)
	}

	tuned := count(newHTTPClient(threads))
	defaultClient := count(&http.Client{Transport: http.DefaultTransport.(*http.Transport).Clone()})
	t.Logf("Connections for 2 rounds of %d parallel requests: %d with the storage client, %d with the default client",
		threads, tuned, defaultClient)
	if tuned != threads {
		t.Errorf("The storage client opened %d connections instead of reusing %d", tuned, threads)
	}
	if defaultClient <= threads {
		t.Errorf("The default client opened only %d connections; the test no longer shows the difference", defaultClient)
	}
}
