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
	"testing"
	"time"
)

func TestConfigFromEnvParsesAndDefaults(t *testing.T) {
	t.Setenv("STUB_STARTUP_DELAY", "15s")
	t.Setenv("STUB_TOKEN_COUNT", "20")
	// STUB_TOKEN_DELAY unset → default.

	cfg := ConfigFromEnv()
	if cfg.StartupDelay != 15*time.Second {
		t.Fatalf("StartupDelay: want 15s, got %v", cfg.StartupDelay)
	}
	if cfg.TokenCount != 20 {
		t.Fatalf("TokenCount: want 20, got %d", cfg.TokenCount)
	}
	if cfg.TokenDelay != 50*time.Millisecond {
		t.Fatalf("TokenDelay default: want 50ms, got %v", cfg.TokenDelay)
	}
}
