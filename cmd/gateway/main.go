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

// Command gateway runs the Hearth data-plane proxy for a single LLMService.
package main

import (
	"log"
	"net/http"
	"os"

	"github.com/hearth-project/hearth/internal/gateway"
)

func main() {
	cfg := gateway.ConfigFromEnv()
	if cfg.BackendURL == "" {
		log.Fatalf("%s is required", gateway.EnvBackendURL)
	}

	gw, err := gateway.New(cfg)
	if err != nil {
		log.Fatalf("Failed to build gateway: %v", err)
	}

	addr := os.Getenv(gateway.EnvListenAddr)
	if addr == "" {
		addr = gateway.DefaultListenAddr
	}

	log.Printf("Hearth gateway listening on %s, backend %s", addr, cfg.BackendURL)
	if err := http.ListenAndServe(addr, gw.Handler()); err != nil { //nolint:gosec // G114: timeouts handled per-request
		log.Fatalf("Gateway server failed: %v", err)
	}
}
