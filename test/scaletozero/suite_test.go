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

// Package scaletozero is the no-GPU end-to-end harness: it drives the full
// gateway + KEDA scale-to-zero loop on kind, backed by the CPU vllm-stub and a
// fake node resource, with the operator running out-of-cluster. No accelerator
// is required, so it runs in CI on every PR.
package scaletozero

import (
	"context"
	"fmt"
	"net"
	"net/http"
	"os"
	"os/exec"
	"path/filepath"
	"runtime"
	"strings"
	"syscall"
	"testing"
	"time"

	. "github.com/onsi/ginkgo/v2"
	. "github.com/onsi/gomega"
)

const (
	ns           = "hearth-e2e"
	gatewayImage = "hearth.dev/hearth-gateway:e2e"
	fakeResource = "hearth.dev/fake-gpu"
)

var (
	repoRoot       string
	scalerMode     string
	operator       *exec.Cmd
	operatorCancel context.CancelFunc
	// httpClient bypasses any ambient HTTP proxy so port-forwarded localhost works.
	httpClient = &http.Client{Transport: &http.Transport{Proxy: nil}}
)

func TestScaleToZero(t *testing.T) {
	RegisterFailHandler(Fail)
	RunSpecs(t, "scale-to-zero e2e suite")
}

// sh runs a command from the repo root and returns combined output.
func sh(name string, args ...string) (string, error) {
	cmd := exec.Command(name, args...)
	cmd.Dir = repoRoot
	out, err := cmd.CombinedOutput()
	return string(out), err
}

func kubectl(args ...string) (string, error) { return sh("kubectl", args...) }

func mustKubectl(args ...string) string {
	out, err := kubectl(args...)
	ExpectWithOffset(1, err).NotTo(HaveOccurred(), "kubectl "+strings.Join(args, " ")+":\n"+out)
	return out
}

func applyManifest(name string) {
	out, err := kubectl("apply", "-f", filepath.Join("test", "scaletozero", "testdata", name))
	Expect(err).NotTo(HaveOccurred(), out)
}

// backendReplicas reads the backend Deployment's desired replica count (KEDA-owned).
func backendReplicas(name string) int {
	out, err := kubectl("get", "deploy", name, "-n", ns, "-o", "jsonpath={.spec.replicas}")
	if err != nil || out == "" {
		return -1
	}
	var n int
	_, _ = fmt.Sscanf(out, "%d", &n)
	return n
}

var _ = BeforeSuite(func() {
	_, file, _, _ := runtime.Caller(0)
	repoRoot, _ = filepath.Abs(filepath.Join(filepath.Dir(file), "..", ".."))
	scalerMode = os.Getenv("HEARTH_E2E_SCALER_MODE")
	if scalerMode == "" {
		scalerMode = "metrics-api"
	}
	Expect([]string{"metrics-api", "external-push"}).To(ContainElement(scalerMode))

	By("requiring KEDA to be installed (a component you install via Helm, not the suite)")
	_, err := kubectl("get", "crd", "scaledobjects.keda.sh")
	Expect(err).NotTo(HaveOccurred(),
		"KEDA is not installed. Install it first, e.g.:\n"+
			"  helm install keda kedacore/keda -n keda --create-namespace")

	By("installing Hearth's own CRDs (the component under test)")
	out, err := sh("make", "install")
	Expect(err).NotTo(HaveOccurred(), out)

	By("advertising a fake accelerator resource on every node (no device plugin)")
	nodeList := strings.Fields(mustKubectl("get", "nodes", "-o", "jsonpath={.items[*].metadata.name}"))
	Expect(nodeList).NotTo(BeEmpty())
	patch := fmt.Sprintf(`{"status":{"capacity":{"%s":"8"}}}`, fakeResource)
	for _, node := range nodeList {
		mustKubectl("patch", "node", node, "--subresource=status", "--type=merge", "-p", patch)
	}

	By("creating the test namespace")
	_, _ = kubectl("create", "namespace", ns)

	By("building and starting the operator out-of-cluster")
	out, err = sh("go", "build", "-o", "bin/manager", "./cmd/main.go")
	Expect(err).NotTo(HaveOccurred(), out)
	ctx, cancel := context.WithCancel(context.Background())
	operatorCancel = cancel
	operator = exec.CommandContext(ctx, filepath.Join(repoRoot, "bin", "manager"),
		"--gateway-image="+gatewayImage,
		"--metrics-bind-address=0",
		"--health-probe-bind-address=:18181",
		"--leader-elect=false",
		"--scaler-mode="+scalerMode)
	operator.Dir = repoRoot
	operator.Stdout = GinkgoWriter
	operator.Stderr = GinkgoWriter
	operator.SysProcAttr = &syscall.SysProcAttr{Setpgid: true}
	Expect(operator.Start()).To(Succeed())
	time.Sleep(3 * time.Second) // let the manager connect and start its workers
	Expect(operator.ProcessState).To(BeNil(), "operator exited during startup")
})

var _ = AfterSuite(func() {
	By("removing test resources")
	for _, f := range []string{"reject.yaml", "drain.yaml", "llmservice.yaml", "runtime.yaml"} {
		_, _ = kubectl("delete", "-f", filepath.Join("test", "scaletozero", "testdata", f), "--ignore-not-found")
	}
	_, _ = kubectl("delete", "namespace", ns, "--ignore-not-found")

	By("stopping the operator")
	if operatorCancel != nil {
		operatorCancel()
	}
	if operator != nil && operator.Process != nil {
		_ = syscall.Kill(-operator.Process.Pid, syscall.SIGKILL)
	}
})

// portForward opens a kubectl port-forward to a gateway Service and waits until the
// local port accepts connections. The returned cancel tears the tunnel down.
func portForward(svcName string, local int) context.CancelFunc {
	// The always-on gateway must have a ready endpoint before port-forward can bind,
	// otherwise kubectl exits immediately and never recovers.
	out, err := kubectl("wait", "--for=condition=Available", "--timeout=120s",
		"deploy/"+svcName+"-gateway", "-n", ns)
	Expect(err).NotTo(HaveOccurred(), out)

	ctx, cancel := context.WithCancel(context.Background())
	cmd := exec.CommandContext(ctx, "kubectl", "port-forward", "-n", ns,
		"svc/"+svcName, fmt.Sprintf("%d:80", local))
	cmd.Stdout = GinkgoWriter
	cmd.Stderr = GinkgoWriter
	cmd.SysProcAttr = &syscall.SysProcAttr{Setpgid: true}
	Expect(cmd.Start()).To(Succeed())
	Eventually(func() error {
		c, err := net.DialTimeout("tcp", fmt.Sprintf("127.0.0.1:%d", local), time.Second)
		if err == nil {
			_ = c.Close()
		}
		return err
	}, 30*time.Second, time.Second).Should(Succeed())
	return cancel
}
