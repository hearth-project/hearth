# Developing Hearth without an accelerator

You can build, test, and exercise the operator, gateway, and scale-to-zero lifecycle on a CPU-only
workstation. The `vllm-stub` replaces the inference process for these tests. Real accelerator-backed
inference and hardware support claims still require validation on the physical device.

This guide complements [`CONTRIBUTING.md`](../CONTRIBUTING.md), which covers running the control
plane against a cluster.

## What runs without accelerator hardware

| Check | Command | What it covers |
|---|---|---|
| Unit + golden tests | `make test` | reconcilers (envtest), adapter manifest rendering (incl. `ascend`), cache, gateway, scaler logic |
| Stub tests | `go test ./test/vllm-stub/...` | the fake vLLM server itself |
| Lint | `make lint` | `golangci-lint` (run before every PR) |
| Control-plane reconcile | `make install && make run` | operator turns an `LLMService` into its child objects (see CONTRIBUTING) |

Adapter rendering is testable without hardware, but support claims require validation on a real
accelerator. Rendering tests live under `internal/backend/*/`.

## The `vllm-stub`

`test/vllm-stub/` is a CPU-only fake of a vLLM OpenAI server. It exposes the surfaces used by the
gateway and optional observability checks, so the gateway + KEDA scale-to-zero path can run on Kind
without an accelerator:

- **`/health`** — returns `503` until `STUB_STARTUP_DELAY` has elapsed since boot, then `200`. This
  drives the gateway's cold-start keepalive and `activationTimeout` paths (mimics vLLM only going
  ready once weights are loaded).
- **`/v1/chat/completions` and `/v1/completions`** — honors `"stream": true` (SSE chunks +
  `[DONE]`) or returns a single JSON body. Emits `STUB_TOKEN_COUNT` tokens at `STUB_TOKEN_DELAY`
  each. A per-request **`?tokens=N`** override sets the stream length for timing-sensitive tests.
- **`/metrics`** — Prometheus text with `vllm:num_requests_waiting`, `vllm:num_requests_running`,
  `vllm:kv_cache_usage_perc`, all settable at runtime via **`POST /control`** (e.g.
  `{"waiting": 5}`).

### Configuration

| Env | Default | Purpose |
|---|---|---|
| `STUB_STARTUP_DELAY` | `0s` | delay before `/health` flips to `200` (fake cold start) |
| `STUB_TOKEN_COUNT` | `1` | tokens per streamed/JSON response |
| `STUB_TOKEN_DELAY` | `50ms` | delay between streamed tokens |
| `STUB_LISTEN_ADDR` | `:8000` | listen address |

### Build the image

```bash
make docker-build-stub                      # uses CONTAINER_TOOL (docker by default)
make docker-build-stub CONTAINER_TOOL=podman
```

**Podman + Kind:** `kind load docker-image` reads Docker's store, so for Podman-built images load
from an archive instead, and tell Kind to use Podman:

```bash
podman save hearth.dev/vllm-stub:e2e -o /tmp/stub.tar
KIND_EXPERIMENTAL_PROVIDER=podman kind load image-archive /tmp/stub.tar --name <cluster>
```

## Full scale-to-zero loop on Kind

The end-to-end `0→1→N→0` loop (idle → cold request wakes the backend → autoscale → drain back to
zero, plus reject-mode 503) runs on Kind without an accelerator, in `test/scaletozero/`. It backs each
`LLMService` with the stub, advertises a fake accelerator resource on the node (via the node-status
API, so no device plugin), and runs the operator out-of-cluster.

The suite mutates an existing cluster and expects both a Kind cluster named `kind` and the current
kube-context to be `kind-kind`. Use a dedicated disposable cluster. For Podman, export the two
variables before creating it; Docker users can omit them.

```bash
# Podman only:
# export CONTAINER_TOOL=podman
# export KIND_EXPERIMENTAL_PROVIDER=podman

kind create cluster --name kind --wait 120s
test "$(kubectl config current-context)" = "kind-kind"

helm repo add kedacore https://kedacore.github.io/charts  
helm repo update
helm upgrade --install keda kedacore/keda \
  --version 2.20.1 \
  -n keda --create-namespace
```

The KEDA version above matches `.github/workflows/test-scale-e2e.yml`. Then run both scaler modes;
each command builds and loads the stub and gateway images before starting the suite:

```bash
make test-scale-e2e
make test-scale-e2e SCALE_SCALER_MODE=external-push
```

The first command verifies the default metrics API polling path; the second verifies the internal
external-push scaler Service, KEDA stream, activation, scale-out, drain, and return to zero. The
suite installs Hearth's CRDs but expects KEDA to be present and fails fast when it is not. It does
not delete the cluster afterward:

```bash
kind delete cluster --name kind
```

CI runs both modes on every PR via `.github/workflows/test-scale-e2e.yml`.
