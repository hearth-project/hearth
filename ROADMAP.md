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
- **Multi-backend abstraction** — NVIDIA implemented & run; **Ascend** scaffolded + golden-tested
  (renders correct manifests) but **not yet run on real NPUs**.
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

### Now — the v1 milestone: Ascend on real NPUs (310P-first)
- [ ] **Validate the Ascend backend on real hardware.** Cheapest credible path first: **Ascend 310P**
      (Atlas 300I Pro/Duo) — vllm-ascend ships dedicated `-310p` images and a runtime sample is
      already in-repo (`config/samples/serving_v1alpha1_inferenceruntime_ascend_310p.yaml`).
      Deliverables: a bring-up runbook, cold-start numbers, and the honest claim *"functionally
      validated on 310P"*. **910B** follows for performance claims.
- [ ] **Volcano live validation** — `scheduler.queue` → `scheduling.volcano.sh/queue-name` rendering
      is golden-tested; verify queue placement + `0→1` under a real Volcano scheduler. HAMi
      sharing / gang scheduling follows.

### P1 — unblock private / enterprise delivery
- [ ] **`imagePullSecrets`** — private-registry support on backend/prewarm/gateway (Xinchuang/enterprise).
- [ ] **`pvc://` + `oci://` model sources** — pre-staged weights, no egress at serve time; the
      prerequisite for the air-gapped bundle.
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

Hearth is deliberately the **no-platform end** of the LLM-serving axis. For fleet-grade serving —
multi-model routing, prefill/decode disaggregation, datacenter scale-out — use
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
- **Ascend/MLU are manifest-only** (golden-tested), not run on hardware.
- **`v1alpha1`** — breaking API changes expected before `v1beta1`.
