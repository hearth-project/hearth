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

// Package gateway is the Hearth data plane: an OpenAI-compatible reverse proxy that
// sits in front of one LLMService. It buffers requests while the backend is cold,
// applies bounded-queue backpressure, and exposes the pending-request count as the
// demand signal the scaler turns into a KEDA scale-from-zero decision.
package gateway

import (
	"context"
	"encoding/json"
	"net/http"
	"net/http/httputil"
	"net/url"
	"os"
	"strconv"
	"sync/atomic"
	"time"
)

// Environment variables read by ConfigFromEnv (and set by the operator-rendered Deployment).
const (
	EnvBackendURL        = "HEARTH_BACKEND_URL"
	EnvMaxQueue          = "HEARTH_MAX_QUEUE"
	EnvActivationTimeout = "HEARTH_ACTIVATION_TIMEOUT"
	EnvListenAddr        = "HEARTH_LISTEN_ADDR"

	// DefaultListenAddr is where the gateway serves the OpenAI API.
	DefaultListenAddr = ":8080"
	// QueuePath reports the pending-request count for the scaler.
	QueuePath = "/hearth/queue"
)

// Config configures a Gateway.
type Config struct {
	BackendURL        string
	MaxQueue          int
	ActivationTimeout time.Duration
	RetryInterval     time.Duration
}

// ConfigFromEnv builds a Config from the gateway's environment.
func ConfigFromEnv() Config {
	cfg := Config{BackendURL: os.Getenv(EnvBackendURL)}
	if v, err := strconv.Atoi(os.Getenv(EnvMaxQueue)); err == nil {
		cfg.MaxQueue = v
	}
	if d, err := time.ParseDuration(os.Getenv(EnvActivationTimeout)); err == nil {
		cfg.ActivationTimeout = d
	}
	return cfg
}

// Gateway is a buffering reverse proxy for a single backend.
type Gateway struct {
	cfg     Config
	backend *url.URL
	proxy   *httputil.ReverseProxy
	sem     chan struct{}
	pending atomic.Int64
	probe   *http.Client
	now     func() time.Time
}

// New builds a Gateway, applying defaults for unset fields.
func New(cfg Config) (*Gateway, error) {
	u, err := url.Parse(cfg.BackendURL)
	if err != nil {
		return nil, err
	}
	if cfg.MaxQueue <= 0 {
		cfg.MaxQueue = 100
	}
	if cfg.ActivationTimeout <= 0 {
		cfg.ActivationTimeout = 5 * time.Minute
	}
	if cfg.RetryInterval <= 0 {
		cfg.RetryInterval = 500 * time.Millisecond
	}
	return &Gateway{
		cfg:     cfg,
		backend: u,
		proxy:   httputil.NewSingleHostReverseProxy(u),
		sem:     make(chan struct{}, cfg.MaxQueue),
		probe:   &http.Client{Timeout: 2 * time.Second},
		now:     time.Now,
	}, nil
}

// Pending is the current demand signal: requests admitted and waiting or in flight.
func (g *Gateway) Pending() int64 { return g.pending.Load() }

// Handler returns the gateway's HTTP routes.
func (g *Gateway) Handler() http.Handler {
	mux := http.NewServeMux()
	mux.HandleFunc("/healthz", func(w http.ResponseWriter, _ *http.Request) { w.WriteHeader(http.StatusOK) })
	mux.HandleFunc(QueuePath, func(w http.ResponseWriter, _ *http.Request) {
		_ = json.NewEncoder(w).Encode(map[string]int64{"pending": g.pending.Load()})
	})
	mux.HandleFunc("/", g.serve)
	return mux
}

func (g *Gateway) serve(w http.ResponseWriter, r *http.Request) {
	// Bounded-queue backpressure: reject rather than buffer to OOM.
	select {
	case g.sem <- struct{}{}:
		defer func() { <-g.sem }()
	default:
		w.Header().Set("Retry-After", "5")
		http.Error(w, "gateway queue full", http.StatusTooManyRequests)
		return
	}

	// Demand signal for the scaler, raised for the whole hold-and-serve window.
	g.pending.Add(1)
	defer g.pending.Add(-1)

	if !g.waitForBackend(r.Context()) {
		w.Header().Set("Retry-After", "10")
		http.Error(w, "backend not ready (activation timeout)", http.StatusServiceUnavailable)
		return
	}
	g.proxy.ServeHTTP(w, r)
}

// waitForBackend blocks until the backend is ready, the request is canceled, or the
// activation timeout elapses (cold-start hold).
func (g *Gateway) waitForBackend(ctx context.Context) bool {
	deadline := g.now().Add(g.cfg.ActivationTimeout)
	for {
		if g.backendReady(ctx) {
			return true
		}
		if g.now().After(deadline) {
			return false
		}
		select {
		case <-ctx.Done():
			return false
		case <-time.After(g.cfg.RetryInterval):
		}
	}
}

func (g *Gateway) backendReady(ctx context.Context) bool {
	req, err := http.NewRequestWithContext(ctx, http.MethodGet, g.backend.String()+"/health", nil)
	if err != nil {
		return false
	}
	resp, err := g.probe.Do(req)
	if err != nil {
		return false
	}
	defer func() { _ = resp.Body.Close() }()
	return resp.StatusCode == http.StatusOK
}
