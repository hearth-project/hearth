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
	"log"
	"net/http"
	"os"
	"strconv"
	"time"
)

const defaultTokenDelay = 50 * time.Millisecond

// ConfigFromEnv builds the stub config from STUB_* environment variables, which the
// e2e test sets on the stub-backed Deployment to shape cold-start and streaming timing.
func ConfigFromEnv() Config {
	cfg := Config{TokenDelay: defaultTokenDelay}
	if d, err := time.ParseDuration(os.Getenv("STUB_STARTUP_DELAY")); err == nil {
		cfg.StartupDelay = d
	}
	if n, err := strconv.Atoi(os.Getenv("STUB_TOKEN_COUNT")); err == nil {
		cfg.TokenCount = n
	}
	if d, err := time.ParseDuration(os.Getenv("STUB_TOKEN_DELAY")); err == nil {
		cfg.TokenDelay = d
	}
	return cfg
}

func main() {
	addr := os.Getenv("STUB_LISTEN_ADDR")
	if addr == "" {
		addr = ":8000"
	}
	s := New(ConfigFromEnv())
	log.Printf("vllm-stub listening on %s (startupDelay=%v tokens=%d)", addr, s.cfg.StartupDelay, s.cfg.TokenCount)
	if err := http.ListenAndServe(addr, s.Handler()); err != nil { //nolint:gosec // G114: test-only stub
		log.Fatalf("vllm-stub server failed: %v", err)
	}
}
