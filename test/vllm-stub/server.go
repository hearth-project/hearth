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

// Command vllm-stub is a CPU-only fake of a vLLM OpenAI server: a /health gate, an
// OpenAI-compatible chat/completions endpoint, and a settable /metrics surface. It lets
// the gateway + KEDA scale-to-zero loop run end-to-end on kind with no GPU.
package main

import (
	"encoding/json"
	"fmt"
	"net/http"
	"strconv"
	"strings"
	"sync"
	"time"
)

type Config struct {
	StartupDelay time.Duration
	TokenCount   int
	TokenDelay   time.Duration
}

type Server struct {
	cfg       Config
	startedAt time.Time
	now       func() time.Time

	mu      sync.Mutex
	metrics vllmMetrics
}

// vllmMetrics mirrors the LLM-aware gauges the Hearth scraper reads off vLLM's /metrics.
type vllmMetrics struct {
	Waiting float64 `json:"waiting"`
	Running float64 `json:"running"`
	KVCache float64 `json:"kv_cache"`
}

func New(cfg Config) *Server {
	return &Server{cfg: cfg, now: time.Now, startedAt: time.Now()}
}

func (s *Server) Handler() http.Handler {
	mux := http.NewServeMux()
	mux.HandleFunc("/health", s.handleHealth)
	mux.HandleFunc("/v1/chat/completions", s.handleCompletions)
	mux.HandleFunc("/v1/completions", s.handleCompletions)
	mux.HandleFunc("/metrics", s.handleMetrics)
	mux.HandleFunc("/control", s.handleControl)
	return mux
}

func (s *Server) handleMetrics(w http.ResponseWriter, _ *http.Request) {
	s.mu.Lock()
	m := s.metrics
	s.mu.Unlock()
	w.Header().Set("Content-Type", "text/plain; version=0.0.4")
	for _, g := range []struct {
		name string
		val  float64
	}{
		{"vllm:num_requests_waiting", m.Waiting},
		{"vllm:num_requests_running", m.Running},
		{"vllm:gpu_cache_usage_perc", m.KVCache},
	} {
		_, _ = fmt.Fprintf(w, "# TYPE %s gauge\n%s %g\n", g.name, g.name, g.val)
	}
}

// handleControl lets a test set the LLM-aware gauges at runtime (e.g. raise queue depth
// to exercise warm 1→N scaling) without restarting the stub.
func (s *Server) handleControl(w http.ResponseWriter, r *http.Request) {
	var in vllmMetrics
	if err := json.NewDecoder(r.Body).Decode(&in); err != nil {
		http.Error(w, err.Error(), http.StatusBadRequest)
		return
	}
	s.mu.Lock()
	s.metrics = in
	s.mu.Unlock()
	w.WriteHeader(http.StatusOK)
}

// ready reports whether the configured startup delay has elapsed since boot, mimicking
// vLLM's /health returning 200 only once the engine has finished loading.
func (s *Server) ready() bool {
	return s.now().Sub(s.startedAt) >= s.cfg.StartupDelay
}

func (s *Server) handleHealth(w http.ResponseWriter, _ *http.Request) {
	if !s.ready() {
		w.WriteHeader(http.StatusServiceUnavailable)
		return
	}
	w.WriteHeader(http.StatusOK)
}

// completionRequest is the slice of the OpenAI request body the stub cares about.
type completionRequest struct {
	Stream bool `json:"stream"`
}

// tokens resolves the number of tokens for a request: a ?tokens= override (used by the
// drain test to launch a deliberately long stream) else the configured default.
func (s *Server) tokens(r *http.Request) int {
	if v, err := strconv.Atoi(r.URL.Query().Get("tokens")); err == nil && v > 0 {
		return v
	}
	if s.cfg.TokenCount > 0 {
		return s.cfg.TokenCount
	}
	return 1
}

func (s *Server) handleCompletions(w http.ResponseWriter, r *http.Request) {
	var req completionRequest
	_ = json.NewDecoder(r.Body).Decode(&req)
	n := s.tokens(r)
	if req.Stream {
		s.streamTokens(w, r, n)
		return
	}
	s.writeJSON(w, n)
}

// streamTokens emits n OpenAI chat.completion.chunk SSE events then a [DONE] sentinel,
// flushing each so the gateway streams them through as they arrive.
func (s *Server) streamTokens(w http.ResponseWriter, r *http.Request, n int) {
	h := w.Header()
	h.Set("Content-Type", "text/event-stream")
	h.Set("Cache-Control", "no-cache")
	flusher, _ := w.(http.Flusher)
	for i := range n {
		delta := map[string]any{"content": fmt.Sprintf("tok%d ", i)}
		chunk := map[string]any{
			"id":      "stub-cmpl",
			"object":  "chat.completion.chunk",
			"choices": []map[string]any{{"index": 0, "delta": delta}},
		}
		b, _ := json.Marshal(chunk)
		if _, err := fmt.Fprintf(w, "data: %s\n\n", b); err != nil {
			return
		}
		if flusher != nil {
			flusher.Flush()
		}
		select {
		case <-r.Context().Done():
			return
		case <-time.After(s.cfg.TokenDelay):
		}
	}
	_, _ = fmt.Fprint(w, "data: [DONE]\n\n")
	if flusher != nil {
		flusher.Flush()
	}
}

func (s *Server) writeJSON(w http.ResponseWriter, n int) {
	var content strings.Builder
	for i := range n {
		fmt.Fprintf(&content, "tok%d ", i)
	}
	message := map[string]any{"role": "assistant", "content": content.String()}
	w.Header().Set("Content-Type", "application/json")
	_ = json.NewEncoder(w).Encode(map[string]any{
		"id":      "stub-cmpl",
		"object":  "chat.completion",
		"choices": []map[string]any{{"index": 0, "message": message, "finish_reason": "stop"}},
	})
}
