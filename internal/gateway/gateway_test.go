/*
Copyright 2026 The Hearth Authors.

Licensed under the Apache License, Version 2.0 (the "License");
you may not use this file except in compliance with the License.
You may obtain a copy of the License at

    http://www.apache.org/licenses/LICENSE-2.0

Unless required by applicable law or agreed to in writing, software
distributed under the License is distributed on an "AS IS" BASIS,
WITHOUT WARRANTIES OR CONDITIONS OF ANY KIND, either express or implied.
See the License for the specific language governing permissions and
limitations under the License.
*/

package gateway_test

import (
	"encoding/json"
	"io"
	"net/http"
	"net/http/httptest"
	"sync/atomic"
	"testing"
	"time"

	. "github.com/onsi/gomega"

	"github.com/hearth-project/hearth/internal/gateway"
)

// stubBackend is a fake vLLM: /health reflects a toggle, everything else echoes.
type stubBackend struct {
	ready   atomic.Bool
	release chan struct{} // when non-nil, /v1 handlers block until it closes
}

func (s *stubBackend) handler() http.Handler {
	mux := http.NewServeMux()
	mux.HandleFunc("/health", func(w http.ResponseWriter, _ *http.Request) {
		if s.ready.Load() {
			w.WriteHeader(http.StatusOK)
		} else {
			w.WriteHeader(http.StatusServiceUnavailable)
		}
	})
	mux.HandleFunc("/", func(w http.ResponseWriter, _ *http.Request) {
		if s.release != nil {
			<-s.release
		}
		_, _ = io.WriteString(w, "ok")
	})
	return mux
}

func newGateway(t *testing.T, backendURL string, maxQueue int, timeout time.Duration) *gateway.Gateway {
	t.Helper()
	g := NewWithT(t)
	gw, err := gateway.New(gateway.Config{
		BackendURL:        backendURL,
		MaxQueue:          maxQueue,
		ActivationTimeout: timeout,
		RetryInterval:     5 * time.Millisecond,
	})
	g.Expect(err).NotTo(HaveOccurred())
	return gw
}

func TestForwardsWhenBackendReady(t *testing.T) {
	g := NewWithT(t)
	be := &stubBackend{}
	be.ready.Store(true)
	srv := httptest.NewServer(be.handler())
	defer srv.Close()

	fe := httptest.NewServer(newGateway(t, srv.URL, 10, time.Second).Handler())
	defer fe.Close()

	resp, err := http.Post(fe.URL+"/v1/chat/completions", "application/json", nil)
	g.Expect(err).NotTo(HaveOccurred())
	defer func() { _ = resp.Body.Close() }()
	body, _ := io.ReadAll(resp.Body)
	g.Expect(resp.StatusCode).To(Equal(http.StatusOK))
	g.Expect(string(body)).To(Equal("ok"))
}

func TestHoldsThenForwardsOnColdStart(t *testing.T) {
	g := NewWithT(t)
	be := &stubBackend{} // starts not ready
	srv := httptest.NewServer(be.handler())
	defer srv.Close()

	// backend becomes ready shortly after the request arrives
	go func() {
		time.Sleep(40 * time.Millisecond)
		be.ready.Store(true)
	}()

	fe := httptest.NewServer(newGateway(t, srv.URL, 10, 2*time.Second).Handler())
	defer fe.Close()

	resp, err := http.Post(fe.URL+"/v1/x", "application/json", nil)
	g.Expect(err).NotTo(HaveOccurred())
	defer func() { _ = resp.Body.Close() }()
	g.Expect(resp.StatusCode).To(Equal(http.StatusOK))
}

func TestReturns503OnActivationTimeout(t *testing.T) {
	g := NewWithT(t)
	be := &stubBackend{} // never ready
	srv := httptest.NewServer(be.handler())
	defer srv.Close()

	fe := httptest.NewServer(newGateway(t, srv.URL, 10, 60*time.Millisecond).Handler())
	defer fe.Close()

	resp, err := http.Post(fe.URL+"/v1/x", "application/json", nil)
	g.Expect(err).NotTo(HaveOccurred())
	defer func() { _ = resp.Body.Close() }()
	g.Expect(resp.StatusCode).To(Equal(http.StatusServiceUnavailable))
	g.Expect(resp.Header.Get("Retry-After")).NotTo(BeEmpty())
}

func TestBackpressureReturns429WhenQueueFull(t *testing.T) {
	g := NewWithT(t)
	release := make(chan struct{})
	be := &stubBackend{release: release}
	be.ready.Store(true)
	srv := httptest.NewServer(be.handler())
	defer srv.Close()

	fe := httptest.NewServer(newGateway(t, srv.URL, 1, time.Second).Handler())
	defer fe.Close()

	// occupy the single slot with a request that blocks in the backend handler
	go func() {
		resp, err := http.Post(fe.URL+"/v1/hold", "application/json", nil)
		if err == nil {
			_ = resp.Body.Close()
		}
	}()
	// wait until the holder has been admitted (pending == 1)
	g.Eventually(func() int64 { return queuePending(fe.URL) }).Should(Equal(int64(1)))

	resp, err := http.Post(fe.URL+"/v1/second", "application/json", nil)
	g.Expect(err).NotTo(HaveOccurred())
	defer func() { _ = resp.Body.Close() }()
	g.Expect(resp.StatusCode).To(Equal(http.StatusTooManyRequests))

	close(release)
}

func queuePending(base string) int64 {
	resp, err := http.Get(base + gateway.QueuePath)
	if err != nil {
		return -1
	}
	defer func() { _ = resp.Body.Close() }()
	var body struct {
		Pending int64 `json:"pending"`
	}
	if json.NewDecoder(resp.Body).Decode(&body) != nil {
		return -1
	}
	return body.Pending
}
