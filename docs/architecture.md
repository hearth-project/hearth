# Architecture

Hearth is a minimal, composable LLM serving control plane for private Kubernetes clusters. A single
`LLMService` manifest produces a declarative, queue-driven, **scale-to-zero** model server across
NVIDIA, Ascend, and other accelerators.

## Boundary (what Hearth is, and isn't)

Hearth owns the **Kubernetes orchestration / lifecycle layer**: rendering workloads, model loading,
health, scheduling adaptation, scale-to-zero, and metrics. It deliberately does **not** re-implement
the inference engine (that's **vLLM** + vendor plugins) or write chip kernels / device plugins /
schedulers (that's the vendors, **HAMi**, **Volcano**). Fleet routing, prefill/decode disaggregation,
and datacenter scale-out belong to **Kthena**, **AIBrix**, and **KServe**/**llm-d**; Hearth composes
with them as the lightweight, scale-to-zero end of that axis (see the
README's ["Hearth and Kthena"](../README.md#hearth-and-kthena)). A new accelerator is a thin
adapter, not a rewrite.

## CRDs

API group `serving.hearth.dev/v1alpha1`.

- **`LLMService`** (namespaced, user-facing) — *what* to serve and *how* to scale: the model source,
  a runtime selection (pin a backend or auto-pick by vendor), abstract resources, scaling intent
  (incl. `min: 0` for scale-to-zero), cache strategy, and cold-start behavior.
- **`InferenceRuntime`** (cluster-scoped, reusable) — a *pluggable backend driver*: container image +
  templated args, the device-plugin resource name (e.g. `nvidia.com/gpu`, `huawei.com/Ascend910`),
  scheduling (nodeSelector/tolerations/scheduler), model-load-aware probes, lifecycle (drain), and
  where the LLM-aware metrics live. This is the multi-backend differentiator.

## Components

- **Operator / controllers** (`internal/controller`) — reconcile an `LLMService` (+ its
  `InferenceRuntime`) into the child objects below via server-side apply, gracefully skipping the
  optional KEDA CRD when absent.
- **Backend abstraction** (`internal/backend`) — a `BackendAdapter` interface + registry. Shared code
  renders the vLLM pod, accelerator request, and metrics source; thin NVIDIA and Ascend adapters
  add vendor specifics. Adapters are golden-tested, so they're provable without hardware.
- **Gateway** (`internal/gateway`) — the data plane: an OpenAI-compatible reverse proxy in front of
  each `LLMService`. It buffers requests during cold start, applies bounded-queue backpressure,
  emits SSE keepalive heartbeats (or fast-rejects), drains in-flight streams on scale-down, and
  exposes its pending-request count as the demand signal.

## Reconcile output

For one `LLMService`, the operator renders: a vLLM **Deployment** (it does *not* set `replicas` —
KEDA owns `0..N`), a backend **Service**, a **gateway** Deployment + Service, a model **cache**
(PVC/HostPath) + optional **prewarm Job**, and a KEDA **ScaledObject**.

```mermaid
flowchart TD
  llm["LLMService<br/>(+ InferenceRuntime)"] --> op["Hearth operator"]
  op --> dep["vLLM Deployment<br/>(replicas owned by KEDA)"]
  op --> bsvc["Backend Service"]
  op --> gwd["Gateway Deployment + Service"]
  op --> cache["Model cache (PVC/HostPath)<br/>+ optional prewarm Job"]
  op --> so["KEDA ScaledObject"]
```

## Scale-to-zero data flow

```mermaid
flowchart LR
  client(["Inference client"])
  subgraph dp["Data plane"]
    gw["Hearth Gateway<br/>buffer · backpressure<br/>keepalive · drain"]
  end
  subgraph wl["Workload"]
    svc["Backend Service"]
    pods["vLLM pods (0..N)"]
    cache[("Model cache<br/>PVC / HostPath")]
  end
  subgraph cp["Autoscaling"]
    keda["KEDA core"]
    so["ScaledObject"]
  end
  client -->|"OpenAI API"| gw
  gw -->|"forward when Ready"| svc --> pods
  pods -.->|"load weights"| cache
  keda -->|"poll /hearth/queue (pending)"| gw
  keda --> so
  so -->|"scale 0..N"| pods

  classDef plane fill:#eef6ff,stroke:#4a90d9,color:#1b3a5b;
  classDef work fill:#f3f0ff,stroke:#8b5cf6,color:#3b2a6b;
  classDef auto fill:#fff7ed,stroke:#fb923c,color:#7c3a12;
  class gw plane;
  class svc,pods,cache work;
  class keda,so auto;
```

1. **Idle** — KEDA holds the backend Deployment at **0**.
2. **Cold request** — the gateway admits the request (bounded queue → `429` if full), raises its
   `pending` count, and holds the connection. In `keepalive` mode it streams SSE heartbeats so the
   client/ingress don't time out; in `reject` mode it returns `503 + Retry-After` and the client retries.
3. **Activation** — KEDA's `metrics-api` trigger polls the gateway's `/hearth/queue`; `pending > 0`
   drives the Deployment **0 → 1**. The pod loads weights from the local cache (prewarmed) and only
   becomes **Ready** once the model is loaded (load-gated readiness probes).
4. **Serve** — the gateway forwards the buffered request and streams tokens back.
5. **Scale up** — sustained queue depth above the per-replica target scales **1 → N** (one whole
   device per replica).
6. **Scale down** — when demand drops, KEDA scales back toward **0**; a `preStop` drain lets
   in-flight streams finish before the pod is terminated.

## Observability

vLLM and the gateway expose `/metrics` through Services with a stable `http` port and
`serving.hearth.dev/llmservice` discovery label. Hearth does not install or reconcile Prometheus
Operator resources. The independent [`examples/observability`](../examples/observability) package
provides an opt-in `ServiceMonitor` and Grafana dashboard.

## Caching

Cold-start cost is dominated by fetching + loading weights, so caching is what makes scale-to-zero
usable. Hearth supports `HostPath` and `NodeLocalPVC` (with a pinnable `storageClassName`), plus a
prewarm Job for Hugging Face or ModelScope weights. A `pvc://` source mounts pre-staged weights
read-only and skips prewarming. Node-local caches are per-node today;
`SharedPVC` (RWX) for multi-node is on the roadmap.
Prewarm Pods inherit the runtime's node selector, tolerations, and scheduler but do not consume an
accelerator.
For clusters without a dynamic StorageClass, see the NVIDIA A100 HostPath example in
[`examples/nvidia/a100/serving_v1alpha1_llmservice_nvidia_a100_hostpath.yaml`](../examples/nvidia/a100/serving_v1alpha1_llmservice_nvidia_a100_hostpath.yaml).
