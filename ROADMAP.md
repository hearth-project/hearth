# Hearth Roadmap

> **Status: v0 (early / alpha).** The core thesis — declarative, scale-to-zero serving of
> self-hosted OSS LLMs on Kubernetes — is implemented and verified end-to-end on real hardware.
> The API is `v1alpha1` (no stability guarantee). **Not production-ready for shared or
> customer-facing workloads** — see [Production readiness](#production-readiness).

This roadmap is honest about what works, what it's good for today, and the prioritized path to a
production-grade release. It's a living document.

---

## What v0 does today (verified)

Verified **live on real hardware** (NVIDIA A100 on Alibaba ACK, single- and multi-node) and on kind:

- **Declarative deploy** — one `LLMService` renders Deployment + Services + KEDA `ScaledObject` +
  `ServiceMonitor` + cache + prewarm Job, with owner-ref cascade.
- **Scale-to-zero** — KEDA holds the backend at 0 when idle; a request wakes it.
- **Cold-start handling** — gateway buffers the request, emits SSE **keepalive heartbeats** so
  clients/ingress don't time out, holds until the model is loaded, then streams real tokens.
  Cold start ≈ **100 s (Qwen3-0.6B)** / **110 s (Qwen3-8B)** from zero, prewarmed.
- **Queue-driven autoscaling** — `0→1→N→0` on gateway queue depth; verified `1→2` across two GPU nodes.
- **Backpressure & limits** — bounded queue → `429`; activation timeout → `503`; `reject` cold-start mode.
- **Model caching + prewarm** — `HostPath` and `NodeLocalPVC` (incl. pinnable `storageClassName`,
  verified against Alibaba ESSD); weights hydrated before first traffic.
- **Graceful drain** — in-flight streams finish before a scale-down SIGTERM.
- **Observability** — Prometheus scrapes gateway + vLLM `/metrics` via `ServiceMonitor`.
- **Packaging** — Helm chart installs operator + RBAC; verified reconciling under chart RBAC.
- **Multi-backend abstraction** — NVIDIA implemented & run; **Ascend 910B** is an experimental
  preview: vLLM-Ascend serves on a real 910B, manifests render correctly, and the gateway data-plane
  is verified on the NPU — only the device-plugin scheduling e2e is pending
  (see [Ascend 910B validation](docs/ascend-910b-validation.md)).
- **Ascend 310P lifecycle** — Atlas 300I Duo is verified through the device plugin and Hearth
  gateway, including `0→1→2→0`, backpressure, reject mode, drain, caching, and reboot recovery
  (see [Ascend 310P validation](docs/ascend-310p-validation.md)). Atlas 300I Pro remains
  rendering-tested only.
- **No-GPU CI loop** — the full `0→1→N→0` scale-to-zero e2e (CPU `vllm-stub` + a fake extended
  resource on kind) runs in CI; contributing needs no accelerator.

## Production readiness

**Use it today for:** internal / dev / staging serving where **GPU cost matters and brief downtime
is tolerable**, **latency-tolerant** traffic (cold start is seconds-to-minutes), and **packing many
mostly-idle models onto few GPUs**. Label deployments as alpha.

**Do not use yet for:** customer-facing low-latency endpoints, shared/multi-tenant clusters, or
anything requiring auth, SLAs, or stability guarantees.

---

## Path to production

### Now — finish domestic hardware coverage

- **Complete the Ascend 910B loop.** Status as of `v0.2.0-rc.1` (see
  [Ascend 910B validation](docs/ascend-910b-validation.md)):

  - [x] vLLM-Ascend serves on a real 910B (CANN 9.0.0 / driver 26.0.rc1, vllm-ascend 0.21.0rc1).
  - [x] Operator renders correct 910B manifests (`huawei.com/Ascend910`, driver mounts, cache, probes).
  - [x] Gateway data-plane verified on the NPU (queue signal, passthrough, cold-start keepalive).
  - [x] Runtime image pinned to the verified tag (`vllm-ascend:v0.21.0rc1`).
  - [ ] **Operator → device plugin → pod scheduled and serving on the NPU**, and the full integrated
        `0→1→N→0` loop on a real NPU node — needs a schedulable node (the preview box was an
        unprivileged container, so nested k8s couldn't run). Closing this earns the claim
        *"validated end-to-end on real domestic silicon (Ascend 910B)"*.

- [x] **Atlas 300I Duo.** The physical run passed the integrated `0→1→2→0` lifecycle,
  streaming inference, bounded-queue backpressure, reject mode, graceful drain, cache persistence,
  self-heal, Helm upgrade, and reboot recovery.
- [ ] **Atlas 300I Pro.** Validate it independently; the Duo result is not evidence for Pro. Follow
  the [310P report and runbook](docs/ascend-310p-validation.md).
- The **Moore Threads (MUSA)** backend (`moorethreads` + `vllm-musa`, MTT S5000) is scaffolded and
  golden-tested, ready as the second backend.

- [ ] **Volcano live validation** — `scheduler.queue` → `scheduling.volcano.sh/queue-name` rendering
  is golden-tested; verify queue placement + `0→1` under a real Volcano scheduler. HAMi sharing /
  gang scheduling follows.

### P1 — unblock private / enterprise delivery
- [x] **`imagePullSecrets`** — private-registry support on backend, prewarm, and gateway Pods.
- [x] **`pvc://` model sources** — pre-staged, read-only weights with no download at serve time.
- [ ] **`oci://` model sources** — portable offline model delivery for the air-gapped bundle.
- [ ] **`SharedPVC` (RWX) cache** — node-local cache is per-node today, so each new replica
      re-downloads weights; RWX shared cache fixes multi-node cold starts.
- [ ] **Reliable multi-node scale-out** — a replica on a node without the runtime image cached pays a
      multi-minute image pull, and **KEDA scale-down churn can cancel an in-progress pull** so the
      replica never becomes Ready under bursty load (observed on the 2-node A100 run). Ship guidance +
      support for **image pre-distribution**: VPC/in-region registry endpoints, node image pre-pull
      (DaemonSet / ACK ImageCache), and/or a `scaleDownStabilization` floor that won't cancel pulls.
- [ ] **Helm/CRD install ergonomics** — document the Helm-v4-SSA vs `kubectl apply` CRD-ownership
      conflict; smooth upgrades.

### P2 — production hardening (shared / exposed use)
- [ ] **Minimal gateway auth** — static API keys on the OpenAI endpoint (explicitly *not*
      multi-tenancy yet). Today any in-cluster caller can hit any model.
- [ ] **Gateway HA hardening** — default is 1 replica (SPOF). Add `PodDisruptionBudget` +
      pod anti-affinity, and **aggregate the demand signal across replicas** (KEDA currently polls a
      single gateway's pending count, which softens activation at >1 replica).
- [ ] **Operator HA** — verify leader-election failover.
- [ ] **API stabilization** → `v1beta1` with validation/conversion webhooks; document compatibility.
- [ ] **Test depth** — soak + failure-injection (node/pod loss, GPU failure) on top of the existing
      no-GPU CI loop.

### Community track (help wanted)
- [ ] **KEDA external push scaler**
      ([#42](https://github.com/hearth-project/hearth/issues/42)) — gRPC `StreamIsActive` push for
      instant `0→1` activation instead of polling; removes the `demandLinger` workaround.

### Demand-driven backlog (parked, not abandoned — built when a named user asks)
- `ModelCatalog` CRD + curated Qwen/DeepSeek/GLM presets (`catalogRef` is unimplemented today).
- KV-cache / TTFT-SLO autoscaling — richer signals beyond queue depth.
- `BakedImage` cache; LoRA hot-swap; canary / blue-green rollouts.
- Multi-tenant quotas, RBAC/SSO, audit, rate limiting.
- **Xinchuang / air-gapped bundle** — offline images + model packs (lands after the P1 enablers).
- Security review + bilingual docs site.

---

## Ecosystem

Hearth is a **minimal, composable serving control plane** for the small end of the LLM-serving
axis. For fleet-grade serving — multi-model routing, prefill/decode disaggregation, datacenter
scale-out — use
[Kthena](https://github.com/volcano-sh/kthena), [AIBrix](https://github.com/vllm-project/aibrix), or
KServe/llm-d; they're excellent, and Hearth composes with them (hot models on the platform, the long
tail scaled to zero with Hearth). We share operational lessons from Hearth's verified scale-to-zero
path with Kthena's design ([kthena#1019](https://github.com/volcano-sh/kthena/issues/1019)). See the
README's ["Hearth and Kthena"](README.md#hearth-and-kthena) for the full positioning.

---

## Known limitations (v0)

- **Cold start is seconds-to-minutes** — scale-to-zero is for latency-tolerant traffic; set
  `scaling.min: 1` for latency-critical models (forgoes the cost saving).
- **Multi-node image pull dominates Nth-replica readiness** — see P1; pre-distribute images.
- **Node-local cache is per-node** — replicas on fresh nodes re-download weights (until `SharedPVC`).
- **`SharedPVC` / `BakedImage` cache strategies and `catalogRef` are not implemented.**
- **No auth, no multi-tenancy, no quotas.**
- **Ascend 910B is an experimental preview** — vLLM-Ascend serving, manifest render, and the gateway
  data-plane are verified on a real 910B, but the operator scheduling a pod onto the NPU (device plugin)
  and the full integrated `0→1→N→0` loop are **not yet run on hardware**. Atlas 300I Duo is fully
  verified for its recorded stack; Atlas 300I Pro and MLU are manifest-only.
- **`v1alpha1`** — breaking API changes expected before `v1beta1`.
