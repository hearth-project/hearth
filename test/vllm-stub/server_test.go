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

package main

import (
	"bufio"
	"encoding/json"
	"io"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
	"time"
)

// fakeClock returns a now() the test can advance to drive the startup gate
// without sleeping.
func fakeClock(t time.Time) (func() time.Time, func(time.Duration)) {
	cur := t
	return func() time.Time { return cur }, func(d time.Duration) { cur = cur.Add(d) }
}

func TestHealthGatedByStartupDelay(t *testing.T) {
	now, advance := fakeClock(time.Unix(1000, 0))
	s := New(Config{StartupDelay: 15 * time.Second})
	s.now = now
	s.startedAt = now()
	srv := httptest.NewServer(s.Handler())
	defer srv.Close()

	get := func() int {
		resp, err := http.Get(srv.URL + "/health")
		if err != nil {
			t.Fatalf("GET /health: %v", err)
		}
		_ = resp.Body.Close()
		return resp.StatusCode
	}

	if code := get(); code != http.StatusServiceUnavailable {
		t.Fatalf("before delay: want 503, got %d", code)
	}
	advance(14 * time.Second)
	if code := get(); code != http.StatusServiceUnavailable {
		t.Fatalf("at 14s (< 15s delay): want 503, got %d", code)
	}
	advance(1 * time.Second)
	if code := get(); code != http.StatusOK {
		t.Fatalf("at 15s (>= delay): want 200, got %d", code)
	}
}

func TestChatCompletionsStreamsTokens(t *testing.T) {
	s := New(Config{TokenCount: 3})
	srv := httptest.NewServer(s.Handler())
	defer srv.Close()

	resp, err := http.Post(srv.URL+"/v1/chat/completions", "application/json",
		strings.NewReader(`{"model":"qwen","stream":true,"messages":[{"role":"user","content":"hi"}]}`))
	if err != nil {
		t.Fatalf("POST: %v", err)
	}
	defer func() { _ = resp.Body.Close() }()

	if ct := resp.Header.Get("Content-Type"); !strings.HasPrefix(ct, "text/event-stream") {
		t.Fatalf("want SSE content-type, got %q", ct)
	}

	var dataChunks, done int
	sc := bufio.NewScanner(resp.Body)
	for sc.Scan() {
		line := sc.Text()
		switch {
		case line == "data: [DONE]":
			done++
		case strings.HasPrefix(line, "data: "):
			var chunk struct {
				Object  string `json:"object"`
				Choices []struct {
					Delta struct {
						Content string `json:"content"`
					} `json:"delta"`
				} `json:"choices"`
			}
			if err := json.Unmarshal([]byte(strings.TrimPrefix(line, "data: ")), &chunk); err != nil {
				t.Fatalf("chunk not valid JSON: %q: %v", line, err)
			}
			if chunk.Object != "chat.completion.chunk" {
				t.Fatalf("want object=chat.completion.chunk, got %q", chunk.Object)
			}
			dataChunks++
		}
	}
	if dataChunks != 3 {
		t.Fatalf("want 3 token chunks, got %d", dataChunks)
	}
	if done != 1 {
		t.Fatalf("want exactly one [DONE], got %d", done)
	}
}

func TestMetricsSettableViaControl(t *testing.T) {
	s := New(Config{})
	srv := httptest.NewServer(s.Handler())
	defer srv.Close()

	metrics := func() string {
		resp, err := http.Get(srv.URL + "/metrics")
		if err != nil {
			t.Fatalf("GET /metrics: %v", err)
		}
		defer func() { _ = resp.Body.Close() }()
		body, _ := io.ReadAll(resp.Body)
		return string(body)
	}

	if got := metrics(); !strings.Contains(got, "vllm:num_requests_waiting 0") {
		t.Fatalf("default waiting metric missing:\n%s", got)
	}

	resp, err := http.Post(srv.URL+"/control", "application/json", strings.NewReader(`{"waiting": 5}`))
	if err != nil {
		t.Fatalf("POST /control: %v", err)
	}
	_ = resp.Body.Close()
	if resp.StatusCode != http.StatusOK {
		t.Fatalf("POST /control: want 200, got %d", resp.StatusCode)
	}

	if got := metrics(); !strings.Contains(got, "vllm:num_requests_waiting 5") {
		t.Fatalf("waiting metric not updated to 5:\n%s", got)
	}
}
