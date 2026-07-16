# Developing Hearth without a GPU

You can build, test, and exercise almost all of Hearth — the operator, the data-plane gateway, and
the **scale-to-zero logic** — on a laptop with no accelerator. The only thing that genuinely needs an
NVIDIA GPU (or an Ascend NPU) is serving *real* model tokens; everything else is faked by the
`vllm-stub`.

This guide complements [`CONTRIBUTING.md`](../CONTRIBUTING.md), which covers running the control
plane against a cluster.

## What runs with no hardware

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
gateway and optional observability checks, so the gateway + KEDA scale-to-zero path can run on kind
with no GPU:

- **`/health`** — returns `503` until `STUB_STARTUP_DELAY` has elapsed since boot, then `200`. This
  drives the gateway's cold-start keepalive and `activationTimeout` paths (mimics vLLM only going
  ready once weights are loaded).
- **`/v1/chat/completions` and `/v1/completions`** — honors `"stream": true` (SSE chunks +
  `[DONE]`) or returns a single JSON body. Emits `STUB_TOKEN_COUNT` tokens at `STUB_TOKEN_DELAY`
  each. A per-request **`?tokens=N`** override sets the stream length for timing-sensitive tests.
- **`/metrics`** — Prometheus text with `vllm:num_requests_waiting`, `vllm:num_requests_running`,
  `vllm:gpu_cache_usage_perc`, all settable at runtime via **`POST /control`** (e.g.
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

**Podman + kind:** `kind load docker-image` reads docker's store, so for podman-built images load
from an archive instead, and tell kind to use podman:

```bash
podman save hearth.dev/vllm-stub:e2e -o /tmp/stub.tar
KIND_EXPERIMENTAL_PROVIDER=podman kind load image-archive /tmp/stub.tar --name <cluster>
```

### Try it directly

```bash
docker run -d --name stub -p 8000:8000 \
  -e STUB_STARTUP_DELAY=2s -e STUB_TOKEN_COUNT=3 hearth.dev/vllm-stub:e2e

curl -s localhost:8000/health                       # 503 for the first 2s, then 200
curl -s localhost:8000/v1/chat/completions -d '{"stream":true,"messages":[]}'
curl -s localhost:8000/metrics | grep waiting       # vllm:num_requests_waiting 0
curl -s localhost:8000/control -d '{"waiting":5}'    # raise the gauge
curl -s 'localhost:8000/v1/completions?tokens=2' -d '{"stream":true}'  # 2-token stream
```

> If `localhost` requests hang or 502 behind a corporate proxy, set
> `NO_PROXY=localhost,127.0.0.1`.

## Full scale-to-zero loop on kind

The end-to-end `0→1→N→0` loop (idle → cold request wakes the backend → autoscale → drain back to
zero, plus reject-mode 503) runs on kind with **no GPU**, in `test/scaletozero/`. It backs each
`LLMService` with the stub, advertises a fake accelerator resource on the node (via the node-status
API, so no device plugin), and runs the operator out-of-cluster.

**Prerequisites you provide:** a running Kind cluster (current kube-context) with **KEDA** installed:

```bash
helm install keda kedacore/keda --version 2.20.1 -n keda --create-namespace
```

**Run it** (builds + loads the stub and gateway images, then runs the suite):

```bash
make test-scale-e2e CONTAINER_TOOL=podman          # or omit CONTAINER_TOOL to use docker
```

For Podman, also export `KIND_EXPERIMENTAL_PROVIDER=podman` so `kind load` targets the right
cluster. The suite installs Hearth's own CRDs but expects KEDA to be present (it fails fast with
instructions otherwise). CI runs the same loop on every PR via `.github/workflows/test-scale-e2e.yml`.
