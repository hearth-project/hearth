//go:build e2e

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

package scaletozero

import (
	"context"
	"fmt"
	"io"
	"net/http"
	"strings"
	"sync"
	"time"

	. "github.com/onsi/ginkgo/v2"
	. "github.com/onsi/gomega"
)

// chat sends one OpenAI chat-completions request through a port-forwarded gateway and
// returns the status code and full (streamed) body. tokens>0 sets the stub stream length.
func chat(local, tokens int, stream bool, timeout time.Duration) (int, string, error) {
	ctx, cancel := context.WithTimeout(context.Background(), timeout)
	defer cancel()
	url := fmt.Sprintf("http://127.0.0.1:%d/v1/chat/completions", local)
	if tokens > 0 {
		url += fmt.Sprintf("?tokens=%d", tokens)
	}
	body := fmt.Sprintf(`{"stream":%t,"messages":[{"role":"user","content":"hi"}]}`, stream)
	req, err := http.NewRequestWithContext(ctx, http.MethodPost, url, strings.NewReader(body))
	if err != nil {
		return 0, "", err
	}
	req.Header.Set("Content-Type", "application/json")
	resp, err := httpClient.Do(req)
	if err != nil {
		return 0, "", err
	}
	defer func() { _ = resp.Body.Close() }()
	b, _ := io.ReadAll(resp.Body)
	return resp.StatusCode, string(b), nil
}

var _ = Describe("scale-to-zero loop", Ordered, func() {
	BeforeAll(func() {
		applyManifest("runtime.yaml")
		applyManifest("llmservice.yaml")
	})

	It("scales the backend to zero when idle", func() {
		Eventually(func() int { return backendReplicas("stub-svc") }, 2*time.Minute, 3*time.Second).
			Should(Equal(0), "KEDA should hold the backend at 0 replicas while idle")
	})

	It("renders the selected KEDA scaler transport", func() {
		Eventually(func() string {
			out, _ := kubectl("get", "scaledobject", "stub-svc", "-n", ns,
				"-o", "jsonpath={.spec.triggers[0].type}")
			return out
		}, time.Minute, 2*time.Second).Should(Equal(scalerMode))
		if scalerMode != "external-push" {
			_, err := kubectl("get", "service", "stub-svc-scaler", "-n", ns)
			Expect(err).To(HaveOccurred(), "metrics-api mode must not leave an internal scaler Service")
			return
		}

		address := mustKubectl("get", "scaledobject", "stub-svc", "-n", ns,
			"-o", "jsonpath={.spec.triggers[0].metadata.scalerAddress}")
		Expect(address).To(Equal("stub-svc-scaler.hearth-e2e.svc:9090"))
		mustKubectl("get", "service", "stub-svc-scaler", "-n", ns)

		cancel := portForward("stub-svc", 18084)
		defer cancel()
		Eventually(func() string {
			resp, err := httpClient.Get("http://127.0.0.1:18084/metrics")
			if err != nil {
				return ""
			}
			defer func() { _ = resp.Body.Close() }()
			body, _ := io.ReadAll(resp.Body)
			return string(body)
		}, time.Minute, 2*time.Second).Should(ContainSubstring("hearth_gateway_scaler_streams 1"))
	})

	It("wakes on a cold request and streams a real response (0→1)", func() {
		cancel := portForward("stub-svc", 18080)
		defer cancel()

		code, body, err := chat(18080, 0, true, 90*time.Second)
		Expect(err).NotTo(HaveOccurred())
		Expect(code).To(Equal(http.StatusOK))
		Expect(body).To(ContainSubstring("[DONE]"), "expected a streamed completion")
		Expect(backendReplicas("stub-svc")).To(BeNumerically(">=", 1), "the request should have woken the backend")
	})

	It("scales out under concurrent load (1→2)", func() {
		cancel := portForward("stub-svc", 18081)
		defer cancel()

		// Hold several long streams in flight so the gateway's pending count (target=1)
		// drives KEDA to the max of 2 replicas.
		var wg sync.WaitGroup
		for range 4 {
			wg.Add(1)
			go func() {
				defer wg.Done()
				defer GinkgoRecover()
				_, _, _ = chat(18081, 120, true, 2*time.Minute)
			}()
		}
		Eventually(func() int { return backendReplicas("stub-svc") }, 90*time.Second, 3*time.Second).
			Should(Equal(2), "concurrent load should scale the backend to its max of 2")
		wg.Wait()
	})

	It("returns to zero after the load drains (N→0)", func() {
		Eventually(func() int { return backendReplicas("stub-svc") }, 2*time.Minute, 3*time.Second).
			Should(Equal(0), "the backend should scale back to zero once idle")
	})

	It("lets an in-flight stream finish while the backend is scaled down (drain)", func() {
		applyManifest("drain.yaml")
		Eventually(func() int { return backendReplicas("stub-drain") }, 2*time.Minute, 3*time.Second).
			Should(Equal(0), "the drain service should start idle at zero")

		cancel := portForward("stub-drain", 18083)
		defer cancel()

		// Launch a deliberately long stream (≈12s of tokens once warm) in the background.
		type result struct {
			code int
			body string
			err  error
		}
		done := make(chan result, 1)
		go func() {
			defer GinkgoRecover()
			code, body, err := chat(18083, 80, true, 2*time.Minute)
			done <- result{code, body, err}
		}()

		By("waiting for the backend pod to be Ready (the stream has started)")
		Eventually(func() string {
			out, _ := kubectl("get", "pods", "-n", ns, "-l", "serving.hearth.dev/llmservice=stub-drain",
				"-o", "jsonpath={.items[*].status.conditions[?(@.type=='Ready')].status}")
			return out
		}, 2*time.Minute, 2*time.Second).Should(ContainSubstring("True"))

		By("deleting the backend pod mid-stream (the scale-down termination path)")
		out, err := kubectl("delete", "pod", "-n", ns,
			"-l", "serving.hearth.dev/llmservice=stub-drain", "--wait=false")
		Expect(err).NotTo(HaveOccurred(), out)

		By("the in-flight stream completing despite the pod terminating")
		var r result
		Eventually(done, 90*time.Second).Should(Receive(&r))
		Expect(r.err).NotTo(HaveOccurred())
		Expect(r.code).To(Equal(http.StatusOK))
		Expect(r.body).To(ContainSubstring("[DONE]"), "the stream must drain to completion, not truncate")
	})

	It("fast-503s in reject mode when the backend cannot schedule", func() {
		applyManifest("reject.yaml")
		cancel := portForward("stub-503", 18082)
		defer cancel()

		code, _, err := chat(18082, 0, false, 30*time.Second)
		Expect(err).NotTo(HaveOccurred())
		Expect(code).To(Equal(http.StatusServiceUnavailable), "reject mode should 503 immediately on a cold request")
	})
})
